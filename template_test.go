package main

import (
	"io"
	"strings"
	"testing"
	"time"
)

// Every tab renders, with and without a stored run, and the today/findings
// views put only critical and high findings up front.
func TestTemplateTabs(t *testing.T) {
	tmpl := parseIndex()
	run := &Run{Started: time.Now(), Score: 91, Light: "yellow", Agent: "claude", Findings: []Finding{
		{Severity: "high", Title: "Key in plain text", Source: "local"},
		{Severity: "medium", Title: "Wildcard allow rule", Source: "ai"},
		{Severity: "low", Title: "SSH key without passphrase", Source: "local"},
	}}
	for _, r := range []*Run{nil, run} {
		for _, tab := range []string{"today", "findings", "checklist", "history", "sent", "settings"} {
			d := pageData{Lang: "ko", Tab: tab, Run: r, AllAgents: allAgents, Lockdown: lockdown, Cfg: defaultConfig()}
			if err := tmpl.Execute(io.Discard, d); err != nil {
				t.Fatalf("%s (run=%v): %v", tab, r != nil, err)
			}
		}
	}
	var b strings.Builder
	if err := tmpl.Execute(&b, pageData{Lang: "ko", Tab: "today", Run: run, Cfg: defaultConfig()}); err != nil {
		t.Fatal(err)
	}
	page := b.String()
	if !strings.Contains(page, "Key in plain text") || strings.Contains(page, "Wildcard allow rule") || !strings.Contains(page, "참고할 것 2개") {
		t.Fatal("today must show the high finding and only a link to the other two")
	}
}
