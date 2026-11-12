// Command release builds the files published on GitHub Releases:
//
//	go run ./tools/release
//
// Windows gets a single .exe; macOS and Linux get a .tar.gz so the program
// keeps its executable bit. SHA256SUMS.txt lists the downloads;
// PROGRAMS-SHA256SUMS.txt lists the program each one runs (the archive's
// is "<archive>:shieldbug"), which the self-check compares with. Output goes to
// dist/release/. Every build sets main.channel=release, which makes a
// downloaded file install itself on first open.
package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type target struct{ goos, goarch, name string }

var targets = []target{
	{"windows", "amd64", "shieldbug-windows-x64.exe"},
	{"windows", "arm64", "shieldbug-windows-arm64.exe"},
	{"darwin", "arm64", "shieldbug-macos-apple-silicon.tar.gz"},
	{"darwin", "amd64", "shieldbug-macos-intel.tar.gz"},
	{"linux", "amd64", "shieldbug-linux-x64.tar.gz"},
	{"linux", "arm64", "shieldbug-linux-arm64.tar.gz"},
}

func main() {
	out := filepath.Join("dist", "release")
	_ = os.RemoveAll(out)
	if err := os.MkdirAll(out, 0o755); err != nil {
		fail(err)
	}
	files := map[string]string{}    // SHA256SUMS.txt: what you download (works with sha256sum -c)
	programs := map[string]string{} // PROGRAMS-SHA256SUMS.txt: the program each one runs (the self-check)
	for _, t := range targets {
		bin := filepath.Join(out, "build-"+t.goos+"-"+t.goarch)
		if t.goos == "windows" {
			bin = filepath.Join(out, t.name)
		}
		// assetName lets the program find its own line: the .exe itself, or
		// the program inside an archive.
		asset := t.name
		if t.goos != "windows" {
			asset += ":shieldbug"
		}
		ld := "-s -w -X main.channel=release -X main.assetName=" + asset
		if t.goos == "windows" {
			ld += " -H=windowsgui"
		}
		cmd := exec.Command("go", "build", "-trimpath", "-ldflags", ld, "-o", bin, ".")
		cmd.Env = append(os.Environ(), "GOOS="+t.goos, "GOARCH="+t.goarch, "CGO_ENABLED=0")
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			fail(fmt.Errorf("%s/%s: %w", t.goos, t.goarch, err))
		}
		programs[asset] = sha(bin)
		file := filepath.Join(out, t.name)
		if t.goos != "windows" {
			if err := tarGz(file, bin, "shieldbug"); err != nil {
				fail(err)
			}
			os.Remove(bin)
		}
		files[t.name] = sha(file)
		fmt.Println("built", file)
	}
	writeSums(filepath.Join(out, "SHA256SUMS.txt"), files)
	writeSums(filepath.Join(out, "PROGRAMS-SHA256SUMS.txt"), programs)
}

func writeSums(path string, sums map[string]string) {
	var names []string
	for n := range sums {
		names = append(names, n)
	}
	sort.Strings(names)
	var b strings.Builder
	for _, n := range names {
		fmt.Fprintf(&b, "%s  %s\n", sums[n], n)
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		fail(err)
	}
	fmt.Print(filepath.Base(path), "\n", b.String())
}

func tarGz(dst, src, name string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	st, err := in.Stat()
	if err != nil {
		return err
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	hdr := &tar.Header{Name: name, Mode: 0o755, Size: st.Size(), ModTime: time.Now(), Typeflag: tar.TypeReg}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	if _, err := io.Copy(tw, in); err != nil {
		return err
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return gz.Close()
}

func sha(path string) string {
	f, err := os.Open(path)
	if err != nil {
		fail(err)
	}
	defer f.Close()
	h := sha256.New()
	_, _ = io.Copy(h, f)
	return hex.EncodeToString(h.Sum(nil))
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "release:", err)
	os.Exit(1)
}
