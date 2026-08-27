//go:build windows

package desktopui

import (
	"os/exec"
	"syscall"
)

// launchCmd hands the line to cmd.exe, so shell built-ins an operator is likely
// to type (`start "" mstsc /v:host:port`, `call …`) work as written.
//
// CmdLine is set explicitly instead of passing arguments: Go escapes arguments
// with backslash-quote pairs (the MSVC convention) and cmd.exe does not read
// those, so a command containing quotes arrived mangled and silently did
// nothing.
func launchCmd(cmdline string) *exec.Cmd {
	cmd := exec.Command("cmd.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW: no console flash
		CmdLine:       `cmd.exe /c ` + cmdline,
	}
	return cmd
}
