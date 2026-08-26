//go:build !windows

package desktopui

import "os/exec"

// runCommandLine runs the line through the shell (dev runs of the Wails client
// on macOS/Linux; the shipped macOS client is the native client-macos/ app).
func runCommandLine(cmdline string) error {
	cmd := exec.Command("sh", "-c", cmdline)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
