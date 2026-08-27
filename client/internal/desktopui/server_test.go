package desktopui

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"mikrotik-psk-knock/client/internal/invite"
	"mikrotik-psk-knock/client/internal/release"
	"mikrotik-psk-knock/client/internal/version"
)

const testToken = "test-session-token"

// testBlob builds a synthetic single-router invite pointing at loopback, with
// the check port set to a real listener the test controls.
func testBlob(t *testing.T, checkPort int) string {
	t.Helper()
	blob, err := invite.Encode(invite.Blob{
		Version:  invite.Version,
		ClientID: "test-laptop",
		Routers: []invite.Router{{
			Router:        "127.0.0.1",
			BucketSeconds: 30,
			PSK:           "synthetic-test-psk",
			Services: []invite.Service{{
				Name: "wg", Stage1: 40001, Stage2: 40002, Token: 40003,
				CheckPort: checkPort, AllowedTimeout: "5m",
			}},
		}},
	})
	if err != nil {
		t.Fatalf("encode blob: %v", err)
	}
	return blob
}

// testBlobRaw is testBlob for tests that need the raw string outside HTTP.
func testBlobRaw(t *testing.T, checkPort int) string { return testBlob(t, checkPort) }

func newTestServer(t *testing.T) (*httptest.Server, *Store) {
	t.Helper()
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	h := NewHandlerTimings(store, testToken, KnockTimings{
		MinBucketAge:  time.Nanosecond, // no bucket-edge wait in tests
		StageDuration: 30 * time.Millisecond,
		TokenDuration: 30 * time.Millisecond,
		CheckTimeout:  200 * time.Millisecond,
		CheckAttempts: 3,
		CheckInterval: 20 * time.Millisecond,
	})
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	return ts, store
}

// newTestServerSrv is newTestServer for tests that need the Server itself —
// mainly to swap the launch runner for a recorder.
func newTestServerSrv(t *testing.T) (*httptest.Server, *Server) {
	t.Helper()
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	srv := New(store, testToken, KnockTimings{
		MinBucketAge:  time.Nanosecond,
		StageDuration: 30 * time.Millisecond,
		TokenDuration: 30 * time.Millisecond,
		CheckTimeout:  200 * time.Millisecond,
		CheckAttempts: 3,
		CheckInterval: 20 * time.Millisecond,
	})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts, srv
}

func call(t *testing.T, ts *httptest.Server, method, path string, body any, out any) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req, err := http.NewRequest(method, ts.URL+path, &buf)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("X-MKPK-Token", testToken)
	res, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	t.Cleanup(func() { _ = res.Body.Close() })
	if out != nil {
		if err := json.NewDecoder(res.Body).Decode(out); err != nil {
			t.Fatalf("decode response: %v", err)
		}
	}
	return res
}

func TestAPIRejectsWrongToken(t *testing.T) {
	ts, _ := newTestServer(t)
	req, _ := http.NewRequest("GET", ts.URL+"/api/state", nil)
	req.Header.Set("X-MKPK-Token", "wrong")
	res, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", res.StatusCode)
	}
}

func TestImportListRemove(t *testing.T) {
	ts, _ := newTestServer(t)

	var imported struct {
		ID string `json:"id"`
	}
	res := call(t, ts, "POST", "/api/import",
		map[string]string{"name": "Laptop.mkpk", "blob": testBlob(t, 443)}, &imported)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("import status = %d", res.StatusCode)
	}
	if imported.ID != "laptop" {
		t.Fatalf("id = %q, want laptop", imported.ID)
	}

	var st struct {
		Invites []struct {
			ID       string `json:"id"`
			ClientID string `json:"client_id"`
			Routers  []struct {
				Router   string `json:"router"`
				Services []struct {
					Name      string `json:"name"`
					CheckPort int    `json:"check_port"`
				} `json:"services"`
			} `json:"routers"`
		} `json:"invites"`
	}
	call(t, ts, "GET", "/api/state", nil, &st)
	if len(st.Invites) != 1 || st.Invites[0].ClientID != "test-laptop" {
		t.Fatalf("unexpected state: %+v", st)
	}
	if st.Invites[0].Routers[0].Services[0].CheckPort != 443 {
		t.Fatalf("check port lost: %+v", st.Invites[0])
	}

	call(t, ts, "POST", "/api/remove", map[string]string{"id": imported.ID}, nil)
	call(t, ts, "GET", "/api/state", nil, &st)
	if len(st.Invites) != 0 {
		t.Fatalf("invite not removed: %+v", st.Invites)
	}
}

func TestImportRejectsGarbage(t *testing.T) {
	ts, _ := newTestServer(t)
	res := call(t, ts, "POST", "/api/import",
		map[string]string{"name": "x", "blob": "not-an-invite"}, nil)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", res.StatusCode)
	}
}

