package desktopui

import (
	"net"
	"net/http"
	"testing"
	"time"
)

// listener is a loopback port the test can open and close at will — the router
// stand-in for the post-knock TCP check.
func listener(t *testing.T) (int, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port, func() { _ = ln.Close() }
}

// Switching keep-open on knocks straight away — otherwise the toggle would look
// broken until the current window happened to expire. And the renewal must not
// run the post-open command: holding a port open for an hour must not open an
// RDP window every few minutes.
func TestKeepOpenKnocksAtOnceAndDoesNotLaunch(t *testing.T) {
	port, closeLn := listener(t)
	defer closeLn()

	ts, srv := newTestServerSrv(t)
	var launched int
	srv.runCmd = func(string) error { launched++; return nil }

	var imported struct {
		ID string `json:"id"`
	}
	call(t, ts, "POST", "/api/import", map[string]string{"name": "keep", "blob": testBlob(t, port)}, &imported)
	call(t, ts, "POST", "/api/launch", map[string]string{
		"invite_id": imported.ID, "router": "127.0.0.1", "service": "wg", "command": "echo hi",
	}, nil)

	if res := call(t, ts, "POST", "/api/keepopen", map[string]any{
		"invite_id": imported.ID, "router": "127.0.0.1", "service": "wg", "on": true,
	}, nil); res.StatusCode != http.StatusOK {
		t.Fatalf("keepopen: %d", res.StatusCode)
	}

	deadline := time.Now().Add(3 * time.Second)
	for !srv.state.isOpen(imported.ID, "127.0.0.1", "wg") {
		if time.Now().After(deadline) {
			t.Fatal("keep-open did not knock after being switched on")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if launched != 0 {
		t.Fatalf("keep-open ran the launch command %d time(s); it must stay silent", launched)
	}

	// The status poll is what the countdown reads.
	var st struct {
		Now      int64 `json:"now"`
		Services map[string]struct {
			KeepOpen  bool  `json:"keep_open"`
			OpenUntil int64 `json:"open_until"`
		} `json:"services"`
	}
	call(t, ts, "GET", "/api/status", nil, &st)
	row, ok := st.Services[LaunchKey(imported.ID, "127.0.0.1", "wg")]
	if !ok || !row.KeepOpen || row.OpenUntil <= st.Now {
		t.Fatalf("status = %+v, want keep_open with a future window", st.Services)
	}
}

// Three failures in a row turn the switch off instead of knocking forever at a
// router that is not there.
func TestKeepOpenGivesUpAfterRepeatedFailures(t *testing.T) {
	port, closeLn := listener(t)
	closeLn() // nothing is listening: every check fails

	ts, srv := newTestServerSrv(t)
	var imported struct {
		ID string `json:"id"`
	}
	call(t, ts, "POST", "/api/import", map[string]string{"name": "gone", "blob": testBlob(t, port)}, &imported)
	call(t, ts, "POST", "/api/keepopen", map[string]any{
		"invite_id": imported.ID, "router": "127.0.0.1", "service": "wg", "on": true,
	}, nil)

	// Switching it on already started one renewal in the background; overlapping
	// calls are skipped by design, so drive it until it gives up or time runs out.
	deadline := time.Now().Add(10 * time.Second)
	for srv.KeepOpen(imported.ID, "127.0.0.1", "wg") {
		if time.Now().After(deadline) {
			t.Fatalf("keep-open never gave up after repeated failures")
		}
		srv.renew(imported.ID, "127.0.0.1", "wg")
		time.Sleep(20 * time.Millisecond)
	}
}

func TestRenewLeadIsClamped(t *testing.T) {
	for _, c := range []struct {
		bucket int64
		want   time.Duration
	}{{5, 15 * time.Second}, {30, 30 * time.Second}, {120, 30 * time.Second}, {20, 20 * time.Second}} {
		if got := renewLead(c.bucket); got != c.want {
			t.Fatalf("renewLead(%d) = %s, want %s", c.bucket, got, c.want)
		}
	}
}
