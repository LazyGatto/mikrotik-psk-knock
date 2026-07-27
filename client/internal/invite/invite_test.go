package invite

import "testing"

func sampleBlob() Blob {
	return Blob{
		Version:  Version,
		ClientID: "phone",
		Routers: []Router{
			{
				Router: "router.example.com", BucketSeconds: 30, PSK: "phone-psk-kz",
				Services: []Service{
					{Name: "svca", Stage1: 41101, Stage2: 41102, Token: 41103, CheckPort: 2201},
					{Name: "svcb", Stage1: 41201, Stage2: 41202, Token: 41203, CheckPort: 2202},
				},
			},
			{
				Router: "home.example", BucketSeconds: 30, PSK: "phone-psk-home",
				Services: []Service{
					{Name: "ssh", Stage1: 42101, Stage2: 42102, Token: 42103, CheckPort: 22},
				},
			},
		},
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	s, err := Encode(sampleBlob())
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	got, err := Decode(s)
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if got.ClientID != "phone" || len(got.Routers) != 2 {
		t.Fatalf("round trip mismatch: %+v", got)
	}
	if got.Routers[0].Services[1].Token != 41203 {
		t.Fatalf("service ports lost: %+v", got.Routers[0].Services[1])
	}
	if got.Routers[1].PSK != "phone-psk-home" {
		t.Fatalf("per-router psk lost: %+v", got.Routers[1])
	}
}

func TestDecodeRejectsBad(t *testing.T) {
	if _, err := Decode("!!!not-base64!!!"); err == nil {
		t.Fatal("Decode of bad base64 should error")
	}
	bad := sampleBlob()
	bad.Version = 99
	s, _ := Encode(bad)
	if _, err := Decode(s); err == nil {
		t.Fatal("Decode of wrong version should error")
	}
	incomplete := Blob{Version: Version, ClientID: "phone"}
	s, _ = Encode(incomplete)
	if _, err := Decode(s); err == nil {
		t.Fatal("Decode of a blob without routers should error")
	}
}

func TestToConfigResolves(t *testing.T) {
	cfg := sampleBlob().ToConfig()
	res, err := cfg.Resolve("phone", "router.example.com", "svcb")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if res.Service.TokenPort != 41203 || res.Service.ServiceName != "svcb" {
		t.Fatalf("resolved wrong service: %+v", res.Service)
	}
	if res.Router.Defaults.BucketSeconds != 30 || res.PSK != "phone-psk-kz" {
		t.Fatalf("resolved wrong router/psk: %+v %q", res.Router.Defaults, res.PSK)
	}
	// The second router carries its own PSK.
	res2, err := cfg.Resolve("phone", "home.example", "ssh")
	if err != nil {
		t.Fatalf("Resolve() on second router error = %v", err)
	}
	if res2.PSK != "phone-psk-home" {
		t.Fatalf("second router psk wrong: %q", res2.PSK)
	}
	// Ambiguous without a service name (two services on kz).
	if _, err := cfg.Resolve("phone", "router.example.com", ""); err == nil {
		t.Fatal("Resolve without service on a multi-service router should error")
	}
}
