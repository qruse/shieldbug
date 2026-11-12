package main

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"
)

// Facts are what Shieldbug collects locally, with no AI involved.
// Every string is redacted before it is stored or sent anywhere.
type Facts struct {
	Collected time.Time         `json:"collected"`
	OS        string            `json:"os"`
	Sections  map[string]string `json:"sections"`
}

// Finding is one problem, either from a deterministic local rule or from the model.
type Finding struct {
	ChecklistID string `json:"checklist_id,omitempty"`
	Severity    string `json:"severity"`
	Title       string `json:"title"`
	TitleKo     string `json:"title_ko,omitempty"`
	Evidence    string `json:"evidence,omitempty"`
	Fix         string `json:"fix,omitempty"`
	FixKo       string `json:"fix_ko,omitempty"`
	Source      string `json:"source"` // "local" or "ai"
}

const maxSection = 6000

func runCmd(timeout time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	hideWindow(cmd)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func readSmall(path string, limit int) (string, bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer f.Close()
	buf := make([]byte, limit)
	n, _ := f.Read(buf)
	return string(buf[:n]), true
}

// readTail reads up to the last limit bytes of a file.
func readTail(path string, limit int64) (string, bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer f.Close()
	if st, err := f.Stat(); err == nil && st.Size() > limit {
		_, _ = f.Seek(-limit, io.SeekEnd)
	}
	b, err := io.ReadAll(io.LimitReader(f, limit))
	return string(b), err == nil
}

func historyFiles() []string {
	h := homeDir()
	p := []string{filepath.Join(h, ".bash_history"), filepath.Join(h, ".zsh_history"),
		filepath.Join(h, ".local", "share", "fish", "fish_history"),
		filepath.Join(h, ".local", "share", "powershell", "PSReadLine", "ConsoleHost_history.txt")}
	if a := os.Getenv("APPDATA"); a != "" {
		p = append(p, filepath.Join(a, "Microsoft", "Windows", "PowerShell", "PSReadLine", "ConsoleHost_history.txt"))
	}
	return p
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "\n…(truncated)"
}

func collect(c Config) (Facts, []Finding) {
	f := Facts{Collected: time.Now(), OS: runtime.GOOS + "/" + runtime.GOARCH, Sections: map[string]string{}}
	var local []Finding
	add := func(name, text string) {
		text = strings.TrimSpace(redact(text))
		if text != "" {
			f.Sections[name] = clip(text, maxSection)
		}
	}

	agentText, agentFindings := collectAgents(c)
	add("ai_agent_configs", agentText)
	local = append(local, agentFindings...)

	secText, secFindings := collectSecrets()
	add("secrets_hygiene", secText)
	local = append(local, secFindings...)

	netText, netFindings := collectListeners()
	add("listening_ports", netText)
	local = append(local, netFindings...)

	sysText, sysFindings := collectSystem()
	add("os_protection", sysText)
	local = append(local, sysFindings...)

	add("installed_tools", collectTools())
	return f, local
}

// ---------- AI agent configuration ----------

func agentConfigPaths() []string {
	h := homeDir()
	p := []string{
		filepath.Join(h, ".claude", "settings.json"),
		filepath.Join(h, ".claude", "settings.local.json"),
		filepath.Join(h, ".codex", "config.toml"),
		filepath.Join(h, ".gemini", "settings.json"),
		filepath.Join(h, ".cursor", "mcp.json"),
		filepath.Join(h, ".codeium", "windsurf", "mcp_config.json"),
	}
	switch runtime.GOOS {
	case "windows":
		if a := os.Getenv("APPDATA"); a != "" {
			p = append(p, filepath.Join(a, "Claude", "claude_desktop_config.json"))
		}
	case "darwin":
		p = append(p, filepath.Join(h, "Library", "Application Support", "Claude", "claude_desktop_config.json"))
	default:
		p = append(p, filepath.Join(h, ".config", "Claude", "claude_desktop_config.json"))
	}
	return p
}

var (
	reBypass   = regexp.MustCompile(`(?i)(dangerously-skip-permissions|bypassPermissions|danger-full-access|dangerously-bypass-approvals|--yolo|"yolo"\s*:\s*true)`)
	reWildcard = regexp.MustCompile(`(?i)"(Bash|PowerShell|Shell|WebFetch)\((\*|curl[^)]*|wget[^)]*|\*\*)\)"`)
	// A hook that downloads and runs code, opens a reverse shell or uploads a
	// file. Plain notification calls (curl to a webhook) do not match.
	reNetHook = regexp.MustCompile(`(?i)((curl|wget)[^|\n]*\|\s*(sudo\s+)?(ba|z|da)?sh\b|(iwr|irm|invoke-webrequest|invoke-restmethod)[^|\n]*\|\s*(iex|invoke-expression)\b|\$\(\s*(curl|wget)\b|\b(nc|ncat)\s[^\n]*-e\b|(curl|wget)[^\n]*(\s-d\s*@|--data(-binary)?\s*@|\s-F\s*\S*=@|\s-T\s|--upload-file|--post-file))`)
)

func collectAgents(c Config) (string, []Finding) {
	var b strings.Builder
	var out []Finding
	paths := append(agentConfigPaths(), c.Checks.ExtraPaths...)
	for _, p := range paths {
		text, ok := readSmall(p, 200_000)
		if !ok {
			continue
		}
		fmt.Fprintf(&b, "### %s\n%s\n\n", hidePaths(p), clip(redact(text), 2500))
		scanAgentText(p, text, &out)
	}
	// ~/.claude.json is large (project history); keep only MCP servers and allowed tools.
	if raw, ok := readSmall(filepath.Join(homeDir(), ".claude.json"), 20_000_000); ok {
		var m map[string]any
		if json.Unmarshal([]byte(raw), &m) == nil {
			slim := map[string]any{}
			if v, ok := m["mcpServers"]; ok {
				slim["mcpServers"] = v
			}
			if projects, ok := m["projects"].(map[string]any); ok {
				pm := map[string]any{}
				for name, pv := range projects {
					if po, ok := pv.(map[string]any); ok {
						keep := map[string]any{}
						for _, k := range []string{"mcpServers", "allowedTools"} {
							if v, ok := po[k]; ok {
								if mm, ok := v.(map[string]any); ok && len(mm) == 0 {
									continue
								}
								if aa, ok := v.([]any); ok && len(aa) == 0 {
									continue
								}
								keep[k] = v
							}
						}
						if len(keep) > 0 {
							pm[name] = keep
						}
					}
				}
				if len(pm) > 0 {
					slim["projects"] = pm
				}
			}
			js, _ := json.MarshalIndent(slim, "", " ")
			fmt.Fprintf(&b, "### ~/.claude.json (mcpServers and allowedTools only)\n%s\n\n", clip(redact(string(js)), 3000))
			scanAgentText("~/.claude.json", string(js), &out)
		}
	}
	// Shell profiles: only lines that matter.
	for _, p := range shellProfiles() {
		text, ok := readSmall(p, 300_000)
		if !ok {
			continue
		}
		var hits []string
		for _, line := range strings.Split(text, "\n") {
			if reBypass.MatchString(line) {
				hits = append(hits, strings.TrimSpace(line))
			}
		}
		if len(hits) > 0 {
			fmt.Fprintf(&b, "### %s (lines with bypass flags)\n%s\n\n", p, strings.Join(hits, "\n"))
			out = append(out, Finding{ChecklistID: "agent-skip-permissions", Severity: "high", Source: "local",
				Title:    "Shell profile launches an AI agent without permission prompts",
				TitleKo:  "셸 프로필이 AI 에이전트를 허락 없이 실행하도록 설정됨",
				Evidence: hidePaths(p) + ": " + redact(hits[0]),
				Fix:      "Remove the bypass flag from the alias or function.", FixKo: "별칭·함수에서 우회 플래그를 빼세요."})
		}
	}
	if b.Len() == 0 {
		b.WriteString("No AI agent configuration files found.")
	}
	return b.String(), out
}

func scanAgentText(path, text string, out *[]Finding) {
	p := hidePaths(path)
	if m := reBypass.FindString(text); m != "" {
		*out = append(*out, Finding{ChecklistID: "agent-skip-permissions", Severity: "high", Source: "local",
			Title: "Agent config allows running without permission prompts", TitleKo: "에이전트 설정이 허락 없는 실행을 허용함",
			Evidence: p + ": " + m, Fix: "Remove the bypass mode and allow only the tools you need.", FixKo: "우회 모드를 빼고 필요한 도구만 허용하세요."})
	}
	if m := reWildcard.FindString(text); m != "" {
		*out = append(*out, Finding{ChecklistID: "agent-wildcard-allow", Severity: "medium", Source: "local",
			Title: "Agent allow-list contains a broad wildcard", TitleKo: "에이전트 허용 목록에 넓은 와일드카드가 있음",
			Evidence: p + ": " + m, Fix: "Replace it with exact commands.", FixKo: "정확한 명령으로 바꾸세요."})
	}
	if patternHits(text) > 0 || plainSecretValues(text) > 0 {
		*out = append(*out, Finding{ChecklistID: "secrets-in-mcp-config", Severity: "medium", Source: "local",
			Title: "A config file contains what looks like a key or token", TitleKo: "설정 파일에 키나 토큰으로 보이는 값이 있음",
			Evidence: p, Fix: "Move it to an environment variable or keychain and rotate it if it was shared.", FixKo: "환경변수·키체인으로 옮기고, 공유된 적 있으면 새로 발급하세요."})
	}
	if strings.Contains(strings.ToLower(text), "hooks") {
		for _, line := range strings.Split(text, "\n") {
			if strings.Contains(line, "command") && reNetHook.MatchString(line) {
				*out = append(*out, Finding{ChecklistID: "agent-hooks-network", Severity: "medium", Source: "local",
					Title: "An agent hook downloads and runs code or uploads files", TitleKo: "에이전트 훅이 코드를 내려받아 실행하거나 파일을 보냄",
					Evidence: p + ": " + redact(strings.TrimSpace(line)), Fix: "Check that you wrote this hook; delete it if not.", FixKo: "직접 만든 훅인지 확인하고 아니면 지우세요."})
				break
			}
		}
	}
}

func shellProfiles() []string {
	h := homeDir()
	p := []string{filepath.Join(h, ".bashrc"), filepath.Join(h, ".zshrc"), filepath.Join(h, ".bash_profile"), filepath.Join(h, ".profile"),
		filepath.Join(h, ".config", "fish", "config.fish")}
	if runtime.GOOS == "windows" {
		docs := filepath.Join(h, "Documents")
		p = append(p, filepath.Join(docs, "WindowsPowerShell", "Microsoft.PowerShell_profile.ps1"),
			filepath.Join(docs, "PowerShell", "Microsoft.PowerShell_profile.ps1"))
	}
	return p
}

// ---------- secrets hygiene (names and presence only, never values) ----------

func collectSecrets() (string, []Finding) {
	h := homeDir()
	var lines []string
	var out []Finding

	if _, err := os.Stat(filepath.Join(h, ".git-credentials")); err == nil {
		lines = append(lines, "~/.git-credentials: present (plain-text git credentials)")
		out = append(out, Finding{ChecklistID: "plaintext-tokens", Severity: "high", Source: "local",
			Title: "Git credentials are stored in plain text", TitleKo: "Git 자격 증명이 평문으로 저장됨", Evidence: "~/.git-credentials exists",
			Fix: "Switch to the OS credential manager and delete ~/.git-credentials.", FixKo: "OS 자격 증명 관리자로 바꾸고 ~/.git-credentials를 지우세요."})
	}
	if helper, err := runCmd(10*time.Second, "git", "config", "--global", "--get", "credential.helper"); err == nil {
		helper = strings.TrimSpace(helper)
		lines = append(lines, "git credential.helper: "+helper)
		if helper == "store" {
			out = append(out, Finding{ChecklistID: "plaintext-tokens", Severity: "high", Source: "local",
				Title: "git is set to store credentials in plain text", TitleKo: "git이 자격 증명을 평문으로 저장하도록 설정됨", Evidence: "credential.helper=store",
				Fix: "git config --global credential.helper manager (Windows) / osxkeychain (macOS) / libsecret (Linux)", FixKo: "OS 자격 증명 관리자 헬퍼로 바꾸세요."})
		}
	}
	for _, name := range []string{".npmrc", ".pypirc", ".netrc", ".docker/config.json"} {
		text, ok := readSmall(filepath.Join(h, filepath.FromSlash(name)), 100_000)
		if !ok {
			continue
		}
		lower := strings.ToLower(text)
		has := strings.Contains(lower, "authtoken") || strings.Contains(lower, "password") || strings.Contains(lower, "\"auth\"")
		lines = append(lines, fmt.Sprintf("~/%s: present, contains credentials: %v", name, has))
		if has {
			out = append(out, Finding{ChecklistID: "plaintext-tokens", Severity: "medium", Source: "local",
				Title: "A credential is stored in ~/" + name, TitleKo: "~/" + name + "에 자격 증명이 저장됨", Evidence: "~/" + name,
				Fix: "Use a short-lived, narrowly scoped token or a credential helper.", FixKo: "기간·권한이 제한된 토큰이나 자격 증명 헬퍼를 쓰세요."})
		}
	}
	// SSH private keys without a passphrase.
	sshDir := filepath.Join(h, ".ssh")
	entries, _ := os.ReadDir(sshDir)
	var plain []string
	for _, e := range entries {
		if e.IsDir() || strings.HasSuffix(e.Name(), ".pub") || strings.HasPrefix(e.Name(), "known_hosts") || e.Name() == "config" || e.Name() == "authorized_keys" {
			continue
		}
		text, ok := readSmall(filepath.Join(sshDir, e.Name()), 20_000)
		if !ok || !strings.Contains(text, "PRIVATE KEY-----") {
			continue
		}
		if !sshKeyEncrypted(text) {
			plain = append(plain, e.Name())
		}
	}
	if len(plain) > 0 {
		lines = append(lines, "SSH private keys without passphrase: "+strings.Join(plain, ", "))
		out = append(out, Finding{ChecklistID: "ssh-key-no-passphrase", Severity: "low", Source: "local",
			Title: "SSH private key without a passphrase", TitleKo: "비밀번호 없는 SSH 개인 키", Evidence: "~/.ssh/" + strings.Join(plain, ", ~/.ssh/"),
			Fix: "ssh-keygen -p -f ~/.ssh/<key>", FixKo: "ssh-keygen -p -f ~/.ssh/<키> 로 비밀번호를 거세요."})
	}
	// Shell history: keys pasted into a command line stay on disk. Only the file
	// name and whether something secret-like is there are reported.
	var hist []string
	for _, p := range historyFiles() {
		text, ok := readTail(p, 1<<20)
		if ok && (patternHits(text) > 0) {
			hist = append(hist, hidePaths(p))
		}
	}
	if len(hist) > 0 {
		lines = append(lines, "shell history with secret-like values: "+strings.Join(hist, ", "))
		out = append(out, Finding{ChecklistID: "shell-history-secrets", Severity: "medium", Source: "local",
			Title: "Shell history contains what looks like a key or token", TitleKo: "셸 기록에 키·토큰으로 보이는 값이 남아 있음",
			Evidence: strings.Join(hist, ", "),
			Fix:      "Delete those lines from the history file and rotate the key. Next time set keys in an environment variable or keychain, not on the command line.",
			FixKo:    "기록 파일에서 그 줄을 지우고 키를 새로 발급하세요. 앞으로 키는 명령줄 대신 환경변수나 키체인에 넣으세요."})
	}
	// Environment variable NAMES that look like secrets (values never read out).
	var names []string
	for _, kv := range os.Environ() {
		k := strings.SplitN(kv, "=", 2)[0]
		u := strings.ToUpper(k)
		if strings.Contains(u, "TOKEN") || strings.Contains(u, "SECRET") || strings.Contains(u, "API_KEY") || strings.HasSuffix(u, "_KEY") || strings.Contains(u, "PASSWORD") {
			names = append(names, k)
		}
	}
	sort.Strings(names)
	if len(names) > 0 {
		lines = append(lines, "secret-like environment variable names (values not read): "+strings.Join(names, ", "))
	}
	return strings.Join(lines, "\n"), out
}

func sshKeyEncrypted(pem string) bool {
	if strings.Contains(pem, "ENCRYPTED") {
		return true
	}
	if !strings.Contains(pem, "BEGIN OPENSSH PRIVATE KEY") {
		return false
	}
	var body strings.Builder
	sc := bufio.NewScanner(strings.NewReader(pem))
	for sc.Scan() {
		l := strings.TrimSpace(sc.Text())
		if l == "" || strings.HasPrefix(l, "-----") {
			continue
		}
		body.WriteString(l)
	}
	raw, err := base64.StdEncoding.DecodeString(body.String())
	if err != nil || len(raw) < 40 {
		return true // cannot tell; do not raise a false alarm
	}
	// "openssh-key-v1\0" + string ciphername
	off := len("openssh-key-v1") + 1
	if len(raw) < off+4 {
		return true
	}
	n := int(raw[off])<<24 | int(raw[off+1])<<16 | int(raw[off+2])<<8 | int(raw[off+3])
	if off+4+n > len(raw) {
		return true
	}
	return string(raw[off+4:off+4+n]) != "none"
}

// ---------- listening ports ----------

var systemProcs = map[string]bool{"system": true, "svchost.exe": true, "lsass.exe": true, "wininit.exe": true, "services.exe": true,
	"spoolsv.exe": true, "system idle process": true, "launchd": true, "mdnsresponder": true, "rapportd": true, "controlce": true,
	"systemd": true, "systemd-resolve": true, "sshd": true, "cupsd": true, "avahi-daemon": true}

// The exposed-listeners check is about AI and developer services only:
// model servers, notebooks, MCP/dev servers and databases. VPNs, sync tools
// and other system programs are out of scope.
var devProcs = map[string]bool{"ollama": true, "ollama app": true, "lms": true, "lm studio": true, "llama-server": true,
	"python": true, "python3": true, "pythonw": true, "node": true, "deno": true, "bun": true, "uvicorn": true, "gunicorn": true,
	"jupyter": true, "jupyter-lab": true, "jupyter-notebook": true, "java": true, "ruby": true, "php": true, "dotnet": true,
	"postgres": true, "mysqld": true, "mariadbd": true, "redis-server": true, "mongod": true, "docker-proxy": true,
	"com.docker.backend": true, "vpnkit": true, "code": true, "cursor": true}
var devPorts = map[string]bool{"11434": true, "1234": true, "8888": true, "7860": true, "8188": true, "3000": true, "5000": true,
	"5173": true, "8000": true, "8080": true, "5432": true, "3306": true, "6379": true, "27017": true}

func isDevService(proc, port string) bool {
	p := strings.TrimSuffix(strings.ToLower(proc), ".exe")
	return devProcs[p] || devPorts[port]
}

type listener struct{ addr, port, proc string }

func collectListeners() (string, []Finding) {
	var ls []listener
	switch runtime.GOOS {
	case "windows":
		ls = windowsListeners()
	case "darwin":
		out, _ := runCmd(20*time.Second, "lsof", "-nP", "-iTCP", "-sTCP:LISTEN")
		for _, line := range strings.Split(out, "\n")[1:] {
			fs := strings.Fields(line)
			if len(fs) < 9 {
				continue
			}
			addr := fs[8]
			i := strings.LastIndex(addr, ":")
			if i < 0 {
				continue
			}
			ls = append(ls, listener{addr[:i], addr[i+1:], fs[0]})
		}
	default:
		out, _ := runCmd(20*time.Second, "ss", "-tlnpH")
		for _, line := range strings.Split(out, "\n") {
			fs := strings.Fields(line)
			if len(fs) < 4 {
				continue
			}
			addr := fs[3]
			i := strings.LastIndex(addr, ":")
			if i < 0 {
				continue
			}
			proc := ""
			if len(fs) >= 6 {
				if m := regexp.MustCompile(`"([^"]+)"`).FindStringSubmatch(fs[5]); m != nil {
					proc = m[1]
				}
			}
			ls = append(ls, listener{addr[:i], addr[i+1:], proc})
		}
	}
	var lines []string
	var exposed []string
	seen := map[string]bool{}
	for _, l := range ls {
		key := l.addr + ":" + l.port + " " + l.proc
		if seen[key] {
			continue
		}
		seen[key] = true
		lines = append(lines, key)
		open := l.addr == "0.0.0.0" || l.addr == "*" || l.addr == "[::]" || l.addr == "::" || l.addr == "[::]:" || l.addr == "*:"
		item := fmt.Sprintf("%s (port %s)", l.proc, l.port)
		if open && !systemProcs[strings.ToLower(l.proc)] && isDevService(l.proc, l.port) && !seen[item] {
			seen[item] = true
			exposed = append(exposed, item)
		}
	}
	sort.Strings(lines)
	var out []Finding
	if len(exposed) > 0 {
		sort.Strings(exposed)
		out = append(out, Finding{ChecklistID: "exposed-listeners", Severity: "medium", Source: "local",
			Title: "AI or dev services reachable from the network", TitleKo: "AI·개발 서비스가 외부에서 접속 가능하게 열려 있음",
			Evidence: strings.Join(exposed, ", "),
			Fix:      "If you do not need access from other devices, bind these to 127.0.0.1 or block them in the firewall.",
			FixKo:    "다른 기기에서 접속할 일이 없으면 127.0.0.1에만 묶거나 방화벽에서 막으세요."})
	}
	if len(lines) == 0 {
		return "Could not list listening ports.", out
	}
	return clip(strings.Join(lines, "\n"), maxSection), out
}

func windowsListeners() []listener {
	out, err := runCmd(20*time.Second, "netstat", "-ano", "-p", "TCP")
	if err != nil {
		return nil
	}
	out6, _ := runCmd(20*time.Second, "netstat", "-ano", "-p", "TCPv6")
	names := map[string]string{}
	if tl, err := runCmd(20*time.Second, "tasklist", "/fo", "csv", "/nh"); err == nil {
		for _, line := range strings.Split(tl, "\n") {
			fs := strings.Split(strings.TrimSpace(line), "\",\"")
			if len(fs) >= 2 {
				names[strings.Trim(fs[1], "\"")] = strings.Trim(fs[0], "\"")
			}
		}
	}
	var ls []listener
	for _, line := range strings.Split(out+"\n"+out6, "\n") {
		fs := strings.Fields(line)
		if len(fs) < 5 || fs[3] != "LISTENING" {
			continue
		}
		addr := fs[1]
		i := strings.LastIndex(addr, ":")
		if i < 0 {
			continue
		}
		ls = append(ls, listener{addr[:i], addr[i+1:], names[fs[4]]})
	}
	return ls
}

// ---------- OS protection ----------

func collectSystem() (string, []Finding) {
	var out []Finding
	switch runtime.GOOS {
	case "windows":
		script := `$ErrorActionPreference='SilentlyContinue';` +
			`$d=Get-MpComputerStatus;` +
			`$f=Get-NetFirewallProfile|ForEach-Object{"$($_.Name)=$($_.Enabled)"};` +
			`$h=Get-HotFix|Where-Object InstalledOn|Sort-Object InstalledOn -Descending|Select-Object -First 1;` +
			// BitLocker state of the system drive, readable without admin rights: 1 on, 2 off, 3 encrypting, 4 decrypting, 5 suspended, 6 locked.
			`$b=(New-Object -ComObject Shell.Application).NameSpace($env:SystemDrive+'\').Self.ExtendedProperty('System.Volume.BitLockerProtection');` +
			`[pscustomobject]@{defender_realtime=$d.RealTimeProtectionEnabled;defender_signature_age_days=$d.AntivirusSignatureAge;bitlocker=$b;` +
			`firewall=($f -join ',');last_update=$(if($h){$h.InstalledOn.ToString('yyyy-MM-dd')}else{''});` +
			`os=(Get-CimInstance Win32_OperatingSystem).Caption}|ConvertTo-Json -Compress`
		raw, err := runCmd(60*time.Second, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
		if err != nil {
			return "Could not read Windows protection status.", nil
		}
		var s struct {
			Realtime   *bool  `json:"defender_realtime"`
			SigAge     *int   `json:"defender_signature_age_days"`
			BitLocker  *int   `json:"bitlocker"`
			Firewall   string `json:"firewall"`
			LastUpdate string `json:"last_update"`
			OS         string `json:"os"`
		}
		i := strings.Index(raw, "{")
		if i >= 0 {
			_ = json.Unmarshal([]byte(raw[i:]), &s)
		}
		if s.Realtime != nil && !*s.Realtime {
			out = append(out, Finding{ChecklistID: "os-protection-off", Severity: "high", Source: "local",
				Title: "Windows Defender real-time protection is off", TitleKo: "Windows Defender 실시간 보호가 꺼져 있음",
				Evidence: "RealTimeProtectionEnabled=False", Fix: "Turn it back on in Windows Security.", FixKo: "Windows 보안에서 다시 켜세요."})
		}
		if strings.Contains(s.Firewall, "False") {
			out = append(out, Finding{ChecklistID: "os-protection-off", Severity: "high", Source: "local",
				Title: "A Windows firewall profile is off", TitleKo: "Windows 방화벽 프로필 중 꺼진 것이 있음",
				Evidence: s.Firewall, Fix: "Turn on all firewall profiles.", FixKo: "모든 방화벽 프로필을 켜세요."})
		}
		if s.BitLocker != nil && (*s.BitLocker == 2 || *s.BitLocker == 4) {
			out = append(out, Finding{ChecklistID: "disk-encryption-off", Severity: "medium", Source: "local",
				Title: "The system drive is not encrypted (BitLocker off)", TitleKo: "시스템 드라이브가 암호화돼 있지 않음 (BitLocker 꺼짐)",
				Evidence: fmt.Sprintf("BitLockerProtection=%d", *s.BitLocker),
				Fix:      "Turn on BitLocker or Device encryption in Settings > Privacy & security. On Windows Home, Device encryption is the option if your PC supports it.",
				FixKo:    "설정 > 개인 정보 및 보안에서 BitLocker나 장치 암호화를 켜세요. Windows Home은 지원되는 PC에서 장치 암호화를 쓸 수 있어요."})
		}
		if s.LastUpdate != "" {
			if t, err := time.Parse("2006-01-02", s.LastUpdate); err == nil && time.Since(t) > 45*24*time.Hour {
				out = append(out, Finding{ChecklistID: "stale-os-updates", Severity: "medium", Source: "local",
					Title: "Last Windows update is more than 45 days old", TitleKo: "마지막 Windows 업데이트가 45일 넘음",
					Evidence: "last update " + s.LastUpdate, Fix: "Run Windows Update.", FixKo: "Windows 업데이트를 실행하세요."})
			}
		}
		return strings.TrimSpace(raw), out
	case "darwin":
		var b strings.Builder
		fw, _ := runCmd(15*time.Second, "/usr/libexec/ApplicationFirewall/socketfilterfw", "--getglobalstate")
		fv, _ := runCmd(15*time.Second, "fdesetup", "status")
		sip, _ := runCmd(15*time.Second, "csrutil", "status")
		ver, _ := runCmd(15*time.Second, "sw_vers")
		fmt.Fprintf(&b, "firewall: %s\nfilevault: %s\nsip: %s\n%s", strings.TrimSpace(fw), strings.TrimSpace(fv), strings.TrimSpace(sip), ver)
		if strings.Contains(fw, "disabled") {
			out = append(out, Finding{ChecklistID: "os-protection-off", Severity: "medium", Source: "local",
				Title: "macOS firewall is off", TitleKo: "macOS 방화벽이 꺼져 있음", Evidence: strings.TrimSpace(fw),
				Fix: "System Settings > Network > Firewall.", FixKo: "시스템 설정 > 네트워크 > 방화벽에서 켜세요."})
		}
		if strings.Contains(fv, "Off") {
			out = append(out, Finding{ChecklistID: "disk-encryption-off", Severity: "medium", Source: "local",
				Title: "FileVault disk encryption is off", TitleKo: "FileVault 디스크 암호화가 꺼져 있음", Evidence: strings.TrimSpace(fv),
				Fix: "System Settings > Privacy & Security > FileVault.", FixKo: "시스템 설정 > 개인정보 보호 및 보안 > FileVault에서 켜세요."})
		}
		return b.String(), out
	default:
		var b strings.Builder
		osr, _ := readSmall("/etc/os-release", 4000)
		pv, _ := readSmall("/proc/version", 1000)
		if strings.Contains(strings.ToLower(pv), "microsoft") {
			// WSL: the Windows side owns the firewall and disk encryption.
			fmt.Fprintf(&b, "WSL (firewall and disk encryption are handled by Windows)\n%s", osr)
			return b.String(), out
		}
		// Firewall: any of the usual services running (no root needed to ask).
		fw, _ := runCmd(15*time.Second, "systemctl", "is-active", "ufw", "firewalld", "nftables")
		states := strings.Fields(fw)
		known, active := len(states) == 3, false
		for _, st := range states {
			switch st {
			case "active":
				active = true
			case "inactive", "failed", "unknown", "activating", "deactivating":
			default:
				known = false // not a systemd answer (no systemd, container, …)
			}
		}
		fmt.Fprintf(&b, "firewall services (ufw firewalld nftables): %s\n", strings.Join(states, " "))
		if known && !active {
			out = append(out, Finding{ChecklistID: "os-protection-off", Severity: "medium", Source: "local",
				Title: "No firewall service is running", TitleKo: "실행 중인 방화벽 서비스가 없음",
				Evidence: "ufw, firewalld, nftables: not active",
				Fix:      "Enable one, for example `sudo ufw enable` on Ubuntu or `sudo systemctl enable --now firewalld` on Fedora.",
				FixKo:    "하나를 켜세요. 우분투는 `sudo ufw enable`, 페도라는 `sudo systemctl enable --now firewalld`."})
		}
		// Disk encryption: any dm-crypt device in the block device tree.
		if blk, err := runCmd(15*time.Second, "lsblk", "-rno", "TYPE"); err == nil {
			enc := strings.Contains(blk, "crypt")
			fmt.Fprintf(&b, "disk encryption (dm-crypt): %v\n", enc)
			if !enc {
				out = append(out, Finding{ChecklistID: "disk-encryption-off", Severity: "medium", Source: "local",
					Title: "No encrypted disk found", TitleKo: "암호화된 디스크가 없음", Evidence: "lsblk: no crypt device",
					Fix:   "Encryption is easiest to set up when installing (LUKS). At least keep keys and tokens out of plain files.",
					FixKo: "암호화(LUKS)는 설치할 때 켜는 게 가장 쉬워요. 그 전까지는 키·토큰을 평문 파일에 두지 마세요."})
			}
		}
		b.WriteString(osr)
		return b.String(), out
	}
}

// ---------- installed tools ----------

func collectTools() string {
	var lines []string
	for _, a := range []string{"claude", "codex", "gemini", "agy", "node", "npm", "python", "python3", "git", "docker", "code", "cursor", "ollama"} {
		bin := agentBinary(a)
		if bin == "" {
			continue
		}
		v, _ := runCmd(15*time.Second, bin, "--version")
		v = strings.TrimSpace(strings.Split(v, "\n")[0])
		lines = append(lines, fmt.Sprintf("%s: %s", a, clip(v, 120)))
	}
	if bin := agentBinary("npm"); bin != "" {
		if out, err := runCmd(40*time.Second, bin, "ls", "-g", "--depth=0", "--json"); err == nil {
			var m struct {
				Dependencies map[string]struct {
					Version string `json:"version"`
				} `json:"dependencies"`
			}
			if json.Unmarshal([]byte(out), &m) == nil {
				var pk []string
				for k, v := range m.Dependencies {
					pk = append(pk, k+"@"+v.Version)
				}
				sort.Strings(pk)
				lines = append(lines, "global npm packages: "+strings.Join(pk, ", "))
			}
		}
	}
	if bin := agentBinary("code"); bin != "" {
		if out, err := runCmd(40*time.Second, bin, "--list-extensions", "--show-versions"); err == nil {
			ext := strings.Fields(out)
			lines = append(lines, fmt.Sprintf("VS Code extensions (%d): %s", len(ext), clip(strings.Join(ext, ", "), 2500)))
		}
	}
	return strings.Join(lines, "\n")
}
