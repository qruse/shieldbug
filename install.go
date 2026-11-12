package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// channel is "release" in the files published on GitHub Releases
// (-ldflags "-X main.channel=release"). Only those install themselves; a
// build from source or `go install` stays where it is.
var channel = "dev"

// installPath: where a downloaded release puts itself, per user, no admin.
func installPath() string {
	h := homeDir()
	switch runtime.GOOS {
	case "windows":
		base := os.Getenv("LOCALAPPDATA")
		if base == "" {
			base = filepath.Join(h, "AppData", "Local")
		}
		return filepath.Join(base, "Programs", "Shieldbug", "shieldbug.exe")
	case "darwin":
		return filepath.Join(h, "Library", "Application Support", "Shieldbug", "shieldbug")
	default:
		return filepath.Join(h, ".local", "bin", "shieldbug")
	}
}

func samePath(a, b string) bool {
	a, _ = filepath.Abs(a)
	b, _ = filepath.Abs(b)
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// selfInstall: a release file opened from Downloads copies itself to
// installPath (replacing an older copy, which also updates an open dashboard
// on its next refresh), points the icon and search entry there, starts the
// installed copy and tells the caller to exit. The downloaded file can then
// be deleted.
func selfInstall(cmd string) bool {
	exe := selfPath()
	dst := installPath()
	if samePath(exe, dst) {
		_ = os.Remove(dst + ".old") // left by the previous update on Windows
		return false
	}
	if channel != "release" || (cmd != "open" && cmd != "install") {
		return false
	}
	if err := copySelf(exe, dst); err != nil {
		fmt.Fprintln(os.Stderr, "shieldbug: install:", err)
		return false // run from where it is instead
	}
	if err := createShortcutsAt(dst); err != nil {
		fmt.Fprintln(os.Stderr, "shieldbug:", err)
	}
	if scheduleStatus() != "" {
		c := exec.Command(dst, "schedule", "install") // point the daily task at the new place
		hideWindow(c)
		_ = c.Run()
	}
	if cmd == "install" {
		fmt.Println("installed to", dst)
		return true
	}
	c := exec.Command(dst, "open")
	if err := c.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "shieldbug:", err)
		return false
	}
	return true
}

func copySelf(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".new"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	// A running program file cannot be overwritten on Windows, but it can be
	// renamed out of the way.
	_ = os.Remove(dst + ".old")
	if _, err := os.Stat(dst); err == nil {
		if err := os.Rename(dst, dst+".old"); err != nil {
			os.Remove(tmp)
			return err
		}
	}
	return os.Rename(tmp, dst)
}
