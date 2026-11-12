package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Run is one daily check, stored as ~/.shieldbug/runs/<time>.json.
type Run struct {
	ID        string      `json:"id"`
	Started   time.Time   `json:"started"`
	Seconds   float64     `json:"seconds"`
	Scheduled bool        `json:"scheduled"`
	Agent     string      `json:"agent"`
	Lockdown  string      `json:"lockdown"`
	Score     int         `json:"score"`
	Light     string      `json:"light"` // green, yellow, red
	Summary   string      `json:"summary,omitempty"`
	SummaryKo string      `json:"summary_ko,omitempty"`
	Findings  []Finding   `json:"findings"`
	Intel     []IntelItem `json:"intel,omitempty"`
	Proposals int         `json:"proposals"`
	Calls     []Usage     `json:"calls"`
	PayloadKB float64     `json:"payload_kb"`
	Checklist int         `json:"checklist_items"`
	Errors    []string    `json:"errors,omitempty"`
	Sections  []string    `json:"sections"`
	OS        string      `json:"os"`
	SelfCheck []string    `json:"self_check,omitempty"` // what the self-check looked at
}

func runsDir() string      { return filepath.Join(dataDir(), "runs") }
func payloadDir() string   { return filepath.Join(dataDir(), "payloads") }
func lockPath() string     { return filepath.Join(dataDir(), "check.lock") }
func progressPath() string { return filepath.Join(dataDir(), "progress.txt") }

var checkMu sync.Mutex

func setProgress(s string) { _ = os.WriteFile(progressPath(), []byte(s), 0o600) }

// runCheck is the whole daily pipeline. It never fixes anything.
func runCheck(c Config, scheduled bool) (*Run, error) {
	checkMu.Lock()
	defer checkMu.Unlock()
	if err := os.MkdirAll(runsDir(), 0o700); err != nil {
		return nil, err
	}
	// A lock file stops a scheduled run and a dashboard run from overlapping.
	if st, err := os.Stat(lockPath()); err == nil && time.Since(st.ModTime()) < time.Duration(c.Limits.TimeoutMin*2+5)*time.Minute {
		return nil, fmt.Errorf("another check is already running")
	}
	_ = os.WriteFile(lockPath(), []byte(fmt.Sprint(os.Getpid())), 0o600)
	defer os.Remove(lockPath())
	defer os.Remove(progressPath())
	loadExtraPatterns(c)

	start := time.Now()
	r := &Run{ID: start.Format("20060102-150405"), Started: start, Scheduled: scheduled}

	setProgress("selftest")
	if err := selfTest(); err != nil {
		r.Errors = append(r.Errors, err.Error())
		return finish(r, start), err
	}

	// Self-check first: if Shieldbug itself was tampered with, say so and stop
	// before it starts other programs or sends anything.
	self := selfCheck()
	r.SelfCheck = self.Lines
	r.Findings = append(r.Findings, self.Findings...)
	for _, f := range self.Findings {
		if f.Severity == "critical" {
			r.Errors = append(r.Errors, "self-check failed; nothing else was run")
			return finish(r, start), nil
		}
	}

	setProgress("collect")
	facts, local := collect(c)
	r.OS = facts.OS
	for k := range facts.Sections {
		r.Sections = append(r.Sections, k)
	}
	sort.Strings(r.Sections)
	items := activeChecklist(c)
	r.Checklist = len(items)
	// Built-in rules only report for checks that are on the user's list. The
	// rest (a bypass flag, a token in a plain file) are not shown on their own
	// but go to the model as evidence for the checks that are.
	active := map[string]bool{}
	var forAI []ChecklistItem // rule-only checks (the self-check) are not the model's job
	for _, it := range items {
		active[it.ID] = it.Kind != "rule"
		if it.Kind != "rule" {
			forAI = append(forAI, it)
		}
	}
	items = forAI
	hints := local
	var shown []Finding
	for _, f := range local {
		if active[f.ChecklistID] {
			shown = append(shown, f)
		}
	}
	r.Findings = append(r.Findings, shown...)
	if len(items) == 0 {
		r.Errors = append(r.Errors, "every check is turned off; nothing was sent to the AI")
		return finish(r, start), nil
	}

	agents, err := runAgents(c)
	if err != nil {
		r.Errors = append(r.Errors, err.Error()+" — showing local checks only")
		return finish(r, start), nil
	}
	r.Agent = agents[0]
	r.Lockdown = lockdown[agents[0]]

	if c.Intel.ModelSearch {
		setProgress("intel")
		for i, agent := range agents {
			intel, u, err := runIntel(c, agent)
			r.Calls = append(r.Calls, u)
			if err == nil {
				r.Intel = intel
				break
			}
			r.Errors = append(r.Errors, "intel ("+agent+"): "+err.Error()+nextNote(i, agents))
		}
	}

	setProgress("analysis")
	prompt := buildAnalysisPrompt(c, facts, hints, items, r.Intel)
	if kb := float64(len(prompt)) / 1024; kb > float64(c.Limits.MaxPayloadKB) {
		r.Errors = append(r.Errors, fmt.Sprintf("payload %.0f KB is over the %d KB limit; nothing was sent", kb, c.Limits.MaxPayloadKB))
		return finish(r, start), nil
	}
	r.PayloadKB = float64(len(prompt)) / 1024
	// Layer 5: scan the finished payload. Anything that still looks like a secret stops the send.
	if hits := gate(prompt); len(hits) > 0 {
		savePayload(r.ID, "BLOCKED — not sent. Detectors: "+strings.Join(hits, ", ")+"\n\n"+maskForStorage(prompt))
		r.Errors = append(r.Errors, errGate.Error()+" ("+strings.Join(hits, ", ")+")")
		return finish(r, start), nil
	}
	savePayload(r.ID, prompt)
	var res *AIResult
	for i, agent := range agents {
		out, u, err := runAnalysis(c, agent, prompt)
		r.Calls = append(r.Calls, u)
		if err == nil {
			res = &out
			r.Agent, r.Lockdown = agent, lockdown[agent]
			break
		}
		r.Errors = append(r.Errors, "analysis ("+agent+"): "+err.Error()+nextNote(i, agents))
	}
	if res == nil {
		return finish(r, start), nil
	}
	r.Summary, r.SummaryKo = res.Summary, res.SummaryKo
	var onList []Finding
	for _, f := range res.Findings {
		if active[f.ChecklistID] {
			onList = append(onList, f)
		}
	}
	r.Findings = mergeFindings(r.Findings, onList)
	var props []ChecklistItem
	for _, p := range res.Proposals {
		props = append(props, p.item())
	}
	r.Proposals = addProposals(props)
	return finish(r, start), nil
}

