package routeros

import (
	"strings"
	"testing"

	"mikrotik-psk-knock/client/internal/config"
)

func TestRenderUsesConfiguredBucketSeconds(t *testing.T) {
	rendered, err := RenderConfig(singleRouter(60))
	if err != nil {
		t.Fatalf("RenderConfig() error = %v", err)
	}
	if !strings.Contains(rendered, ":local nowBucket ([:timestamp] / 60s)") {
		t.Fatalf("rendered script does not use configured bucket seconds:\n%s", rendered)
	}
	if strings.Contains(rendered, ":local nowBucket ([:timestamp] / 30s)") {
		t.Fatalf("rendered script still contains hardcoded 30s bucket:\n%s", rendered)
	}
}

func TestRenderIncludesSafePSKLiteral(t *testing.T) {
	rendered, err := RenderConfig(singleRouter(30))
	if err != nil {
		t.Fatalf("RenderConfig() error = %v", err)
	}
	if !strings.Contains(rendered, `"psk"="mkpk-prototype-psk"`) {
		t.Fatalf("rendered script does not include expected PSK literal:\n%s", rendered)
	}
}

func TestRenderConfigMultiProfile(t *testing.T) {
	rendered, err := RenderConfig(multiRouter())
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

	// Per (user×service) token rules, hit lists and poller entries. Pair key is
	// <client>-<service>.
	for _, want := range []string{
		`comment="mkpk-tt token now alice-svc-a"`,
		`comment="mkpk-tt token prev bob-svc-b"`,
		"address-list=mkpk-tt-hit-now-alice-svc-a",
		"address-list=mkpk-tt-hit-prev-bob-svc-b",
		`"key"="alice-svc-a"`,
		`"key"="bob-svc-b"`,
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered script missing %q:\n%s", want, rendered)
		}
	}

	// Exactly one poller script and one poller scheduler.
	if strings.Contains(rendered, `add name="mkpk-tt-poller-`) {
		t.Fatalf("rendered script still uses per-client poller scripts:\n%s", rendered)
	}
	if got := strings.Count(rendered, `add name="mkpk-tt-poller" policy=read,write,test source={`); got != 1 {
		t.Fatalf("expected exactly one mkpk-tt-poller script, got %d:\n%s", got, rendered)
	}

	// alice on svc-a gated on svc-a's stage2; per-service allowed-list isolation.
	if !strings.Contains(rendered, "src-address-list=mkpk-tt-stage2-svc-a content=\"mkpk-tt-token-not-initialized\" \\\n    address-list=mkpk-tt-hit-now-alice-svc-a") {
		t.Fatalf("alice token rule not gated on svc-a stage2:\n%s", rendered)
	}
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

func TestRenderUserOnMultipleServices(t *testing.T) {
	r := multiRouter()
	// alice gets both services.
	a := r.Clients["alice"]
	a.Services = []string{"svc-a", "svc-b"}
	r.Clients["alice"] = a

	rendered, err := RenderConfig(r)
	if err != nil {
		t.Fatalf("RenderConfig() error = %v", err)
	}
	for _, want := range []string{
		`"key"="alice-svc-a"`,
		`"key"="alice-svc-b"`,
		"address-list=mkpk-tt-hit-now-alice-svc-a",
		"address-list=mkpk-tt-hit-now-alice-svc-b",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered script missing %q for multi-service user:\n%s", want, rendered)
		}
	}
}

func TestRenderSkipsDisabledService(t *testing.T) {
	r := multiRouter()
	svc := r.Services["svc-b"]
	svc.Disabled = true
	r.Services["svc-b"] = svc

	rendered, err := RenderConfig(r)
	if err != nil {
		t.Fatalf("RenderConfig() error = %v", err)
	}
	if strings.Contains(rendered, "mkpk-tt-stage1-svc-b") || strings.Contains(rendered, "bob-svc-b") {
		t.Fatalf("disabled service svc-b was rendered:\n%s", rendered)
	}
	if !strings.Contains(rendered, "mkpk-tt-stage1-svc-a") {
		t.Fatalf("enabled service svc-a missing:\n%s", rendered)
	}
}

func TestRenderStampsConfigHash(t *testing.T) {
	r := multiRouter()
	rendered, err := RenderConfig(r)
	if err != nil {
		t.Fatalf("RenderConfig() error = %v", err)
	}
	if !strings.Contains(rendered, `add name="mkpk-tt-meta"`) {
		t.Fatalf("rendered script missing mkpk-tt-meta marker:\n%s", rendered)
	}
	if !strings.Contains(rendered, "mkpk-config-hash="+r.Hash()) {
		t.Fatalf("rendered script does not stamp config hash %s:\n%s", r.Hash(), rendered)
	}
}

func TestRenderConfigDeterministicOrdering(t *testing.T) {
	a, err := RenderConfig(multiRouter())
	if err != nil {
		t.Fatalf("RenderConfig() error = %v", err)
	}
	b, err := RenderConfig(multiRouter())
	if err != nil {
		t.Fatalf("RenderConfig() error = %v", err)
	}
	if a != b {
		t.Fatal("RenderConfig() output is not deterministic across runs")
	}
}

