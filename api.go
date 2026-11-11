package main

// API-key mode: for people who have an API key instead of (or besides) an AI
// subscription CLI. Same two-call split as the CLI adapters:
//
//   intel    gets the provider's own server-side web search, nothing else
//   analysis is sent with no tools at all, so the model can only answer in text
//
// Keys are read from environment variables only. They are never written to
// config.json, payloads, run files or error messages.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"os"
	"strings"
	"time"
)

type apiProvider struct {
	keyEnv  []string // first one that is set wins
	baseEnv string   // optional base-URL override (proxies, tests)
	base    string
}

var apiProviders = map[string]apiProvider{
	"claude-api": {keyEnv: []string{"ANTHROPIC_API_KEY"}, baseEnv: "ANTHROPIC_BASE_URL", base: "https://api.anthropic.com"},
	"openai-api": {keyEnv: []string{"OPENAI_API_KEY"}, baseEnv: "OPENAI_BASE_URL", base: "https://api.openai.com/v1"},
	"gemini-api": {keyEnv: []string{"GEMINI_API_KEY", "GOOGLE_API_KEY"}, baseEnv: "GEMINI_BASE_URL", base: "https://generativelanguage.googleapis.com"},
}

func isAPI(agent string) bool { _, ok := apiProviders[agent]; return ok }

// hasEffort: adapters that accept a reasoning-effort setting.
func hasEffort(agent string) bool {
	switch agent {
	case "claude", "codex", "claude-api", "openai-api", "gemini-api":
		return true
	}
	return false
}

func apiKey(agent string) string {
	for _, k := range apiProviders[agent].keyEnv {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return ""
}

func apiBase(agent string) string {
	p := apiProviders[agent]
	if v := strings.TrimRight(os.Getenv(p.baseEnv), "/"); v != "" {
		return v
	}
	return p.base
}

var apiClient = &http.Client{}

func callAPI(c Config, agent string, kind callKind, prompt string) (string, Usage, error) {
	u := Usage{Agent: agent, Model: c.Models[agent], Effort: c.Effort[agent]}
	key := apiKey(agent)
	if key == "" {
		return "", u, fmt.Errorf("%s: set %s", agent, strings.Join(apiProviders[agent].keyEnv, " or "))
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(c.Limits.TimeoutMin)*time.Minute)
	defer cancel()
	start := time.Now()
	var text string
	var err error
	switch agent {
	case "claude-api":
		text, err = anthropicCall(ctx, c, key, kind, prompt, &u)
	case "openai-api":
		text, err = openaiCall(ctx, c, key, kind, prompt, &u)
	case "gemini-api":
		text, err = geminiCall(ctx, c, key, kind, prompt, &u)
	}
	u.Seconds = time.Since(start).Seconds()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			err = fmt.Errorf("timed out after %d minutes", c.Limits.TimeoutMin)
		}
		return "", u, fmt.Errorf("%s: %s", agent, safeErr(err.Error(), key))
	}
	if strings.TrimSpace(text) == "" {
		return "", u, fmt.Errorf("%s returned no text", agent)
	}
	return text, u, nil
}

// safeErr makes sure neither the key nor anything key-shaped from a provider's
// error body ends up in a run file or on screen.
func safeErr(s, key string) string {
	if key != "" {
		s = strings.ReplaceAll(s, key, "[REDACTED]")
	}
	return hidePaths(clip(redactText(s), 400))
}

func postJSON(ctx context.Context, url string, headers map[string]string, body, out any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "shieldbug/"+version)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := apiClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode/100 != 2 {
		var e struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		msg := strings.TrimSpace(string(raw))
		if json.Unmarshal(raw, &e) == nil && e.Error.Message != "" {
			msg = e.Error.Message
		}
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, msg)
	}
	return json.Unmarshal(raw, out)
}

// ---------- Anthropic Messages API ----------

// Published per-million-token prices, used only for the cost estimate shown in
// the dashboard. Unknown models show no estimate rather than a wrong one.
var anthropicPrice = map[string][3]float64{ // input, output, cache read
	"claude-sonnet-5-5": {2, 10, 0.10},
	"claude-haiku-5-5":  {0.10, 0.50, 0.01},
	"claude-opus-5-5":   {4, 20, 0.20},
	"claude-fable-5-2":  {8, 40, 0.20},
}

