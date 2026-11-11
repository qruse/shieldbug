//go:build windows

package main

import (
	"os"
	"os/exec"
	"syscall"
)

// hideWindow stops child commands (netstat, powershell, the AI CLI) from
// flashing a console window.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}

// attachConsole: the release build is a GUI program (no black window on
// double-click). When it is run from a terminal, attach to that terminal so
// command output is visible.
func attachConsole() {
	k := syscall.NewLazyDLL("kernel32.dll")
	proc := k.NewProc("AttachConsole")
	const attachParent = ^uintptr(0) // ATTACH_PARENT_PROCESS (-1)
	if r, _, _ := proc.Call(attachParent); r == 0 {
		return
	}
	if f, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0); err == nil {
		os.Stdout = f
		os.Stderr = f
	}
}
