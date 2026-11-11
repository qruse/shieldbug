package main

import (
	"strings"
	"testing"
)

func TestSelfTest(t *testing.T) {
	if err := selfTest(); err != nil {
		t.Fatal(err)
	}
}

func TestFakeKeysNeverSurvive(t *testing.T) {
	cases := []string{
		"sk-ant-api03-" + strings.Repeat("Qw8", 15),
		"sk-proj-" + strings.Repeat("Rt5", 12),
		"github_pat_" + strings.Repeat("Ab3_", 8),
		"glpat-" + strings.Repeat("Xy7", 8),
		"hf_" + strings.Repeat("Ab3", 12),
		"npm_" + strings.Repeat("Cd4", 12),
		"AIza" + strings.Repeat("Ef5", 12),
		"xoxb-123456789012-" + strings.Repeat("Gh6", 6),
		"sk_live_" + strings.Repeat("Ij7", 8),
		"eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U",
		"-----BEGIN OPENSSH PRIVATE KEY-----\nb3BlbnNzaC1rZXktdjEAAAAA\n-----END OPENSSH PRIVATE KEY-----",
		"password = \"correct horse battery\"",
		`"apiKey": "short1"`,
	}
	for _, c := range cases {
		out := redact("prefix " + c + " suffix")
		if strings.Contains(out, c) {
			t.Errorf("not redacted: %q -> %q", c, out)
		}
		if hits := gate(out); len(hits) > 0 {
			t.Errorf("gate still fires on %q: %v (%q)", c, hits, out)
		}
	}
}

func TestStructuredJSON(t *testing.T) {
	in := `{"mcpServers":{"db":{"command":"uvx","args":["mcp-db"],"env":{"DB_URL":"postgres://a:b@h/db","MODE":"x"}},"web":{"url":"https://example.com/mcp","headers":{"X-Api":"plainish"}}}}`
	out := redact(in)
	for _, leak := range []string{"postgres://a:b@h/db", "plainish", `"x"`} {
		if strings.Contains(out, leak) {
			t.Errorf("leaked %q in %s", leak, out)
		}
	}
	if !strings.Contains(out, "uvx") || !strings.Contains(out, "mcp-db") {
		t.Errorf("lost harmless structure: %s", out)
	}
}

func TestStructuredTOML(t *testing.T) {
	in := "model = \"gpt-6.1-terra\"\n[mcp_servers.gh]\ncommand = \"npx\"\n[mcp_servers.gh.env]\nGITHUB_TOKEN = \"anything-at-all\"\n"
	out := redact(in)
	if strings.Contains(out, "anything-at-all") {
		t.Errorf("TOML env value leaked: %s", out)
	}
	if !strings.Contains(out, "gpt-6.1-terra") || !strings.Contains(out, "npx") {
		t.Errorf("harmless TOML lost: %s", out)
	}
}

func TestHarmlessTextKept(t *testing.T) {
	keep := []string{"@modelcontextprotocol/server-filesystem", "claude-sonnet-5-5", "node_modules", "2026-11-11T21:00:00+09:00",
		"Microsoft.PowerShell_profile.ps1", "127.0.0.1:7878", "192.168.0.10"}
	for _, k := range keep {
		if out := redact(k); !strings.Contains(out, k) {
			t.Errorf("harmless text changed: %q -> %q", k, out)
		}
	}
}

func TestPlainSecretValues(t *testing.T) {
	harmless := `{"env":{"CLAUDE_AUTOCOMPACT_PCT_OVERRIDE":"70","FLAG":"false"},"security":{"auth":{"selectedType":"oauth-personal"}}}`
	if n := plainSecretValues(harmless); n != 0 {
		t.Errorf("harmless settings counted as %d secrets", n)
	}
	real := `{"mcpServers":{"gh":{"env":{"GITHUB_TOKEN":"abcd1234efgh5678"}}}}`
	if n := plainSecretValues(real); n != 1 {
		t.Errorf("token not counted: %d", n)
	}
	toml := "[mcp_servers.x.env]\nAPI_KEY = \"zx81mn22pp\"\nMODE = \"fast\"\n"
	if n := plainSecretValues(toml); n != 1 {
		t.Errorf("toml key not counted: %d", n)
	}
}

func TestShortUserNameDoesNotRewriteWords(t *testing.T) {
	if got := replaceWord("username userland user", "user", "<u>"); got != "username userland <u>" {
		t.Errorf("got %q", got)
	}
}
