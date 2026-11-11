package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"
)

const fakeKey = "sk-test-" + "Zq9Lm3Xv7Rt2Wp5Ns8Kd"

// mockProvider records the last request body and replies with reply (or an error status).
func mockProvider(t *testing.T, status int, reply string, got *map[string]any, hdr *http.Header) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		*got = map[string]any{}
		_ = json.Unmarshal(b, got)
		*hdr = r.Header.Clone()
		(*got)["_path"] = r.URL.Path
		w.WriteHeader(status)
		io.WriteString(w, reply)
	}))
}

func testCfg() Config {
	c := defaultConfig()
	c.Limits.TimeoutMin = 1
	return c
}

func TestAnthropicAdapter(t *testing.T) {
	var got map[string]any
	var hdr http.Header
	srv := mockProvider(t, 200, `{"model":"claude-sonnet-5-5","stop_reason":"end_turn","content":[{"type":"text","text":"{\"items\":[]}"}],"usage":{"input_tokens":1000,"output_tokens":100,"server_tool_use":{"web_search_requests":2}}}`, &got, &hdr)
	defer srv.Close()
	t.Setenv("ANTHROPIC_API_KEY", fakeKey)
	t.Setenv("ANTHROPIC_BASE_URL", srv.URL)
	c := testCfg()

	text, u, err := callAgent(c, "claude-api", callIntel, "find news")
	if err != nil || text != `{"items":[]}` {
		t.Fatalf("intel: %q %v", text, err)
	}
	if got["_path"] != "/v1/messages" || hdr.Get("x-api-key") != fakeKey || hdr.Get("anthropic-version") == "" {
		t.Fatalf("bad request: %v %v", got["_path"], hdr)
	}
	tools, _ := got["tools"].([]any)
	if len(tools) != 1 || tools[0].(map[string]any)["type"] != "web_search_20260209" {
		t.Fatalf("intel must carry web search only: %v", got["tools"])
	}
	if got["model"] != "claude-sonnet-5-5" || got["output_config"].(map[string]any)["effort"] != "medium" {
		t.Fatalf("model/effort not sent: %v", got)
	}
	want := (1000*2+100*10)/1e6 + 2*0.01
	if u.CostUSD < want-1e-9 || u.CostUSD > want+1e-9 || u.TokensIn != 1000 || u.TokensOut != 100 {
		t.Fatalf("usage: %+v", u)
	}

	if _, _, err := callAgent(c, "claude-api", callAnalysis, "facts"); err != nil {
		t.Fatal(err)
	}
	if _, has := got["tools"]; has {
		t.Fatal("analysis call must not send any tools")
	}
}

func TestAnthropicRefusalAndPause(t *testing.T) {
	var got map[string]any
	var hdr http.Header
	srv := mockProvider(t, 200, `{"model":"claude-sonnet-5-5","stop_reason":"refusal","content":[],"stop_details":{"category":"cyber"}}`, &got, &hdr)
	defer srv.Close()
	t.Setenv("ANTHROPIC_API_KEY", fakeKey)
	t.Setenv("ANTHROPIC_BASE_URL", srv.URL)
	_, _, err := callAgent(testCfg(), "claude-api", callAnalysis, "x")
	if err == nil || !strings.Contains(err.Error(), "cyber") {
		t.Fatalf("refusal not reported: %v", err)
	}
}

func TestOpenAIAdapter(t *testing.T) {
	var got map[string]any
	var hdr http.Header
	srv := mockProvider(t, 200, `{"model":"gpt-6.1-terra","status":"completed","output":[{"type":"web_search_call"},{"type":"message","content":[{"type":"output_text","text":"{\"ok\":1}"}]}],"usage":{"input_tokens":50,"output_tokens":7}}`, &got, &hdr)
	defer srv.Close()
	t.Setenv("OPENAI_API_KEY", fakeKey)
	t.Setenv("OPENAI_BASE_URL", srv.URL)
	c := testCfg()
	text, u, err := callAgent(c, "openai-api", callIntel, "q")
	if err != nil || text != `{"ok":1}` || u.TokensIn != 50 {
		t.Fatalf("%q %+v %v", text, u, err)
	}
	if got["_path"] != "/responses" || hdr.Get("Authorization") != "Bearer "+fakeKey || got["store"] != false {
		t.Fatalf("bad request: %v", got)
	}
	if tools, _ := got["tools"].([]any); len(tools) != 1 {
		t.Fatalf("intel tools: %v", got["tools"])
	}
	callAgent(c, "openai-api", callAnalysis, "facts")
	if _, has := got["tools"]; has {
		t.Fatal("analysis call must not send any tools")
	}
}

