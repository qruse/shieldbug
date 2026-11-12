package main

import (
	"slices"
	"testing"
)

// Five core checks run unless the user turns them off (they are never
// deleted); checks the user adds run next to them.
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
	if len(act) != 4 || slices.Contains(act, "risky-agent-combo") {
		t.Fatalf("a core check turned off must be skipped: %v", act)
	}
	if len(loadBase()) != 5 {
		t.Fatal("turning off must not delete the core check")
	}
	c.Checks.DisabledIDs = nil
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
