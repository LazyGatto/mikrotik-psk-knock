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

	if !strings.Contains(rendered, `:local psk "mkpk-prototype-psk"`) {
		t.Fatalf("rendered script does not include expected PSK literal:\n%s", rendered)
	}
}

func TestRenderSingleClientOnlyRendersThatClient(t *testing.T) {
	rendered, err := Render(resolvedConfig(30))
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	if !strings.Contains(rendered, `add name="mkpk-tt-poller-demo-client"`) {
		t.Fatalf("rendered script missing poller for demo-client:\n%s", rendered)
	}
	if strings.Contains(rendered, "mkpk-tt-poller-alice") {
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

	// Per-client token rules, hit lists, pollers and schedulers.
	for _, want := range []string{
		`comment="mkpk-tt token now alice"`,
		`comment="mkpk-tt token prev bob"`,
		"address-list=mkpk-tt-hit-now-alice",
		"address-list=mkpk-tt-hit-prev-bob",
		`add name="mkpk-tt-poller-alice"`,
		`add name="mkpk-tt-poller-bob"`,
		`on-event="/system script run mkpk-tt-poller-alice"`,
		`on-event="/system script run mkpk-tt-poller-bob"`,
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered script missing %q:\n%s", want, rendered)
		}
	}

	// alice is on svc-a (token port 41003), bob on svc-b (token port 42003).
	if !strings.Contains(rendered, "src-address-list=mkpk-tt-stage2-svc-a content=\"mkpk-tt-token-not-initialized\" \\\n    address-list=mkpk-tt-hit-now-alice") {
		t.Fatalf("alice token rule not gated on svc-a stage2:\n%s", rendered)
	}

	// Per-service allowed-list isolation in NAT and pollers.
	for _, want := range []string{
		"src-address-list=mkpk-tt-allowed-svc-a",
		"src-address-list=mkpk-tt-allowed-svc-b",
		`:local allowedList "mkpk-tt-allowed-svc-a"`,
		`:local allowedList "mkpk-tt-allowed-svc-b"`,
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered script missing %q:\n%s", want, rendered)
		}
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
