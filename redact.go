package main

import (
	"encoding/json"
	"errors"
	"math"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Everything that leaves this machine for a model goes through redact first.
//
//   layer 0  collectors read presence/counts for secret files, never contents
//   layer 1  structure: values under env / headers / auth-like keys become [SET]
//   layer 2  known token patterns
//   layer 3  high-entropy strings
//   layer 4  identity: home path, user name, host name, e-mail, public IP
//   layer 5  gate: the finished payload is scanned again; any hit aborts the send

type redactRule struct {
	name string
	re   *regexp.Regexp
}

var redactRules = []redactRule{
	{"private-key", regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----[\s\S]*?(-----END [A-Z ]*PRIVATE KEY-----|$)`)},
	{"anthropic", regexp.MustCompile(`sk-ant-[A-Za-z0-9_\-]{10,}`)},
	{"openai", regexp.MustCompile(`sk-(?:proj-|svcacct-|admin-)?[A-Za-z0-9_\-]{20,}`)},
	{"github", regexp.MustCompile(`(?:gh[pousr]_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,})`)},
	{"gitlab", regexp.MustCompile(`glpat-[A-Za-z0-9_\-]{20,}`)},
	{"huggingface", regexp.MustCompile(`hf_[A-Za-z0-9]{30,}`)},
	{"npm", regexp.MustCompile(`npm_[A-Za-z0-9]{30,}`)},
	{"google", regexp.MustCompile(`AIza[0-9A-Za-z_\-]{30,}`)},
	{"aws", regexp.MustCompile(`(?:AKIA|ASIA)[0-9A-Z]{16}`)},
	{"slack", regexp.MustCompile(`xox[abposr]-[A-Za-z0-9\-]{10,}`)},
	{"stripe", regexp.MustCompile(`(?:sk|rk|pk)_(?:live|test)_[A-Za-z0-9]{16,}`)},
	{"discord", regexp.MustCompile(`[MNO][A-Za-z\d]{23,25}\.[\w-]{6}\.[\w-]{27,38}`)},
	{"telegram", regexp.MustCompile(`\b\d{8,10}:[A-Za-z0-9_\-]{35}\b`)},
	{"jwt", regexp.MustCompile(`eyJ[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{6,}`)},
}

var (
	reURLCred   = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.\-]*://)[^\s:/@"']+:[^\s/@"']+@`)
	reURLQuery  = regexp.MustCompile(`(https?://[^\s"'?#]+)\?[^\s"'#]+`)
	reBearer    = regexp.MustCompile(`(?i)\b(bearer|basic|token)\s+([A-Za-z0-9._~+/=\-]{12,})`)
	reNamed     = regexp.MustCompile(`(?i)("?[A-Za-z0-9_.\-]*(?:token|secret|passw(?:or)?d|pwd|api[_\-]?key|apikey|auth|credential|private[_\-]?key|access[_\-]?key|session)[A-Za-z0-9_.\-]*"?\s*[:=]\s*)("[^"\n]*"|'[^'\n]*'|[^\s,}\]]+)`)
	reEntropyTk = regexp.MustCompile(`[A-Za-z0-9_+=\-]{20,}`)
	reEmail     = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	reIPv4      = regexp.MustCompile(`\b(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})\b`)
)

var secretKeyNames = regexp.MustCompile(`(?i)^(env|environment|headers?|auth.*|.*token.*|.*secret.*|.*passw.*|.*api.?key.*|.*credential.*|.*session.*|cookie.*)$`)

// redact applies layers 1–4 to free text. Layer 1 runs when the text is JSON.
func redact(s string) string {
	s = redactStructured(s)
	return redactText(s)
}

func redactText(s string) string {
	for _, r := range redactRules {
		s = r.re.ReplaceAllString(s, "[REDACTED:"+r.name+"]")
	}
	s = reURLCred.ReplaceAllString(s, "${1}[REDACTED:credentials]@")
	s = reURLQuery.ReplaceAllString(s, "${1}?[REDACTED:query]")
	s = reBearer.ReplaceAllString(s, "${1} [REDACTED:bearer]")
	s = reNamed.ReplaceAllStringFunc(s, func(m string) string {
		sub := reNamed.FindStringSubmatch(m)
		v := strings.Trim(sub[2], `"'`)
		if strings.HasPrefix(v, "[REDACTED") || v == "[SET]" || v == "" || isPlainWord(v) {
			return m
		}
		return sub[1] + `"[REDACTED:value]"`
	})
	s = reEntropyTk.ReplaceAllStringFunc(s, func(t string) string {
		if looksRandom(t) {
			return "[REDACTED:high-entropy]"
		}
		return t
	})
	for _, p := range extraPatterns {
		s = p.ReplaceAllString(s, "[REDACTED:custom]")
	}
	return hideIdentity(s)
}

// isPlainWord keeps harmless config values such as "true", "oauth", "keychain".
func isPlainWord(v string) bool {
	if len(v) > 24 {
		return false
	}
	for _, r := range v {
		if !(r >= 'a' && r <= 'z' || r == '-' || r == '_') {
			return false
		}
	}
	return true
}

// looksRandom flags strings that look like keys: long, mixed classes, high entropy.
func looksRandom(t string) bool {
	if strings.HasPrefix(t, "REDACTED") {
		return false
	}
	var lower, upper, digit int
	for _, r := range t {
		switch {
		case r >= 'a' && r <= 'z':
			lower++
		case r >= 'A' && r <= 'Z':
			upper++
		case r >= '0' && r <= '9':
			digit++
		}
	}
	classes := 0
	for _, n := range []int{lower, upper, digit} {
		if n > 0 {
			classes++
		}
	}
	if classes < 2 || digit == 0 && len(t) < 32 {
		return false
	}
	// package names and versions: mostly lowercase words joined by - or _
	if upper == 0 && strings.Count(t, "-")+strings.Count(t, "_") >= 2 && digit*3 < len(t) {
		return false
	}
	return shannon(t) >= 4.0
}

func shannon(s string) float64 {
	freq := map[rune]float64{}
	for _, r := range s {
		freq[r]++
	}
	n := float64(len([]rune(s)))
	var h float64
	for _, c := range freq {
		p := c / n
		h -= p * math.Log2(p)
	}
	return h
}

// redactStructured replaces every value under env/headers/auth-like keys with
// "[SET]" when the text is JSON, regardless of what the value looks like.
// TOML-style "[section.env]" blocks get the same treatment line by line.
func redactStructured(s string) string {
	t := strings.TrimSpace(s)
	if strings.HasPrefix(t, "{") || strings.HasPrefix(t, "[") && json.Valid([]byte(t)) {
		var v any
		if json.Unmarshal([]byte(t), &v) == nil {
			v = walkRedact(v, false)
			b, err := json.MarshalIndent(v, "", " ")
			if err == nil {
				return string(b)
			}
		}
	}
	lines := strings.Split(s, "\n")
	inSecret := false
	for i, l := range lines {
		tl := strings.TrimSpace(l)
		if strings.HasPrefix(tl, "[") && strings.HasSuffix(tl, "]") {
			sec := strings.ToLower(strings.Trim(tl, "[]"))
			last := sec[strings.LastIndex(sec, ".")+1:]
			inSecret = secretKeyNames.MatchString(last)
			continue
		}
		if k, _, ok := strings.Cut(tl, "="); ok && !strings.HasPrefix(tl, "#") {
			key := strings.Trim(strings.TrimSpace(k), `"'`)
			if inSecret || secretKeyNames.MatchString(key) && key != "auth_method" {
				if strings.HasPrefix(strings.TrimSpace(strings.SplitN(l, "=", 2)[1]), "{") {
					lines[i] = strings.SplitN(l, "=", 2)[0] + `= { [SET] }`
				} else {
					lines[i] = strings.SplitN(l, "=", 2)[0] + `= "[SET]"`
				}
			}
		}
	}
	return strings.Join(lines, "\n")
}

func walkRedact(v any, secret bool) any {
	switch x := v.(type) {
	case map[string]any:
		for k, val := range x {
			x[k] = walkRedact(val, secret || secretKeyNames.MatchString(k))
		}
		return x
	case []any:
		for i := range x {
			x[i] = walkRedact(x[i], secret)
		}
		return x
	case string:
		if secret {
			return "[SET]"
		}
		return x
	case nil, bool, float64:
		if secret && x != nil {
			return "[SET]"
		}
		return x
	}
	return v
}

// ---------- identity ----------

var identity []string // filled once: home, user name, host name

func identityStrings() []string {
	if identity != nil {
		return identity
	}
	set := map[string]bool{}
	if h := homeDir(); h != "" && h != "." {
		set[h] = true
		set[filepath.ToSlash(h)] = true
		set[strings.ReplaceAll(h, `\`, `\\`)] = true
	}
	for _, k := range []string{"USERNAME", "USER", "COMPUTERNAME", "HOSTNAME", "USERDOMAIN"} {
		if v := os.Getenv(k); len(v) >= 4 {
			set[v] = true
		}
	}
	if hn, err := os.Hostname(); err == nil && len(hn) >= 4 {
		set[hn] = true
	}
	for k := range set {
		identity = append(identity, k)
	}
	sort.Slice(identity, func(i, j int) bool { return len(identity[i]) > len(identity[j]) })
	return identity
}

func hideIdentity(s string) string {
	h := homeDir()
	for _, v := range identityStrings() {
		repl := "<user>"
		if v == h || v == filepath.ToSlash(h) || v == strings.ReplaceAll(h, `\`, `\\`) {
			repl = "~"
		} else if hn, _ := os.Hostname(); v == hn || v == os.Getenv("COMPUTERNAME") || v == os.Getenv("USERDOMAIN") {
			repl = "<host>"
		}
		s = replaceWord(s, v, repl)
	}
	s = reEmail.ReplaceAllStringFunc(s, func(m string) string {
		if strings.HasSuffix(m, "@users.noreply.github.com") || strings.HasPrefix(m, "noreply@") {
			return m
		}
		// git@github.com style remotes are not personal
		if strings.HasPrefix(m, "git@") {
			return m
		}
		return "<email>"
	})
	s = reIPv4.ReplaceAllStringFunc(s, func(m string) string {
		ip := net.ParseIP(m)
		if ip == nil || ip.IsPrivate() || ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() {
			return m
		}
		return "<public-ip>"
	})
	return s
}

// replaceWord replaces v only where it is not part of a longer word, so a short
// user name does not rewrite unrelated text.
func replaceWord(s, v, repl string) string {
	if v == "" {
		return s
	}
	var b strings.Builder
	for {
		i := strings.Index(s, v)
		if i < 0 {
			b.WriteString(s)
			return b.String()
		}
		before := i == 0 || !isWordByte(s[i-1])
		after := i+len(v) >= len(s) || !isWordByte(s[i+len(v)])
		b.WriteString(s[:i])
		if before && after {
			b.WriteString(repl)
		} else {
			b.WriteString(v)
		}
		s = s[i+len(v):]
	}
}

func isWordByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_'
}

// hidePaths is kept for evidence strings built from file paths.
func hidePaths(s string) string { return hideIdentity(s) }

// ---------- layer 5: the gate ----------

var errGate = errors.New("redaction gate: the payload still contains something that looks like a secret; nothing was sent")

// gate scans a finished payload. It returns the names of the detectors that fired.
func gate(payload string) []string {
	hits := secretHits(payload)
	for _, v := range identityStrings() {
		if replaceWord(payload, v, "\x00") != payload {
			hits = append(hits, "identity")
			break
		}
	}
	return hits
}

// secretHits runs the detectors that look for credentials (not identity).
func secretHits(payload string) []string {
	var hits []string
	for _, r := range redactRules {
		if r.re.MatchString(payload) {
			hits = append(hits, r.name)
		}
	}
	if reURLCred.MatchString(payload) && !strings.Contains(payload, "[REDACTED:credentials]@") {
		hits = append(hits, "url-credentials")
	}
	for _, t := range reEntropyTk.FindAllString(payload, -1) {
		if looksRandom(t) {
			hits = append(hits, "high-entropy")
			break
		}
	}
	return hits
}

// plainSecretValues counts literal values stored under env/headers/auth-like keys
// in a JSON or TOML config. "${VAR}" references and empty values do not count.
func plainSecretValues(s string) int {
	isRef := func(v string) bool {
		v = strings.TrimSpace(strings.Trim(strings.TrimSpace(v), `"'`))
		return v == "" || strings.HasPrefix(v, "${") || strings.HasPrefix(v, "$") || strings.HasPrefix(v, "%") || strings.HasPrefix(v, "{")
	}
	// A value counts as a stored secret when its own key says so (…TOKEN, …_KEY,
	// PASSWORD…) or when it looks like a credential. Settings such as "70" or
	// "false" under env do not count.
	strongKey := regexp.MustCompile(`(?i)(token|secret|passw|api.?key|_key$|^key$|credential|private|auth(orization)?$|cookie|session)`)
	looksSecret := func(key, v string) bool {
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		if isRef(v) || isPlainWord(v) {
			return false
		}
		if strongKey.MatchString(key) && len(v) >= 8 {
			return true
		}
		// Without a secret-sounding key, only a known token format counts. Random
		// pipe names or IDs in paths are still redacted, but do not raise a warning.
		return patternHits(v) > 0
	}
	n := 0
	var walk func(v any, key string)
	walk = func(v any, key string) {
		switch x := v.(type) {
		case map[string]any:
			for k, val := range x {
				walk(val, k)
			}
		case []any:
			for _, e := range x {
				walk(e, key)
			}
		case string:
			if looksSecret(key, x) {
				n++
			}
		}
	}
	t := strings.TrimSpace(s)
	var v any
	if strings.HasPrefix(t, "{") && json.Unmarshal([]byte(t), &v) == nil {
		walk(v, "")
		return n
	}
	for _, l := range strings.Split(s, "\n") {
		tl := strings.TrimSpace(l)
		if strings.HasPrefix(tl, "#") || strings.HasPrefix(tl, "[") {
			continue
		}
		if k, val, ok := strings.Cut(tl, "="); ok {
			if looksSecret(strings.Trim(strings.TrimSpace(k), `"'`), val) {
				n++
			}
		}
	}
	return n
}

// patternHits is secretHits without the entropy detector: used to decide whether
// to warn the user, where a random-looking setting name should not raise an alarm.
func patternHits(s string) int {
	n := 0
	for _, r := range redactRules {
		if r.re.MatchString(s) {
			n++
		}
	}
	if reURLCred.MatchString(s) {
		n++
	}
	return n
}

// selfTest proves on every run that the layers catch a set of fake keys.
func selfTest() error {
	samples := []string{
		`{"mcpServers":{"x":{"command":"npx","args":["-y","pkg"],"env":{"WHATEVER":"zzzzplainvalue"}}}}`,
		"ANTHROPIC_API_KEY=sk-ant-api03-" + strings.Repeat("Ab1", 12),
		"token: ghp_" + strings.Repeat("aB3", 12),
		"https://user:hunter2pass@example.com/repo.git",
		"key " + "AKIA" + "ABCDEFGHIJKLMNOP",
		"Authorization: Bearer " + strings.Repeat("Zx9", 10),
		"opaque Q7f2LmN9pR4sT8vW1xY5zA3bC6dE0gHj",
		homeDir() + string(filepath.Separator) + "secret",
	}
	// Each sample is redacted on its own, the way collectors redact each file.
	var parts []string
	for _, s := range samples {
		parts = append(parts, redact(s))
	}
	out := strings.Join(parts, "\n")
	if hits := gate(out); len(hits) > 0 {
		return errors.New("self-test failed: " + strings.Join(hits, ", "))
	}
	if strings.Contains(out, "zzzzplainvalue") || strings.Contains(out, "hunter2pass") {
		return errors.New("self-test failed: structured or URL value leaked")
	}
	return nil
}

var extraPatterns []*regexp.Regexp

func loadExtraPatterns(c Config) {
	extraPatterns = nil
	for _, p := range c.Privacy.ExtraRedactPatterns {
		if re, err := regexp.Compile(p); err == nil {
			extraPatterns = append(extraPatterns, re)
		}
	}
}
