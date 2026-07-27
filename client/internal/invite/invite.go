// Package invite defines the per-user invite blob: a compact, base64url-encoded
// JSON slice of one user's state. It can carry several routers (the user's
// common client_id plus, per router, that router's address, PSK and openable
// services). The admin exports it; the client decodes it and knocks. It carries
// exactly what the runtime needs — no other users' secrets.
package invite

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"mikrotik-psk-knock/client/internal/config"
)

// Version is the current blob format version.
const Version = 2

// Blob is one user's runtime configuration across one or more routers.
type Blob struct {
	Version  int      `json:"v"`
	ClientID string   `json:"client_id"` // common identity across routers
	Routers  []Router `json:"routers"`
}

// Router is one router the user can reach: its address, bucket seconds, the PSK
// for this router and the services openable on it.
type Router struct {
	Router        string    `json:"router"` // address
	BucketSeconds int64     `json:"bucket_seconds"`
	PSK           string    `json:"psk"`
	Services      []Service `json:"services"`
}

// Service is one openable service the user has access to.
type Service struct {
	Name      string `json:"name"` // service_name, part of the token
	Stage1    int    `json:"stage1"`
	Stage2    int    `json:"stage2"`
	Token     int    `json:"token"`
	CheckPort int    `json:"check_port"` // external port for post-knock TCP check
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
	if b.ClientID == "" || len(b.Routers) == 0 {
		return Blob{}, fmt.Errorf("invite: incomplete blob")
	}
	for _, r := range b.Routers {
		if r.Router == "" || r.PSK == "" || len(r.Services) == 0 {
			return Blob{}, fmt.Errorf("invite: incomplete router entry")
		}
	}
	return b, nil
}

// ToConfig converts the blob into a minimal config.Config the runtime can
// resolve against: each blob router becomes a config router (keyed by its
// address) and the user's access to it, with just the ports the client needs.
// The returned config is a runtime-only projection, not meant to pass
// config.Validate.
func (b Blob) ToConfig() config.Config {
	routers := map[string]config.Router{}
	access := map[string]config.UserAccess{}
	for _, rb := range b.Routers {
		services := map[string]config.Service{}
		names := make([]string, 0, len(rb.Services))
		for _, s := range rb.Services {
			services[s.Name] = config.Service{
				ServiceName: s.Name,
				Stage1Port:  s.Stage1,
				Stage2Port:  s.Stage2,
				TokenPort:   s.Token,
				Target:      config.Target{Port: s.CheckPort},
			}
			names = append(names, s.Name)
		}
		routers[rb.Router] = config.Router{
			Address:  rb.Router,
			Defaults: config.Defaults{BucketSeconds: rb.BucketSeconds},
			Services: services,
		}
		access[rb.Router] = config.UserAccess{Services: names, PSK: rb.PSK}
	}
	return config.Config{
		Routers: routers,
		Users:   map[string]config.User{b.ClientID: {ClientID: b.ClientID, Access: access}},
	}
}

// RouterNames returns the blob's router addresses (keys) in slice order.
func (b Blob) RouterNames() []string {
	names := make([]string, 0, len(b.Routers))
	for _, r := range b.Routers {
		names = append(names, r.Router)
	}
	return names
}
