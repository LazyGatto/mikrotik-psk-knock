package web

import (
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"mikrotik-psk-knock/client/internal/admin"
)

func testConfigPath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "mkpk.yaml")
	cfg, err := admin.InitConfig(admin.InitOptions{RouterAddress: "r.example", ServiceName: "svc", ClientName: "cli"})
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

	body := `{"name":"web","stage1_port":42001,"stage2_port":42002,"token_port":42003,"nat":{"dst_port":3443,"to_address":"192.0.2.20","to_port":443}}`
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