func TestRenderNotifyUsesJSONPayload(t *testing.T) {
	rendered, err := RenderConfig(multiRouter())
	if err != nil {
		t.Fatalf("RenderConfig() error = %v", err)
	}
	if !strings.Contains(rendered, `[:serialize {"router"=$mkpkTtNotifyRouter`) {
		t.Fatalf("notify payload is not built via :serialize:\n%s", rendered)
	}
	if !strings.Contains(rendered, `"client_id"=$mkpkTtNotifyClientId`) {
		t.Fatalf("notify client_id key is not quoted:\n%s", rendered)
	}
	if !strings.Contains(rendered, `http-header-field="Content-Type: application/json"`) {
		t.Fatalf("notify fetch does not set json content-type:\n%s", rendered)
	}
}

func TestRenderNotifyTelegramChannel(t *testing.T) {
	r := config.Router{
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
			"phone": {ClientID: "phone", Services: []string{"tg"}, PSK: "phone-psk"},
		},
	}
	rendered, err := RenderConfig(r)
	if err != nil {
		t.Fatalf("RenderConfig() error = %v", err)
	}
	for _, want := range []string{
		`:if ($mkpkTtNotifyChannel = "telegram")`,
		`("https://api.telegram.org/bot" . $mkpkTtNotifyBotToken . "/sendMessage")`,
		`"notifyChannel"="telegram"`,
		`"notifyBotToken"="123456:AA-bb_CC"`,
		`"notifyChatId"="-100200300"`,
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered script missing %q:\n%s", want, rendered)
		}
	}
}

func TestRenderNotifyEmailChannel(t *testing.T) {
	r := config.Router{
		Defaults: config.Defaults{BucketSeconds: 30, StageTimeout: "5s", TokenHitTimeout: "2s", AllowedTimeout: "3m", UsedTimeout: "65s"},
		Services: map[string]config.Service{
			"mail": {
				ServiceName: "mail", Stage1Port: 41001, Stage2Port: 41002, TokenPort: 41003,
				AllowedList: "mkpk-tt-allowed-mail",
				NAT:         config.NAT{Comment: "mkpk-tt dst-nat mail", DstPort: 2222, ToAddress: "192.0.2.10", ToPort: 22},
				Notify: config.Notify{
					Enabled: true, Channel: "email",
					Email: config.NotifyEmail{To: "alerts@example.com", From: "mkpk@example.com", Server: "smtp.example.com", Port: 587, TLS: "starttls", User: "u", Password: "p"},
				},
			},
		},
		Clients: map[string]config.Client{
			"phone": {ClientID: "phone", Services: []string{"mail"}, PSK: "phone-psk"},
		},
	}
	rendered, err := RenderConfig(r)
	if err != nil {
		t.Fatalf("RenderConfig() error = %v", err)
	}
	for _, want := range []string{
		`:if ($mkpkTtNotifyChannel = "email")`,
		`/tool e-mail send to=$mkpkTtNotifyEmailTo from=$mkpkTtNotifyEmailFrom server=$mkpkTtNotifyEmailServer`,
		`"emailServer"="smtp.example.com"`,
		`"emailPort"=587`,
		`"emailTls"="starttls"`,
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered script missing %q:\n%s", want, rendered)
		}
	}
}

func singleRouter(bucketSeconds int64) config.Router {
	return config.Router{
		Address:  "router.example",
		Defaults: config.Defaults{BucketSeconds: bucketSeconds, StageTimeout: "5s", TokenHitTimeout: "2s", AllowedTimeout: "3m", UsedTimeout: "65s"},
		Services: map[string]config.Service{
			"demo-service": {
				ServiceName: "demo-service", Stage1Port: 41001, Stage2Port: 41002, TokenPort: 41003,
				AllowedList: "mkpk-tt-allowed-demo-service",
				NAT:         config.NAT{Comment: "mkpk-tt dst-nat demo-service", DstPort: 2222, ToAddress: "192.0.2.10", ToPort: 22},
			},
		},
		Clients: map[string]config.Client{
			"demo-client": {ClientID: "demo-client", Services: []string{"demo-service"}, PSK: "mkpk-prototype-psk"},
		},
	}
}

func multiRouter() config.Router {
	return config.Router{
		Address:  "router.example",
		Defaults: config.Defaults{BucketSeconds: 30, StageTimeout: "5s", TokenHitTimeout: "2s", AllowedTimeout: "3m", UsedTimeout: "65s"},
		Services: map[string]config.Service{
			"svc-a": {
				ServiceName: "svc-a", Stage1Port: 41001, Stage2Port: 41002, TokenPort: 41003,
				AllowedList: "mkpk-tt-allowed-svc-a",
				NAT:         config.NAT{Comment: "mkpk-tt dst-nat svc-a", DstPort: 2222, ToAddress: "192.0.2.10", ToPort: 22},
			},
			"svc-b": {
				ServiceName: "svc-b", Stage1Port: 42001, Stage2Port: 42002, TokenPort: 42003,
				AllowedList: "mkpk-tt-allowed-svc-b",
				NAT:         config.NAT{Comment: "mkpk-tt dst-nat svc-b", DstPort: 3333, ToAddress: "192.0.2.20", ToPort: 443},
			},
		},
		Clients: map[string]config.Client{
			"alice": {ClientID: "alice", Services: []string{"svc-a"}, PSK: "alice-psk"},
			"bob":   {ClientID: "bob", Services: []string{"svc-b"}, PSK: "bob-psk"},
		},
	}
}
