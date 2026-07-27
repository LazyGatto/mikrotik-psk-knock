package config

import "testing"

func TestValidateAcceptsSafeConfig(t *testing.T) {
	if err := validConfig().Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateRejectsUnsafePSK(t *testing.T) {
	cfg := validConfig()
	u := cfg.Users["u1"]
	a := u.Access["r1"]
	a.PSK = "bad$psk"
	u.Access["r1"] = a
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want unsafe PSK error")
	}
}

func TestValidateRejectsDuplicateStagePorts(t *testing.T) {
	r := validRouter()
	svc := r.Services["demo-service"]
	svc.Stage2Port = svc.Stage1Port
	r.Services["demo-service"] = svc
	if err := r.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want duplicate port error")
	}
}

func TestValidateRejectsInvalidTimeouts(t *testing.T) {
	r := validRouter()
	r.Defaults.AllowedTimeout = "1d"
	if err := r.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want invalid timeout error")
	}
}

func TestValidateRejectsUsedTimeoutShorterThanAcceptedBuckets(t *testing.T) {
	r := validRouter()
	r.Defaults.BucketSeconds = 30
	r.Defaults.UsedTimeout = "59s"
	if err := r.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want short used timeout error")
	}
}

func TestValidateRejectsUnsafeServiceName(t *testing.T) {
	r := validRouter()
	svc := r.Services["demo-service"]
	delete(r.Services, "demo-service")
	r.Services["bad name"] = svc
	if err := r.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want unsafe service name error")
	}
}

func TestValidateRejectsUserUnknownService(t *testing.T) {
	cfg := validConfig()
	u := cfg.Users["u1"]
	u.Access["r1"] = UserAccess{Services: []string{"nope"}, PSK: "ok"}
	cfg.Users["u1"] = u
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want unknown service error")
	}
}

func TestValidateRejectsUserUnknownRouter(t *testing.T) {
	cfg := validConfig()
	u := cfg.Users["u1"]
	u.Access["ghost"] = UserAccess{Services: nil, PSK: "ok"}
	cfg.Users["u1"] = u
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want unknown router error")
	}
}

func TestValidateRejectsEmptyRouters(t *testing.T) {
	if err := (Config{}).Validate(); err == nil {
		t.Fatal("Validate() error = nil, want empty routers error")
	}
}

func TestApplyDefaultsPerServiceAllowedList(t *testing.T) {
	cfg := Config{Routers: map[string]Router{
		"r1": {Services: map[string]Service{"ssh-home": {Stage1Port: 1, Stage2Port: 2, TokenPort: 3}}},
	}}
	cfg.applyDefaults()
	if got := cfg.Routers["r1"].Services["ssh-home"].AllowedList; got != "mkpk-tt-allowed-ssh-home" {
		t.Fatalf("allowed_list = %q, want mkpk-tt-allowed-ssh-home", got)
	}
}

func TestApplyDefaultsUserClientID(t *testing.T) {
	cfg := Config{Users: map[string]User{"phone": {Access: map[string]UserAccess{}}}}
	cfg.applyDefaults()
	if got := cfg.Users["phone"].ClientID; got != "phone" {
		t.Fatalf("client_id default = %q, want phone", got)
	}
}

func TestApplyDefaultsEmailPortAndTLS(t *testing.T) {
	cfg := Config{Routers: map[string]Router{
		"r1": {
			Notify:   Notify{Enabled: true, Channel: "email"},
			Services: map[string]Service{"s": {Stage1Port: 1, Stage2Port: 2, TokenPort: 3}},
		},
	}}
	cfg.applyDefaults()
	got := cfg.Routers["r1"].Notify.Email
	if got.Port != 587 || got.TLS != "starttls" {
		t.Fatalf("email defaults = port %d tls %q, want 587/starttls", got.Port, got.TLS)
	}
}

func TestServiceEnabled(t *testing.T) {
	if !(Service{}).Enabled() {
		t.Fatal("service with Disabled=false should be enabled")
	}
	if (Service{Disabled: true}).Enabled() {
		t.Fatal("service with Disabled=true should not be enabled")
	}
}

