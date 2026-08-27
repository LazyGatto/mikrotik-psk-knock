package invite

import "testing"

func fpBlob() Blob {
	return Blob{
		Version:  Version,
		ClientID: "alice-laptop",
		Routers: []Router{{
			Router:        "router.example.com",
			BucketSeconds: 30,
			PSK:           "synthetic-test-psk",
			Services: []Service{
				{Name: "rdp", Stage1: 1001, Stage2: 1002, Token: 1003, CheckPort: 60001, AllowedTimeout: "45m", Launch: "rdp"},
				{Name: "ssh", Stage1: 2001, Stage2: 2002, Token: 2003, CheckPort: 60002},
			},
		}},
	}
}

// Cosmetic fields must not move the fingerprint: a false "your invites are
// dead" warning is worse than no warning, because it teaches people to ignore
// the real one.
func TestFingerprintIgnoresCosmeticFields(t *testing.T) {
	base := Fingerprint(fpBlob())
	b := fpBlob()
	b.Routers[0].Services[0].AllowedTimeout = "10m"
	b.Routers[0].Services[0].Launch = "ssh"
	if got := Fingerprint(b); got != base {
		t.Fatalf("fingerprint changed on a cosmetic edit: %s → %s", base, got)
	}
}

// Everything a knock actually depends on must move it.
func TestFingerprintTracksKnockRelevantFields(t *testing.T) {
	base := Fingerprint(fpBlob())
	for name, mutate := range map[string]func(*Blob){
		"psk":        func(b *Blob) { b.Routers[0].PSK = "another-psk" },
		"address":    func(b *Blob) { b.Routers[0].Router = "other.example.com" },
		"bucket":     func(b *Blob) { b.Routers[0].BucketSeconds = 60 },
		"client_id":  func(b *Blob) { b.ClientID = "alice-desktop" },
		"stage1":     func(b *Blob) { b.Routers[0].Services[0].Stage1 = 9999 },
		"stage2":     func(b *Blob) { b.Routers[0].Services[0].Stage2 = 9999 },
		"token_port": func(b *Blob) { b.Routers[0].Services[0].Token = 9999 },
		"check_port": func(b *Blob) { b.Routers[0].Services[0].CheckPort = 9999 },
		"svc_name":   func(b *Blob) { b.Routers[0].Services[0].Name = "rdp2" },
		"svc_gone":   func(b *Blob) { b.Routers[0].Services = b.Routers[0].Services[:1] },
	} {
		b := fpBlob()
		mutate(&b)
		if got := Fingerprint(b); got == base {
			t.Fatalf("fingerprint did not change when %s changed", name)
		}
	}
}

// Service order is an artefact of map iteration on the admin side; it must not
// leak into the fingerprint.
func TestFingerprintStableAcrossServiceOrder(t *testing.T) {
	b := fpBlob()
	base := Fingerprint(b)
	b.Routers[0].Services[0], b.Routers[0].Services[1] = b.Routers[0].Services[1], b.Routers[0].Services[0]
	if got := Fingerprint(b); got != base {
		t.Fatalf("fingerprint depends on service order: %s → %s", base, got)
	}
}

func TestFingerprintLength(t *testing.T) {
	if got := len(Fingerprint(fpBlob())); got != FingerprintLen {
		t.Fatalf("fingerprint length = %d, want %d", got, FingerprintLen)
	}
	if got := len(RouterFingerprint("alice-laptop", fpBlob().Routers[0])); got != FingerprintLen {
		t.Fatalf("router fingerprint length = %d, want %d", got, FingerprintLen)
	}
}

// The PSK must not be reconstructible or visible in the output.
func TestFingerprintDoesNotLeakPSK(t *testing.T) {
	fp := Fingerprint(fpBlob())
	if len(fp) != FingerprintLen {
		t.Fatalf("unexpected fingerprint %q", fp)
	}
	for _, s := range []string{"synthetic", "psk", "alice"} {
		if contains(fp, s) {
			t.Fatalf("fingerprint %q contains %q", fp, s)
		}
	}
}

func contains(hay, needle string) bool {
	for i := 0; i+len(needle) <= len(hay); i++ {
		if hay[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
