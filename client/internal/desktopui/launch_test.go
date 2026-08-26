package desktopui

import (
	"net"
	"strings"
	"testing"
	"time"

	"mikrotik-psk-knock/client/internal/invite"
)

func TestExpandLaunch(t *testing.T) {
	got := expandLaunch(`start "" mstsc /v:{host}:{port}`, "kz.example.com", 10800, "socks5")
	want := `start "" mstsc /v:kz.example.com:10800`
	if got != want {
		t.Fatalf("expandLaunch = %q, want %q", got, want)
	}
	if got := expandLaunch("open {service}", "h", 1, "web"); got != "open web" {
		t.Fatalf("service placeholder not expanded: %q", got)
	}
	// No placeholders → the line is passed through untouched.
	if got := expandLaunch("myapp --flag", "h", 1, "s"); got != "myapp --flag" {
		t.Fatalf("plain line changed: %q", got)
	}
}

func TestSetAndClearLaunchCommand(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	srv := New(store, "tok", KnockTimings{})
	if _, err := store.Add("laptop", testBlobRaw(t, 443)); err != nil {
		t.Fatal(err)
	}

	if err := srv.SetLaunchCommand("laptop", "127.0.0.1", "wg", "  echo hi  "); err != nil {
		t.Fatalf("set: %v", err)
	}
	if got := srv.launchCommand("laptop", "127.0.0.1", "wg"); got != "echo hi" {
		t.Fatalf("command = %q, want trimmed 'echo hi'", got)
	}
	// Survives a reload from disk.
	store2, _ := NewStore(store.dir)
	srv2 := New(store2, "tok", KnockTimings{})
	if got := srv2.launchCommand("laptop", "127.0.0.1", "wg"); got != "echo hi" {
		t.Fatalf("command not persisted: %q", got)
	}

	if err := srv.SetLaunchCommand("laptop", "127.0.0.1", "wg", ""); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if got := srv.launchCommand("laptop", "127.0.0.1", "wg"); got != "" {
		t.Fatalf("command not cleared: %q", got)
	}

	// An unknown invite is rejected rather than silently stored.
	if err := srv.SetLaunchCommand("nope", "127.0.0.1", "wg", "x"); err == nil {
		t.Fatal("setting a command for an unknown invite succeeded")
	}
}

// TestLaunchRunsOnlyWhenOpen is the security-relevant one: the command must run
// after a confirmed open and never when the port stayed closed.
func TestLaunchRunsOnlyWhenOpen(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()
	port := ln.Addr().(*net.TCPAddr).Port

	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	srv := New(store, "tok", KnockTimings{
		MinBucketAge:  time.Nanosecond,
		StageDuration: 30 * time.Millisecond,
		TokenDuration: 30 * time.Millisecond,
		CheckTimeout:  200 * time.Millisecond,
		CheckAttempts: 2,
		CheckInterval: 20 * time.Millisecond,
	})
	var ran []string
	srv.runCmd = func(line string) error { ran = append(ran, line); return nil }

	inv, err := store.Add("laptop", testBlobRaw(t, port))
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.SetLaunchCommand(inv.ID, "127.0.0.1", "wg", "connect {host}:{port}"); err != nil {
		t.Fatal(err)
	}

	res, err := srv.Knock(inv.ID, "127.0.0.1", "wg")
	if err != nil {
		t.Fatalf("knock: %v", err)
	}
	if res.Status != "open" {
		t.Fatalf("status = %q, want open", res.Status)
	}
	wantLine := "connect 127.0.0.1:" + itoa(port)
	if len(ran) != 1 || ran[0] != wantLine {
		t.Fatalf("ran = %v, want [%q]", ran, wantLine)
	}
	if res.Launched != wantLine {
		t.Fatalf("result.Launched = %q, want %q", res.Launched, wantLine)
	}

	// Port closes → the command must NOT run again.
	ln.Close()
	ran = nil
	res, err = srv.Knock(inv.ID, "127.0.0.1", "wg")
	if err != nil {
		t.Fatalf("knock: %v", err)
	}
	if res.Status == "open" {
		t.Fatal("service still reported open after the listener closed")
	}
	if len(ran) != 0 {
		t.Fatalf("command ran on a failed knock: %v", ran)
	}
	if res.Launched != "" {
		t.Fatalf("result reports a launch on a failed knock: %q", res.Launched)
	}
}

// TestPresetFromInviteIsHonored closes the loop: a launch KIND set by the admin
// travels in the invite and the client turns it into a real invocation, without
// the user configuring anything.
func TestPresetFromInviteIsHonored(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()
	port := ln.Addr().(*net.TCPAddr).Port

	blob, err := invite.Encode(invite.Blob{
		Version:  invite.Version,
		ClientID: "test-laptop",
		Routers: []invite.Router{{
			Router:        "127.0.0.1",
			BucketSeconds: 30,
			PSK:           "synthetic-test-psk",
			Services: []invite.Service{{
				Name: "web", Stage1: 40001, Stage2: 40002, Token: 40003,
				CheckPort: port, Launch: "http",
			}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	srv := New(store, "tok", KnockTimings{
		MinBucketAge: time.Nanosecond, StageDuration: 30 * time.Millisecond,
		TokenDuration: 30 * time.Millisecond, CheckTimeout: 200 * time.Millisecond,
		CheckAttempts: 2, CheckInterval: 20 * time.Millisecond,
	})
	var ran []string
	srv.runCmd = func(line string) error { ran = append(ran, line); return nil }

	inv, err := store.Add("laptop", blob)
	if err != nil {
		t.Fatal(err)
	}
	res, err := srv.Knock(inv.ID, "127.0.0.1", "web")
	if err != nil {
		t.Fatalf("knock: %v", err)
	}
	if res.Status != "open" {
		t.Fatalf("status = %q, want open", res.Status)
	}
	want := "127.0.0.1:" + itoa(port)
	if len(ran) != 1 || !strings.Contains(ran[0], want) || !strings.Contains(ran[0], "http") {
		t.Fatalf("ran = %v, want one http line containing %q", ran, want)
	}

	// The user's own command overrides the admin preset.
	ran = nil
	if err := srv.SetLaunchCommand(inv.ID, "127.0.0.1", "web", "mine {port}"); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.Knock(inv.ID, "127.0.0.1", "web"); err != nil {
		t.Fatal(err)
	}
	if len(ran) != 1 || ran[0] != "mine "+itoa(port) {
		t.Fatalf("ran = %v, want the user command to win", ran)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