// TestKnockAgainstLoopback runs the real knock flow: UDP stages go to loopback
// (fire-and-forget), the TCP check hits a live listener → status "open"; after
// the listener closes → "closed"/"error" (connection refused counts as closed).
func TestKnockAgainstLoopback(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
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

	ts, _ := newTestServer(t)
	var imported struct {
		ID string `json:"id"`
	}
	call(t, ts, "POST", "/api/import",
		map[string]string{"name": "kn", "blob": testBlob(t, port)}, &imported)

	var res struct {
		Knocked  bool   `json:"knocked"`
		Status   string `json:"status"`
		Attempts int    `json:"attempts"`
	}
	call(t, ts, "POST", "/api/knock",
		map[string]string{"invite_id": imported.ID, "router": "127.0.0.1", "service": "wg"}, &res)
	if !res.Knocked || res.Status != "open" {
		t.Fatalf("knock result = %+v, want knocked open", res)
	}

	ln.Close()
	var closed struct {
		Status string `json:"status"`
	}
	call(t, ts, "POST", "/api/knock",
		map[string]string{"invite_id": imported.ID, "router": "127.0.0.1", "service": "wg"}, &closed)
	if closed.Status == "open" {
		t.Fatalf("status = open after listener closed")
	}
}

func TestKnockUnknownService(t *testing.T) {
	ts, _ := newTestServer(t)
	var imported struct {
		ID string `json:"id"`
	}
	call(t, ts, "POST", "/api/import",
		map[string]string{"name": "u", "blob": testBlob(t, 443)}, &imported)
	res := call(t, ts, "POST", "/api/knock",
		map[string]string{"invite_id": imported.ID, "router": "127.0.0.1", "service": "nope"}, nil)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", res.StatusCode)
	}
}

// TestRelaunchRunsCommandWithoutKnocking covers the "reconnect while the port
// is still open" button: no knock is sent, the command runs again, and once the
// window is gone the answer is "closed" rather than a launch into nothing.
func TestRelaunchRunsCommandWithoutKnocking(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
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

	ts, srv := newTestServerSrv(t)
	var ran []string
	srv.runCmd = func(cmdline string) error { ran = append(ran, cmdline); return nil }

	var imported struct {
		ID string `json:"id"`
	}
	call(t, ts, "POST", "/api/import",
		map[string]string{"name": "re", "blob": testBlob(t, port)}, &imported)

	// No command yet → nothing to replay.
	if res := call(t, ts, "POST", "/api/relaunch",
		map[string]string{"invite_id": imported.ID, "router": "127.0.0.1", "service": "wg"}, nil); res.StatusCode != http.StatusBadRequest {
		t.Fatalf("relaunch without a command: status = %d, want 400", res.StatusCode)
	}

	call(t, ts, "POST", "/api/launch", map[string]string{
		"invite_id": imported.ID, "router": "127.0.0.1", "service": "wg",
		"command": "echo {host}:{port}",
	}, nil)

	var res RelaunchResult
	call(t, ts, "POST", "/api/relaunch",
		map[string]string{"invite_id": imported.ID, "router": "127.0.0.1", "service": "wg"}, &res)
	if res.Status != "open" || res.Launched != "echo 127.0.0.1:"+strconv.Itoa(port) {
		t.Fatalf("relaunch = %+v, want open and the expanded command", res)
	}
	if len(ran) != 1 {
		t.Fatalf("runCmd calls = %d, want 1", len(ran))
	}

	ln.Close()
	var closed RelaunchResult
	call(t, ts, "POST", "/api/relaunch",
		map[string]string{"invite_id": imported.ID, "router": "127.0.0.1", "service": "wg"}, &closed)
	if closed.Status == "open" || closed.Launched != "" {
		t.Fatalf("relaunch after close = %+v, want no launch", closed)
	}
	if len(ran) != 1 {
		t.Fatalf("command ran again on a closed port: %v", ran)
	}
}

func TestIndexInjectsToken(t *testing.T) {
	ts, _ := newTestServer(t)
	res, err := ts.Client().Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("get index: %v", err)
	}
	defer res.Body.Close()
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(res.Body); err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte(testToken)) {
		t.Fatalf("index does not embed the session token")
	}
	if bytes.Contains(buf.Bytes(), []byte("__MKPK_TOKEN__")) {
		t.Fatalf("token placeholder left unreplaced")
	}
}

// The client only reports that a newer release exists — it ships as a zip, so
// there is nothing to install from the app. Offline must stay silent, which is
// the caller's job: the endpoint says so with a 502.
func TestUpdateEndpointReportsNewerRelease(t *testing.T) {
	feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"v9.9.9","html_url":"https://github.com/LazyGatto/mikrotik-psk-knock/releases/tag/v9.9.9"}`))
	}))
	defer feed.Close()
	oldURL, oldVer := release.FeedURL, version.Version
	release.FeedURL, version.Version = feed.URL, "v0.5.0"
	defer func() { release.FeedURL, version.Version = oldURL, oldVer }()

	ts, _ := newTestServerSrv(t)
	var got struct {
		Latest string `json:"latest"`
		Newer  bool   `json:"newer"`
		URL    string `json:"url"`
	}
	call(t, ts, "GET", "/api/update", nil, &got)
	if !got.Newer || got.Latest != "v9.9.9" || got.URL == "" {
		t.Fatalf("update = %+v, want newer v9.9.9 with a URL", got)
	}
	// The URL must be one /api/open will accept, or the button does nothing.
	if !strings.HasPrefix(got.URL, RepoURL) {
		t.Fatalf("release URL %q is outside the project pages", got.URL)
	}
}
