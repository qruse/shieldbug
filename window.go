package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// openAppWindow shows the dashboard in a window without an address bar, using
// a browser that is already installed (Edge ships with Windows). Nothing extra
// is installed. If no app-mode browser is found, the default browser opens.
func openAppWindow(url string) {
	app := "--app=" + url
	size := "--window-size=1180,860"
	switch runtime.GOOS {
	case "windows":
		var cands []string
		for _, base := range []string{os.Getenv("ProgramFiles(x86)"), os.Getenv("ProgramFiles"), os.Getenv("LOCALAPPDATA")} {
			if base == "" {
				continue
			}
			cands = append(cands,
				filepath.Join(base, "Microsoft", "Edge", "Application", "msedge.exe"),
				filepath.Join(base, "Google", "Chrome", "Application", "chrome.exe"))
		}
		for _, c := range cands {
			if _, err := os.Stat(c); err == nil {
				if exec.Command(c, app, size).Start() == nil {
					return
				}
			}
		}
		_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		for _, name := range []string{"Google Chrome", "Microsoft Edge", "Brave Browser", "Chromium"} {
			if _, err := os.Stat("/Applications/" + name + ".app"); err == nil {
				if exec.Command("open", "-na", name, "--args", app, size).Start() == nil {
					return
				}
			}
		}
		_ = exec.Command("open", url).Start()
	default:
		for _, name := range []string{"google-chrome", "chromium", "chromium-browser", "microsoft-edge", "brave-browser"} {
			if p, err := exec.LookPath(name); err == nil {
				if exec.Command(p, app, size).Start() == nil {
					return
				}
			}
		}
		_ = exec.Command("xdg-open", url).Start()
	}
}
