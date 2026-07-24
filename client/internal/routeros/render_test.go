package routeros

import (
	"strings"
	"testing"

	"mikrotik-psk-knock/client/internal/config"
)

func TestRenderUsesConfiguredBucketSeconds(t *testing.T) {
	rendered, err := Render(resolvedConfig(60))
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	if !strings.Contains(rendered, ":local nowBucket ([:timestamp] / 60s)") {
		t.Fatalf("rendered script does not use configured bucket seconds:\n%s", rendered)
	}
	if strings.Contains(rendered, ":local nowBucket ([:timestamp] / 30s)") {
		t.Fatalf("rendered script still contains hardcoded 30s bucket:\n%s", rendered)
	}
}

func TestRenderIncludesSafePSKLiteral(t *testing.T) {
	rendered, err := Render(resolvedConfig(30))
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	if !strings.Contains(rendered, `"psk"="mkpk-prototype-psk"`) {
		t.Fatalf("rendered script does not include expected PSK literal:\n%s", rendered)
	}
}

func TestRenderSingleClientOnlyRendersThatClient(t *testing.T) {
	rendered, err := Render(resolvedConfig(30))
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	// One shared data-driven poller, not per-client scripts.
	if !strings.Contains(rendered, `add name="mkpk-tt-poller"`) {
		t.Fatalf("rendered script missing shared poller:\n%s", rendered)
	}
	if strings.Contains(rendered, `add name="mkpk-tt-poller-`) {
		t.Fatalf("rendered script still uses per-client poller scripts:\n%s", rendered)
	}
	if !strings.Contains(rendered, `"key"="demo-client"`) {
		t.Fatalf("rendered client array missing demo-client:\n%s", rendered)
	}
	if strings.Contains(rendered, `"key"="alice"`) {
		t.Fatalf("single-client render leaked another client:\n%s", rendered)
	}
}

func TestRenderConfigMultiProfile(t *testing.T) {
	rendered, err := RenderConfig(multiConfig())
	if err != nil {
		t.Fatalf("RenderConfig() error = %v", err)
	}

	// Per-service stage lists.
	for _, want := range []string{
		"address-list=mkpk-tt-stage1-svc-a",
		"address-list=mkpk-tt-stage2-svc-a",
		"address-list=mkpk-tt-stage1-svc-b",
		"address-list=mkpk-tt-stage2-svc-b",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered script missing %q:\n%s", want, rendered)
		}
	}

	// Per-client token rules and hit lists, plus per-client entries in the
	// single data-driven poller's client array.
	for _, want := range []string{
		`comment="mkpk-tt token now alice"`,
		`comment="mkpk-tt token prev bob"`,
		"address-list=mkpk-tt-hit-now-alice",
		"address-list=mkpk-tt-hit-prev-bob",
		`"key"="alice"`,
		`"key"="bob"`,
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered script missing %q:\n%s", want, rendered)
		}
	}

	// Exactly one poller script and one poller scheduler (no per-client pollers).
	if strings.Contains(rendered, `add name="mkpk-tt-poller-`) {
		t.Fatalf("rendered script still uses per-client poller scripts:\n%s", rendered)
	}
	if got := strings.Count(rendered, `add name="mkpk-tt-poller" policy=read,write,test source={`); got != 1 {
		t.Fatalf("expected exactly one mkpk-tt-poller script, got %d:\n%s", got, rendered)
	}
	if got := strings.Count(rendered, `add name="mkpk-tt-poller" interval=1s`); got != 1 {
		t.Fatalf("expected exactly one mkpk-tt-poller scheduler, got %d:\n%s", got, rendered)
	}

	// alice is on svc-a (token port 41003), bob on svc-b (token port 42003).
	if !strings.Contains(rendered, "src-address-list=mkpk-tt-stage2-svc-a content=\"mkpk-tt-token-not-initialized\" \\\n    address-list=mkpk-tt-hit-now-alice") {
		t.Fatalf("alice token rule not gated on svc-a stage2:\n%s", rendered)
	}

	// Per-service allowed-list isolation in NAT and client array.
	for _, want := range []string{
		"src-address-list=mkpk-tt-allowed-svc-a",
		"src-address-list=mkpk-tt-allowed-svc-b",
		`"allowedList"="mkpk-tt-allowed-svc-a"`,
		`"allowedList"="mkpk-tt-allowed-svc-b"`,
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered script missing %q:\n%s", want, rendered)
		}
	}
}

