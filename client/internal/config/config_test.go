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

func validConfig() Config {
	return Config{
		Defaults: Defaults{
			BucketSeconds:   30,
			StageTimeout:    "5s",
			TokenHitTimeout: "2s",
			AllowedTimeout:  "3m",
			UsedTimeout:     "35s",
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
