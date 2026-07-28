package web

import (
	"strings"
	"testing"
)

// TestDeployStreamEmitsNDJSON checks the streaming endpoint's shape without a
// reachable router: it starts 200 with an ndjson content-type, emits the initial
// log event, and ends with an error event (the test router does not resolve).
func TestDeployStreamEmitsNDJSON(t *testing.T) {
	h := Handler(testConfigPath(t), "tok")
	rr := do(t, h, "POST", "/api/deploy/stream", `{"action":"status","router":"r1"}`)
	if rr.Code != 200 {
		t.Fatalf("code = %d, want 200", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "ndjson") {
		t.Fatalf("content-type = %q, want ndjson", ct)
	}
	body := rr.Body.String()
	if !strings.Contains(body, `"type":"log"`) {
		t.Fatalf("no initial log event: %s", body)
	}
	if !strings.Contains(body, `"type":"error"`) {
		t.Fatalf("expected an error event for the unreachable test router: %s", body)
	}
}
