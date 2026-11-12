package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"
)

// Facts are what Shieldbug collects locally, with no AI involved.
// Every string is redacted before it is stored or sent anywhere.
type Facts struct {
	Collected time.Time         `json:"collected"`
	OS        string            `json:"os"`
	Sections  map[string]string `json:"sections"`
}

// Finding is one problem, either from a deterministic local rule or from the model.
type Finding struct {
	ChecklistID string `json:"checklist_id,omitempty"`
	Severity    string `json:"severity"`
	Title       string `json:"title"`
	TitleKo     string `json:"title_ko,omitempty"`
	Evidence    string `json:"evidence,omitempty"`
	Fix         string `json:"fix,omitempty"`
	FixKo       string `json:"fix_ko,omitempty"`
	Source      string `json:"source"` // "local" or "ai"
}

const maxSection = 6000

func runCmd(timeout time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	hideWindow(cmd)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func readSmall(path string, limit int) (string, bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer f.Close()
	buf := make([]byte, limit)
	n, _ := f.Read(buf)
	return string(buf[:n]), true
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "\n…(truncated)"
}

func collect(c Config) (Facts, []Finding) {
	f := Facts{Collected: time.Now(), OS: runtime.GOOS + "/" + runtime.GOARCH, Sections: map[string]string{}}
	var local []Finding
	add := func(name, text string) {
		text = strings.TrimSpace(redact(text))
		if text != "" {
			f.Sections[name] = clip(text, maxSection)
		}
	}

	agentText, agentFindings := collectAgents(c)
	add("ai_agent_configs", agentText)
	local = append(local, agentFindings...)

	extText, extFindings := collectExtensions()
	add("agent_extensions", extText)
	local = append(local, extFindings...)

	secText, secFindings := collectSecrets()
	add("secrets_hygiene", secText)
	local = append(local, secFindings...)

	add("installed_tools", collectTools())
	return f, local
}

// ---------- AI agent configuration ----------

func agentConfigPaths() []string {
	h := homeDir()
	p := []string{
		filepath.Join(h, ".claude", "settings.json"),
		filepath.Join(h, ".claude", "settings.local.json"),
		filepath.Join(h, ".codex", "config.toml"),
		filepath.Join(h, ".gemini", "settings.json"),
		filepath.Join(h, ".cursor", "mcp.json"),
		filepath.Join(h, ".codeium", "windsurf", "mcp_config.json"),
	}
	switch runtime.GOOS {
	case "windows":
		if a := os.Getenv("APPDATA"); a != "" {
			p = append(p, filepath.Join(a, "Claude", "claude_desktop_config.json"))
		}
	case "darwin":
		p = append(p, filepath.Join(h, "Library", "Application Support", "Claude", "claude_desktop_config.json"))
	default:
		p = append(p, filepath.Join(h, ".config", "Claude", "claude_desktop_config.json"))
	}
	return p
}

var (
	reBypass = regexp.MustCompile(`(?i)(dangerously-skip-permissions|bypassPermissions|danger-full-access|dangerously-bypass-approvals|--yolo|"yolo"\s*:\s*true)`)
	// Allow rules that cover every shell command ("Bash", "Bash(*)") or any download.
	reWildcard = regexp.MustCompile(`(?i)"(Bash|PowerShell|Shell)(\((\*|\*\*|curl[^)]*|wget[^)]*|iwr[^)]*|Invoke-WebRequest[^)]*)\))?"`)
	// Claude Code auto mode reviews each action and drops blanket shell rules.
	reAutoMode = regexp.MustCompile(`"defaultMode"\s*:\s*"auto"`)
	// Only the allow lists count; hook matchers also say "Bash".
	reAllowList = regexp.MustCompile(`"(allow|allowedTools)"\s*:\s*\[[^\]]*\]`)
	// A hook that downloads and runs code, opens a reverse shell or uploads a
	// file. Plain notification calls (curl to a webhook) do not match.
	reNetHook = regexp.MustCompile(`(?i)((curl|wget)[^|\n]*\|\s*(sudo\s+)?(ba|z|da)?sh\b|(iwr|irm|invoke-webrequest|invoke-restmethod)[^|\n]*\|\s*(iex|invoke-expression)\b|\$\(\s*(curl|wget)\b|\b(nc|ncat)\s[^\n]*-e\b|(curl|wget)[^\n]*(\s-d\s*@|--data(-binary)?\s*@|\s-F\s*\S*=@|\s-T\s|--upload-file|--post-file))`)
)

func collectAgents(c Config) (string, []Finding) {
	var b strings.Builder
	var out []Finding
	paths := append(agentConfigPaths(), c.Checks.ExtraPaths...)
	for _, p := range paths {
		text, ok := readSmall(p, 200_000)
		if !ok {
			continue
		}
		fmt.Fprintf(&b, "### %s\n%s\n\n", hidePaths(p), clip(redact(text), 2500))
		scanAgentText(p, text, &out)
	}
	// ~/.claude.json is large (project history); keep only MCP servers and allowed tools.
	if raw, ok := readSmall(filepath.Join(homeDir(), ".claude.json"), 20_000_000); ok {
		var m map[string]any
		if json.Unmarshal([]byte(raw), &m) == nil {
			slim := map[string]any{}
			if v, ok := m["mcpServers"]; ok {
				slim["mcpServers"] = v
			}
			if projects, ok := m["projects"].(map[string]any); ok {
				pm := map[string]any{}
				for name, pv := range projects {
					if po, ok := pv.(map[string]any); ok {
						keep := map[string]any{}
						for _, k := range []string{"mcpServers", "allowedTools"} {
							if v, ok := po[k]; ok {
								if mm, ok := v.(map[string]any); ok && len(mm) == 0 {
									continue
								}
								if aa, ok := v.([]any); ok && len(aa) == 0 {
									continue
								}
								keep[k] = v
							}
						}
						if len(keep) > 0 {
							pm[name] = keep
						}
					}
				}
				if len(pm) > 0 {
					slim["projects"] = pm
				}
			}
			js, _ := json.MarshalIndent(slim, "", " ")
			fmt.Fprintf(&b, "### ~/.claude.json (mcpServers and allowedTools only)\n%s\n\n", clip(redact(string(js)), 3000))
			scanAgentText("~/.claude.json", string(js), &out)
		}
	}
	// Shell profiles: only lines that matter.
	for _, p := range shellProfiles() {
		text, ok := readSmall(p, 300_000)
		if !ok {
			continue
		}
		var hits []string
		for _, line := range strings.Split(text, "\n") {
			if reBypass.MatchString(line) {
				hits = append(hits, strings.TrimSpace(line))
			}
		}
		if len(hits) > 0 {
			fmt.Fprintf(&b, "### %s (lines with bypass flags)\n%s\n\n", p, strings.Join(hits, "\n"))
			out = append(out, Finding{ChecklistID: "agent-skip-permissions", Severity: "high", Source: "local",
				Title:    "Shell profile launches an AI agent without permission prompts",
				TitleKo:  "셸 프로필이 AI 에이전트를 허락 없이 실행하도록 설정됨",
				Evidence: hidePaths(p) + ": " + redact(hits[0]),
				Fix:      "Remove the bypass flag from the alias or function.", FixKo: "별칭·함수에서 우회 플래그를 빼세요."})
		}
	}
	if b.Len() == 0 {
		b.WriteString("No AI agent configuration files found.")
	}
	return b.String(), out
}

func scanAgentText(path, text string, out *[]Finding) {
	p := hidePaths(path)
	if m := reBypass.FindString(text); m != "" {
		*out = append(*out, Finding{ChecklistID: "agent-skip-permissions", Severity: "high", Source: "local",
			Title: "Agent config allows running without permission prompts", TitleKo: "에이전트 설정이 허락 없는 실행을 허용함",
			Evidence: p + ": " + m, Fix: "Remove the bypass mode and allow only the tools you need.", FixKo: "우회 모드를 빼고 필요한 도구만 허용하세요."})
	}
	allow := strings.Join(reAllowList.FindAllString(text, -1), "\n")
	if m := reWildcard.FindString(allow); m != "" && !reAutoMode.MatchString(text) && !claudeAutoMode() && !reBypass.MatchString(text) {
		*out = append(*out, Finding{ChecklistID: "agent-wildcard-allow", Severity: "medium", Source: "local",
			Title: "Agent allow-list contains a broad wildcard", TitleKo: "에이전트 허용 목록에 넓은 와일드카드가 있음",
			Evidence: p + ": " + m, Fix: "Replace it with exact commands.", FixKo: "정확한 명령으로 바꾸세요."})
	}
	if patternHits(text) > 0 || plainSecretValues(text) > 0 {
		*out = append(*out, Finding{ChecklistID: "secrets-in-mcp-config", Severity: "medium", Source: "local",
			Title: "A config file contains what looks like a key or token", TitleKo: "설정 파일에 키나 토큰으로 보이는 값이 있음",
			Evidence: p, Fix: "Move it to an environment variable or keychain and rotate it if it was shared.", FixKo: "환경변수·키체인으로 옮기고, 공유된 적 있으면 새로 발급하세요."})
	}
	if strings.Contains(strings.ToLower(text), "hooks") {
		for _, line := range strings.Split(text, "\n") {
			if strings.Contains(line, "command") && reNetHook.MatchString(line) {
				*out = append(*out, Finding{ChecklistID: "hook-intent", Severity: "high", Source: "local",
					Title: "An agent hook downloads and runs code or uploads files", TitleKo: "에이전트 훅이 코드를 내려받아 실행하거나 파일을 보냄",
					Evidence: p + ": " + redact(strings.TrimSpace(line)), Fix: "Check that you wrote this hook; delete it if not.", FixKo: "직접 만든 훅인지 확인하고 아니면 지우세요."})
				break
			}
		}
	}
}

// claudeAutoMode: the user's Claude Code settings default to auto mode.
func claudeAutoMode() bool {
	for _, n := range []string{"settings.json", "settings.local.json"} {
		if t, ok := readSmall(filepath.Join(homeDir(), ".claude", n), 200_000); ok && reAutoMode.MatchString(t) {
			return true
		}
	}
	return false
}

func shellProfiles() []string {
	h := homeDir()
	p := []string{filepath.Join(h, ".bashrc"), filepath.Join(h, ".zshrc"), filepath.Join(h, ".bash_profile"), filepath.Join(h, ".profile"),
		filepath.Join(h, ".config", "fish", "config.fish")}
	if runtime.GOOS == "windows" {
		docs := filepath.Join(h, "Documents")
		p = append(p, filepath.Join(docs, "WindowsPowerShell", "Microsoft.PowerShell_profile.ps1"),
			filepath.Join(docs, "PowerShell", "Microsoft.PowerShell_profile.ps1"))
	}
	return p
}

// ---------- secrets hygiene (names and presence only, never values) ----------

func collectSecrets() (string, []Finding) {
	h := homeDir()
	var lines []string
	var out []Finding

	if _, err := os.Stat(filepath.Join(h, ".git-credentials")); err == nil {
		lines = append(lines, "~/.git-credentials: present (plain-text git credentials)")
		out = append(out, Finding{ChecklistID: "plaintext-tokens", Severity: "high", Source: "local",
			Title: "Git credentials are stored in plain text", TitleKo: "Git 자격 증명이 평문으로 저장됨", Evidence: "~/.git-credentials exists",
			Fix: "Switch to the OS credential manager and delete ~/.git-credentials.", FixKo: "OS 자격 증명 관리자로 바꾸고 ~/.git-credentials를 지우세요."})
	}
	if helper, err := runCmd(10*time.Second, "git", "config", "--global", "--get", "credential.helper"); err == nil {
		helper = strings.TrimSpace(helper)
		lines = append(lines, "git credential.helper: "+helper)
		if helper == "store" {
			out = append(out, Finding{ChecklistID: "plaintext-tokens", Severity: "high", Source: "local",
				Title: "git is set to store credentials in plain text", TitleKo: "git이 자격 증명을 평문으로 저장하도록 설정됨", Evidence: "credential.helper=store",
				Fix: "git config --global credential.helper manager (Windows) / osxkeychain (macOS) / libsecret (Linux)", FixKo: "OS 자격 증명 관리자 헬퍼로 바꾸세요."})
		}
	}
	for _, name := range []string{".npmrc", ".pypirc", ".netrc", ".docker/config.json"} {
		text, ok := readSmall(filepath.Join(h, filepath.FromSlash(name)), 100_000)
		if !ok {
			continue
		}
		lower := strings.ToLower(text)
		has := strings.Contains(lower, "authtoken") || strings.Contains(lower, "password") || strings.Contains(lower, "\"auth\"")
		lines = append(lines, fmt.Sprintf("~/%s: present, contains credentials: %v", name, has))
		if has {
			out = append(out, Finding{ChecklistID: "plaintext-tokens", Severity: "medium", Source: "local",
				Title: "A credential is stored in ~/" + name, TitleKo: "~/" + name + "에 자격 증명이 저장됨", Evidence: "~/" + name,
				Fix: "Use a short-lived, narrowly scoped token or a credential helper.", FixKo: "기간·권한이 제한된 토큰이나 자격 증명 헬퍼를 쓰세요."})
		}
	}
	// Environment variable NAMES that look like secrets (values never read out).
	var names []string
	for _, kv := range os.Environ() {
		k := strings.SplitN(kv, "=", 2)[0]
		u := strings.ToUpper(k)
		if strings.Contains(u, "TOKEN") || strings.Contains(u, "SECRET") || strings.Contains(u, "API_KEY") || strings.HasSuffix(u, "_KEY") || strings.Contains(u, "PASSWORD") {
			names = append(names, k)
		}
	}
	sort.Strings(names)
	if len(names) > 0 {
		lines = append(lines, "secret-like environment variable names (values not read): "+strings.Join(names, ", "))
	}
	return strings.Join(lines, "\n"), out
}

// ---------- installed tools ----------

func collectTools() string {
	var lines []string
	for _, a := range []string{"claude", "codex", "gemini", "agy", "node", "npm", "python", "python3", "git", "docker", "code", "cursor", "ollama"} {
		bin := agentBinary(a)
		if bin == "" {
			continue
		}
		v, _ := runCmd(15*time.Second, bin, "--version")
		v = strings.TrimSpace(strings.Split(v, "\n")[0])
		lines = append(lines, fmt.Sprintf("%s: %s", a, clip(v, 120)))
	}
	if bin := agentBinary("npm"); bin != "" {
		if out, err := runCmd(40*time.Second, bin, "ls", "-g", "--depth=0", "--json"); err == nil {
			var m struct {
				Dependencies map[string]struct {
					Version string `json:"version"`
				} `json:"dependencies"`
			}
			if json.Unmarshal([]byte(out), &m) == nil {
				var pk []string
				for k, v := range m.Dependencies {
					pk = append(pk, k+"@"+v.Version)
				}
				sort.Strings(pk)
				lines = append(lines, "global npm packages: "+strings.Join(pk, ", "))
			}
		}
	}
	if bin := agentBinary("code"); bin != "" {
		if out, err := runCmd(40*time.Second, bin, "--list-extensions", "--show-versions"); err == nil {
			ext := strings.Fields(out)
			lines = append(lines, fmt.Sprintf("VS Code extensions (%d): %s", len(ext), clip(strings.Join(ext, ", "), 2500)))
		}
	}
	return strings.Join(lines, "\n")
}
