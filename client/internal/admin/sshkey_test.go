package admin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestInstanceKeyLifecycle(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "mkpk.yaml")

	// Missing key is a state, not an error.
	info, err := ReadInstanceKey(cfg)
	if err != nil || info.Exists {
		t.Fatalf("fresh install: exists=%v err=%v", info.Exists, err)
	}

	created, err := CreateInstanceKey(cfg, "mkpk-provision", false)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if !strings.HasPrefix(created.PublicKey, "ssh-ed25519 ") {
		t.Fatalf("public key = %q, want an ssh-ed25519 authorized_keys line", created.PublicKey)
	}
	if !strings.HasPrefix(created.Fingerprint, "SHA256:") {
		t.Fatalf("fingerprint = %q", created.Fingerprint)
	}

	// The private key must be usable by an SSH client and locked down.
	raw, err := os.ReadFile(created.Path)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.ParsePrivateKey(raw)
	if err != nil {
		t.Fatalf("generated key does not parse as an SSH private key: %v", err)
	}
	if got := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signer.PublicKey()))); !strings.HasPrefix(created.PublicKey, got) {
		t.Fatal("public key does not match the private key")
	}
	fi, err := os.Stat(created.Path)
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("key mode = %v (err %v), want 0600", fi.Mode().Perm(), err)
	}

	// Reading it back reports the same key.
	info, err = ReadInstanceKey(cfg)
	if err != nil || !info.Exists || info.Fingerprint != created.Fingerprint {
		t.Fatalf("read back = %+v, err %v", info, err)
	}

	// Regenerating is refused unless explicitly requested: it would lock out
	// every router that already trusts the old key.
	if _, err := CreateInstanceKey(cfg, "", false); err == nil {
		t.Fatal("overwrote an existing key without replace")
	}
	replaced, err := CreateInstanceKey(cfg, "", true)
	if err != nil {
		t.Fatalf("replace: %v", err)
	}
	if replaced.Fingerprint == created.Fingerprint {
		t.Fatal("replace produced the same key")
	}
}
