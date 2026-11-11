package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// The daily check is registered with the OS scheduler as the current user.
// No admin rights, no resident process.

const taskName = "Shieldbug daily check"
const launchdLabel = "space.hslab.shieldbug"

func selfPath() string {
	p, err := os.Executable()
	if err != nil {
		return "shieldbug"
	}
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return p
}

func scheduleInstall(c Config) error {
	exe := selfPath()
	hm := c.Schedule.DailyAt
	if _, err := time.Parse("15:04", hm); err != nil {
		return fmt.Errorf("bad time %q", hm)
	}
	switch runtime.GOOS {
	case "windows":
		tr := fmt.Sprintf(`"%s" check --scheduled`, exe)
		out, err := runCmd(30*time.Second, "schtasks", "/Create", "/F", "/SC", "DAILY", "/TN", taskName, "/TR", tr, "/ST", hm)
		if err != nil {
			return fmt.Errorf("schtasks: %s", strings.TrimSpace(out))
		}
		return nil
	case "darwin":
		h, m := hm[:2], hm[3:]
		plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>%s</string>
<key>ProgramArguments</key><array><string>%s</string><string>check</string><string>--scheduled</string></array>
<key>StartCalendarInterval</key><dict><key>Hour</key><integer>%s</integer><key>Minute</key><integer>%s</integer></dict>
<key>StandardOutPath</key><string>%s</string><key>StandardErrorPath</key><string>%s</string>
</dict></plist>
`, launchdLabel, exe, strings.TrimLeft(h, "0")+zeroIfEmpty(h), strings.TrimLeft(m, "0")+zeroIfEmpty(m),
			filepath.Join(dataDir(), "scheduled.log"), filepath.Join(dataDir(), "scheduled.log"))
		p := filepath.Join(homeDir(), "Library", "LaunchAgents", launchdLabel+".plist")
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		_, _ = runCmd(10*time.Second, "launchctl", "unload", p)
		if err := os.WriteFile(p, []byte(plist), 0o644); err != nil {
			return err
		}
		if out, err := runCmd(10*time.Second, "launchctl", "load", p); err != nil {
			return fmt.Errorf("launchctl: %s", strings.TrimSpace(out))
		}
		return nil
	default:
		if _, err := exec.LookPath("systemctl"); err == nil {
			dir := filepath.Join(homeDir(), ".config", "systemd", "user")
			_ = os.MkdirAll(dir, 0o755)
			svc := fmt.Sprintf("[Unit]\nDescription=Shieldbug daily check\n\n[Service]\nType=oneshot\nExecStart=%s check --scheduled\n", exe)
			tmr := fmt.Sprintf("[Unit]\nDescription=Shieldbug daily check\n\n[Timer]\nOnCalendar=*-*-* %s:00\nPersistent=true\n\n[Install]\nWantedBy=timers.target\n", hm)
			if err := os.WriteFile(filepath.Join(dir, "shieldbug.service"), []byte(svc), 0o644); err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(dir, "shieldbug.timer"), []byte(tmr), 0o644); err != nil {
				return err
			}
			_, _ = runCmd(15*time.Second, "systemctl", "--user", "daemon-reload")
			if out, err := runCmd(15*time.Second, "systemctl", "--user", "enable", "--now", "shieldbug.timer"); err == nil {
				return nil
			} else if !strings.Contains(out, "Failed to connect") {
				return fmt.Errorf("systemctl: %s", strings.TrimSpace(out))
			}
		}
		// cron fallback
		cur, _ := runCmd(10*time.Second, "crontab", "-l")
		var keep []string
		for _, l := range strings.Split(cur, "\n") {
			if strings.TrimSpace(l) != "" && !strings.Contains(l, "# shieldbug") && !strings.HasPrefix(l, "no crontab") {
				keep = append(keep, l)
			}
		}
		keep = append(keep, fmt.Sprintf("%s %s * * * %s check --scheduled # shieldbug", strings.TrimLeft(hm[3:], "0")+zeroIfEmpty(hm[3:]), strings.TrimLeft(hm[:2], "0")+zeroIfEmpty(hm[:2]), exe))
		cmd := exec.Command("crontab", "-")
		cmd.Stdin = strings.NewReader(strings.Join(keep, "\n") + "\n")
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("crontab: %s", strings.TrimSpace(string(out)))
		}
		return nil
	}
}

func zeroIfEmpty(s string) string {
	if strings.TrimLeft(s, "0") == "" {
		return "0"
	}
	return ""
}

func scheduleRemove() error {
	switch runtime.GOOS {
	case "windows":
		out, err := runCmd(30*time.Second, "schtasks", "/Delete", "/F", "/TN", taskName)
		if err != nil && !strings.Contains(out, "cannot find") {
			return fmt.Errorf("schtasks: %s", strings.TrimSpace(out))
		}
	case "darwin":
		p := filepath.Join(homeDir(), "Library", "LaunchAgents", launchdLabel+".plist")
		_, _ = runCmd(10*time.Second, "launchctl", "unload", p)
		_ = os.Remove(p)
	default:
		_, _ = runCmd(15*time.Second, "systemctl", "--user", "disable", "--now", "shieldbug.timer")
		dir := filepath.Join(homeDir(), ".config", "systemd", "user")
		_ = os.Remove(filepath.Join(dir, "shieldbug.timer"))
		_ = os.Remove(filepath.Join(dir, "shieldbug.service"))
		if cur, err := runCmd(10*time.Second, "crontab", "-l"); err == nil && strings.Contains(cur, "# shieldbug") {
			var keep []string
			for _, l := range strings.Split(cur, "\n") {
				if strings.TrimSpace(l) != "" && !strings.Contains(l, "# shieldbug") {
					keep = append(keep, l)
				}
			}
			cmd := exec.Command("crontab", "-")
			cmd.Stdin = strings.NewReader(strings.Join(keep, "\n") + "\n")
			_ = cmd.Run()
		}
	}
	return nil
}

// scheduleStatus returns a short description, or "" when nothing is registered.
func scheduleStatus() string {
	switch runtime.GOOS {
	case "windows":
		out, err := runCmd(15*time.Second, "schtasks", "/Query", "/TN", taskName, "/FO", "LIST")
		if err != nil {
			return ""
		}
		for _, l := range strings.Split(out, "\n") {
			l = strings.TrimSpace(l)
			if strings.HasPrefix(l, "Next Run Time:") || strings.HasPrefix(l, "다음 실행 시간:") {
				return strings.TrimSpace(l[strings.Index(l, ":")+1:])
			}
		}
		return "registered"
	case "darwin":
		if _, err := os.Stat(filepath.Join(homeDir(), "Library", "LaunchAgents", launchdLabel+".plist")); err == nil {
			return "launchd"
		}
	default:
		if out, err := runCmd(10*time.Second, "systemctl", "--user", "is-enabled", "shieldbug.timer"); err == nil && strings.TrimSpace(out) == "enabled" {
			return "systemd timer"
		}
		if cur, err := runCmd(10*time.Second, "crontab", "-l"); err == nil && strings.Contains(cur, "# shieldbug") {
			return "cron"
		}
	}
	return ""
}

// createShortcuts puts "Shieldbug" on the desktop and in the Start menu (Windows)
// or a .desktop entry (Linux). macOS users open it from Terminal or Spotlight via the alias.
func createShortcuts() error {
	exe := selfPath()
	switch runtime.GOOS {
	case "windows":
		script := `$s=New-Object -ComObject WScript.Shell;` +
			`foreach($d in @([Environment]::GetFolderPath('Desktop'),(Join-Path ([Environment]::GetFolderPath('Programs')) '')) ){` +
			`$l=$s.CreateShortcut((Join-Path $d 'Shieldbug.lnk'));$l.TargetPath=$env:SB_EXE;$l.Arguments='open';` +
			`$l.WorkingDirectory=(Split-Path $env:SB_EXE);$l.Description='Shieldbug security check';$l.Save()}`
		cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
		cmd.Env = append(os.Environ(), "SB_EXE="+exe)
		hideWindow(cmd)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("shortcut: %s", strings.TrimSpace(string(out)))
		}
		return nil
	case "linux":
		dir := filepath.Join(homeDir(), ".local", "share", "applications")
		_ = os.MkdirAll(dir, 0o755)
		entry := fmt.Sprintf("[Desktop Entry]\nType=Application\nName=Shieldbug\nComment=Daily AI security check\nExec=%s open\nTerminal=false\nCategories=Utility;Security;\n", exe)
		return os.WriteFile(filepath.Join(dir, "shieldbug.desktop"), []byte(entry), 0o644)
	default:
		return errors.New("on macOS, run `shieldbug open` or add it to the Dock from Finder")
	}
}

func removeShortcuts() {
	switch runtime.GOOS {
	case "windows":
		script := `foreach($d in @([Environment]::GetFolderPath('Desktop'),[Environment]::GetFolderPath('Programs'))){Remove-Item (Join-Path $d 'Shieldbug.lnk') -ErrorAction SilentlyContinue}`
		cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
		hideWindow(cmd)
		_ = cmd.Run()
	case "linux":
		_ = os.Remove(filepath.Join(homeDir(), ".local", "share", "applications", "shieldbug.desktop"))
	}
}