func TestGeminiAdapter(t *testing.T) {
	var got map[string]any
	var hdr http.Header
	srv := mockProvider(t, 200, `{"candidates":[{"content":{"parts":[{"text":"thinking","thought":true},{"text":"{\"ok\":2}"}]}}],"usageMetadata":{"promptTokenCount":30,"candidatesTokenCount":5,"thoughtsTokenCount":10}}`, &got, &hdr)
	defer srv.Close()
	t.Setenv("GEMINI_API_KEY", fakeKey)
	t.Setenv("GEMINI_BASE_URL", srv.URL)
	c := testCfg()
	text, u, err := callAgent(c, "gemini-api", callAnalysis, "facts")
	if err != nil || text != `{"ok":2}` || u.TokensOut != 15 {
		t.Fatalf("%q %+v %v", text, u, err)
	}
	if got["_path"] != "/v1beta/models/gemini-3.8-flash:generateContent" || hdr.Get("x-goog-api-key") != fakeKey {
		t.Fatalf("bad request: %v", got["_path"])
	}
	if _, has := got["tools"]; has {
		t.Fatal("analysis call must not send any tools")
	}
	callAgent(c, "gemini-api", callIntel, "q")
	if tools, _ := got["tools"].([]any); len(tools) != 1 {
		t.Fatalf("intel tools: %v", got["tools"])
	}
}

func TestAPIKeyNeverInErrors(t *testing.T) {
	var got map[string]any
	var hdr http.Header
	// A provider that echoes the key back in its error body.
	srv := mockProvider(t, 401, `{"error":{"message":"invalid x-api-key `+fakeKey+`"}}`, &got, &hdr)
	defer srv.Close()
	t.Setenv("ANTHROPIC_API_KEY", fakeKey)
	t.Setenv("ANTHROPIC_BASE_URL", srv.URL)
	_, _, err := callAgent(testCfg(), "claude-api", callAnalysis, "x")
	if err == nil || strings.Contains(err.Error(), fakeKey) || !strings.Contains(err.Error(), "401") {
		t.Fatalf("key leaked or error lost: %v", err)
	}
}

func TestAPIAgentDetection(t *testing.T) {
	t.Setenv("PATH", "")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	t.Setenv("LOCALAPPDATA", t.TempDir())
	for _, k := range []string{"ANTHROPIC_API_KEY", "OPENAI_API_KEY", "GEMINI_API_KEY", "GOOGLE_API_KEY"} {
		t.Setenv(k, "")
	}
	c := testCfg()
	if _, err := pickAgent(c); err == nil {
		t.Fatal("expected no agent")
	}
	t.Setenv("GOOGLE_API_KEY", fakeKey)
	if a, err := pickAgent(c); err != nil || a != "gemini-api" {
		t.Fatalf("got %q %v", a, err)
	}
	c.Agent = "claude-api"
	if _, err := pickAgent(c); err == nil || !strings.Contains(err.Error(), "ANTHROPIC_API_KEY") {
		t.Fatalf("missing-key message: %v", err)
	}
}

// An older config that still names Gemini CLI loads cleanly and falls back to auto.
func TestDroppedGeminiCLI(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if err := os.MkdirAll(dataDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	old := `{"agent":"gemini","agent_order":["claude","codex","gemini","agy"],"models":{"gemini":"gemini-3.8-flash","claude":"claude-sonnet-5-5"}}`
	if err := os.WriteFile(configPath(), []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if c.Agent != "auto" || slices.Contains(c.AgentOrder, "gemini") || c.Models["gemini"] != "" || agentAvailable("gemini") {
		t.Fatalf("gemini CLI still present: %q %v %v", c.Agent, c.AgentOrder, c.Models)
	}
}
