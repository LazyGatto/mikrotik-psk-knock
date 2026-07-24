package admin

import (
	"testing"

	"mikrotik-psk-knock/client/internal/config"
)

func TestInitConfigValidAndRequiresAddress(t *testing.T) {
	cfg, err := InitConfig(InitOptions{RouterAddress: "r.example", ServiceName: "svc", ClientName: "cli"})
	if err != nil {
		t.Fatalf("InitConfig() error = %v", err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("InitConfig produced invalid config: %v", err)
	}
	if cfg.Clients["cli"].PSK == "" {
		t.Fatal("InitConfig did not generate a PSK")
	}

	if _, err := InitConfig(InitOptions{ServiceName: "svc", ClientName: "cli"}); err == nil {
		t.Fatal("InitConfig without address should error")
	}
}

func TestAddServiceDefaultsAndValidation(t *testing.T) {
	cfg, _ := InitConfig(InitOptions{RouterAddress: "r.example", ServiceName: "svc", ClientName: "cli"})

	cfg, err := AddService(cfg, ServiceOptions{
		Name: "email-svc", Stage1Port: 43001, Stage2Port: 43002, TokenPort: 43003,
		NAT:    config.NAT{DstPort: 2022, ToAddress: "192.0.2.10", ToPort: 22},
		Notify: config.Notify{Enabled: true, Channel: "email", Email: config.NotifyEmail{To: "a@b.co", From: "m@b.co", Server: "smtp.b.co"}},
	})
	if err != nil {
		t.Fatalf("AddService() error = %v", err)
	}
	svc := cfg.Services["email-svc"]
	if svc.AllowedList != "mkpk-tt-allowed-email-svc" {
		t.Fatalf("allowed_list default = %q", svc.AllowedList)
	}
	if svc.NAT.Comment != "mkpk-tt dst-nat email-svc" {
		t.Fatalf("nat comment default = %q", svc.NAT.Comment)
	}
	if svc.Notify.Email.Port != 587 || svc.Notify.Email.TLS != "starttls" {
		t.Fatalf("email defaults = %d/%q", svc.Notify.Email.Port, svc.Notify.Email.TLS)
	}

	if _, err := AddService(cfg, ServiceOptions{Name: "bad"}); err == nil {
		t.Fatal("AddService without ports should error")
	}
	if _, err := AddService(cfg, ServiceOptions{Name: "email-svc", Stage1Port: 1, Stage2Port: 2, TokenPort: 3, NAT: config.NAT{DstPort: 1, ToPort: 1, ToAddress: "x"}}); err == nil {
		t.Fatal("AddService on existing name without force should error")
	}
}

func TestAddClientGeneratesPSK(t *testing.T) {
	cfg, _ := InitConfig(InitOptions{RouterAddress: "r.example", ServiceName: "svc", ClientName: "cli"})

	res, err := AddClient(cfg, ClientOptions{Name: "phone", Service: "svc"})
	if err != nil {
		t.Fatalf("AddClient() error = %v", err)
	}
	if res.PSKSource != "generated" || res.Config.Clients["phone"].PSK == "" {
		t.Fatalf("AddClient did not generate PSK: source=%s", res.PSKSource)
	}

	res, err = AddClient(res.Config, ClientOptions{Name: "laptop", Service: "svc", PSK: "provided-psk"})
	if err != nil {
		t.Fatalf("AddClient() error = %v", err)
	}
	if res.PSKSource != "provided" {
		t.Fatalf("AddClient psk source = %s, want provided", res.PSKSource)
	}

	if _, err := AddClient(cfg, ClientOptions{Name: "x", Service: "missing"}); err == nil {
		t.Fatal("AddClient with unknown service should error")
	}
}

func TestSummarizeIsSecretFreeAndOrdered(t *testing.T) {
	cfg, _ := InitConfig(InitOptions{RouterAddress: "r.example", ServiceName: "svc", ClientName: "cli"})
	s := Summarize(cfg)
	if len(s.Services) != 1 || len(s.Clients) != 1 {
		t.Fatalf("summary services/clients = %d/%d", len(s.Services), len(s.Clients))
	}
	if !s.Clients[0].PSKSet {
		t.Fatal("summary should mark psk set")
	}
	if s.Clients[0].Name != "cli" {
		t.Fatalf("client name = %q", s.Clients[0].Name)
	}
}
