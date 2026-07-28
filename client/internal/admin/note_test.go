package admin

import "testing"

func TestSetNoteOnRouterServiceUser(t *testing.T) {
	cfg := initCfg(t)

	cfg, err := SetNote(cfg, "router", "", rn, "router note")
	if err != nil {
		t.Fatalf("SetNote(router) error = %v", err)
	}
	if got := cfg.Routers[rn].Note; got != "router note" {
		t.Fatalf("router note = %q, want %q", got, "router note")
	}

	cfg, err = SetNote(cfg, "service", rn, "svc", "service note")
	if err != nil {
		t.Fatalf("SetNote(service) error = %v", err)
	}
	if got := cfg.Routers[rn].Services["svc"].Note; got != "service note" {
		t.Fatalf("service note = %q, want %q", got, "service note")
	}

	cfg, err = SetNote(cfg, "user", "", "cli", "user note")
	if err != nil {
		t.Fatalf("SetNote(user) error = %v", err)
	}
	if got := cfg.Users["cli"].Note; got != "user note" {
		t.Fatalf("user note = %q, want %q", got, "user note")
	}

	// Notes must survive validation and appear in the summary.
	if err := cfg.Validate(); err != nil {
		t.Fatalf("config with notes is invalid: %v", err)
	}
	s := Summarize(cfg)
	if s.Routers[0].Note != "router note" {
		t.Errorf("summary router note = %q", s.Routers[0].Note)
	}
	if s.Routers[0].Services[0].Note != "service note" {
		t.Errorf("summary service note = %q", s.Routers[0].Services[0].Note)
	}
	if s.Users[0].Note != "user note" {
		t.Errorf("summary user note = %q", s.Users[0].Note)
	}
}

func TestSetNoteClearsWithEmptyString(t *testing.T) {
	cfg := initCfg(t)
	cfg, _ = SetNote(cfg, "router", "", rn, "temporary")
	cfg, err := SetNote(cfg, "router", "", rn, "")
	if err != nil {
		t.Fatalf("SetNote(clear) error = %v", err)
	}
	if got := cfg.Routers[rn].Note; got != "" {
		t.Fatalf("router note after clear = %q, want empty", got)
	}
}

func TestSetNoteDoesNotChangeRouterHash(t *testing.T) {
	cfg := initCfg(t)
	before := cfg.RouterHash(rn)
	cfg, err := SetNote(cfg, "router", "", rn, "a note")
	if err != nil {
		t.Fatalf("SetNote() error = %v", err)
	}
	if after := cfg.RouterHash(rn); after != before {
		t.Fatalf("RouterHash changed after a note edit: %s -> %s (notes are local-only)", before, after)
	}
}

func TestSetNoteUnknownTargets(t *testing.T) {
	cfg := initCfg(t)
	if _, err := SetNote(cfg, "router", "", "nope", "x"); err == nil {
		t.Error("SetNote(unknown router) error = nil, want error")
	}
	if _, err := SetNote(cfg, "service", rn, "nope", "x"); err == nil {
		t.Error("SetNote(unknown service) error = nil, want error")
	}
	if _, err := SetNote(cfg, "user", "", "nope", "x"); err == nil {
		t.Error("SetNote(unknown user) error = nil, want error")
	}
	if _, err := SetNote(cfg, "bogus", "", rn, "x"); err == nil {
		t.Error("SetNote(unknown kind) error = nil, want error")
	}
}
