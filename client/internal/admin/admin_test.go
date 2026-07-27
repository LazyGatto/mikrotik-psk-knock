package admin

import (
	"testing"

	"mikrotik-psk-knock/client/internal/config"
)

const rn = "r1"

func initCfg(t *testing.T) config.Config {
	t.Helper()
	cfg, err := InitConfig(InitOptions{RouterName: rn, RouterAddress: "r.example", ServiceName: "svc", ClientName: "cli"})
	if err != nil {
		t.Fatalf("InitConfig() error = %v", err)
	}
	return cfg
}

func TestInitConfigValidAndRequiresAddress(t *testing.T) {
	cfg := initCfg(t)
	if err := cfg.Validate(); err != nil {
		t.Fatalf("InitConfig produced invalid config: %v", err)
	}
	if cfg.Routers[rn].Clients["cli"].PSK == "" {
		t.Fatal("InitConfig did not generate a PSK")
	}
	if cfg.Routers[rn].Clients["cli"].Services[0] != "svc" {
		t.Fatal("InitConfig client not assigned the service")
	}
	if _, err := InitConfig(InitOptions{RouterName: rn, ServiceName: "svc", ClientName: "cli"}); err == nil {
		t.Fatal("InitConfig without address should error")
	}
}

func TestAddServiceDefaultsAndValidation(t *testing.T) {
	cfg := initCfg(t)

	cfg, err := AddService(cfg, rn, ServiceOptions{
		Name: "email-svc", Stage1Port: 43001, Stage2Port: 43002, TokenPort: 43003,
		NAT:    config.NAT{DstPort: 2022, ToAddress: "192.0.2.10", ToPort: 22},
		Notify: config.Notify{Enabled: true, Channel: "email", Email: config.NotifyEmail{To: "a@b.co", From: "m@b.co", Server: "smtp.b.co"}},
	})
	if err != nil {
		t.Fatalf("AddService() error = %v", err)
	}
	svc := cfg.Routers[rn].Services["email-svc"]
	if svc.AllowedList != "mkpk-tt-allowed-email-svc" {
		t.Fatalf("allowed_list default = %q", svc.AllowedList)
	}
	if svc.NAT.Comment != "mkpk-tt dst-nat email-svc" {
		t.Fatalf("nat comment default = %q", svc.NAT.Comment)
	}
	if svc.Notify.Email.Port != 587 || svc.Notify.Email.TLS != "starttls" {
		t.Fatalf("email defaults = %d/%q", svc.Notify.Email.Port, svc.Notify.Email.TLS)
	}

	if _, err := AddService(cfg, rn, ServiceOptions{Name: "bad"}); err == nil {
		t.Fatal("AddService without ports should error")
	}
	if _, err := AddService(cfg, rn, ServiceOptions{Name: "email-svc", Stage1Port: 1, Stage2Port: 2, TokenPort: 3, NAT: config.NAT{DstPort: 1, ToPort: 1, ToAddress: "x"}}); err == nil {
		t.Fatal("AddService on existing name without force should error")
	}
	if _, err := AddService(cfg, "nope", ServiceOptions{Name: "x", Stage1Port: 1, Stage2Port: 2, TokenPort: 3, NAT: config.NAT{DstPort: 1, ToPort: 1, ToAddress: "x"}}); err == nil {
		t.Fatal("AddService on unknown router should error")
	}
}

func TestSetServiceEnabled(t *testing.T) {
	cfg := initCfg(t)
	cfg, err := SetServiceEnabled(cfg, rn, "svc", false)
	if err != nil {
		t.Fatalf("SetServiceEnabled() error = %v", err)
	}
	if cfg.Routers[rn].Services["svc"].Enabled() {
		t.Fatal("service should be disabled")
	}
	cfg, _ = SetServiceEnabled(cfg, rn, "svc", true)
	if !cfg.Routers[rn].Services["svc"].Enabled() {
		t.Fatal("service should be enabled")
	}
}

func TestRemoveServiceRefusesReferenced(t *testing.T) {
	cfg := initCfg(t)
	if _, err := RemoveService(cfg, rn, "svc"); err == nil {
		t.Fatal("RemoveService should refuse a referenced service")
	}
	cfg, _ = RemoveClient(cfg, rn, "cli")
	if _, err := RemoveService(cfg, rn, "svc"); err != nil {
		t.Fatalf("RemoveService after unref error = %v", err)
	}
}

func TestAddClientGeneratesPSKAndServices(t *testing.T) {
	cfg := initCfg(t)
	cfg, _ = AddService(cfg, rn, ServiceOptions{Name: "web", Stage1Port: 42001, Stage2Port: 42002, TokenPort: 42003, NAT: config.NAT{DstPort: 3443, ToAddress: "192.0.2.20", ToPort: 443}})

	res, err := AddClient(cfg, rn, ClientOptions{Name: "phone", Services: []string{"svc", "web"}})
	if err != nil {
		t.Fatalf("AddClient() error = %v", err)
	}
	if res.PSKSource != "generated" || res.Config.Routers[rn].Clients["phone"].PSK == "" {
		t.Fatalf("AddClient did not generate PSK: source=%s", res.PSKSource)
	}
	if len(res.Config.Routers[rn].Clients["phone"].Services) != 2 {
		t.Fatal("AddClient did not assign both services")
	}

	res, err = AddClient(res.Config, rn, ClientOptions{Name: "laptop", Services: []string{"svc"}, PSK: "provided-psk"})
	if err != nil {
		t.Fatalf("AddClient() error = %v", err)
	}
	if res.PSKSource != "provided" {
		t.Fatalf("AddClient psk source = %s, want provided", res.PSKSource)
	}

	if _, err := AddClient(cfg, rn, ClientOptions{Name: "x", Services: []string{"missing"}}); err == nil {
		t.Fatal("AddClient with unknown service should error")
	}
}

func TestSummarizeIsSecretFreeAndOrdered(t *testing.T) {
	cfg := initCfg(t)
	s := Summarize(cfg)
	if len(s.Routers) != 1 {
		t.Fatalf("summary routers = %d", len(s.Routers))
	}
	r := s.Routers[0]
	if r.Name != rn || len(r.Services) != 1 || len(r.Clients) != 1 {
		t.Fatalf("summary shape wrong: %+v", r)
	}
	if !r.Clients[0].PSKSet {
		t.Fatal("summary should mark psk set")
	}
	if !r.Services[0].Enabled {
		t.Fatal("summary service should be enabled")
	}
}
