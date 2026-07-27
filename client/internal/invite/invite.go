// Package invite defines the per-user invite blob: a compact, base64url-encoded
// JSON slice of one user's state (router address, their PSK and the services they
// may open). The admin exports it; the client decodes it and knocks. It carries
// exactly what the runtime needs — no other users' secrets.
package invite

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"mikrotik-psk-knock/client/internal/config"
)

// Version is the current blob format version.
const Version = 1

// Blob is one user's runtime configuration.
type Blob struct {
	Version       int       `json:"v"`
	Router        string    `json:"router"` // address
	BucketSeconds int64     `json:"bucket_seconds"`
	ClientID      string    `json:"client_id"`
	PSK           string    `json:"psk"`
	Services      []Service `json:"services"`
}

// Service is one openable service the user has access to.
type Service struct {
	Name      string `json:"name"` // service_name, part of the token
	Stage1    int    `json:"stage1"`
	Stage2    int    `json:"stage2"`
	Token     int    `json:"token"`
	CheckPort int    `json:"check_port"` // external dst-nat port for post-knock TCP check
}

// Encode marshals the blob to a base64url string.
func Encode(b Blob) (string, error) {
	data, err := json.Marshal(b)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

// Decode parses a base64url blob string.
func Decode(s string) (Blob, error) {
	data, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return Blob{}, fmt.Errorf("invite: base64 decode: %w", err)
	}
	var b Blob
	if err := json.Unmarshal(data, &b); err != nil {
		return Blob{}, fmt.Errorf("invite: json decode: %w", err)
	}
	if b.Version != Version {
		return Blob{}, fmt.Errorf("invite: unsupported version %d (want %d)", b.Version, Version)
	}
	if b.Router == "" || b.ClientID == "" || b.PSK == "" || len(b.Services) == 0 {
		return Blob{}, fmt.Errorf("invite: incomplete blob")
	}
	return b, nil
}

// ToRouter converts the blob into a minimal config.Router the runtime can resolve
// against (address, bucket seconds, the user and its services with just the ports
// the client needs). The returned router is not meant to pass config.Validate —
// it is a runtime-only projection.
func (b Blob) ToRouter() config.Router {
	services := map[string]config.Service{}
	names := make([]string, 0, len(b.Services))
	for _, s := range b.Services {
		services[s.Name] = config.Service{
			ServiceName: s.Name,
			Stage1Port:  s.Stage1,
			Stage2Port:  s.Stage2,
			TokenPort:   s.Token,
			NAT:         config.NAT{DstPort: s.CheckPort},
		}
		names = append(names, s.Name)
	}
	return config.Router{
		Address:  b.Router,
		Defaults: config.Defaults{BucketSeconds: b.BucketSeconds},
		Services: services,
		Clients: map[string]config.Client{
			b.ClientID: {ClientID: b.ClientID, Services: names, PSK: b.PSK},
		},
	}
}