func TestRenderStampsConfigHash(t *testing.T) {
	cfg := multiConfig()
	rendered, err := RenderConfig(cfg)
	if err != nil {
		t.Fatalf("RenderConfig() error = %v", err)
	}
	if !strings.Contains(rendered, `add name="mkpk-tt-meta"`) {
		t.Fatalf("rendered script missing mkpk-tt-meta marker:\n%s", rendered)
	}
	if !strings.Contains(rendered, "mkpk-config-hash="+cfg.Hash()) {
		t.Fatalf("rendered script does not stamp config hash %s:\n%s", cfg.Hash(), rendered)
	}
}

func TestRenderConfigDeterministicOrdering(t *testing.T) {
	a, err := RenderConfig(multiConfig())
	if err != nil {
		t.Fatalf("RenderConfig() error = %v", err)
	}
	b, err := RenderConfig(multiConfig())
	if err != nil {
		t.Fatalf("RenderConfig() error = %v", err)
	}
	if a != b {
		t.Fatal("RenderConfig() output is not deterministic across runs")
	}
}

func TestRenderNotifyUsesJSONPayload(t *testing.T) {
	rendered, err := RenderConfig(multiConfig())
	if err != nil {
		t.Fatalf("RenderConfig() error = %v", err)
	}

	if !strings.Contains(rendered, `[:serialize {"router"=$mkpkTtNotifyRouter`) {
		t.Fatalf("notify payload is not built via :serialize:\n%s", rendered)
	}
	// Keys must be quoted; bare underscore/camelCase keys break RouterOS array literals.
	if !strings.Contains(rendered, `"client_id"=$mkpkTtNotifyClientId`) {
		t.Fatalf("notify client_id key is not quoted:\n%s", rendered)
	}
	if !strings.Contains(rendered, "to=json]") {
		t.Fatalf("notify payload is not serialized to json:\n%s", rendered)
	}
	if !strings.Contains(rendered, `http-header-field="Content-Type: application/json"`) {
		t.Fatalf("notify fetch does not set json content-type:\n%s", rendered)
	}
	if strings.Contains(rendered, `"&service=" . $mkpkTtNotifyService`) {
		t.Fatalf("notify still uses raw form-encoded concatenation:\n%s", rendered)
	}
}

func TestRenderNotifyTelegramChannel(t *testing.T) {
	cfg := config.Config{
		Defaults: config.Defaults{BucketSeconds: 30, StageTimeout: "5s", TokenHitTimeout: "2s", AllowedTimeout: "3m", UsedTimeout: "65s"},
		Services: map[string]config.Service{
			"tg": {
				ServiceName: "tg", Stage1Port: 41001, Stage2Port: 41002, TokenPort: 41003,
				AllowedList: "mkpk-tt-allowed-tg",
				NAT:         config.NAT{Comment: "mkpk-tt dst-nat tg", DstPort: 2222, ToAddress: "192.0.2.10", ToPort: 22},
				Notify: config.Notify{
					Enabled: true, Channel: "telegram",
					Telegram: config.NotifyTelegram{BotToken: "123456:AA-bb_CC", ChatID: "-100200300"},
				},
			},
		},
		Clients: map[string]config.Client{
			"phone": {ClientID: "phone", Service: "tg", PSK: "phone-psk"},
		},
	}

	rendered, err := RenderConfig(cfg)
	if err != nil {
		t.Fatalf("RenderConfig() error = %v", err)
	}

	for _, want := range []string{
		`:if ($mkpkTtNotifyChannel = "telegram")`,
		`("https://api.telegram.org/bot" . $mkpkTtNotifyBotToken . "/sendMessage")`,
		`("{\"chat_id\":\"" . $mkpkTtNotifyChatId . "\",\"text\":" . [:serialize $text to=json] . "}")`,
		`"notifyChannel"="telegram"`,
		`"notifyBotToken"="123456:AA-bb_CC"`,
		`"notifyChatId"="-100200300"`,
		`:global mkpkTtNotifyChannel ($c->"notifyChannel")`,
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered script missing %q:\n%s", want, rendered)
		}
	}
}

