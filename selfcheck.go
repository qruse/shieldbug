package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Self-check: a security tool that has been tampered with is worse than
// none, because it runs every day with the user's trust. These rules need no
// AI, always run and cannot be turned off.
//
// 1. The program file matches the official release (release builds only).
// 2. The daily task runs this program and nothing else.
// 3. The Shieldbug shortcuts open this program.
// 4. The AI CLIs it starts are not in places anyone can drop a file into.
// 5. The user's own checks contain no text that steers the AI.

const selfID = "self-integrity"

// assetName is set by release builds: the published file this program came
// from ("shieldbug-windows-x64.exe", or "<archive>.tar.gz:shieldbug").
var assetName = ""

var releaseSums = "https://github.com/qruse/shieldbug/releases/download/v%s/PROGRAMS-SHA256SUMS.txt"

type selfResult struct {
	Lines    []string
	Findings []Finding
}

func (s *selfResult) note(format string, a ...any) {
	s.Lines = append(s.Lines, fmt.Sprintf(format, a...))
}

func (s *selfResult) bad(sev, en, ko, evidence, fix, fixKo string) {
	s.Findings = append(s.Findings, Finding{ChecklistID: selfID, Severity: sev, Source: "local",
		Title: en, TitleKo: ko, Evidence: hidePaths(evidence), Fix: fix, FixKo: fixKo})
}

func selfCheck() selfResult {
	var s selfResult
	exe := selfPath()
	// A Shieldbug program file: this one, the installed copy, or a sibling
	// build in the same folder (shieldbug.exe next to shieldbug-cli.exe).
	ok := func(p string) bool {
		sibling := samePath(filepath.Dir(p), filepath.Dir(exe)) && strings.HasPrefix(strings.ToLower(filepath.Base(p)), "shieldbug")
		return samePath(p, exe) || samePath(p, installPath()) || sibling
	}

	// 1. Program file
	sum, err := fileSHA256(exe)
	switch {
	case err != nil:
		s.note("program file: cannot read (%v)", err)
	case channel != "release" || assetName == "":
		s.note("program file: built from source, not compared with a release")
	default:
		want, err := officialSum(version, assetName)
		switch {
		case err != nil:
			s.note("program file: could not fetch the official checksum (%v)", err)
		case want == "":
			s.note("program file: %s is not listed in the v%s checksums", assetName, version)
		case !strings.EqualFold(want, sum):
			s.bad("critical", "The Shieldbug program file differs from the official release", "Shieldbug 프로그램 파일이 공식 배포본과 다름",
				fmt.Sprintf("%s: sha256 %s, official %s", exe, sum[:16], want[:16]),
				"Do not run it again. Delete it, download Shieldbug from github.com/qruse/shieldbug/releases, and check the PC with your antivirus.",
				"다시 실행하지 마세요. 지우고 github.com/qruse/shieldbug/releases에서 새로 받은 뒤, 백신으로 PC를 검사하세요.")
		default:
			s.note("program file: matches the official v%s release", version)
		}
	}

	// 2. Daily task
	if cmd, args, found := scheduledCommand(); found {
		if !ok(cmd) || !strings.Contains(args, "check") {
			s.bad("critical", "The daily check runs a different program", "매일 점검 예약이 다른 프로그램을 실행함",
				strings.TrimSpace(cmd+" "+args),
				"Turn the daily check off and on again in Settings, then find out who changed it.",
				"설정에서 매일 점검을 껐다가 다시 켜고, 누가 바꿨는지 확인하세요.")
		} else {
			s.note("daily task: runs this program")
		}
	} else {
		s.note("daily task: not registered")
	}

	// 3. Shortcuts
	for _, t := range shortcutTargets() {
		if t != "" && !ok(t) {
			s.bad("high", "A Shieldbug shortcut opens a different program", "Shieldbug 바로가기가 다른 프로그램을 엶", t,
				"Delete the shortcut and run `shieldbug shortcut` to make it again.",
				"바로가기를 지우고 `shieldbug shortcut`으로 다시 만드세요.")
		}
	}

	// 4. AI CLIs Shieldbug starts every day
	for _, a := range allAgents {
		if isAPI(a) {
			continue
		}
		p := agentBinary(a)
		if p == "" {
			continue
		}
		if where, whereKo := riskyPlace(p); where != "" {
			s.bad("high", "The AI program Shieldbug runs is in "+where, "Shieldbug가 실행하는 AI 프로그램이 "+whereKo+"에 있음", p,
				"Reinstall that AI CLI from its vendor and remove the copy found here.",
				"그 AI CLI를 제작사에서 다시 설치하고, 여기서 찾은 파일은 지우세요.")
		} else {
			s.note("%s CLI: %s", a, hidePaths(p))
		}
	}

	// 5. The user's own checks and pending proposals go into the AI prompt.
	for _, name := range []string{"local.json", "proposed.json"} {
		for _, it := range loadItems(name) {
			text := it.Check + " " + it.Title["en"] + " " + it.Title["ko"]
			if reInvisible.MatchString(text) || overrideLine(text) || strings.Contains(strings.ToLower(text), "report nothing") {
				s.bad("high", "One of your checks contains text that steers the AI", "내 점검 항목에 AI를 조종하는 문장이 있음",
					name+": "+it.ID,
					"Remove that item from ~/.shieldbug/checklist/"+name+".",
					"~/.shieldbug/checklist/"+name+"에서 그 항목을 지우세요.")
			}
		}
	}
	return s
}

