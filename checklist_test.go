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

// Ten core checks always run; optional ones run only once added; core checks
// cannot be disabled or removed.
func TestCoreAndExtras(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	core := loadBase()
	if len(core) != 10 {
		t.Fatalf("want 10 core checks, got %d", len(core))
	}
	c := defaultConfig()
	c.Checks.DisabledIDs = []string{"agent-skip-permissions"}
	act := ids(activeChecklist(c))
	if len(act) != 10 || !slices.Contains(act, "agent-skip-permissions") {
		t.Fatalf("core must stay on: %v", act)
	}
	if !addExtra("disk-encryption-off") || !slices.Contains(ids(activeChecklist(c)), "disk-encryption-off") {
		t.Fatal("added extra should be active")
	}
	if slices.Contains(ids(availableExtras()), "disk-encryption-off") {
		t.Fatal("added extra should leave the available list")
	}
	if removeAdded("agent-skip-permissions") {
		t.Fatal("core check removed")
	}
	if !removeAdded("disk-encryption-off") || len(activeChecklist(c)) != 10 {
		t.Fatal("remove extra")
	}
	for _, e := range loadExtras() {
		if slices.Contains(ids(core), e.ID) {
			t.Fatalf("%s is both core and extra", e.ID)
		}
	}
}
