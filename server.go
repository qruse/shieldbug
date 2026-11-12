package main

import (
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

//go:embed web/index.html
var indexHTML string

// Pretendard (SIL Open Font License 1.1, web/fonts/LICENSE-Pretendard.txt),
// KS X 1001 subset. Served from the binary so the dashboard never loads
// anything from the internet.
//
//go:embed web/fonts/*.woff2
var fontFS embed.FS

func handleFont(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/fonts/")
	b, err := fontFS.ReadFile("web/fonts/" + name)
	if err != nil || strings.ContainsAny(name, "/\\") {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "font/woff2")
	w.Header().Set("Cache-Control", "max-age=31536000, immutable")
	w.Write(b)
}

type server struct {
	cfg      Config
	token    string
	port     int
	lastSeen atomic.Int64 // unix seconds of the last request
	pinged   atomic.Bool  // the window has sent at least one heartbeat
	running  atomic.Bool
	lastErr  atomic.Value
	mu       sync.Mutex
	cfgMod   time.Time // config.json modification time when cfg was loaded
	tmpl     *template.Template
	srv      *http.Server
	exePath  string    // this program file
	exeMod   time.Time // its modification time when this process started
}

// updated: the program file was replaced (a new version) while this dashboard
// was running.
func (s *server) updated() bool {
	if s.exePath == "" {
		return false
	}
	st, err := os.Stat(s.exePath)
	return err == nil && !st.ModTime().Equal(s.exeMod)
}

// Handoff to a new version: the old process frees the port and starts the new
// program file on the same port with the same token, so the open window only
// has to reload. Passed through the environment, never the command line.
const (
	envHandoffToken = "SHIELDBUG_HANDOFF_TOKEN"
	envHandoffPort  = "SHIELDBUG_HANDOFF_PORT"
)

func (s *server) handleRestart(w http.ResponseWriter, r *http.Request) {
	if p, _ := os.ReadFile(progressPath()); s.running.Load() || len(p) > 0 {
		http.Error(w, "a check is running", http.StatusConflict)
		return
	}
	if !s.updated() {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.WriteHeader(http.StatusAccepted)
	go func() {
		time.Sleep(150 * time.Millisecond) // let this response reach the page
		// Start the new version first (it waits for the port), then close this
		// server: closing makes serve() return and this process exit.
		cmd := exec.Command(s.exePath, "serve")
		cmd.Env = append(os.Environ(), envHandoffToken+"="+s.token, envHandoffPort+"="+strconv.Itoa(s.port))
		if f, err := os.OpenFile(filepath.Join(dataDir(), "restart.log"), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600); err == nil {
			cmd.Stdout, cmd.Stderr = f, f // why a handoff failed, if it does (no token is printed)
		}
		hideWindow(cmd)
		if err := cmd.Start(); err != nil {
			fmt.Fprintln(os.Stderr, "shieldbug: restart:", err)
			return // keep serving the old version
		}
		_ = s.srv.Close()
	}()
}

// reloadIfChanged picks up edits made to config.json outside the dashboard
// (by hand or by the user's own AI), so a page refresh shows them.
func (s *server) reloadIfChanged() {
	st, err := os.Stat(configPath())
	if err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if st.ModTime().Equal(s.cfgMod) {
		return
	}
	if c, err := loadConfig(); err == nil {
		s.cfg = c
		loadExtraPatterns(c)
	}
	s.cfgMod = st.ModTime()
}

func newToken() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// serve starts the dashboard on 127.0.0.1 only and returns its URL.
func serve(c Config, openWindow bool, stay bool) error {
	s := &server{cfg: c, token: newToken()}
	s.lastSeen.Store(time.Now().Unix())
	s.lastErr.Store("")
	s.tmpl = parseIndex()
	if st, err := os.Stat(configPath()); err == nil {
		s.cfgMod = st.ModTime()
	}
	return serveListen(s, c, openWindow, stay)
}

func parseIndex() *template.Template {
	funcs := template.FuncMap{
		"t":   func(lang, key string) string { return tr(lang, key) },
		"sev": func(lang, s string) string { return tr(lang, "sev_"+s) },
		"pick": func(lang, en, ko string) string {
			if lang == "ko" && ko != "" {
				return ko
			}
			return en
		},
		"ago":     func(lang string, t time.Time) string { return ago(lang, t) },
		"fmtTime": func(t time.Time) string { return t.Format("2006-01-02 15:04") },
		"money": func(f float64) string {
			if f <= 0 {
				return "—"
			}
			return fmt.Sprintf("$%.3f", f)
		},
		"secs":   func(f float64) string { return fmt.Sprintf("%.0fs", f) },
		"sum":    sumCost,
		"tokens": sumTokens,
		"ring":   func(score int) float64 { return 2 * 3.14159 * 52 * float64(score) / 100 },
		"spark":  sparkPath,
		"json":   func(v any) string { b, _ := json.Marshal(v); return string(b) },
		"lower":  strings.ToLower,
		"has": func(list []string, v string) bool {
			for _, x := range list {
				if x == v {
					return true
				}
			}
			return false
		},
		"count": countSev,
		"selfbad": func(fs []Finding) bool {
			return slices.ContainsFunc(fs, func(f Finding) bool { return f.ChecklistID == selfID })
		},
		"isapi":   isAPI,
		"tf":      trf,
		"aname":   agentName,
		"pct":     pct,
		"major":   func(fs []Finding) []Finding { return splitFindings(fs, true) },
		"minor":   func(fs []Finding) []Finding { return splitFindings(fs, false) },
		"plan5h":  func(r *Run) float64 { v, _, _ := planShare(*r); return v },
		"planwk":  func(r *Run) float64 { _, v, _ := planShare(*r); return v },
		"hasplan": func(r *Run) bool { _, _, ok := planShare(*r); return ok },
		"effort":  hasEffort,
		"models":  modelChoices,
		"efforts": effortChoices,
	}
	return template.Must(template.New("index").Funcs(funcs).Parse(indexHTML))
}

func serveListen(s *server, c Config, openWindow bool, stay bool) error {
	s.exePath = selfPath()
	if st, err := os.Stat(s.exePath); err == nil {
		s.exeMod = st.ModTime()
	}
	port := c.Dashboard.Port
	handoff := false
	if tok := os.Getenv(envHandoffToken); tok != "" {
		handoff = true
		// Started by an older version handing over: keep its token and port.
		s.token = tok
		if p, err := strconv.Atoi(os.Getenv(envHandoffPort)); err == nil {
			port = p
		}
		os.Unsetenv(envHandoffToken)
		os.Unsetenv(envHandoffPort)
		openWindow = false
	}
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	for i := 0; err != nil && handoff && i < 20; i++ {
		time.Sleep(150 * time.Millisecond) // the old process is still letting go of the port
		ln, err = net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	}
	if err != nil {
		ln, err = net.Listen("tcp", "127.0.0.1:0") // port in use: take any free one
		if err != nil {
			return err
		}
	}
	s.port = ln.Addr().(*net.TCPAddr).Port
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.guard(s.handleIndex))
	mux.HandleFunc("/fonts/", s.guard(handleFont))
	mux.HandleFunc("/ping", s.guard(s.handlePing))
	mux.HandleFunc("/status", s.guard(s.handleStatus))
	mux.HandleFunc("/run", s.guard(s.post(s.handleRun)))
	mux.HandleFunc("/proposal", s.guard(s.post(s.handleProposal)))
	mux.HandleFunc("/settings", s.guard(s.post(s.handleSettings)))
	mux.HandleFunc("/schedule", s.guard(s.post(s.handleSchedule)))
	mux.HandleFunc("/restart", s.guard(s.post(s.handleRestart)))
	mux.HandleFunc("/checklist", s.guard(s.post(s.handleChecklist)))
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	s.srv = srv

	url := fmt.Sprintf("http://127.0.0.1:%d/?t=%s", s.port, s.token)
	fmt.Println("Shieldbug dashboard:", url)
	if openWindow {
		go func() { time.Sleep(200 * time.Millisecond); openAppWindow(url) }()
	}
	if !stay {
		go s.idleWatch(srv)
	}
	err = srv.Serve(ln)
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

// idleWatch shuts the server down when the window is closed (no heartbeat for
// 60 s) or nothing has asked for anything in idle_exit_min minutes.
func (s *server) idleWatch(srv *http.Server) {
	for range time.Tick(10 * time.Second) {
		if s.running.Load() {
			continue
		}
		idle := time.Since(time.Unix(s.lastSeen.Load(), 0))
		if (s.pinged.Load() && idle > 60*time.Second) || idle > time.Duration(s.cfg.Dashboard.IdleExitMin)*time.Minute {
			_ = srv.Close()
			return
		}
	}
}

// guard: localhost Host header only (DNS rebinding), and the per-launch token
// from the URL or the cookie it sets.
func (s *server) guard(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		okHost := host == fmt.Sprintf("127.0.0.1:%d", s.port) || host == fmt.Sprintf("localhost:%d", s.port)
		if !okHost {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		tok := r.URL.Query().Get("t")
		if tok == "" {
			if ck, err := r.Cookie("sb"); err == nil {
				tok = ck.Value
			}
		}
		if subtle.ConstantTimeCompare([]byte(tok), []byte(s.token)) != 1 {
			http.Error(w, "Open Shieldbug from its shortcut or run `shieldbug open`.", http.StatusUnauthorized)
			return
		}
		if r.URL.Query().Get("t") != "" {
			http.SetCookie(w, &http.Cookie{Name: "sb", Value: s.token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
		}
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; script-src 'unsafe-inline'; img-src data:; font-src 'self'; connect-src 'self'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		// same-origin, not no-referrer: with no-referrer Chromium sends "Origin: null"
		// on form posts and post() would reject every save. The token-bearing URL
		// still never leaves this origin.
		w.Header().Set("Referrer-Policy", "same-origin")
		s.lastSeen.Store(time.Now().Unix())
		h(w, r)
	}
}

// post: POST only, same-origin, and the form must carry the token again.
func (s *server) post(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		if o := r.Header.Get("Origin"); o != "" && o != fmt.Sprintf("http://127.0.0.1:%d", s.port) && o != fmt.Sprintf("http://localhost:%d", s.port) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		_ = r.ParseForm()
		if subtle.ConstantTimeCompare([]byte(r.FormValue("csrf")), []byte(s.token)) != 1 {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		h(w, r)
	}
}

type pageData struct {
	Lang      string
	Tab       string
	Token     string
	Run       *Run
	Runs      []Run
	Running   bool
	Progress  string
	Checklist []ChecklistItem
	Pending   []ChecklistItem
	Cfg       Config
	Agents    []string
	AllAgents []string
	Chosen    []string
	Update    bool // a newer program file is on disk
	Core      []ChecklistItem
	Added     []ChecklistItem
	Lockdown  map[string]string
	Payload   string
	PayloadID string
	Schedule  string
	Flash     string
	Version   string
	NextRun   string
}

func (s *server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	s.reloadIfChanged()
	s.mu.Lock()
	cfg := s.cfg
	s.mu.Unlock()
	lang := cfg.lang()
	if l := r.URL.Query().Get("lang"); l == "ko" || l == "en" {
		lang = l
	}
	tab := r.URL.Query().Get("tab")
	if tab == "" {
		tab = "today"
	}
	runs := listRuns(60)
	d := pageData{Lang: lang, Tab: tab, Token: s.token, Runs: runs, Running: s.running.Load(), Cfg: cfg, Update: s.updated(),
		AllAgents: allAgents, Lockdown: lockdown,
		Checklist: activeChecklist(cfg), Pending: pendingProposals(), Version: version,
		Flash: r.URL.Query().Get("msg")}
	if tab == "settings" {
		d.Schedule = scheduleStatus()
		d.Agents = detectAgents(cfg)
		d.Chosen = chosenAgents(cfg)
	}
	if tab == "checklist" {
		d.Core, d.Added = loadBase(), addedChecklist()
	}
	if b, err := os.ReadFile(progressPath()); err == nil {
		d.Progress = strings.TrimSpace(string(b))
		if d.Progress != "" {
			d.Running = true
		}
	}
	if len(runs) > 0 {
		d.Run = &runs[0]
	}
	if tab == "sent" {
		d.PayloadID = r.URL.Query().Get("id")
		if d.PayloadID == "" && d.Run != nil {
			d.PayloadID = d.Run.ID
		}
		d.Payload = readPayload(d.PayloadID)
	}
	if e, _ := s.lastErr.Load().(string); e != "" && d.Flash == "" {
		d.Flash = e
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tmpl.Execute(w, d); err != nil {
		http.Error(w, err.Error(), 500)
	}
}

func (s *server) handlePing(w http.ResponseWriter, r *http.Request) {
	s.pinged.Store(true)
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) handleStatus(w http.ResponseWriter, r *http.Request) {
	p, _ := os.ReadFile(progressPath())
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"running": s.running.Load() || len(p) > 0, "progress": strings.TrimSpace(string(p)), "update": s.updated()})
}

func (s *server) handleRun(w http.ResponseWriter, r *http.Request) {
	s.reloadIfChanged()
	if s.running.CompareAndSwap(false, true) {
		s.lastErr.Store("")
		go func() {
			defer s.running.Store(false)
			s.mu.Lock()
			cfg := s.cfg
			s.mu.Unlock()
			if _, err := runCheck(cfg, false); err != nil {
				s.lastErr.Store(err.Error())
			}
		}()
	}
	http.Redirect(w, r, "/?tab=today", http.StatusSeeOther)
}

// handleChecklist turns a core check off or on (kept in config.json), or
// removes a check the user added. Core checks are never deleted, only skipped.
func (s *server) handleChecklist(w http.ResponseWriter, r *http.Request) {
	id := r.FormValue("id")
	switch r.FormValue("action") {
	case "remove":
		removeAdded(id)
	case "off", "on":
		known := false
		for _, it := range loadBase() {
			known = known || (it.ID == id && !it.Locked)
		}
		if !known {
			break // not a core check, or the self-check, which stays on
		}
		s.mu.Lock()
		c := s.cfg
		c.Checks.DisabledIDs = slices.DeleteFunc(slices.Clone(c.Checks.DisabledIDs), func(x string) bool { return x == id })
		if r.FormValue("action") == "off" {
			c.Checks.DisabledIDs = append(c.Checks.DisabledIDs, id)
		}
		_ = saveConfig(c)
		s.cfg = c
		s.mu.Unlock()
	}
	http.Redirect(w, r, "/?tab=checklist", http.StatusSeeOther)
}

func (s *server) handleProposal(w http.ResponseWriter, r *http.Request) {
	decideProposal(r.FormValue("id"), r.FormValue("decision") == "approve")
	http.Redirect(w, r, "/?tab=checklist", http.StatusSeeOther)
}

func (s *server) handleSettings(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	c := s.cfg
	use := []string{} // ticked AIs; none ticked = local checks only
	for _, v := range r.Form["use"] {
		if slices.Contains(allAgents, v) && !slices.Contains(use, v) {
			use = append(use, v)
		}
	}
	c.Use = use
	for _, a := range allAgents {
		if v := strings.TrimSpace(r.FormValue("model_" + a)); v != "" && len(v) < 80 && !strings.ContainsAny(v, " \"'`;&|<>?#%") {
			c.Models[a] = v
		}
		if v := r.FormValue("effort_" + a); v == "" || slices.Contains(effortChoices(a), v) {
			if hasEffort(a) {
				c.Effort[a] = v
			}
		}
	}
	if v := r.FormValue("daily_at"); len(v) == 5 && v[2] == ':' {
		if h, err := strconv.Atoi(v[:2]); err == nil && h < 24 {
			if m, err := strconv.Atoi(v[3:]); err == nil && m < 60 {
				c.Schedule.DailyAt = v
			}
		}
	}
	c.Intel.ModelSearch = r.FormValue("model_search") == "on"
	if v := r.FormValue("language"); v == "auto" || v == "ko" || v == "en" {
		c.Language = v
	}
	_ = saveConfig(c)
	s.cfg = c
	s.mu.Unlock()
	msg := "saved"
	if scheduleStatus() != "" {
		if err := scheduleInstall(c); err != nil {
			msg = err.Error()
		}
	}
	http.Redirect(w, r, "/?tab=settings&msg="+msg, http.StatusSeeOther)
}

func (s *server) handleSchedule(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	c := s.cfg
	s.mu.Unlock()
	var err error
	if r.FormValue("action") == "remove" {
		err = scheduleRemove()
	} else {
		err = scheduleInstall(c)
	}
	msg := "saved"
	if err != nil {
		msg = err.Error()
	}
	http.Redirect(w, r, "/?tab=settings&msg="+msg, http.StatusSeeOther)
}

// ---------- helpers for the template ----------

func sumCost(calls []Usage) float64 {
	var t float64
	for _, c := range calls {
		t += c.CostUSD
	}
	return t
}

func sumTokens(calls []Usage) string {
	in, out := 0, 0
	for _, c := range calls {
		in += c.TokensIn
		out += c.TokensOut
	}
	if in+out == 0 {
		return "—"
	}
	return fmt.Sprintf("%s / %s", kfmt(in), kfmt(out))
}

func kfmt(n int) string {
	if n >= 1000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return strconv.Itoa(n)
}

func countSev(fs []Finding, sev string) int {
	n := 0
	for _, f := range fs {
		if f.Severity == sev {
			n++
		}
	}
	return n
}

// sparkPath draws the score trend, oldest on the left, as an SVG path.
func sparkPath(runs []Run) string {
	n := len(runs)
	if n < 2 {
		return ""
	}
	if n > 30 {
		n = 30
	}
	var b strings.Builder
	for i := 0; i < n; i++ {
		r := runs[n-1-i]
		x := 4 + float64(i)*(292/float64(n-1))
		y := 56 - float64(r.Score)*0.52
		if i == 0 {
			fmt.Fprintf(&b, "M%.1f %.1f", x, y)
		} else {
			fmt.Fprintf(&b, " L%.1f %.1f", x, y)
		}
	}
	return b.String()
}

func ago(lang string, t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return tr(lang, "just_now")
	case d < time.Hour:
		return fmt.Sprintf(tr(lang, "min_ago"), int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf(tr(lang, "h_ago"), int(d.Hours()))
	default:
		return fmt.Sprintf(tr(lang, "d_ago"), int(d.Hours()/24))
	}
}
