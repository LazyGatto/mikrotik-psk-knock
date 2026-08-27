package routeros

import (
	"regexp"
	"strings"
	"testing"

	"mikrotik-psk-knock/client/internal/config"
)

func TestRenderUsesConfiguredBucketSeconds(t *testing.T) {
	rendered, err := RenderConfig(singleRouter(60), singleClients())
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
	rendered, err := RenderConfig(singleRouter(30), singleClients())
	if err != nil {
		t.Fatalf("RenderConfig() error = %v", err)
	}
	if !strings.Contains(rendered, `"psk"="mkpk-prototype-psk"`) {
		t.Fatalf("rendered script does not include expected PSK literal:\n%s", rendered)
	}
}

func TestRenderConfigMultiProfile(t *testing.T) {
	rendered, err := RenderConfig(multiRouter(), multiClients())
	if err != nil {
		t.Fatalf("RenderConfig() error = %v", err)
	}

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

	if strings.Contains(rendered, `add name="mkpk-tt-poller-`) {
		t.Fatalf("rendered script still uses per-client poller scripts:\n%s", rendered)
	}
	if got := strings.Count(rendered, `add name="mkpk-tt-poller" policy=read,write,test source={`); got != 1 {
		t.Fatalf("expected exactly one mkpk-tt-poller script, got %d:\n%s", got, rendered)
	}

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
	clients := multiClients()
	// alice gets both services.
	clients[0].Services = []string{"svc-a", "svc-b"}

	rendered, err := RenderConfig(multiRouter(), clients)
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

// A router with services but no users granted access used to render
// `:set mkpkTtClients { }` — an empty code block, which RouterOS rejects with a
// syntax error that aborts the whole /import.
func TestRenderWithoutClientsEmitsEmptyArray(t *testing.T) {
	rendered, err := RenderConfig(multiRouter(), nil)
	if err != nil {
		t.Fatalf("RenderConfig() error = %v", err)
	}
	if !strings.Contains(rendered, `:set mkpkTtClients [:toarray ""]`) {
		t.Fatalf("clientless render does not use the empty-array idiom:\n%s", rendered)
	}
	if strings.Contains(rendered, ":set mkpkTtClients {") {
		t.Fatalf("clientless render still emits an empty code block:\n%s", rendered)
	}
	// The rest of the router must still be there — the services exist, only
	// nobody may knock yet.
	if !strings.Contains(rendered, "mkpk-tt-stage1-svc-a") {
		t.Fatalf("services missing from a clientless render:\n%s", rendered)
	}
}

// Same class of bug one step further: a router with no enabled services at all
// would render `source={ }` for the apply script. An always-present `:return 0`
// keeps every generated block non-empty.
func TestRenderWithoutServicesHasNoEmptyBlocks(t *testing.T) {
	r := multiRouter()
	r.Services = nil
	rendered, err := RenderConfig(r, nil)
	if err != nil {
		t.Fatalf("RenderConfig() error = %v", err)
	}
	if m := regexp.MustCompile(`(?m)=\{[ \t]*\n[ \t]*\}`).FindAllString(rendered, -1); len(m) > 0 {
		t.Fatalf("rendered script contains empty RouterOS blocks %q:\n%s", m, rendered)
	}
}

func TestRenderSkipsDisabledService(t *testing.T) {
	r := multiRouter()
	svc := r.Services["svc-b"]
	svc.Disabled = true
	r.Services["svc-b"] = svc

	rendered, err := RenderConfig(r, multiClients())
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
	clients := multiClients()
	rendered, err := RenderConfig(r, clients)
	if err != nil {
		t.Fatalf("RenderConfig() error = %v", err)
	}
	if !strings.Contains(rendered, `add name="mkpk-tt-meta"`) {
		t.Fatalf("rendered script missing mkpk-tt-meta marker:\n%s", rendered)
	}
	if !strings.Contains(rendered, "mkpk-config-hash="+config.RenderHash(r, clients)) {
		t.Fatalf("rendered script does not stamp the render hash:\n%s", rendered)
	}
}

func TestRenderConfigDeterministicOrdering(t *testing.T) {
	a, err := RenderConfig(multiRouter(), multiClients())
	if err != nil {
		t.Fatalf("RenderConfig() error = %v", err)
	}
	b, err := RenderConfig(multiRouter(), multiClients())
	if err != nil {
		t.Fatalf("RenderConfig() error = %v", err)
	}
	if a != b {
		t.Fatal("RenderConfig() output is not deterministic across runs")
	}
}

func TestRenderNotifyUsesJSONPayload(t *testing.T) {
	rendered, err := RenderConfig(multiRouter(), multiClients())
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
	r := multiRouter()
	r.Notify = config.Notify{
		Telegram: config.NotifyTelegram{Enabled: true, BotToken: "123456:AA-bb_CC", ChatID: "-100200300"},
	}
	rendered, err := RenderConfig(r, multiClients())
	if err != nil {
		t.Fatalf("RenderConfig() error = %v", err)
	}
	for _, want := range []string{
		`:if (($nTelegram = true)`,
		`("https://api.telegram.org/bot" . $nBotToken . "/sendMessage")`,
		`:local nTelegram true`,
		`:local nBotToken "123456:AA-bb_CC"`,
		`:local nChatId "-100200300"`,
		`/system script run mkpk-tt-notify`, // wired in processHits when active
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered script missing %q:\n%s", want, rendered)
		}
	}
}

