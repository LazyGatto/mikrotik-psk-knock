package admin

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"
)

// The instance SSH key: one deploy identity belonging to the provision
// installation rather than to whoever set it up. A shared instance in Docker has
// no personal ~/.ssh to borrow, and pinning deploys to one admin's key means
// routers stop accepting deploys the day that admin leaves.
//
// The private half never leaves the machine — no API returns it. The public half
// is meant to be handed around: that is what gets imported on each router.

// InstanceKeyPath returns where the instance key lives for a given config path:
// an `ssh/` directory beside the config, so a single mounted volume carries the
// config, the admin password record and the deploy key together.
func InstanceKeyPath(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), "ssh", "id_ed25519")
}

// InstanceKey describes the instance key without ever exposing the private half.
type InstanceKey struct {
	Exists      bool   `json:"exists"`
	Path        string `json:"path"`
	PublicKey   string `json:"public_key"`  // authorized_keys line
	Fingerprint string `json:"fingerprint"` // SHA256:… as ssh-keygen prints it
}

// ReadInstanceKey reports the current key. A missing key is not an error — the
// UI offers to create one.
func ReadInstanceKey(configPath string) (InstanceKey, error) {
	path := InstanceKeyPath(configPath)
	info := InstanceKey{Path: path}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return info, nil
	}
	if err != nil {
		return info, err
	}
	signer, err := ssh.ParsePrivateKey(data)
	if err != nil {
		return info, fmt.Errorf("instance ssh key %s: %w", path, err)
	}
	info.Exists = true
	info.PublicKey = strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signer.PublicKey())))
	info.Fingerprint = ssh.FingerprintSHA256(signer.PublicKey())
	return info, nil
}

// CreateInstanceKey generates a fresh ed25519 key. It refuses to overwrite an
// existing key unless replace is set: regenerating invalidates every router that
// already trusts the old public key, so it must be a deliberate act.
func CreateInstanceKey(configPath, comment string, replace bool) (InstanceKey, error) {
	path := InstanceKeyPath(configPath)
	if _, err := os.Stat(path); err == nil && !replace {
		return InstanceKey{}, fmt.Errorf("instance ssh key already exists at %s", path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return InstanceKey{}, err
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return InstanceKey{}, err
	}
	if comment == "" {
		comment = "mkpk-provision"
	}
	block, err := ssh.MarshalPrivateKey(priv, comment)
	if err != nil {
		return InstanceKey{}, err
	}
	// Write 0600 atomically: an SSH client refuses a world-readable key, and a
	// half-written key would break deploys until someone noticed.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, pem.EncodeToMemory(block), 0o600); err != nil {
		return InstanceKey{}, err
	}
	if err := os.Rename(tmp, path); err != nil {
		return InstanceKey{}, err
	}
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		return InstanceKey{}, err
	}
	line := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPub))) + " " + comment
	// The .pub file is a convenience for scp/import; the API serves the line.
	if err := os.WriteFile(path+".pub", []byte(line+"\n"), 0o644); err != nil {
		return InstanceKey{}, err
	}
	return InstanceKey{
		Exists: true, Path: path,
		PublicKey:   line,
		Fingerprint: ssh.FingerprintSHA256(sshPub),
	}, nil
}
