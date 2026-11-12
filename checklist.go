package main

import (
	_ "embed"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// base.json: the core checks that need an AI's judgement; they cannot be
// turned off. The user's own checks and the AI proposals they approved live
// in local.json.
//
//go:embed checklist/base.json
var baseChecklist []byte

// ChecklistItem is one living check. Titles and fixes are per language.
type ChecklistItem struct {
	ID       string            `json:"id"`
	Rev      int               `json:"rev,omitempty"`
	Status   string            `json:"status,omitempty"` // active, pending, rejected
	Category string            `json:"category"`
	Severity string            `json:"severity"`
	Kind     string            `json:"kind,omitempty"` // rule or ai
	Title    map[string]string `json:"title"`
	Check    string            `json:"check"`
	Fix      map[string]string `json:"fix"`
	Refs     []string          `json:"refs,omitempty"`
	Added    string            `json:"added,omitempty"`
	Source   string            `json:"source,omitempty"` // base, ai, user, feed
	Locked   bool              `json:"locked,omitempty"` // always on (the self-check)
}

type checklistFile struct {
	Version int             `json:"version"`
	Items   []ChecklistItem `json:"items"`
}

func checklistDir() string { return filepath.Join(dataDir(), "checklist") }

// loadBase: the built-in core checks. Fixed per release, so a user file can
// add checks but never remove these.
func loadBase() []ChecklistItem {
	var f checklistFile
	_ = json.Unmarshal(baseChecklist, &f)
	return f.Items
}

// addedChecklist: checks the user added (their own, approved AI
// proposals), skipping any id that is already a core check.
func addedChecklist() []ChecklistItem {
	core := map[string]bool{}
	for _, it := range loadBase() {
		core[it.ID] = true
	}
	var out []ChecklistItem
	for _, it := range loadItems("local.json") {
		if !core[it.ID] && it.Status != "rejected" && it.Status != "pending" {
			out = append(out, it)
		}
	}
	return out
}

// removeAdded drops a check the user added. Core checks are not in local.json,
// so they cannot be removed this way.
func removeAdded(id string) bool {
	items := loadItems("local.json")
	var keep []ChecklistItem
	for _, it := range items {
		if it.ID != id {
			keep = append(keep, it)
		}
	}
	if len(keep) == len(items) {
		return false
	}
	_ = saveItems("local.json", keep)
	logHistory("removed", id, "user")
	return true
}

func loadItems(name string) []ChecklistItem {
	var f checklistFile
	_ = readJSON(filepath.Join(checklistDir(), name), &f)
	return f.Items
}

func saveItems(name string, items []ChecklistItem) error {
	return writeJSON(filepath.Join(checklistDir(), name), checklistFile{Version: 2, Items: items})
}

func ids(items []ChecklistItem) []string {
	var out []string
	for _, it := range items {
		out = append(out, it.ID)
	}
	return out
}

// activeChecklist = the core checks + the checks the user added, minus the
// ones they turned off (core ones too: some people run only their own) and
// added ones whose category they turned off.
func activeChecklist(c Config) []ChecklistItem {
	disabled := map[string]bool{}
	for _, id := range c.Checks.DisabledIDs {
		disabled[id] = true
	}
	cats := map[string]bool{}
	for _, k := range c.Checks.Categories {
		cats[k] = true
	}
	var out []ChecklistItem
	for _, it := range loadBase() {
		if it.Locked || !disabled[it.ID] {
			out = append(out, it)
		}
	}
	for _, it := range addedChecklist() {
		if disabled[it.ID] || (len(cats) > 0 && !cats[it.Category]) {
			continue
		}
		out = append(out, it)
	}
	return out
}

// addProposals stores model proposals as pending, skipping ids that already
// exist or were rejected before. At most three per day.
func addProposals(props []ChecklistItem) int {
	known := map[string]bool{}
	for _, it := range loadBase() {
		known[it.ID] = true
	}
	for _, it := range loadItems("local.json") {
		known[it.ID] = true
	}
	pending := loadItems("proposed.json")
	for _, it := range pending {
		known[it.ID] = true
	}
	added := 0
	for _, p := range props {
		p.ID = strings.ToLower(strings.TrimSpace(p.ID))
		if p.ID == "" || known[p.ID] || added >= 3 {
			continue
		}
		p.Status, p.Source, p.Kind, p.Rev = "pending", "ai", "ai", 1
		p.Added = time.Now().Format("2006-01-02")
		pending = append(pending, p)
		known[p.ID] = true
		added++
		logHistory("proposed", p.ID, "ai")
	}
	if added > 0 {
		_ = saveItems("proposed.json", pending)
	}
	return added
}

// decideProposal approves or rejects one pending item.
func decideProposal(id string, approve bool) bool {
	pending := loadItems("proposed.json")
	var keep []ChecklistItem
	var hit *ChecklistItem
	for i := range pending {
		if pending[i].ID == id && hit == nil {
			p := pending[i]
			hit = &p
			continue
		}
		keep = append(keep, pending[i])
	}
	if hit == nil {
		return false
	}
	if approve {
		hit.Status = "active"
		_ = saveItems("local.json", append(loadItems("local.json"), *hit))
		logHistory("approved", id, "user")
	} else {
		hit.Status = "rejected"
		_ = saveItems("rejected.json", append(loadItems("rejected.json"), *hit))
		logHistory("rejected", id, "user")
	}
	_ = saveItems("proposed.json", keep)
	return true
}

func pendingProposals() []ChecklistItem {
	p := loadItems("proposed.json")
	sort.Slice(p, func(i, j int) bool { return p[i].Added > p[j].Added })
	return p
}

func logHistory(action, id, by string) {
	_ = os.MkdirAll(checklistDir(), 0o700)
	f, err := os.OpenFile(filepath.Join(checklistDir(), "history.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	b, _ := json.Marshal(map[string]string{"time": time.Now().Format(time.RFC3339), "action": action, "id": id, "by": by})
	f.Write(append(b, '\n'))
}

func (it ChecklistItem) T(lang string) string {
	if v := it.Title[lang]; v != "" {
		return v
	}
	return it.Title["en"]
}

func (it ChecklistItem) F(lang string) string {
	if v := it.Fix[lang]; v != "" {
		return v
	}
	return it.Fix["en"]
}

func sortedKeys(m map[string]string) []string {
	var k []string
	for x := range m {
		k = append(k, x)
	}
	sort.Strings(k)
	return k
}
