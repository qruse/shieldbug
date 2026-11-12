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
	if len(core) != 6 {
		t.Fatalf("want 6 core checks (5 AI + self-check), got %d", len(core))
	}
	for _, it := range core {
		if it.Title["ko"] == "" || it.Fix["ko"] == "" || (it.Kind != "ai") != (it.ID == selfID) {
			t.Errorf("%s: incomplete item", it.ID)
		}
		if it.Locked != (it.ID == selfID) {
			t.Errorf("%s: only the self-check is locked", it.ID)
		}
	}
	c := defaultConfig()
	c.Checks.DisabledIDs = []string{"risky-agent-combo", selfID}
	act := ids(activeChecklist(c))
	if len(act) != 5 || slices.Contains(act, "risky-agent-combo") || !slices.Contains(act, selfID) {
		t.Fatalf("a core check turned off is skipped, the self-check never: %v", act)
	}
	if len(loadBase()) != 6 {
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
	if !removeAdded("my-check") || len(activeChecklist(c)) != 6 {
		t.Fatal("remove user check")
	}
}
