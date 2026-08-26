package desktopui

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"mikrotik-psk-knock/client/internal/invite"
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