func fileSHA256(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// officialSum fetches the published checksum list for this version. It is a
// plain download of a public file; nothing about this machine is sent.
func officialSum(ver, asset string) (string, error) {
	cl := &http.Client{Timeout: 15 * time.Second}
	req, _ := http.NewRequest("GET", fmt.Sprintf(releaseSums, ver), nil)
	req.Header.Set("User-Agent", "shieldbug/"+ver)
	resp, err := cl.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return lookupSum(io.LimitReader(resp.Body, 64<<10), asset), nil
}

func lookupSum(r io.Reader, asset string) string {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 2 && f[1] == asset && len(f[0]) == 64 {
			return f[0]
		}
	}
	return ""
}

// scheduledCommand reads back what the OS will run for the daily check.
func scheduledCommand() (cmd, args string, found bool) {
	switch runtime.GOOS {
	case "windows":
		out, err := runCmd(30*time.Second, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command",
			`[Console]::OutputEncoding=[Text.Encoding]::UTF8;$t=Get-ScheduledTask -TaskName '`+taskName+`' -ErrorAction SilentlyContinue;`+
				`if($t){$t.Actions|Select-Object -First 1 Execute,Arguments|ConvertTo-Json -Compress}`)
		if err != nil {
			return "", "", false
		}
		var a struct{ Execute, Arguments string }
		i := strings.Index(out, "{")
		if i < 0 || json.Unmarshal([]byte(out[i:]), &a) != nil {
			return "", "", false
		}
		return strings.Trim(a.Execute, `"`), a.Arguments, true
	case "darwin":
		b, err := os.ReadFile(filepath.Join(homeDir(), "Library", "LaunchAgents", launchdLabel+".plist"))
		if err != nil {
			return "", "", false
		}
		var vals []string
		s := string(b)
		if i := strings.Index(s, "<key>ProgramArguments</key>"); i >= 0 {
			rest := s[i:]
			if j := strings.Index(rest, "</array>"); j >= 0 {
				rest = rest[:j]
			}
			for _, part := range strings.Split(rest, "<string>")[1:] {
				vals = append(vals, xmlUnescape(strings.SplitN(part, "</string>", 2)[0]))
			}
		}
		if len(vals) == 0 {
			return "", "", true
		}
		return vals[0], strings.Join(vals[1:], " "), true
	default:
		if b, err := os.ReadFile(filepath.Join(homeDir(), ".config", "systemd", "user", "shieldbug.service")); err == nil {
			for _, l := range strings.Split(string(b), "\n") {
				if strings.HasPrefix(l, "ExecStart=") {
					f := strings.Fields(strings.TrimPrefix(l, "ExecStart="))
					if len(f) > 0 {
						return f[0], strings.Join(f[1:], " "), true
					}
				}
			}
			return "", "", true
		}
		if cur, err := runCmd(10*time.Second, "crontab", "-l"); err == nil {
			for _, l := range strings.Split(cur, "\n") {
				if strings.Contains(l, "# shieldbug") {
					f := strings.Fields(strings.SplitN(l, "#", 2)[0])
					if len(f) >= 6 {
						return f[5], strings.Join(f[6:], " "), true
					}
					return "", "", true
				}
			}
		}
		return "", "", false
	}
}

func xmlUnescape(s string) string {
	return strings.NewReplacer("&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`, "&apos;", "'").Replace(s)
}

// shortcutTargets: the program each Shieldbug shortcut opens.
func shortcutTargets() []string {
	switch runtime.GOOS {
	case "windows":
		out, err := runCmd(30*time.Second, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command",
			`[Console]::OutputEncoding=[Text.Encoding]::UTF8;$s=New-Object -ComObject WScript.Shell;`+
				`foreach($d in @([Environment]::GetFolderPath('Desktop'),[Environment]::GetFolderPath('Programs'))){`+
				`$p=Join-Path $d 'Shieldbug.lnk';if(Test-Path $p){$s.CreateShortcut($p).TargetPath}}`)
		if err != nil {
			return nil
		}
		var t []string
		for _, l := range strings.Split(out, "\n") {
			if l = strings.TrimSpace(l); l != "" {
				t = append(t, l)
			}
		}
		return t
	case "darwin":
		b, err := os.ReadFile(filepath.Join(homeDir(), "Applications", "Shieldbug.app", "Contents", "MacOS", "shieldbug-open"))
		if err != nil {
			return nil
		}
		for _, l := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(l, "exec ") {
				p := strings.TrimSuffix(strings.TrimPrefix(l, "exec "), " open")
				return []string{strings.ReplaceAll(strings.Trim(p, "'"), `'\''`, "'")}
			}
		}
		return nil
	default:
		b, err := os.ReadFile(filepath.Join(homeDir(), ".local", "share", "applications", "shieldbug.desktop"))
		if err != nil {
			return nil
		}
		for _, l := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(l, "Exec=") {
				p := strings.TrimSuffix(strings.TrimPrefix(l, "Exec="), " open")
				return []string{strings.Trim(p, `"`)}
			}
		}
		return nil
	}
}

// riskyPlace: folders where any program or download can drop a file.
func riskyPlace(p string) (string, string) {
	l := strings.ToLower(filepath.ToSlash(p))
	tmp := strings.ToLower(filepath.ToSlash(os.TempDir()))
	switch {
	case tmp != "" && strings.HasPrefix(l, tmp+"/"), strings.HasPrefix(l, "/tmp/"), strings.HasPrefix(l, "/var/tmp/"):
		return "a temporary folder", "임시 폴더"
	case strings.Contains(l, "/downloads/"):
		return "the Downloads folder", "다운로드 폴더"
	case strings.HasPrefix(l, strings.ToLower(filepath.ToSlash(dataDir()))+"/"):
		return "Shieldbug's data folder", "Shieldbug 데이터 폴더"
	}
	return "", ""
}
