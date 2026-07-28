package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateRequiresAddress(t *testing.T) {
	r := validRouter()
	r.Address = ""
	if err := r.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want required-address error")
	}
}

func TestValidateAcceptsIPAndHostnameAddress(t *testing.T) {
	for _, addr := range []string{"10.0.0.1", "router.example.com", "router", "a-b.example.io"} {
		r := validRouter()
		r.Address = addr
		if err := r.Validate(); err != nil {
			t.Errorf("Validate() with address %q error = %v, want nil", addr, err)
		}
	}
}

func TestValidateRejectsBadAddress(t *testing.T) {
	for _, addr := range []string{"has space", "-leadinghyphen", "under_score", "a..b", "bad!host"} {
		r := validRouter()
		r.Address = addr
		if err := r.Validate(); err == nil {
			t.Errorf("Validate() with address %q error = nil, want format error", addr)
		}
	}
}

func TestValidateRejectsBadDeployAddress(t *testing.T) {
	r := validRouter()
	r.Deploy.Address = "not a host"
	if err := r.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want bad deploy.address error")
	}
}

func TestValidateAcceptsEmptyDeployAddress(t *testing.T) {
	r := validRouter()
	r.Deploy.Address = "" // optional — falls back to the public address
	if err := r.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil for empty deploy.address", err)
	}
}

func TestIsHostOrIP(t *testing.T) {
	ok := []string{"1.2.3.4", "::1", "host", "a.b.c", "x-y.example.com", "255.255.255.255"}
	bad := []string{"", "a b", "a_b", "-a", "a-", "a..b", "toolong" + strings.Repeat("x", 300)}
	for _, s := range ok {
		if !isHostOrIP(s) {
			t.Errorf("isHostOrIP(%q) = false, want true", s)
		}
	}
	for _, s := range bad {
		if isHostOrIP(s) {
			t.Errorf("isHostOrIP(%q) = true, want false", s)
		}
	}
}

func TestValidateRejectsTooLongName(t *testing.T) {
	cfg := validConfig()
	r := cfg.Routers["r1"]
	delete(cfg.Routers, "r1")
	cfg.Routers[strings.Repeat("a", maxNameLen+1)] = r
	if err := cfg.Validate(); err == nil {
		t.Fatalf("Validate() error = nil, want name-too-long error (>%d)", maxNameLen)
	}
}

func TestValidateRejectsTooLongNote(t *testing.T) {
	r := validRouter()
	r.Note = strings.Repeat("x", maxNoteLen+1)
	if err := r.Validate(); err == nil {
		t.Fatalf("Validate() error = nil, want note-too-long error (>%d)", maxNoteLen)
	}
}

func TestRenderHashIgnoresNotes(t *testing.T) {
	r := validRouter()
	base := RenderHash(r, nil)

	r.Note = "an operator note on the router"
	if RenderHash(r, nil) != base {
		t.Error("RenderHash changed after setting a router note; notes must not affect the .rsc hash")
	}

	svc := r.Services["demo-service"]
	svc.Note = "a note on the service"
	r.Services["demo-service"] = svc
	if RenderHash(r, nil) != base {
		t.Error("RenderHash changed after setting a service note; notes must not affect the .rsc hash")
	}
}

func TestLoadOrEmptyMissingFileReturnsEmptyConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.yaml")
	cfg, err := LoadOrEmpty(path)
	if err != nil {
		t.Fatalf("LoadOrEmpty(missing) error = %v, want nil", err)
	}
	if len(cfg.Routers) != 0 || len(cfg.Users) != 0 {
		t.Fatalf("LoadOrEmpty(missing) = %d routers / %d users, want 0/0", len(cfg.Routers), len(cfg.Users))
	}
	if cfg.Routers == nil || cfg.Users == nil {
		t.Fatal("LoadOrEmpty(missing) must return non-nil maps so callers can add to them")
	}
}

func TestLoadOrEmptyExistingFileLoadsNormally(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mkpk.yaml")
	// A minimal valid config with one router and one service.
	yaml := `routers:
  r1:
    address: router.example.com
    defaults: {bucket_seconds: 30, stage_timeout: 5s, token_hit_timeout: 2s, allowed_timeout: 3m, used_timeout: 65s}
    services:
      svc:
        service_name: svc
        stage1_port: 41001
        stage2_port: 41002
        token_port: 41003
        allowed_list: mkpk-tt-allowed-svc
        target: {type: forward, protocol: tcp, port: 2222, to_address: 192.0.2.10, to_port: 22}
users: {}
`
	if err := os.WriteFile(path, []byte(yaml), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadOrEmpty(path)
	if err != nil {
		t.Fatalf("LoadOrEmpty(existing) error = %v", err)
	}
	if _, ok := cfg.Routers["r1"]; !ok {
		t.Fatal("LoadOrEmpty(existing) did not load router r1")
	}
}
