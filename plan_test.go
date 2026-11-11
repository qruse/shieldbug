package main

import "testing"

func TestPlanShare(t *testing.T) {
	r := Run{Agent: "claude", Calls: []Usage{{CostUSD: 0.112}, {CostUSD: 0.112}}}
	h, w, ok := planShare(r)
	if !ok || h < 0.99 || h > 1.01 || w < 0.10 || w > 0.11 {
		t.Fatalf("claude: %v %v %v", h, w, ok)
	}
	if pct("ko", h) != "약 1.0%" || pct("en", 0.05) != "under 0.1%" {
		t.Fatalf("format: %q %q", pct("ko", h), pct("en", 0.05))
	}
	if _, _, ok := planShare(Run{Agent: "claude-api", Calls: []Usage{{CostUSD: 1}}}); ok {
		t.Fatal("API keys draw from no plan")
	}
	if _, _, ok := planShare(Run{Agent: "claude"}); ok {
		t.Fatal("no usage, no estimate")
	}
}

func TestCodexEvents(t *testing.T) {
	stream := `{"type":"thread.started","thread_id":"x"}
{"type":"item.completed","item":{"id":"i0","type":"agent_message","text":"{\"items\":[]}"}}
{"type":"turn.completed","usage":{"input_tokens":72000,"cached_input_tokens":0,"output_tokens":0}}`
	var u Usage
	last := codexEvents(stream, &u)
	if last != `{"items":[]}` || u.TokensIn != 72000 || u.TokensOut != 0 {
		t.Fatalf("%q %+v", last, u)
	}
	h, _, ok := planShare(Run{Agent: "codex", Calls: []Usage{u}})
	if !ok || h < 0.99 || h > 1.01 {
		t.Fatalf("codex share %v", h)
	}
}
