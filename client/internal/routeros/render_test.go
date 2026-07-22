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

	if !strings.Contains(rendered, `:global mkpkTtPsk "mkpk-prototype-psk"`) {
		t.Fatalf("rendered script does not include expected PSK literal:\n%s", rendered)
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
			UsedTimeout:     "35s",
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
