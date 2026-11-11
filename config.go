package main

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// Config lives in ~/.shieldbug/config.json. Every field can be edited by hand,
// by the user's AI, or from the dashboard's settings screen.
type Config struct {
	Language   string            `json:"language"`    // "auto", "ko", "en"
	Agent      string            `json:"agent"`       // "auto", a CLI ("claude", "codex", "gemini", "agy") or an API adapter ("claude-api", "openai-api", "gemini-api")
	AgentOrder []string          `json:"agent_order"` // auto-detection order
	Models     map[string]string `json:"models"`
	Effort     map[string]string `json:"effort"` // "" = CLI default
	Schedule   struct {
		Enabled bool   `json:"enabled"`
		DailyAt string `json:"daily_at"` // HH:MM local time
	} `json:"schedule"`
	Checks struct {
		Categories  []string `json:"categories"`   // empty = all
		DisabledIDs []string `json:"disabled_ids"` // checklist items to skip
		ExtraPaths  []string `json:"extra_paths"`  // extra config files to inspect (redacted, 64 KB cap)
	} `json:"checks"`
	Intel struct {
		ModelSearch  bool `json:"model_search"` // separate web-search call that never sees machine data
		LookbackDays int  `json:"lookback_days"`
	} `json:"intel"`
	Limits struct {
		TimeoutMin   int     `json:"timeout_min"`
		MaxBudgetUSD float64 `json:"max_budget_usd"` // applied where the CLI supports it
		MaxPayloadKB int     `json:"max_payload_kb"`
	} `json:"limits"`
	Privacy struct {
		KeepPayloadDays     int      `json:"keep_payload_days"`
		ExtraRedactPatterns []string `json:"extra_redact_patterns"`
	} `json:"privacy"`
	Dashboard struct {
		Port        int `json:"port"`
		IdleExitMin int `json:"idle_exit_min"`
	} `json:"dashboard"`
	RetentionDays int `json:"retention_days"`
}

func defaultConfig() Config {
	var c Config
	c.Language = "auto"
	c.Agent = "auto"
	// Subscription CLIs first (the point of Shieldbug is your own subscription),
	// Claude first among them: it is the only CLI where removing every tool is
	// confirmed by a flag. API keys are used only when no CLI is installed.
	c.AgentOrder = append([]string(nil), allAgents...)
	c.Models = map[string]string{
		"claude":     "claude-sonnet-5-5",
		"codex":      "gpt-6.1-terra",
		"gemini":     "gemini-3.8-flash",
		"agy":        "gemini-3.8-flash-low",
		"claude-api": "claude-sonnet-5-5",
		"openai-api": "gpt-6.1-terra",
		"gemini-api": "gemini-3.8-flash",
	}
	c.Effort = map[string]string{"claude": "low", "codex": "low", "claude-api": "low", "openai-api": "low"}
	c.Schedule.Enabled = true
	c.Schedule.DailyAt = "09:00"
	c.Intel.ModelSearch = true
	c.Intel.LookbackDays = 3
	c.Limits.TimeoutMin = 15
	c.Limits.MaxBudgetUSD = 0.5
	c.Limits.MaxPayloadKB = 200
	c.Privacy.KeepPayloadDays = 14
	c.Dashboard.Port = 7878
	c.Dashboard.IdleExitMin = 10
	c.RetentionDays = 180
	return c
}

func homeDir() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return h
}

func dataDir() string {
	if d := os.Getenv("SHIELDBUG_HOME"); d != "" {
		return d
	}
	return filepath.Join(homeDir(), ".shieldbug")
}

func configPath() string { return filepath.Join(dataDir(), "config.json") }

