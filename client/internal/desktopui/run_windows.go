//go:build windows

package desktopui

import (
	"os/exec"
	"syscall"
)

// runCommandLine hands the line to cmd.exe, so shell built-ins the user is
// likely to type (`start "" mstsc /v:host:port`, `call …`) work as written.
// The command is detached: we never wait for the launched app to exit, and the
// console window stays hidden.
func runCommandLine(cmdline string) error {
	cmd := exec.Command("cmd", "/c", cmdline)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }() // reap without blocking
	return nil
}