func TestRenderNotifyEmailChannel(t *testing.T) {
	r := multiRouter()
	r.Notify = config.Notify{
		Email: config.NotifyEmail{Enabled: true, To: "alerts@example.com", From: "mkpk@example.com", Server: "smtp.example.com", Port: 587, TLS: "starttls", User: "u", Password: "p"},
	}
	rendered, err := RenderConfig(r, multiClients())
	if err != nil {
		t.Fatalf("RenderConfig() error = %v", err)
	}
	for _, want := range []string{
		`:if (($nEmail = true)`,
		`/tool e-mail send to=$nEmailTo from=$nEmailFrom server=$nEmailServer`,
		`:local nEmailServer "smtp.example.com"`,
		`:local nEmailPort 587`,
		`:local nEmailTls "starttls"`,
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered script missing %q:\n%s", want, rendered)
		}
	}
}

func TestRenderDisabledNotifySkipsRun(t *testing.T) {
	// multiRouter has no notify; the poller must not wire the notify run.
	rendered, err := RenderConfig(multiRouter(), multiClients())
	if err != nil {
		t.Fatalf("RenderConfig() error = %v", err)
	}
	if strings.Contains(rendered, `/system script run mkpk-tt-notify`) {
		t.Fatalf("notify run should be omitted when notify is disabled:\n%s", rendered)
	}
	if !strings.Contains(rendered, `:local nWebhook false`) {
		t.Fatalf("notify script should bake nWebhook false:\n%s", rendered)
	}
}

func TestRenderForwardTargetEmitsNatAndForwardAccept(t *testing.T) {
	rendered, err := RenderConfig(multiRouter(), multiClients())
	if err != nil {
		t.Fatalf("RenderConfig() error = %v", err)
	}
	for _, want := range []string{
		`/ip firewall nat add chain=dstnat action=dst-nat protocol=tcp dst-port=2222`,
		`to-addresses="192.0.2.10" to-ports=22`,
		`/ip firewall filter add chain=forward action=accept protocol=tcp dst-address="192.0.2.10" dst-port=22`,
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("forward target missing %q:\n%s", want, rendered)
		}
	}
}

func TestRenderLocalTargetEmitsInputAccept(t *testing.T) {
	r := multiRouter()
	svc := r.Services["svc-a"]
	svc.Target = config.Target{Type: config.TargetLocal, Protocol: "tcp", Port: 8291, Comment: "mkpk-tt target svc-a"}
	r.Services["svc-a"] = svc

	rendered, err := RenderConfig(r, multiClients())
	if err != nil {
		t.Fatalf("RenderConfig() error = %v", err)
	}
	if !strings.Contains(rendered, `/ip firewall filter add chain=input action=accept protocol=tcp dst-port=8291`) {
		t.Fatalf("local target missing input accept:\n%s", rendered)
	}
	if strings.Contains(rendered, `dst-nat protocol=tcp dst-port=8291`) {
		t.Fatalf("local target should not produce a dst-nat rule:\n%s", rendered)
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
				Target:      config.Target{Type: config.TargetForward, Protocol: "tcp", Comment: "mkpk-tt target demo-service", Port: 2222, ToAddress: "192.0.2.10", ToPort: 22},
			},
		},
	}
}

func singleClients() []config.RenderClient {
	return []config.RenderClient{{Name: "demo-client", ClientID: "demo-client", PSK: "mkpk-prototype-psk", Services: []string{"demo-service"}}}
}

func multiRouter() config.Router {
	return config.Router{
		Address:  "router.example",
		Defaults: config.Defaults{BucketSeconds: 30, StageTimeout: "5s", TokenHitTimeout: "2s", AllowedTimeout: "3m", UsedTimeout: "65s"},
		Services: map[string]config.Service{
			"svc-a": {
				ServiceName: "svc-a", Stage1Port: 41001, Stage2Port: 41002, TokenPort: 41003,
				AllowedList: "mkpk-tt-allowed-svc-a",
				Target:      config.Target{Type: config.TargetForward, Protocol: "tcp", Comment: "mkpk-tt target svc-a", Port: 2222, ToAddress: "192.0.2.10", ToPort: 22},
			},
			"svc-b": {
				ServiceName: "svc-b", Stage1Port: 42001, Stage2Port: 42002, TokenPort: 42003,
				AllowedList: "mkpk-tt-allowed-svc-b",
				Target:      config.Target{Type: config.TargetForward, Protocol: "tcp", Comment: "mkpk-tt target svc-b", Port: 3333, ToAddress: "192.0.2.20", ToPort: 443},
			},
		},
	}
}

func multiClients() []config.RenderClient {
	return []config.RenderClient{
		{Name: "alice", ClientID: "alice", PSK: "alice-psk", Services: []string{"svc-a"}},
		{Name: "bob", ClientID: "bob", PSK: "bob-psk", Services: []string{"svc-b"}},
	}
}