func loadConfig() (Config, error) {
	c := defaultConfig()
	b, err := os.ReadFile(configPath())
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return c, err
	}
	d := defaultConfig()
	if c.Models == nil {
		c.Models = map[string]string{}
	}
	for k, v := range d.Models {
		if c.Models[k] == "" {
			c.Models[k] = v
		}
	}
	if c.Effort == nil {
		c.Effort = map[string]string{}
	}
	if len(c.AgentOrder) == 0 {
		c.AgentOrder = d.AgentOrder
	}
	if c.Dashboard.Port == 0 {
		c.Dashboard.Port = d.Dashboard.Port
	}
	if c.Dashboard.IdleExitMin == 0 {
		c.Dashboard.IdleExitMin = d.Dashboard.IdleExitMin
	}
	if c.Limits.TimeoutMin == 0 {
		c.Limits.TimeoutMin = d.Limits.TimeoutMin
	}
	if c.Limits.MaxPayloadKB == 0 {
		c.Limits.MaxPayloadKB = d.Limits.MaxPayloadKB
	}
	if c.Schedule.DailyAt == "" {
		c.Schedule.DailyAt = d.Schedule.DailyAt
	}
	if c.Intel.LookbackDays == 0 {
		c.Intel.LookbackDays = d.Intel.LookbackDays
	}
	if c.Privacy.KeepPayloadDays == 0 {
		c.Privacy.KeepPayloadDays = d.Privacy.KeepPayloadDays
	}
	if c.RetentionDays == 0 {
		c.RetentionDays = d.RetentionDays
	}
	return c, nil
}

func saveConfig(c Config) error { return writeJSON(configPath(), c) }

func (c Config) lang() string {
	if c.Language == "ko" || c.Language == "en" {
		return c.Language
	}
	osLangOnce.Do(func() { osLang = detectOSLang() })
	return osLang
}

var (
	osLangOnce sync.Once
	osLang     string
)

func detectOSLang() string {
	for _, k := range []string{"LANG", "LC_ALL", "LANGUAGE"} {
		if strings.HasPrefix(strings.ToLower(os.Getenv(k)), "ko") {
			return "ko"
		}
	}
	if runtime.GOOS == "windows" {
		if out, err := runCmd(5e9, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", "(Get-Culture).Name"); err == nil &&
			strings.HasPrefix(strings.TrimSpace(strings.ToLower(out)), "ko") {
			return "ko"
		}
	}
	return "en"
}

func writeJSON(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func agentBinary(name string) string {
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	var cands []string
	local := os.Getenv("LOCALAPPDATA")
	switch name {
	case "agy":
		if local != "" {
			cands = append(cands, filepath.Join(local, "agy", "bin", "agy.exe"))
		}
	case "claude":
		cands = append(cands, filepath.Join(homeDir(), ".local", "bin", "claude"), filepath.Join(homeDir(), ".local", "bin", "claude.exe"),
			filepath.Join(homeDir(), ".claude", "local", "claude"))
	case "codex":
		cands = append(cands, filepath.Join(homeDir(), ".codex", "bin", "codex"))
	}
	for _, c := range cands {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c
		}
	}
	return ""
}

// allAgents: subscription CLIs, then API-key adapters.
var allAgents = []string{"claude", "codex", "gemini", "agy", "claude-api", "openai-api", "gemini-api"}

// agentAvailable: a CLI is available when its binary is found, an API adapter
// when its key is set in the environment.
func agentAvailable(name string) bool {
	if isAPI(name) {
		return apiKey(name) != ""
	}
	return agentBinary(name) != ""
}

func detectAgents(c Config) []string {
	var found []string
	seen := map[string]bool{}
	order := append(append([]string(nil), c.AgentOrder...), allAgents...) // older configs may lack the API adapters
	for _, a := range order {
		if !seen[a] && agentAvailable(a) {
			found = append(found, a)
		}
		seen[a] = true
	}
	return found
}

const noAgentMsg = "no AI found: install claude, codex, gemini or agy, or set ANTHROPIC_API_KEY, OPENAI_API_KEY or GEMINI_API_KEY"

func pickAgent(c Config) (string, error) {
	if c.Agent != "" && c.Agent != "auto" {
		if !agentAvailable(c.Agent) {
			if isAPI(c.Agent) {
				return "", errors.New("configured agent '" + c.Agent + "' has no API key: set " + strings.Join(apiProviders[c.Agent].keyEnv, " or "))
			}
			return "", errors.New("configured agent '" + c.Agent + "' was not found")
		}
		return c.Agent, nil
	}
	found := detectAgents(c)
	if len(found) == 0 {
		return "", errors.New(noAgentMsg)
	}
	return found[0], nil
}
