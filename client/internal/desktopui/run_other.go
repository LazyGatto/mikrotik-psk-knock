//go:build !windows

package desktopui

import "os/exec"

// launchCmd runs the line through the shell (dev runs of the Wails client on
// macOS/Linux; the shipped macOS client is the native client-macos/ app).
func launchCmd(cmdline string) *exec.Cmd { return exec.Command("sh", "-c", cmdline) }
