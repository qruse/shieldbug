package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMacApp(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "Desktop"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := macApp(home, "/Users/x/My Apps/shieldbug"); err != nil {
		t.Fatal(err)
	}
	app := filepath.Join(home, "Applications", "Shieldbug.app", "Contents")
	run, err := os.ReadFile(filepath.Join(app, "MacOS", "shieldbug-open"))
	if err != nil || !strings.Contains(string(run), `exec '/Users/x/My Apps/shieldbug' open`) {
		t.Fatalf("launcher: %q %v", run, err)
	}
	plist, _ := os.ReadFile(filepath.Join(app, "Info.plist"))
	if !bytes.Contains(plist, []byte("<string>shieldbug-open</string>")) {
		t.Fatal("plist must name the launcher")
	}
	ic, _ := os.ReadFile(filepath.Join(app, "Resources", "Shieldbug.icns"))
	if len(ic) != 16+len(iconPNG) || string(ic[:4]) != "icns" || string(ic[8:12]) != "ic08" || !bytes.HasPrefix(ic[16:], []byte("\x89PNG")) {
		t.Fatal("bad icns")
	}
}

func TestLinuxEntry(t *testing.T) {
	home := t.TempDir()
	desk := filepath.Join(home, "Schreibtisch")
	os.MkdirAll(desk, 0o755)
	os.MkdirAll(filepath.Join(home, ".config"), 0o755)
	os.WriteFile(filepath.Join(home, ".config", "user-dirs.dirs"), []byte(`XDG_DESKTOP_DIR="$HOME/Schreibtisch"`+"\n"), 0o644)
	if err := linuxEntry(home, "/opt/sb $1/shieldbug"); err != nil {
		t.Fatal(err)
	}
	e, err := os.ReadFile(filepath.Join(home, ".local", "share", "applications", "shieldbug.desktop"))
	if err != nil || !strings.Contains(string(e), `Exec="/opt/sb \$1/shieldbug" open`) || !strings.Contains(string(e), "Icon=") {
		t.Fatalf("entry: %s %v", e, err)
	}
	if _, err := os.Stat(filepath.Join(desk, "shieldbug.desktop")); err != nil {
		t.Fatal("desktop copy missing")
	}
}
