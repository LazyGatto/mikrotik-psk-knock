package web

import (
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestNoteEndpointPersists(t *testing.T) {
	h := Handler(testConfigPath(t), "tok")
	if rr := do(t, h, "POST", "/api/note", `{"kind":"router","name":"r1","note":"prod router"}`); rr.Code != 200 {
		t.Fatalf("set note: %d %s", rr.Code, rr.Body.String())
	}
	rr := do(t, h, "GET", "/api/config", "")
	if !strings.Contains(rr.Body.String(), `"note":"prod router"`) {
		t.Fatalf("note not reflected in config: %s", rr.Body.String())
	}
}

func TestFirstRunMissingConfigShowsEmpty(t *testing.T) {
	// A path that does not exist yet — the first-run state.
	path := filepath.Join(t.TempDir(), "not-created-yet.yaml")
	h := Handler(path, "tok")
	rr := do(t, h, "GET", "/api/config", "")
	if rr.Code != 200 {
		t.Fatalf("first-run config: code = %d, want 200 (should not error on a missing file)", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, `"routers":[]`) || !strings.Contains(body, `"users":[]`) {
		t.Fatalf("first-run config should be empty arrays: %s", body)
	}
}

func TestEmbeddedHandlerSkipsHostGuard(t *testing.T) {
	h := EmbeddedHandler(testConfigPath(t), "tok")
	// A non-loopback Host (as the Wails webview sends) must be served, since the
	// desktop app has no TCP listener to rebind against.
	req := httptest.NewRequest("GET", "/api/config", nil)
	req.Host = "wails.localhost"
	req.Header.Set("X-MKPK-Token", "tok")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("embedded handler rejected wails host: %d %s", rr.Code, rr.Body.String())
	}

	// The token still gates the API in the desktop build.
	req = httptest.NewRequest("GET", "/api/config", nil)
	req.Host = "wails.localhost"
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 403 {
		t.Fatalf("embedded handler served without a token: %d", rr.Code)
	}
}
