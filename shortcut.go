package main

import (
	_ "embed"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Shortcuts: a desktop icon, plus an entry the OS search can find
// (Windows Start menu, macOS ~/Applications for Spotlight, Linux app list).
// Created on the first run and again by `shieldbug shortcut`; they point at
// wherever this program file is, so moving the file means running that again.

//go:embed assets/icon-256.png
var iconPNG []byte

// tempBuild: started with `go run`, so this program file is deleted when it exits.
func tempBuild(exe string) bool {
	return strings.Contains(filepath.ToSlash(exe), "/go-build")
}

func createShortcuts() error {
	exe := selfPath()
	if tempBuild(exe) {
		return errors.New("started with `go run`: build or install first (go install github.com/qruse/shieldbug@latest), then run shieldbug shortcut")
	}
	home := homeDir()
	switch runtime.GOOS {
	case "windows":
		script := `$s=New-Object -ComObject WScript.Shell;` +
			`foreach($d in @([Environment]::GetFolderPath('Desktop'),[Environment]::GetFolderPath('Programs'))){` +
			`$l=$s.CreateShortcut((Join-Path $d 'Shieldbug.lnk'));$l.TargetPath=$env:SB_EXE;$l.Arguments='open';` +
			`$l.IconLocation=$env:SB_EXE+',0';$l.WorkingDirectory=(Split-Path $env:SB_EXE);$l.Description='Shieldbug security check';$l.Save()}`
		cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
		cmd.Env = append(os.Environ(), "SB_EXE="+exe)
		hideWindow(cmd)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("shortcut: %s", strings.TrimSpace(string(out)))
		}
		return nil
	case "darwin":
		return macApp(home, exe)
	default:
		return linuxEntry(home, exe)
	}
}

func removeShortcuts() {
	home := homeDir()
	switch runtime.GOOS {
	case "windows":
		script := `foreach($d in @([Environment]::GetFolderPath('Desktop'),[Environment]::GetFolderPath('Programs'))){Remove-Item (Join-Path $d 'Shieldbug.lnk') -ErrorAction SilentlyContinue}`
		cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
		hideWindow(cmd)
		_ = cmd.Run()
	case "darwin":
		_ = os.Remove(filepath.Join(home, "Desktop", "Shieldbug.app"))
		_ = os.RemoveAll(filepath.Join(home, "Applications", "Shieldbug.app"))
	default:
		_ = os.Remove(filepath.Join(home, ".local", "share", "applications", "shieldbug.desktop"))
		_ = os.Remove(filepath.Join(home, ".local", "share", "icons", "hicolor", "256x256", "apps", "shieldbug.png"))
		_ = os.Remove(filepath.Join(desktopDir(home), "shieldbug.desktop"))
	}
}

// macApp writes a small app bundle to ~/Applications (Spotlight indexes it)
// and links it on the desktop. The bundle only launches this program file.
func macApp(home, exe string) error {
	app := filepath.Join(home, "Applications", "Shieldbug.app")
	if err := os.MkdirAll(filepath.Join(app, "Contents", "MacOS"), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(app, "Contents", "Resources"), 0o755); err != nil {
		return err
	}
	plist := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleName</key><string>Shieldbug</string>
<key>CFBundleDisplayName</key><string>Shieldbug</string>
<key>CFBundleIdentifier</key><string>` + launchdLabel + `.app</string>
<key>CFBundleExecutable</key><string>shieldbug-open</string>
<key>CFBundleIconFile</key><string>Shieldbug</string>
<key>CFBundlePackageType</key><string>APPL</string>
<key>CFBundleShortVersionString</key><string>` + version + `</string>
</dict></plist>
`
	files := []struct {
		path string
		data []byte
		mode os.FileMode
	}{
		{filepath.Join(app, "Contents", "Info.plist"), []byte(plist), 0o644},
		{filepath.Join(app, "Contents", "MacOS", "shieldbug-open"), []byte("#!/bin/sh\nexec " + shQuote(exe) + " open\n"), 0o755},
		{filepath.Join(app, "Contents", "Resources", "Shieldbug.icns"), icns(iconPNG), 0o644},
	}
	for _, f := range files {
		if err := os.WriteFile(f.path, f.data, f.mode); err != nil {
			return err
		}
	}
	link := filepath.Join(home, "Desktop", "Shieldbug.app")
	_ = os.Remove(link)
	if _, err := os.Stat(filepath.Join(home, "Desktop")); err == nil {
		_ = os.Symlink(app, link)
	}
	return nil
}

// linuxEntry adds Shieldbug to the app list (searchable) and the desktop.
func linuxEntry(home, exe string) error {
	icons := filepath.Join(home, ".local", "share", "icons", "hicolor", "256x256", "apps")
	apps := filepath.Join(home, ".local", "share", "applications")
	for _, d := range []string{icons, apps} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	icon := filepath.Join(icons, "shieldbug.png")
	if err := os.WriteFile(icon, iconPNG, 0o644); err != nil {
		return err
	}
	entry := fmt.Sprintf("[Desktop Entry]\nType=Application\nName=Shieldbug\nComment=Daily AI security check\nExec=%s open\nIcon=%s\nTerminal=false\nCategories=Utility;Security;\n", desktopQuote(exe), icon)
	if err := os.WriteFile(filepath.Join(apps, "shieldbug.desktop"), []byte(entry), 0o644); err != nil {
		return err
	}
	if d := desktopDir(home); d != "" {
		if _, err := os.Stat(d); err == nil {
			_ = os.WriteFile(filepath.Join(d, "shieldbug.desktop"), []byte(entry), 0o755)
		}
	}
	return nil
}

// desktopDir: XDG_DESKTOP_DIR from user-dirs.dirs, else ~/Desktop.
func desktopDir(home string) string {
	b, err := os.ReadFile(filepath.Join(home, ".config", "user-dirs.dirs"))
	if err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			if v, ok := strings.CutPrefix(strings.TrimSpace(line), "XDG_DESKTOP_DIR="); ok {
				v = strings.Trim(v, `"`)
				return strings.Replace(v, "$HOME", home, 1)
			}
		}
	}
	return filepath.Join(home, "Desktop")
}

// icns wraps one 256x256 PNG as an Apple icon file ("ic08" holds PNG data).
func icns(png []byte) []byte {
	out := make([]byte, 16, 16+len(png))
	copy(out[0:4], "icns")
	binary.BigEndian.PutUint32(out[4:8], uint32(16+len(png)))
	copy(out[8:12], "ic08")
	binary.BigEndian.PutUint32(out[12:16], uint32(8+len(png)))
	return append(out, png...)
}

func shQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// desktopQuote quotes a path for a .desktop Exec line.
func desktopQuote(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "`", "\\`", "$", `\$`)
	return `"` + r.Replace(s) + `"`
}
