package admin

import (
	"testing"

	"mikrotik-psk-knock/client/internal/config"
)

func fpConfig() config.Config {
	return config.Config{
		Routers: map[string]config.Router{
			"edge": {
				Address:  "router.example.com",
				Defaults: defaultDefaults(),
				Services: map[string]config.Service{
					"rdp": {
						ServiceName: "rdp",
						Stage1Port:  41001, Stage2Port: 41002, TokenPort: 41003,
						AllowedList: "mkpk-tt-allowed-rdp",
						Target:      config.Target{Type: config.TargetForward, Protocol: "tcp", Port: 60001, ToAddress: "192.0.2.10", ToPort: 3389},
					},
				},
			},
		},
		Users: map[string]config.User{
			"alice": {ClientID: "alice-laptop", Access: map[string]config.UserAccess{
				"edge": {Services: []string{"rdp"}, PSK: "synthetic-psk-alice"},
			}},
			"bob": {ClientID: "bob-laptop", Access: map[string]config.UserAccess{
				"edge": {Services: []string{"rdp"}, PSK: "synthetic-psk-bob"},
			}},
		},
	}
}

func TestAccessFingerprintIsPerUser(t *testing.T) {
	cfg := fpConfig()
	a, b := AccessFingerprint(cfg, "alice", "edge"), AccessFingerprint(cfg, "bob", "edge")
	if a == "" || b == "" {
		t.Fatalf("empty fingerprint: alice=%q bob=%q", a, b)
	}
	if a == b {
		t.Fatalf("two users share a fingerprint %q — the PSK is not in it", a)
	}
	if AccessFingerprint(cfg, "alice", "nope") != "" {
		t.Fatalf("fingerprint for a router the user cannot reach must be empty")
	}
}

// Rotating one user's PSK invalidates that user's invite and nobody else's.
func TestInvalidatedInvitesOnPSKRotation(t *testing.T) {
	before := fpConfig()
	after, err := RotateUserPSK(fpConfig(), "alice", "edge")
	if err != nil {
		t.Fatalf("RotateUserPSK() error = %v", err)
	}
	got := InvalidatedInvites(before, after)
	if len(got) != 1 || got[0] != "alice" {
		t.Fatalf("InvalidatedInvites() = %v, want [alice]", got)
	}
}

// A port change hits everyone who can reach that service.
func TestInvalidatedInvitesOnPortChange(t *testing.T) {
	before := fpConfig()
	after := fpConfig()
	svc := after.Routers["edge"].Services["rdp"]
	svc.TokenPort = 45999
	after.Routers["edge"].Services["rdp"] = svc

	got := InvalidatedInvites(before, after)
	if len(got) != 2 || got[0] != "alice" || got[1] != "bob" {
		t.Fatalf("InvalidatedInvites() = %v, want [alice bob]", got)
	}
}

// Cosmetic edits must stay quiet, or the warning becomes noise people click
// through without reading.
func TestInvalidatedInvitesIgnoresCosmeticEdits(t *testing.T) {
	before := fpConfig()
	after, err := SetNote(fpConfig(), "user", "", "alice", "just a note")
	if err != nil {
		t.Fatalf("SetNote() error = %v", err)
	}
	if got := InvalidatedInvites(before, after); len(got) != 0 {
		t.Fatalf("InvalidatedInvites() = %v after a note edit, want none", got)
	}
}

// Revoking access on purpose is not a "your invite broke" event.
func TestInvalidatedInvitesIgnoresRevokedAccess(t *testing.T) {
	before := fpConfig()
	after := fpConfig()
	delete(after.Users["alice"].Access, "edge")
	if got := InvalidatedInvites(before, after); len(got) != 0 {
		t.Fatalf("InvalidatedInvites() = %v after a revoke, want none", got)
	}
}