func TestValidateAcceptsTelegramNotify(t *testing.T) {
	r := validRouter()
	r.Notify = Notify{Enabled: true, Channel: "telegram", Telegram: NotifyTelegram{BotToken: "123456:AA-bb_CC", ChatID: "-100200300"}}
	if err := r.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateRejectsBadTelegramToken(t *testing.T) {
	r := validRouter()
	r.Notify = Notify{Enabled: true, Channel: "telegram", Telegram: NotifyTelegram{BotToken: "not-a-token", ChatID: "123"}}
	if err := r.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want bad telegram token error")
	}
}

func TestValidateRejectsWebhookWithoutURL(t *testing.T) {
	r := validRouter()
	r.Notify = Notify{Enabled: true, Channel: "webhook", URL: ""}
	if err := r.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want missing webhook url error")
	}
}

func TestValidateRejectsUnknownNotifyChannel(t *testing.T) {
	r := validRouter()
	r.Notify = Notify{Enabled: true, Channel: "carrier-pigeon", URL: "https://x"}
	if err := r.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want unknown channel error")
	}
}

func TestValidateAcceptsEmailNotify(t *testing.T) {
	r := validRouter()
	r.Notify = Notify{Enabled: true, Channel: "email", Email: NotifyEmail{To: "alerts@example.com", From: "mkpk@example.com", Server: "smtp.example.com", Port: 587, TLS: "starttls"}}
	if err := r.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateRejectsEmailWithoutServer(t *testing.T) {
	r := validRouter()
	r.Notify = Notify{Enabled: true, Channel: "email", Email: NotifyEmail{To: "a@b.co", From: "m@b.co", Port: 587, TLS: "starttls"}}
	if err := r.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want missing email server error")
	}
}

func TestValidateRejectsEmailBadTLS(t *testing.T) {
	r := validRouter()
	r.Notify = Notify{Enabled: true, Channel: "email", Email: NotifyEmail{To: "a@b.co", From: "m@b.co", Server: "s", Port: 587, TLS: "ssl"}}
	if err := r.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want bad email tls error")
	}
}

func TestValidateRejectsPortCollisionAcrossServices(t *testing.T) {
	r := validRouter()
	// second service reuses the first service's stage1 port
	r.Services["web"] = Service{
		ServiceName: "web", Stage1Port: 41001, Stage2Port: 52002, TokenPort: 52003,
		AllowedList: "mkpk-tt-allowed-web",
		Target:      Target{Type: TargetForward, Protocol: "tcp", Port: 3443, ToAddress: "192.0.2.20", ToPort: 443},
	}
	if err := r.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want cross-service port collision error")
	}
}

func TestUsedPortsCollectsAll(t *testing.T) {
	r := validRouter()
	used := r.UsedPorts()
	for _, p := range []int{41001, 41002, 41003, 2222} { // stage1/2/token + target port
		if !used[p] {
			t.Fatalf("UsedPorts missing %d: %v", p, used)
		}
	}
}

func TestValidateLocalTargetRejectsForwardFields(t *testing.T) {
	r := validRouter()
	svc := r.Services["demo-service"]
	svc.Target = Target{Type: TargetLocal, Protocol: "tcp", Port: 8291, ToAddress: "192.0.2.10"}
	r.Services["demo-service"] = svc
	if err := r.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want local-target-with-to_address error")
	}
}

func TestValidateAcceptsLocalTarget(t *testing.T) {
	r := validRouter()
	svc := r.Services["demo-service"]
	svc.Target = Target{Type: TargetLocal, Protocol: "tcp", Port: 8291}
	r.Services["demo-service"] = svc
	if err := r.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want a valid local target", err)
	}
}

func TestValidateRejectsUnknownTargetType(t *testing.T) {
	r := validRouter()
	svc := r.Services["demo-service"]
	svc.Target = Target{Type: "banana", Protocol: "tcp", Port: 1}
	r.Services["demo-service"] = svc
	if err := r.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want unknown target type error")
	}
}

func TestValidateRejectsBadDeployPort(t *testing.T) {
	r := validRouter()
	r.Deploy.Port = 70000
	if err := r.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want bad deploy port error")
	}
}