func TestRenderNotifyEmailChannel(t *testing.T) {
	cfg := config.Config{
		Defaults: config.Defaults{BucketSeconds: 30, StageTimeout: "5s", TokenHitTimeout: "2s", AllowedTimeout: "3m", UsedTimeout: "65s"},
		Services: map[string]config.Service{
			"mail": {
				ServiceName: "mail", Stage1Port: 41001, Stage2Port: 41002, TokenPort: 41003,
				AllowedList: "mkpk-tt-allowed-mail",
				NAT:         config.NAT{Comment: "mkpk-tt dst-nat mail", DstPort: 2222, ToAddress: "192.0.2.10", ToPort: 22},
				Notify: config.Notify{
					Enabled: true, Channel: "email",
					Email: config.NotifyEmail{
						To: "alerts@example.com", From: "mkpk@example.com",
						Server: "smtp.example.com", Port: 587, TLS: "starttls", User: "u", Password: "p",
					},
				},
			},
		},
		Clients: map[string]config.Client{
			"phone": {ClientID: "phone", Service: "mail", PSK: "phone-psk"},
		},
	}

	rendered, err := RenderConfig(cfg)
	if err != nil {
		t.Fatalf("RenderConfig() error = %v", err)
	}

	for _, want := range []string{
		`:if ($mkpkTtNotifyChannel = "email")`,
		`/tool e-mail send to=$mkpkTtNotifyEmailTo from=$mkpkTtNotifyEmailFrom server=$mkpkTtNotifyEmailServer`,
		`"emailServer"="smtp.example.com"`,
		`"emailPort"=587`,
		`"emailTls"="starttls"`,
		`:global mkpkTtNotifyEmailServer ($c->"emailServer")`,
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered script missing %q:\n%s", want, rendered)
		}
	}
}

func resolvedConfig(bucketSeconds int64) config.Resolved {
	cfg := config.Config{
		Router: config.Router{Name: "test-router", Address: "router.example"},
		Defaults: config.Defaults{
			BucketSeconds:   bucketSeconds,
			StageTimeout:    "5s",
			TokenHitTimeout: "2s",
			AllowedTimeout:  "3m",
			UsedTimeout:     "65s",
		},
	}
	svc := config.Service{
		ServiceName: "demo-service",
		Stage1Port:  41001,
		Stage2Port:  41002,
		TokenPort:   41003,
		AllowedList: "mkpk-tt-allowed",
		NAT: config.NAT{
			Comment:   "mkpk-tt dst-nat demo ssh",
			DstPort:   2222,
			ToAddress: "192.0.2.10",
			ToPort:    22,
		},
	}
	client := config.Client{
		ClientID: "demo-client",
		Service:  "demo-service",
		PSK:      "mkpk-prototype-psk",
	}
	return config.Resolved{
		Config:  cfg,
		Service: svc,
		Client:  client,
	}
}

func multiConfig() config.Config {
	return config.Config{
		Router: config.Router{Name: "test-router", Address: "router.example"},
		Defaults: config.Defaults{
			BucketSeconds:   30,
			StageTimeout:    "5s",
			TokenHitTimeout: "2s",
			AllowedTimeout:  "3m",
			UsedTimeout:     "65s",
		},
		Services: map[string]config.Service{
			"svc-a": {
				ServiceName: "svc-a",
				Stage1Port:  41001,
				Stage2Port:  41002,
				TokenPort:   41003,
				AllowedList: "mkpk-tt-allowed-svc-a",
				NAT:         config.NAT{Comment: "mkpk-tt dst-nat svc-a", DstPort: 2222, ToAddress: "192.0.2.10", ToPort: 22},
			},
			"svc-b": {
				ServiceName: "svc-b",
				Stage1Port:  42001,
				Stage2Port:  42002,
				TokenPort:   42003,
				AllowedList: "mkpk-tt-allowed-svc-b",
				NAT:         config.NAT{Comment: "mkpk-tt dst-nat svc-b", DstPort: 3333, ToAddress: "192.0.2.20", ToPort: 443},
			},
		},
		Clients: map[string]config.Client{
			"alice": {ClientID: "alice", Service: "svc-a", PSK: "alice-psk"},
			"bob":   {ClientID: "bob", Service: "svc-b", PSK: "bob-psk"},
		},
	}
}
