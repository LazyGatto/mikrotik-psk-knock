package desktopui

import (
	"os"
	"strings"
	"testing"
)

func newRunServer(t *testing.T) *Server {
	t.Helper()
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return New(store, "tok", KnockTimings{})
}

// TestFailedCommandIsReported is the regression: the first version only checked
// that the shell process started, so a command that could not run looked
// exactly like a working one — the UI said "launched" and nothing happened.
func TestFailedCommandIsReported(t *testing.T) {
	srv := newRunServer(t)
	if err := srv.runCommandLine("exit 3"); err == nil {
		t.Fatal("a command exiting non-zero reported success")
	}
	if err := srv.runCommandLine("this-command-does-not-exist-mkpk"); err == nil {
		t.Fatal("a missing command reported success")
	}
}

func TestSuccessfulCommandIsAccepted(t *testing.T) {
	srv := newRunServer(t)
	if err := srv.runCommandLine("true"); err != nil {
		t.Fatalf("a working command reported failure: %v", err)
	}
}

// TestLaunchLogRecordsAttempts backs the "why did nothing happen?" question
// with a file the operator can read.
func TestLaunchLogRecordsAttempts(t *testing.T) {
	srv := newRunServer(t)
	_ = srv.runCommandLine("true")
	_ = srv.runCommandLine("exit 7")

	data, err := os.ReadFile(srv.LaunchLogPath())
	if err != nil {
		t.Fatalf("no launch log: %v", err)
	}
	log := string(data)
	if !strings.Contains(log, "exit 7") || !strings.Contains(log, "FAILED") {
		t.Fatalf("the failure is not in the log:\n%s", log)
	}
	if strings.Count(log, "\n") < 2 {
		t.Fatalf("expected a line per attempt:\n%s", log)
	}
}

// TestCommandKeepsItsQuoting guards the actual Windows bug: the shell must
// receive the line as written, quotes included.
func TestCommandKeepsItsQuoting(t *testing.T) {
	srv := newRunServer(t)
	out := srv.store.dir + "/quoted.txt"
	if err := srv.runCommandLine(`printf '%s' "a b" > ` + out); err != nil {
		t.Fatalf("quoted command failed: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "a b" {
		t.Fatalf("quoting was mangled: %q", string(data))
	}
}
