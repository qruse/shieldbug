package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// ---------- skills, plugins and instruction files ----------
//
// Names and sources of what the agents load, plus only the suspicious lines
// of instruction and skill files. File contents are never sent whole.

// Invisible characters used to hide text from people but not from models:
// zero-width, bidi overrides, word joiners and Unicode tag characters.
// U+200D (emoji joiner) and U+FE0F (emoji variation) are left out on purpose.
var reInvisible = regexp.MustCompile("[\u200B\u200C\u200E\u200F\u202A-\u202E\u2060-\u2064\u2066-\u2069]|[\U000E0020-\U000E007F]{4,}")

// Lines that tell the agent to override its instructions, or HTML comments
// (hidden when rendered) that ask it to run or send something. These go to
// the model for judgement only; a phrase in quotes is someone quoting an
// attack, not making one, and is skipped.
var reOverride = regexp.MustCompile(`(?i)(ignore (all |any )?(the |your )?(previous|prior|above|earlier) instructions|disregard (the |your )?(system prompt|previous instructions)|<!--[^>]*\b(run|execute|send|upload|curl|wget|token|api key)\b[^>]*-->)`)

func overrideLine(line string) bool {
	for _, loc := range reOverride.FindAllStringIndex(line, -1) {
		if loc[0] > 0 && strings.ContainsAny(line[loc[0]-1:loc[0]], "\"'`“‘«") {
			continue
		}
		return true
	}
	return false
}

// Skills synced from the user's own Claude account, and the tools' own
// built-in skills, are managed by the vendor and not scanned.
func vendorManaged(p string) bool {
	h := homeDir()
	for _, d := range []string{filepath.Join(h, ".claude", "skills", "synced"), filepath.Join(h, ".codex", "skills", ".system")} {
		if strings.HasPrefix(p, d+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

type extDir struct{ label, path string }

func extensionDirs() []extDir {
	h := homeDir()
	return []extDir{
		{"Claude Code skills", filepath.Join(h, ".claude", "skills")},
		{"Claude Code plugins", filepath.Join(h, ".claude", "plugins", "cache")},
		{"Claude Code agents", filepath.Join(h, ".claude", "agents")},
		{"Claude Code commands", filepath.Join(h, ".claude", "commands")},
		{"Codex skills", filepath.Join(h, ".codex", "skills")},
		{"Agent skills", filepath.Join(h, ".agents", "skills")},
		{"Gemini extensions", filepath.Join(h, ".gemini", "extensions")},
	}
}

func instructionFiles() []string {
	h := homeDir()
	return []string{
		filepath.Join(h, ".claude", "CLAUDE.md"),
		filepath.Join(h, ".codex", "AGENTS.md"),
		filepath.Join(h, ".gemini", "GEMINI.md"),
	}
}

func collectExtensions() (string, []Finding) {
	var b strings.Builder
	var out []Finding

	// Plugin marketplaces: name and where they come from.
	if raw, ok := readSmall(filepath.Join(homeDir(), ".claude", "plugins", "known_marketplaces.json"), 200_000); ok {
		var m map[string]struct {
			Source map[string]any `json:"source"`
		}
		if json.Unmarshal([]byte(raw), &m) == nil && len(m) > 0 {
			var lines []string
			for name, v := range m {
				src := ""
				for _, k := range []string{"repo", "url", "path"} {
					if s, ok := v.Source[k].(string); ok && s != "" {
						src = s
						break
					}
				}
				lines = append(lines, fmt.Sprintf("- %s: %s", name, hidePaths(src)))
			}
			sort.Strings(lines)
			fmt.Fprintf(&b, "### Claude Code plugin marketplaces (name: source)\n%s\n\n", strings.Join(lines, "\n"))
		}
	}

	// Installed skills, plugins, agents and extensions: names only.
	for _, d := range extensionDirs() {
		names := listNames(d.path)
		if len(names) > 0 {
			fmt.Fprintf(&b, "### %s\n%s\n\n", d.label, strings.Join(names, ", "))
		}
	}

	// Instruction and skill files: only suspicious lines.
	files := instructionFiles()
	for _, d := range extensionDirs() {
		files = append(files, markdownFiles(d.path, 400)...)
	}
	var hits []string
	invisible := map[string]bool{}
	for _, p := range files {
		text, ok := readSmall(p, 300_000)
		if !ok {
			continue
		}
		text = strings.TrimPrefix(text, "\uFEFF")
		for i, line := range strings.Split(text, "\n") {
			inv := reInvisible.MatchString(line)
			if !inv && !overrideLine(line) {
				continue
			}
			shown := strings.TrimSpace(reInvisible.ReplaceAllString(line, "[HIDDEN-CHAR]"))
			hits = append(hits, fmt.Sprintf("%s:%d: %s", hidePaths(p), i+1, clip(redact(shown), 240)))
			if inv && !invisible[p] {
				invisible[p] = true
				out = append(out, Finding{ChecklistID: "hidden-instructions", Severity: "high", Source: "local",
					Title:    "An instruction or skill file contains invisible characters",
					TitleKo:  "지시 파일·스킬에 보이지 않는 글자가 숨어 있음",
					Evidence: fmt.Sprintf("%s:%d", hidePaths(p), i+1),
					Fix:      "Open the file in an editor that shows hidden characters, remove them, and remove the skill if you did not write it.",
					FixKo:    "숨은 글자가 보이는 편집기로 열어 지우고, 직접 만든 게 아니면 그 스킬을 지우세요."})
			}
			if len(hits) >= 40 {
				break
			}
		}
	}
	if len(hits) > 0 {
		fmt.Fprintf(&b, "### Suspicious lines in instruction and skill files (file:line: text)\n%s\n\n", strings.Join(hits, "\n"))
	}
	if b.Len() == 0 {
		b.WriteString("No skills, plugins or instruction files found.")
	}
	return b.String(), out
}

// listNames lists entries one or two levels down (skill folders sit either
// directly in the folder or under a publisher folder), skipping hidden ones.
func listNames(dir string) []string {
	top, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range top {
		n := e.Name()
		if strings.HasPrefix(n, ".") {
			continue
		}
		if e.IsDir() && !hasFile(filepath.Join(dir, n), "SKILL.md") {
			if sub, err := os.ReadDir(filepath.Join(dir, n)); err == nil && len(sub) > 0 {
				var inner []string
				for _, s := range sub {
					if !strings.HasPrefix(s.Name(), ".") {
						inner = append(inner, s.Name())
					}
				}
				if len(inner) > 0 && len(inner) <= 30 {
					n += " (" + strings.Join(inner, ", ") + ")"
				} else if len(inner) > 30 {
					n += fmt.Sprintf(" (%d items)", len(inner))
				}
			}
		}
		names = append(names, n)
		if len(names) >= 60 {
			names = append(names, "…")
			break
		}
	}
	return names
}

func hasFile(dir, name string) bool {
	_, err := os.Stat(filepath.Join(dir, name))
	return err == nil
}

// markdownFiles walks dir for .md files, at most limit of them.
func markdownFiles(dir string, limit int) []string {
	var out []string
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if len(out) >= limit {
			return fs.SkipAll
		}
		if err != nil {
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() && (d.Name() == "node_modules" || d.Name() == ".git" || vendorManaged(p+string(filepath.Separator))) {
			return filepath.SkipDir
		}
		if !d.IsDir() && strings.EqualFold(filepath.Ext(p), ".md") {
			out = append(out, p)
		}
		return nil
	})
	return out
}
