package config

import "testing"

func TestValidateAcceptsSafePSK(t *testing.T) {
	cfg := validConfig()

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateRejectsUnsafePSK(t *testing.T) {
	cfg := validConfig()
	client := cfg.Clients["demo-client"]
	client.PSK = "bad$psk"
	cfg.Clients["demo-client"] = client

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want unsafe PSK error")
	}
}

func TestValidateRejectsDuplicateStagePorts(t *testing.T) {
	cfg := validConfig()
	svc := cfg.Services["demo-service"]
	svc.Stage2Port = svc.Stage1Port
	cfg.Services["demo-service"] = svc

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want duplicate port error")
	}
}

func TestValidateRejectsInvalidTimeouts(t *testing.T) {
	cfg := validConfig()
	cfg.Defaults.AllowedTimeout = "1d"

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want invalid timeout error")
	}
}

func TestValidateRejectsUsedTimeoutShorterThanAcceptedBuckets(t *testing.T) {
	cfg := validConfig()
	cfg.Defaults.BucketSeconds = 30
	cfg.Defaults.UsedTimeout = "59s"

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want short used timeout error")
	}
}

func TestValidateRejectsUnsafeServiceName(t *testing.T) {
	cfg := validConfig()
	svc := cfg.Services["demo-service"]
	delete(cfg.Services, "demo-service")
	cfg.Services["bad name"] = svc
	client := cfg.Clients["demo-client"]
	client.Service = "bad name"
	cfg.Clients["demo-client"] = client

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want unsafe service name error")
	}
}

func TestValidateRejectsUnsafeAllowedList(t *testing.T) {
	cfg := validConfig()
	svc := cfg.Services["demo-service"]
	svc.AllowedList = "bad list"
	cfg.Services["demo-service"] = svc

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want unsafe allowed_list error")
	}
}

func TestApplyDefaultsPerServiceAllowedList(t *testing.T) {
	cfg := Config{
		Services: map[string]Service{
			"ssh-home": {Stage1Port: 1, Stage2Port: 2, TokenPort: 3},
		},
		Clients: map[string]Client{},
	}
	cfg.applyDefaults()

	if got := cfg.Services["ssh-home"].AllowedList; got != "mkpk-tt-allowed-ssh-home" {
		t.Fatalf("allowed_list = %q, want mkpk-tt-allowed-ssh-home", got)
	}
}

func TestValidateAcceptsTelegramNotify(t *testing.T) {
	cfg := validConfig()
	svc := cfg.Services["demo-service"]
	svc.Notify = Notify{
		Enabled:  true,
		Channel:  "telegram",
		Telegram: NotifyTelegram{BotToken: "123456:AA-bb_CC", ChatID: "-100200300"},
	}
	cfg.Services["demo-service"] = svc

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateRejectsBadTelegramToken(t *testing.T) {
	cfg := validConfig()
	svc := cfg.Services["demo-service"]
	svc.Notify = Notify{
		Enabled:  true,
		Channel:  "telegram",
		Telegram: NotifyTelegram{BotToken: "not-a-token", ChatID: "123"},
	}
	cfg.Services["demo-service"] = svc

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want bad telegram token error")
	}
}

func TestValidateRejectsWebhookWithoutURL(t *testing.T) {
	cfg := validConfig()
	svc := cfg.Services["demo-service"]
	svc.Notify = Notify{Enabled: true, Channel: "webhook", URL: ""}
	cfg.Services["demo-service"] = svc

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want missing webhook url error")
	}
}

func TestValidateRejectsUnknownNotifyChannel(t *testing.T) {
	cfg := validConfig()
	svc := cfg.Services["demo-service"]
	svc.Notify = Notify{Enabled: true, Channel: "carrier-pigeon", URL: "https://x"}
	cfg.Services["demo-service"] = svc

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want unknown channel error")
	}
}

func TestValidateAcceptsEmailNotify(t *testing.T) {
	cfg := validConfig()
	svc := cfg.Services["demo-service"]
	svc.Notify = Notify{
		Enabled: true,
		Channel: "email",
		Email: NotifyEmail{
			To: "alerts@example.com", From: "mkpk@example.com",
			Server: "smtp.example.com", Port: 587, TLS: "starttls",
		},
	}
	cfg.Services["demo-service"] = svc

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateRejectsEmailWithoutServer(t *testing.T) {
	cfg := validConfig()
	svc := cfg.Services["demo-service"]
	svc.Notify = Notify{
		Enabled: true,
		Channel: "email",
		Email:   NotifyEmail{To: "a@b.co", From: "m@b.co", Port: 587, TLS: "starttls"},
	}
	cfg.Services["demo-service"] = svc

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want missing email server error")
	}
}

func TestValidateRejectsEmailBadTLS(t *testing.T) {
	cfg := validConfig()
	svc := cfg.Services["demo-service"]
	svc.Notify = Notify{
		Enabled: true,
		Channel: "email",
		Email:   NotifyEmail{To: "a@b.co", From: "m@b.co", Server: "s", Port: 587, TLS: "ssl"},
	}
	cfg.Services["demo-service"] = svc

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want bad email tls error")
	}
}

func TestApplyDefaultsEmailPortAndTLS(t *testing.T) {
	cfg := Config{
		Services: map[string]Service{
			"s": {Stage1Port: 1, Stage2Port: 2, TokenPort: 3, Notify: Notify{Enabled: true, Channel: "email"}},
		},
		Clients: map[string]Client{},
	}
	cfg.applyDefaults()

	got := cfg.Services["s"].Notify.Email
	if got.Port != 587 || got.TLS != "starttls" {
		t.Fatalf("email defaults = port %d tls %q, want 587/starttls", got.Port, got.TLS)
	}
}

func validConfig() Config {
	return Config{
		Defaults: Defaults{
			BucketSeconds:   30,
			StageTimeout:    "5s",
			TokenHitTimeout: "2s",
			AllowedTimeout:  "3m",
			UsedTimeout:     "65s",
		},
		Services: map[string]Service{
			"demo-service": {
				ServiceName: "demo-service",
				Stage1Port:  41001,
				Stage2Port:  41002,
				TokenPort:   41003,
				AllowedList: "mkpk-tt-allowed",
				NAT: NAT{
					DstPort:   2222,
					ToAddress: "192.0.2.10",
					ToPort:    22,
				},
			},
		},
		Clients: map[string]Client{
			"demo-client": {
				ClientID: "demo-client",
				Service:  "demo-service",
				PSK:      "mkpk-prototype-psk",
			},
		},
	}
}