func anthropicCall(ctx context.Context, c Config, key string, kind callKind, prompt string, u *Usage) (string, error) {
	model := c.Models["claude-api"]
	body := map[string]any{
		"model":      model,
		"max_tokens": 16000,
		"messages":   []map[string]any{{"role": "user", "content": prompt}},
		// A security review can trip a safety classifier; let the API re-serve
		// a declined request on a suitable model instead of failing the check.
		"fallbacks": "default",
	}
	if e := c.Effort["claude-api"]; e != "" {
		body["output_config"] = map[string]any{"effort": e}
	}
	if kind == callIntel {
		body["tools"] = []map[string]any{{"type": "web_search_20260209", "name": "web_search", "max_uses": 5}}
	}
	headers := map[string]string{
		"x-api-key":         key,
		"anthropic-version": "2026-10-21",
		"anthropic-beta":    "server-side-fallback-2026-07-01",
	}
	type block struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	var text strings.Builder
	var webSearches int
	// pause_turn: a long server-side search turn can pause; send it back to continue.
	for turn := 0; turn < 4; turn++ {
		var resp struct {
			Model      string            `json:"model"`
			StopReason string            `json:"stop_reason"`
			Content    []json.RawMessage `json:"content"`
			Usage      struct {
				In     int `json:"input_tokens"`
				Out    int `json:"output_tokens"`
				Read   int `json:"cache_read_input_tokens"`
				Create int `json:"cache_creation_input_tokens"`
				Server struct {
					WebSearch int `json:"web_search_requests"`
				} `json:"server_tool_use"`
			} `json:"usage"`
			StopDetails *struct {
				Category    string `json:"category"`
				Explanation string `json:"explanation"`
			} `json:"stop_details"`
		}
		if err := postJSON(ctx, apiBase("claude-api")+"/v1/messages", headers, body, &resp); err != nil {
			return "", err
		}
		if resp.Model != "" {
			u.Model = resp.Model
		}
		u.TokensIn += resp.Usage.In + resp.Usage.Read + resp.Usage.Create
		u.TokensOut += resp.Usage.Out
		webSearches += resp.Usage.Server.WebSearch
		if p, ok := anthropicPrice[resp.Model]; ok {
			u.CostUSD += (float64(resp.Usage.In+resp.Usage.Create)*p[0] + float64(resp.Usage.Out)*p[1] + float64(resp.Usage.Read)*p[2]) / 1e6
		}
		if resp.StopReason == "refusal" {
			why := "the model declined this request"
			if resp.StopDetails != nil && resp.StopDetails.Category != "" {
				why += " (" + resp.StopDetails.Category + ")"
			}
			return "", errors.New(why)
		}
		for _, raw := range resp.Content {
			var b block
			if json.Unmarshal(raw, &b) == nil && b.Type == "text" {
				text.WriteString(b.Text)
			}
		}
		if resp.StopReason != "pause_turn" {
			break
		}
		msgs := body["messages"].([]map[string]any)
		body["messages"] = append(msgs, map[string]any{"role": "assistant", "content": resp.Content})
	}
	u.CostUSD += float64(webSearches) * 0.01 // $10 per 1,000 searches
	return text.String(), nil
}

// ---------- OpenAI Responses API ----------

func openaiCall(ctx context.Context, c Config, key string, kind callKind, prompt string, u *Usage) (string, error) {
	body := map[string]any{
		"model": c.Models["openai-api"],
		"input": prompt,
		"store": false,
	}
	if e := c.Effort["openai-api"]; e != "" {
		body["reasoning"] = map[string]any{"effort": e}
	}
	if kind == callIntel {
		body["tools"] = []map[string]any{{"type": "web_search"}}
	}
	var resp struct {
		Model  string `json:"model"`
		Status string `json:"status"`
		Output []struct {
			Type    string `json:"type"`
			Content []struct {
				Type    string `json:"type"`
				Text    string `json:"text"`
				Refusal string `json:"refusal"`
			} `json:"content"`
		} `json:"output"`
		Incomplete *struct {
			Reason string `json:"reason"`
		} `json:"incomplete_details"`
		Usage struct {
			In  int `json:"input_tokens"`
			Out int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := postJSON(ctx, apiBase("openai-api")+"/responses", map[string]string{"Authorization": "Bearer " + key}, body, &resp); err != nil {
		return "", err
	}
	if resp.Model != "" {
		u.Model = resp.Model
	}
	u.TokensIn, u.TokensOut = resp.Usage.In, resp.Usage.Out
	var text strings.Builder
	for _, o := range resp.Output {
		if o.Type != "message" {
			continue
		}
		for _, p := range o.Content {
			if p.Type == "refusal" {
				return "", errors.New("the model declined this request")
			}
			if p.Type == "output_text" {
				text.WriteString(p.Text)
			}
		}
	}
	if text.Len() == 0 && resp.Incomplete != nil {
		return "", errors.New("response incomplete: " + resp.Incomplete.Reason)
	}
	return text.String(), nil
}

// ---------- Gemini API ----------

func geminiCall(ctx context.Context, c Config, key string, kind callKind, prompt string, u *Usage) (string, error) {
	model := c.Models["gemini-api"]
	body := map[string]any{
		"contents": []map[string]any{{"role": "user", "parts": []map[string]any{{"text": prompt}}}},
	}
	gen := map[string]any{}
	if e := c.Effort["gemini-api"]; e != "" {
		gen["thinkingConfig"] = map[string]any{"thinkingLevel": e}
	}
	if kind == callIntel {
		body["tools"] = []map[string]any{{"google_search": map[string]any{}}}
	} else {
		gen["responseMimeType"] = "application/json"
	}
	if len(gen) > 0 {
		body["generationConfig"] = gen
	}
	var resp struct {
		Candidates []struct {
			FinishReason string `json:"finishReason"`
			Content      struct {
				Parts []struct {
					Text    string `json:"text"`
					Thought bool   `json:"thought"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
		Feedback *struct {
			BlockReason string `json:"blockReason"`
		} `json:"promptFeedback"`
		Usage struct {
			In       int `json:"promptTokenCount"`
			Out      int `json:"candidatesTokenCount"`
			Thoughts int `json:"thoughtsTokenCount"`
		} `json:"usageMetadata"`
	}
	endpoint := apiBase("gemini-api") + "/v1beta/models/" + neturl.PathEscape(model) + ":generateContent"
	if err := postJSON(ctx, endpoint, map[string]string{"x-goog-api-key": key}, body, &resp); err != nil {
		return "", err
	}
	u.TokensIn, u.TokensOut = resp.Usage.In, resp.Usage.Out+resp.Usage.Thoughts
	if resp.Feedback != nil && resp.Feedback.BlockReason != "" {
		return "", errors.New("the model declined this request (" + resp.Feedback.BlockReason + ")")
	}
	var text strings.Builder
	for _, cand := range resp.Candidates {
		for _, p := range cand.Content.Parts {
			if !p.Thought {
				text.WriteString(p.Text)
			}
		}
		break
	}
	return text.String(), nil
}
