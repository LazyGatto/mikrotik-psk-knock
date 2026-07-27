package invite

import "testing"

func sampleBlob() Blob {
	return Blob{
		Version: Version, Router: "router.example.com", BucketSeconds: 30,
		ClientID: "phone", PSK: "phone-psk-value",
		Services: []Service{
			{Name: "svca", Stage1: 41101, Stage2: 41102, Token: 41103, CheckPort: 2201},
			{Name: "svcb", Stage1: 41201, Stage2: 41202, Token: 41203, CheckPort: 2202},
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
	if got.Router != "router.example.com" || got.ClientID != "phone" || len(got.Services) != 2 {
		t.Fatalf("round trip mismatch: %+v", got)
	}
	if got.Services[1].Token != 41203 {
		t.Fatalf("service ports lost: %+v", got.Services[1])
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
	incomplete := Blob{Version: Version, Router: "r"}
	s, _ = Encode(incomplete)
	if _, err := Decode(s); err == nil {
		t.Fatal("Decode of incomplete blob should error")
	}
}

func TestToRouterResolves(t *testing.T) {
	r := sampleBlob().ToRouter()
	res, err := r.Resolve("invite", "phone", "svcb")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if res.Service.TokenPort != 41203 || res.Service.ServiceName != "svcb" {
		t.Fatalf("resolved wrong service: %+v", res.Service)
	}
	if res.Router.Defaults.BucketSeconds != 30 || res.Client.PSK != "phone-psk-value" {
		t.Fatalf("resolved wrong router/client: %+v %+v", res.Router.Defaults, res.Client)
	}
	// Ambiguous without a service name (two services).
	if _, err := r.Resolve("invite", "phone", ""); err == nil {
		t.Fatal("Resolve without service on multi-service blob should error")
	}
}
