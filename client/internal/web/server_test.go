package web

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"mikrotik-psk-knock/client/internal/admin"
	"mikrotik-psk-knock/client/internal/config"
)

func testConfigPath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "mkpk.yaml")
	cfg, err := admin.InitConfig(admin.InitOptions{RouterName: "r1", RouterAddress: "r.example", ServiceName: "svc", ClientName: "cli"})
	if err != nil {
		t.Fatalf("InitConfig() error = %v", err)
	}
	if err := admin.SaveConfig(path, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	return path
}

func TestConfigRequiresTokenAndLoopbackHost(t *testing.T) {
	h := Handler(testConfigPath(t), "tok")

	cases := []struct {
		name  string
		host  string
		token string
		want  int
	}{
		{"ok", "127.0.0.1:8765", "tok", 200},
		{"no token", "127.0.0.1:8765", "", 403},
		{"bad host", "evil.com", "tok", 403},
	}
	for _, c := range cases {
		req := httptest.NewRequest("GET", "/api/config", nil)
		req.Host = c.host
		if c.token != "" {
			req.Header.Set("X-MKPK-Token", c.token)
		}
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != c.want {
			t.Fatalf("%s: code = %d, want %d", c.name, rr.Code, c.want)
		}
	}
}

func TestAddServicePersists(t *testing.T) {
	path := testConfigPath(t)
	h := Handler(path, "tok")

	body := `{"router":"r1","name":"web","stage1_port":42001,"stage2_port":42002,"token_port":42003,"target":{"type":"forward","protocol":"tcp","port":3443,"to_address":"192.0.2.20","to_port":443}}`
	req := httptest.NewRequest("POST", "/api/service", strings.NewReader(body))
	req.Host = "127.0.0.1:8765"
	req.Header.Set("X-MKPK-Token", "tok")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("add service code = %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"name":"web"`) {
		t.Fatalf("response missing new service: %s", rr.Body.String())
	}
}

func do(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var rdr *strings.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	var req *http.Request
	if rdr != nil {
		req = httptest.NewRequest(method, path, rdr)
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	req.Host = "127.0.0.1:8765"
	req.Header.Set("X-MKPK-Token", "tok")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func TestRouterAddAndServiceEnableAndExport(t *testing.T) {
	h := Handler(testConfigPath(t), "tok")

	// add a second router
	if rr := do(t, h, "POST", "/api/router", `{"name":"r2","address":"10.0.0.2"}`); rr.Code != 200 {
		t.Fatalf("add router: %d %s", rr.Code, rr.Body.String())
	}

	// disable the demo service on r1
	rr := do(t, h, "POST", "/api/service/enable", `{"router":"r1","name":"svc","enabled":false}`)
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), `"enabled":false`) {
		t.Fatalf("disable service: %d %s", rr.Code, rr.Body.String())
	}
	// re-enable so export has an enabled service
	if rr := do(t, h, "POST", "/api/service/enable", `{"router":"r1","name":"svc","enabled":true}`); rr.Code != 200 {
		t.Fatalf("enable service: %d %s", rr.Code, rr.Body.String())
	}

	// export the demo user's invite blob
	rr = do(t, h, "GET", "/api/export?router=r1&user=cli", "")
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), `"blob":`) {
		t.Fatalf("export: %d %s", rr.Code, rr.Body.String())
	}
}

