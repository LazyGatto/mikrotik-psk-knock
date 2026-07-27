package config

import "testing"

func TestValidateAcceptsSafePSK(t *testing.T) {
	if err := validRouter().Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateRejectsUnsafePSK(t *testing.T) {
	r := validRouter()
	c := r.Clients["demo-client"]
	c.PSK = "bad$psk"
	r.Clients["demo-client"] = c
	if err := r.Validate(); err == nil {
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
	c := r.Clients["demo-client"]
	c.Services = []string{"bad name"}
	r.Clients["demo-client"] = c
	if err := r.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want unsafe service name error")
	}
}

func TestValidateRejectsClientUnknownService(t *testing.T) {
	r := validRouter()
	c := r.Clients["demo-client"]
	c.Services = []string{"nope"}
	r.Clients["demo-client"] = c
	if err := r.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want unknown service error")
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

func TestApplyDefaultsEmailPortAndTLS(t *testing.T) {
	cfg := Config{Routers: map[string]Router{
		"r1": {Services: map[string]Service{
			"s": {Stage1Port: 1, Stage2Port: 2, TokenPort: 3, Notify: Notify{Enabled: true, Channel: "email"}},
		}},
	}}
	cfg.applyDefaults()
	got := cfg.Routers["r1"].Services["s"].Notify.Email
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
	svc := r.Services["demo-service"]
	svc.Notify = Notify{Enabled: true, Channel: "telegram", Telegram: NotifyTelegram{BotToken: "123456:AA-bb_CC", ChatID: "-100200300"}}
	r.Services["demo-service"] = svc
	if err := r.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateRejectsBadTelegramToken(t *testing.T) {
	r := validRouter()
	svc := r.Services["demo-service"]
	svc.Notify = Notify{Enabled: true, Channel: "telegram", Telegram: NotifyTelegram{BotToken: "not-a-token", ChatID: "123"}}
	r.Services["demo-service"] = svc
	if err := r.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want bad telegram token error")
	}
}

func TestValidateRejectsWebhookWithoutURL(t *testing.T) {
	r := validRouter()
	svc := r.Services["demo-service"]
	svc.Notify = Notify{Enabled: true, Channel: "webhook", URL: ""}
	r.Services["demo-service"] = svc
	if err := r.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want missing webhook url error")
	}
}

func TestValidateRejectsUnknownNotifyChannel(t *testing.T) {
	r := validRouter()
	svc := r.Services["demo-service"]
	svc.Notify = Notify{Enabled: true, Channel: "carrier-pigeon", URL: "https://x"}
	r.Services["demo-service"] = svc
	if err := r.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want unknown channel error")
	}
}

func TestValidateAcceptsEmailNotify(t *testing.T) {
	r := validRouter()
	svc := r.Services["demo-service"]
	svc.Notify = Notify{Enabled: true, Channel: "email", Email: NotifyEmail{To: "alerts@example.com", From: "mkpk@example.com", Server: "smtp.example.com", Port: 587, TLS: "starttls"}}
	r.Services["demo-service"] = svc
	if err := r.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateRejectsEmailWithoutServer(t *testing.T) {
	r := validRouter()
	svc := r.Services["demo-service"]
	svc.Notify = Notify{Enabled: true, Channel: "email", Email: NotifyEmail{To: "a@b.co", From: "m@b.co", Port: 587, TLS: "starttls"}}
	r.Services["demo-service"] = svc
	if err := r.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want missing email server error")
	}
}

func TestValidateRejectsEmailBadTLS(t *testing.T) {
	r := validRouter()
	svc := r.Services["demo-service"]
	svc.Notify = Notify{Enabled: true, Channel: "email", Email: NotifyEmail{To: "a@b.co", From: "m@b.co", Server: "s", Port: 587, TLS: "ssl"}}
	r.Services["demo-service"] = svc
	if err := r.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want bad email tls error")
	}
}

func TestRouterHashStableAndSensitive(t *testing.T) {
	h1 := validRouter().Hash()
	if h1 == "" || h1 != validRouter().Hash() {
		t.Fatal("Hash() empty or not stable")
	}
	r := validRouter()
	c := r.Clients["demo-client"]
	c.PSK = "different-psk-value"
	r.Clients["demo-client"] = c
	if r.Hash() == h1 {
		t.Fatal("Hash() did not change after config change")
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
	c := r.Clients["demo-client"]
	c.Services = []string{"demo-service", "web"}
	r.Clients["demo-client"] = c
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

func TestRouterHashIgnoresDeployCredentials(t *testing.T) {
	base := validRouter().Hash()
	r := validRouter()
	r.Deploy = Deploy{Port: 2222, User: "admin", KeyPath: "~/.ssh/id_ed25519", UseAgent: true, Password: "secret"}
	if r.Hash() != base {
		t.Fatal("Hash() changed after setting deploy credentials; creds must not affect the fingerprint")
	}
}

func TestValidateRejectsBadDeployPort(t *testing.T) {
	r := validRouter()
	r.Deploy.Port = 70000
	if err := r.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want bad deploy port error")
	}
}

func TestResolveMultiService(t *testing.T) {
	r := validRouter()
	r.Services["web"] = Service{ServiceName: "web", Stage1Port: 42001, Stage2Port: 42002, TokenPort: 42003, AllowedList: "mkpk-tt-allowed-web", Target: Target{Type: TargetForward, Protocol: "tcp", Port: 3443, ToAddress: "192.0.2.20", ToPort: 443}}
	c := r.Clients["demo-client"]
	c.Services = []string{"demo-service", "web"}
	r.Clients["demo-client"] = c

	// ambiguous without a service name
	if _, err := r.Resolve("r1", "demo-client", ""); err == nil {
		t.Fatal("Resolve without service on multi-service client should error")
	}
	// explicit service works
	res, err := r.Resolve("r1", "demo-client", "web")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if res.Service.TokenPort != 42003 {
		t.Fatalf("resolved wrong service: token port %d", res.Service.TokenPort)
	}
	// unassigned service rejected
	r.Services["other"] = r.Services["demo-service"]
	if _, err := r.Resolve("r1", "demo-client", "other"); err == nil {
		t.Fatal("Resolve of unassigned service should error")
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
		Clients: map[string]Client{
			"demo-client": {ClientID: "demo-client", Services: []string{"demo-service"}, PSK: "mkpk-prototype-psk"},
		},
	}
}
