package main

import (
	"slices"
	"testing"
)

func ids(items []ChecklistItem) []string {
	var out []string
	for _, it := range items {
		out = append(out, it.ID)
	}
	return out
}

// Five core checks always run and cannot be disabled or removed; checks the
// user adds (their own or approved AI proposals) run next to them.
func TestCoreChecks(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	core := loadBase()
	if len(core) != 5 {
		t.Fatalf("want 5 core checks, got %d", len(core))
	}
	for _, it := range core {
		if it.Kind != "ai" || it.Title["ko"] == "" || it.Fix["ko"] == "" {
			t.Errorf("%s: incomplete item", it.ID)
		}
	}
	c := defaultConfig()
	c.Checks.DisabledIDs = []string{"risky-agent-combo"}
	act := ids(activeChecklist(c))
	if len(act) != 5 || !slices.Contains(act, "risky-agent-combo") {
		t.Fatalf("core must stay on: %v", act)
	}
	if removeAdded("risky-agent-combo") {
		t.Fatal("core check removed")
	}
	mine := ChecklistItem{ID: "my-check", Status: "active", Category: "ai-agents", Severity: "medium",
		Title: map[string]string{"en": "Mine"}, Check: "x", Fix: map[string]string{"en": "y"}}
	if err := saveItems("local.json", []ChecklistItem{mine}); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(ids(activeChecklist(c)), "my-check") {
		t.Fatal("user check should be active")
	}
	if !removeAdded("my-check") || len(activeChecklist(c)) != 5 {
		t.Fatal("remove user check")
	}
}