func TestRouterHashIgnoresDeployCredentials(t *testing.T) {
	cfg := validConfig()
	base := cfg.RouterHash("r1")
	r := cfg.Routers["r1"]
	r.Deploy = Deploy{Port: 2222, User: "admin", KeyPath: "~/.ssh/id_ed25519", UseAgent: true, Password: "secret"}
	cfg.Routers["r1"] = r
	if cfg.RouterHash("r1") != base {
		t.Fatal("RouterHash changed after setting deploy credentials; creds must not affect the fingerprint")
	}
}

func TestRouterHashSensitiveToUserPSK(t *testing.T) {
	cfg := validConfig()
	base := cfg.RouterHash("r1")
	u := cfg.Users["u1"]
	a := u.Access["r1"]
	a.PSK = "different-psk-value"
	u.Access["r1"] = a
	if cfg.RouterHash("r1") == base {
		t.Fatal("RouterHash did not change after a user's PSK on the router changed")
	}
}

func TestRouterHashSensitiveToNotify(t *testing.T) {
	cfg := validConfig()
	base := cfg.RouterHash("r1")
	r := cfg.Routers["r1"]
	r.Notify = Notify{Enabled: true, Channel: "webhook", URL: "https://hook.example"}
	cfg.Routers["r1"] = r
	if cfg.RouterHash("r1") == base {
		t.Fatal("RouterHash should change when the router's notify config changes")
	}
}

func TestResolveMultiService(t *testing.T) {
	cfg := validConfig()
	r := cfg.Routers["r1"]
	r.Services["web"] = Service{ServiceName: "web", Stage1Port: 42001, Stage2Port: 42002, TokenPort: 42003, AllowedList: "mkpk-tt-allowed-web", Target: Target{Type: TargetForward, Protocol: "tcp", Port: 3443, ToAddress: "192.0.2.20", ToPort: 443}}
	cfg.Routers["r1"] = r
	u := cfg.Users["u1"]
	u.Access["r1"] = UserAccess{Services: []string{"demo-service", "web"}, PSK: "mkpk-prototype-psk"}
	cfg.Users["u1"] = u

	// ambiguous without a service name
	if _, err := cfg.Resolve("u1", "r1", ""); err == nil {
		t.Fatal("Resolve without service on a multi-service grant should error")
	}
	// explicit service works
	res, err := cfg.Resolve("u1", "r1", "web")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if res.Service.TokenPort != 42003 || res.ClientID != "demo-client" {
		t.Fatalf("resolved wrong: %+v", res)
	}
	// unassigned service rejected
	if _, err := cfg.Resolve("u1", "r1", "demo-service-x"); err == nil {
		t.Fatal("Resolve of unassigned service should error")
	}
}

func TestRenderClientsProjectsPerRouter(t *testing.T) {
	cfg := validConfig()
	rc := cfg.RenderClients("r1")
	if len(rc) != 1 || rc[0].Name != "u1" || rc[0].ClientID != "demo-client" || rc[0].PSK != "mkpk-prototype-psk" {
		t.Fatalf("RenderClients wrong: %+v", rc)
	}
	if len(cfg.RenderClients("ghost")) != 0 {
		t.Fatal("RenderClients for an unknown router should be empty")
	}
}

func validRouter() Router {
	return Router{
		Address:  "r.example",
		Defaults: Defaults{BucketSeconds: 30, StageTimeout: "5s", TokenHitTimeout: "2s", AllowedTimeout: "3m", UsedTimeout: "65s"},
		Services: map[string]Service{
			"demo-service": {
				ServiceName: "demo-service",
				Stage1Port:  41001, Stage2Port: 41002, TokenPort: 41003,
				AllowedList: "mkpk-tt-allowed-demo-service",
				Target:      Target{Type: TargetForward, Protocol: "tcp", Port: 2222, ToAddress: "192.0.2.10", ToPort: 22},
			},
		},
	}
}

func validConfig() Config {
	return Config{
		Routers: map[string]Router{"r1": validRouter()},
		Users: map[string]User{
			"u1": {
				ClientID: "demo-client",
				Access: map[string]UserAccess{
					"r1": {Services: []string{"demo-service"}, PSK: "mkpk-prototype-psk"},
				},
			},
		},
	}
}