// nextNote tells the user a failed call moved on to the next ticked AI.
func nextNote(i int, agents []string) string {
	if i+1 < len(agents) {
		return " — trying " + agents[i+1]
	}
	return ""
}

// maskForStorage is used only when the gate blocked a payload: what is kept on
// disk gets one more pass so the blocked text is not stored in the clear.
func maskForStorage(s string) string {
	s = redactText(s)
	for _, t := range reEntropyTk.FindAllString(s, -1) {
		if looksRandom(t) {
			s = strings.ReplaceAll(s, t, "[MASKED]")
		}
	}
	return s
}

func mergeFindings(local, ai []Finding) []Finding {
	seen := map[string]bool{}
	for _, f := range local {
		seen[f.ChecklistID+"|"+f.Title] = true
	}
	out := append([]Finding{}, local...)
	for _, f := range ai {
		if f.Title == "" || seen[f.ChecklistID+"|"+f.Title] {
			continue
		}
		out = append(out, f)
	}
	return out
}

// Only critical and high findings move the score much; medium ones are shown
// as "worth a look" and cost little, low ones nothing.
var sevWeight = map[string]int{"critical": 40, "high": 20, "medium": 3, "low": 0}

// isMajor: what the dashboard puts in front of the user.
func isMajor(f Finding) bool { return f.Severity == "critical" || f.Severity == "high" }

func splitFindings(fs []Finding, major bool) []Finding {
	var out []Finding
	for _, f := range fs {
		if isMajor(f) == major {
			out = append(out, f)
		}
	}
	return out
}

var sevOrder = map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3}

func finish(r *Run, start time.Time) *Run {
	score(r)
	r.Seconds = time.Since(start).Seconds()
	_ = writeJSON(filepath.Join(runsDir(), r.ID+".json"), r)
	return r
}

// score sets Score and Light from the findings. Stored runs are scored again
// when loaded, so a change of weights shows up without a new check.
func score(r *Run) {
	sort.SliceStable(r.Findings, func(i, j int) bool { return sevOrder[r.Findings[i].Severity] < sevOrder[r.Findings[j].Severity] })
	score := 100
	worst := 3
	for _, f := range r.Findings {
		score -= sevWeight[f.Severity]
		if sevOrder[f.Severity] < worst {
			worst = sevOrder[f.Severity]
		}
	}
	if score < 0 {
		score = 0
	}
	r.Score = score
	switch {
	case score < 60 || worst == 0:
		r.Light = "red"
	case score < 85 || worst == 1:
		r.Light = "yellow"
	default:
		r.Light = "green"
	}
}

func savePayload(id, text string) {
	_ = os.MkdirAll(payloadDir(), 0o700)
	_ = os.WriteFile(filepath.Join(payloadDir(), id+".txt"), []byte(text), 0o600)
}

func readPayload(id string) string {
	if strings.ContainsAny(id, `/\.`) {
		return ""
	}
	b, _ := os.ReadFile(filepath.Join(payloadDir(), id+".txt"))
	return string(b)
}

func listRuns(n int) []Run {
	entries, _ := os.ReadDir(runsDir())
	var names []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json") {
			names = append(names, e.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	var out []Run
	for _, name := range names {
		if n > 0 && len(out) >= n {
			break
		}
		var r Run
		if readJSON(filepath.Join(runsDir(), name), &r) == nil {
			score(&r)
			out = append(out, r)
		}
	}
	return out
}

// prune removes runs and payloads older than the retention settings.
func prune(c Config) {
	cut := func(dir string, days int) {
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			if info, err := e.Info(); err == nil && time.Since(info.ModTime()) > time.Duration(days)*24*time.Hour {
				os.Remove(filepath.Join(dir, e.Name()))
			}
		}
	}
	cut(runsDir(), c.RetentionDays)
	cut(payloadDir(), c.Privacy.KeepPayloadDays)
}
