package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopySelf(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "download", "shieldbug.exe")
	dst := filepath.Join(dir, "Programs", "Shieldbug", "shieldbug.exe")
	_ = os.MkdirAll(filepath.Dir(src), 0o755)
	_ = os.WriteFile(src, []byte("v2"), 0o644)
	_ = os.MkdirAll(filepath.Dir(dst), 0o755)
	_ = os.WriteFile(dst, []byte("v1"), 0o755)
	if err := copySelf(src, dst); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(dst); string(b) != "v2" {
		t.Fatalf("installed copy = %q", b)
	}
	if b, _ := os.ReadFile(dst + ".old"); string(b) != "v1" {
		t.Fatal("the older copy is moved aside, not overwritten in place")
	}
	if _, err := os.Stat(dst + ".new"); err == nil {
		t.Fatal("temporary file left behind")
	}
}

func TestDevBuildDoesNotInstall(t *testing.T) {
	if channel == "release" {
		t.Skip("release build")
	}
	if selfInstall("open") {
		t.Fatal("a build from source must not install itself")
	}
}
