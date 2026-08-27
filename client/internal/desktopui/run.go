package desktopui

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// launchGrace is how long a launch is watched before it is called a success.
// `start "" mstsc …` returns at once; a command that is going to fail usually
// fails within this window, and reporting that is the whole point — the first
// version only checked that the shell itself started, so a broken command line
// looked identical to a working one.
const launchGrace = 2 * time.Second

// runCommandLine starts the user's (or the preset's) command, gives it a moment
// to fail, and records what happened. Anything the command printed goes to the
// launch log next to the config, so "nothing happened" can be diagnosed instead
// of guessed at.
func (s *Server) runCommandLine(cmdline string) error {
	cmd := launchCmd(cmdline)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out

	if err := cmd.Start(); err != nil {
		s.logLaunch(cmdline, "", err)
		return err
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		text := strings.TrimSpace(out.String())
		s.logLaunch(cmdline, text, err)
		if err != nil {
			if text != "" {
				return fmt.Errorf("%w: %s", err, firstLine(text))
			}
			return err
		}
		return nil
	case <-time.After(launchGrace):
		// Still running: a GUI app that stuck around. Reap in the background.
		go func() {
			err := <-done
			s.logLaunch(cmdline, strings.TrimSpace(out.String()), err)
		}()
		s.logLaunch(cmdline, "", nil)
		return nil
	}
}

// LaunchLogPath is where launch attempts are recorded.
func (s *Server) LaunchLogPath() string { return filepath.Join(s.store.dir, "launch.log") }

func (s *Server) logLaunch(cmdline, output string, err error) {
	status := "ok"
	if err != nil {
		status = "FAILED: " + err.Error()
	}
	line := fmt.Sprintf("%s\t%s\t%s", time.Now().Format(time.RFC3339), status, cmdline)
	if output != "" {
		line += "\n\t" + strings.ReplaceAll(output, "\n", "\n\t")
	}
	f, ferr := os.OpenFile(s.LaunchLogPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if ferr != nil {
		return // logging must never break a launch
	}
	defer f.Close()
	_, _ = f.WriteString(line + "\n")
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