func TestRouterCredsPersistAndSecretKeptOnEdit(t *testing.T) {
	path := testConfigPath(t)
	h := Handler(path, "tok")

	// Create a router with a password secret.
	if rr := do(t, h, "POST", "/api/router",
		`{"name":"r2","address":"10.0.0.2","user":"admin","port":2222,"key_path":"~/.ssh/id_ed25519","password":"s3cret"}`); rr.Code != 200 {
		t.Fatalf("add router: %d %s", rr.Code, rr.Body.String())
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := cfg.Routers["r2"].Deploy; got.User != "admin" || got.Port != 2222 || got.Password != "s3cret" {
		t.Fatalf("creds not persisted: %+v", got)
	}

	// Response must not leak the secret; it exposes only password_set.
	rr := do(t, h, "GET", "/api/config", "")
	if strings.Contains(rr.Body.String(), "s3cret") {
		t.Fatal("config response leaked the ssh password")
	}
	if !strings.Contains(rr.Body.String(), `"password_set":true`) {
		t.Fatalf("summary missing password_set: %s", rr.Body.String())
	}

	// Edit with a blank password keeps the stored one.
	if rr := do(t, h, "POST", "/api/router", `{"name":"r2","address":"10.0.0.3","user":"root","password":""}`); rr.Code != 200 {
		t.Fatalf("edit router: %d %s", rr.Code, rr.Body.String())
	}
	cfg, _ = config.Load(path)
	if got := cfg.Routers["r2"].Deploy; got.Password != "s3cret" || got.User != "root" {
		t.Fatalf("edit did not keep secret / update user: %+v", got)
	}
}

func TestPortsSuggestReturnsFreePorts(t *testing.T) {
	h := Handler(testConfigPath(t), "tok")
	rr := do(t, h, "GET", "/api/ports/suggest?router=r1&count=3", "")
	if rr.Code != 200 {
		t.Fatalf("suggest: %d %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"ports":[`) {
		t.Fatalf("suggest response missing ports: %s", rr.Body.String())
	}
}

func TestNewUserAccessIsEmptyArrayNotNull(t *testing.T) {
	h := Handler(testConfigPath(t), "tok")
	if rr := do(t, h, "POST", "/api/user", `{"name":"phone"}`); rr.Code != 200 {
		t.Fatalf("create user: %d %s", rr.Code, rr.Body.String())
	}
	rr := do(t, h, "GET", "/api/config", "")
	body := rr.Body.String()
	// The frontend does u.access.length; a JSON null would crash it.
	if strings.Contains(body, `"access":null`) {
		t.Fatalf("access serialized as null (would blank the UI): %s", body)
	}
	if !strings.Contains(body, `"name":"phone","client_id":"phone","access":[]`) {
		t.Fatalf("new user access not an empty array: %s", body)
	}
}

func TestIndexInjectsToken(t *testing.T) {
	h := Handler(testConfigPath(t), "sekret-token")
	req := httptest.NewRequest("GET", "/", nil)
	req.Host = "127.0.0.1:8765"
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if !strings.Contains(rr.Body.String(), `"sekret-token"`) {
		t.Fatal("index did not inject session token")
	}
}

// A change that moves a knock port must come back with the users whose invites
// it just killed; a note edit must not.
func TestSaveWarnsAboutStaleInvites(t *testing.T) {
	h := Handler(testConfigPath(t), "tok")

	rr := do(t, h, "POST", "/api/note", `{"kind":"user","name":"cli","note":"hi"}`)
	if rr.Code != 200 {
		t.Fatalf("note: %d %s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "stale_invites") {
		t.Fatalf("a note edit warned about stale invites: %s", rr.Body.String())
	}

	rr = do(t, h, "POST", "/api/service",
		`{"router":"r1","name":"svc","stage1_port":43001,"stage2_port":43002,"token_port":43003,"target":{"type":"forward","protocol":"tcp","port":2222,"to_address":"192.0.2.10","to_port":22}}`)
	if rr.Code != 200 {
		t.Fatalf("service edit: %d %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"stale_invites":["cli"]`) {
		t.Fatalf("port change did not warn about the affected user: %s", rr.Body.String())
	}
	// The fingerprint travels with the summary so the admin can compare it with
	// what `mkpk invite show` prints on the user's side.
	if !strings.Contains(rr.Body.String(), `"fingerprint":`) {
		t.Fatalf("summary carries no fingerprint: %s", rr.Body.String())
	}
}

// The container log is the only window into a networked instance, so a failing
// request must carry its reason and the page's polling must not bury it.
func TestLogRequestsShowsErrorsAndHidesPolling(t *testing.T) {
	var buf bytes.Buffer
	h := LogRequests(Handler(testConfigPath(t), "tok"), log.New(&buf, "", 0))

	if rr := do(t, h, "GET", "/api/config", ""); rr.Code != 200 {
		t.Fatalf("config: %d", rr.Code)
	}
	if strings.Contains(buf.String(), "/api/config") {
		t.Fatalf("a successful poll was logged: %q", buf.String())
	}

	if rr := do(t, h, "POST", "/api/router", `{"name":"","address":""}`); rr.Code != 400 {
		t.Fatalf("router: %d", rr.Code)
	}
	line := buf.String()
	if !strings.Contains(line, "-> 400") || !strings.Contains(line, "router name and address are required") {
		t.Fatalf("failure logged without its reason: %q", line)
	}
}
