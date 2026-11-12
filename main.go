// Shieldbug — a daily AI security check that runs on the AI you already pay for.
//
// It collects local facts without AI, redacts them, asks your own AI CLI to
// review them with every tool switched off, and shows the result in a small
// local dashboard. It never fixes anything by itself and never stays resident.
package main

import (
	"fmt"
	"os"
	"strings"
)

const version = "0.1.0"

const usage = `Shieldbug %s — a daily AI security check on your own AI subscription

Usage:
  shieldbug                 open the dashboard (same as "open")
  shieldbug open            open the dashboard in an app window
  shieldbug check           run a check now and print the result
  shieldbug init            write ~/.shieldbug/config.json with defaults
  shieldbug doctor          verify the AI CLI works and cannot use tools
  shieldbug collect         print the redacted local facts (no AI)
  shieldbug payload [id]    print exactly what was sent to the model
  shieldbug schedule install|remove|status
  shieldbug install         (downloaded release) copy this file to the install
                            folder without opening the dashboard
  shieldbug shortcut        add the desktop icon and the search entry again
                            (done on the first run; run again after moving this file)
  shieldbug uninstall       remove schedule, shortcuts and ~/.shieldbug
  shieldbug version
`

func main() {
	attachConsole()
	args := os.Args[1:]
	cmd := "open"
	if len(args) > 0 {
		cmd = args[0]
	}
	// A downloaded release installs itself and continues from there.
	if selfInstall(cmd) {
		return
	}
	_, statErr := os.Stat(configPath())
	firstRun := os.IsNotExist(statErr)
	c, err := loadConfig()
	if err != nil {
		fail("config: %v (fix or delete %s)", err, configPath())
	}
	loadExtraPatterns(c)
	// First run: write the config and add the desktop icon and search entry once.
	if firstRun && (cmd == "open" || cmd == "dashboard" || cmd == "init") {
		if err := saveConfig(c); err == nil && !tempBuild(selfPath()) {
			if err := createShortcuts(); err != nil {
				fmt.Fprintln(os.Stderr, "shieldbug:", err)
			}
		}
	}
	switch cmd {
	case "open", "dashboard":
		prune(c)
		if err := serve(c, !has(args, "--no-window"), has(args, "--stay")); err != nil {
			fail("%v", err)
		}
	case "serve":
		if err := serve(c, false, has(args, "--stay")); err != nil {
			fail("%v", err)
		}
	case "check":
		r, err := runCheck(c, has(args, "--scheduled"))
		if r != nil {
			printRun(c, r)
		}
		prune(c)
		if err != nil {
			fail("%v", err)
		}
	case "init":
		if _, err := os.Stat(configPath()); err == nil && !has(args, "--force") {
			fmt.Println("config already exists:", configPath())
		} else if err := saveConfig(c); err != nil {
			fail("%v", err)
		} else {
			fmt.Println("wrote", configPath())
		}
		agents := detectAgents(c)
		if len(agents) == 0 {
			fmt.Println("no AI CLI or API key found; local checks will still run")
		} else {
			fmt.Println("AI found:", strings.Join(agents, ", "))
		}
	case "doctor":
		doctor(c)
	case "collect":
		facts, local := collect(c)
		for _, k := range sortedKeys(facts.Sections) {
			fmt.Printf("## %s\n%s\n\n", k, facts.Sections[k])
		}
		fmt.Printf("## local findings: %d\n", len(local))
		for _, f := range local {
			fmt.Printf("- [%s] %s — %s\n", f.Severity, f.Title, f.Evidence)
		}
	case "payload":
		id := ""
		if len(args) > 1 {
			id = args[1]
		} else if runs := listRuns(1); len(runs) > 0 {
			id = runs[0].ID
		}
		p := readPayload(id)
		if p == "" {
			fail("no payload stored for %q", id)
		}
		fmt.Println(p)
	case "schedule":
		sub := "status"
		if len(args) > 1 {
			sub = args[1]
		}
		switch sub {
		case "install":
			if err := scheduleInstall(c); err != nil {
				fail("%v", err)
			}
			fmt.Println("daily check registered at", c.Schedule.DailyAt)
		case "remove":
			if err := scheduleRemove(); err != nil {
				fail("%v", err)
			}
			fmt.Println("daily check removed")
		default:
			if s := scheduleStatus(); s != "" {
				fmt.Println("on —", s)
			} else {
				fmt.Println("off")
			}
		}
	case "shortcut":
		if err := createShortcuts(); err != nil {
			fail("%v", err)
		}
		fmt.Println("shortcuts created")
	case "uninstall":
		_ = scheduleRemove()
		removeShortcuts()
		if err := os.RemoveAll(dataDir()); err != nil {
			fail("%v", err)
		}
		fmt.Println("removed the schedule, shortcuts and", dataDir())
		fmt.Println("delete this program file to finish:", selfPath())
	case "install":
		if channel == "release" {
			fmt.Println("already installed:", selfPath())
		} else {
			fmt.Println("a build from source runs from where it is:", selfPath())
		}
	case "version", "--version", "-v":
		fmt.Println("shieldbug", version)
	case "help", "--help", "-h":
		fmt.Printf(usage, version)
	default:
		fmt.Printf(usage, version)
		os.Exit(2)
	}
}

func has(args []string, flag string) bool {
	for _, a := range args {
		if a == flag {
			return true
		}
	}
	return false
}

func fail(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "shieldbug: "+format+"\n", a...)
	os.Exit(1)
}

func printRun(c Config, r *Run) {
	lang := c.lang()
	fmt.Printf("\nShieldbug — %s — score %d (%s)\n", r.Started.Format("2006-01-02 15:04"), r.Score, tr(lang, "light_"+r.Light))
	if s := pickStr(lang, r.Summary, r.SummaryKo); s != "" {
		fmt.Println(s)
	}
	for _, f := range r.Findings {
		fmt.Printf("  [%s] %s\n", tr(lang, "sev_"+f.Severity), pickStr(lang, f.Title, f.TitleKo))
	}
	for _, u := range r.Calls {
		fmt.Printf("  %s %s · %.0fs · tokens %d/%d · $%.3f (estimate)\n", u.Agent, u.Model, u.Seconds, u.TokensIn, u.TokensOut, u.CostUSD)
	}
	for _, e := range r.Errors {
		fmt.Println("  note:", e)
	}
}

func pickStr(lang, en, ko string) string {
	if lang == "ko" && ko != "" {
		return ko
	}
	return en
}

func doctor(c Config) {
	ok := true
	if err := selfTest(); err != nil {
		fmt.Println("✗ redaction self-test:", err)
		ok = false
	} else {
		fmt.Println("✓ redaction self-test passed")
	}
	agents := detectAgents(c)
	if len(agents) == 0 {
		fmt.Println("✗", noAgentMsg)
		os.Exit(1)
	}
	agent, err := pickAgent(c)
	if err != nil {
		fmt.Println("✗", err)
		os.Exit(1)
	}
	fmt.Printf("• using %s (%s), tool lockdown: %s\n", agent, c.Models[agent], lockdown[agent])
	reply, err := probe(c, agent)
	if err != nil {
		fmt.Println("✗ probe call failed:", err)
		os.Exit(1)
	}
	up := strings.ToUpper(reply)
	switch {
	case strings.Contains(up, "DONE"):
		fmt.Println("✗ the model says it created a file — tools are NOT locked down. Do not use this agent for checks.")
		ok = false
	default:
		fmt.Printf("✓ the model could not use tools (reply: %q)\n", hidePaths(clip(reply, 80)))
	}
	if !ok {
		os.Exit(1)
	}
}
