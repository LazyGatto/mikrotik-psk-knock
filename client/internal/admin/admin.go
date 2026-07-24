// Package admin holds the provisioning operations shared by the mkpk-provision
// CLI and the (upcoming) local web UI: build and mutate config, summarize it,
// render, and deploy over SSH. Frontends stay thin — they parse input, call these
// functions, and present the results.
package admin

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
	"mikrotik-psk-knock/client/internal/config"
	"mikrotik-psk-knock/client/internal/routeros"
)

// GenerateSecret returns a base64url-safe random secret of n bytes.
func GenerateSecret(n int) (string, error) {
	if n < 16 {
		return "", fmt.Errorf("secret must be at least 16 bytes")
	}
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// SaveConfig validates cfg and writes it to path with 0600 perms.
func SaveConfig(path string, cfg config.Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// InitOptions parameterizes a starter config.
type InitOptions struct {
	RouterName    string
	RouterAddress string
	ServiceName   string
	ClientName    string
}

// InitConfig builds a starter config with safe defaults and a generated PSK.
func InitConfig(o InitOptions) (config.Config, error) {
	if o.RouterAddress == "" {
		return config.Config{}, fmt.Errorf("router address is required")
	}
	if o.ServiceName == "" || o.ClientName == "" {
		return config.Config{}, fmt.Errorf("service and client names are required")
	}
	psk, err := GenerateSecret(32)
	if err != nil {
		return config.Config{}, err
	}
	return config.Config{
		Router: config.Router{Name: o.RouterName, Address: o.RouterAddress},
		Defaults: config.Defaults{
			BucketSeconds:   30,
			StageTimeout:    "5s",
			TokenHitTimeout: "2s",
			AllowedTimeout:  "3m",
			UsedTimeout:     "65s",
		},
		Services: map[string]config.Service{
			o.ServiceName: {
				ServiceName: o.ServiceName,
				Stage1Port:  41001,
				Stage2Port:  41002,
				TokenPort:   41003,
				AllowedList: "mkpk-tt-allowed-" + o.ServiceName,
				NAT:         config.NAT{Comment: "mkpk-tt dst-nat " + o.ServiceName, DstPort: 2222, ToAddress: "192.0.2.10", ToPort: 22},
				Notify:      config.Notify{Channel: "webhook"},
			},
		},
		Clients: map[string]config.Client{
			o.ClientName: {ClientID: o.ClientName, Service: o.ServiceName, PSK: psk},
		},
	}, nil
}

// ServiceOptions describes a service to add. Zero AllowedList / NAT.Comment and
// email port/tls get sensible per-name defaults.
type ServiceOptions struct {
	Name        string
	ServiceName string
	Stage1Port  int
	Stage2Port  int
	TokenPort   int
	AllowedList string
	NAT         config.NAT
	Notify      config.Notify
	Force       bool
}

// AddService adds or replaces a service in cfg and returns the updated config.
func AddService(cfg config.Config, o ServiceOptions) (config.Config, error) {
	if o.Name == "" {
		return cfg, fmt.Errorf("service name is required")
	}
	if o.Stage1Port == 0 || o.Stage2Port == 0 || o.TokenPort == 0 {
		return cfg, fmt.Errorf("stage1, stage2 and token ports are required")
	}
	if o.NAT.DstPort == 0 || o.NAT.ToPort == 0 || o.NAT.ToAddress == "" {
		return cfg, fmt.Errorf("nat dst_port, to_address and to_port are required")
	}
	if cfg.Services == nil {
		cfg.Services = map[string]config.Service{}
	}
	if _, ok := cfg.Services[o.Name]; ok && !o.Force {
		return cfg, fmt.Errorf("service %q already exists; use force to replace", o.Name)
	}
	id := o.ServiceName
	if id == "" {
		id = o.Name
	}
	nat := o.NAT
	if nat.Comment == "" {
		nat.Comment = "mkpk-tt dst-nat " + o.Name
	}
	allowed := o.AllowedList
	if allowed == "" {
		allowed = "mkpk-tt-allowed-" + o.Name
	}
	notify := o.Notify
	if notify.Channel == "" {
		notify.Channel = "webhook"
	}
	if notify.Channel == "email" {
		if notify.Email.Port == 0 {
			notify.Email.Port = 587
		}
		if notify.Email.TLS == "" {
			notify.Email.TLS = "starttls"
		}
	}
	cfg.Services[o.Name] = config.Service{
		ServiceName: id,
		Stage1Port:  o.Stage1Port,
		Stage2Port:  o.Stage2Port,
		TokenPort:   o.TokenPort,
		AllowedList: allowed,
		NAT:         nat,
		Notify:      notify,
	}
	return cfg, nil
}

// ClientOptions describes a client to add. Empty PSK is generated.
type ClientOptions struct {
	Name     string
	ClientID string
	Service  string
	PSK      string
	Force    bool
}

// AddClientResult carries the updated config and where the PSK came from.
type AddClientResult struct {
	Config    config.Config
	PSKSource string // "provided" or "generated"
}

// AddClient adds or replaces a client in cfg.
func AddClient(cfg config.Config, o ClientOptions) (AddClientResult, error) {
	if o.Name == "" {
		return AddClientResult{}, fmt.Errorf("client name is required")
	}
	if o.Service == "" {
		return AddClientResult{}, fmt.Errorf("service is required")
	}
	if _, ok := cfg.Services[o.Service]; !ok {
		return AddClientResult{}, fmt.Errorf("unknown service %q", o.Service)
	}
	if cfg.Clients == nil {
		cfg.Clients = map[string]config.Client{}
	}
	if _, ok := cfg.Clients[o.Name]; ok && !o.Force {
		return AddClientResult{}, fmt.Errorf("client %q already exists; use force to replace", o.Name)
	}
	id := o.ClientID
	if id == "" {
		id = o.Name
	}
	psk := o.PSK
	source := "provided"
	if psk == "" {
		var err error
		psk, err = GenerateSecret(32)
		if err != nil {
			return AddClientResult{}, err
		}
		source = "generated"
	}
	cfg.Clients[o.Name] = config.Client{ClientID: id, Service: o.Service, PSK: psk}
	return AddClientResult{Config: cfg, PSKSource: source}, nil
}

// RemoveService removes a service. It refuses if any client still references it.
func RemoveService(cfg config.Config, name string) (config.Config, error) {
	if _, ok := cfg.Services[name]; !ok {
		return cfg, fmt.Errorf("service %q not found", name)
	}
	var refs []string
	for cn, c := range cfg.Clients {
		if c.Service == name {
			refs = append(refs, cn)
		}
	}
	if len(refs) > 0 {
		sort.Strings(refs)
		return cfg, fmt.Errorf("service %q is referenced by clients: %s", name, strings.Join(refs, ", "))
	}
	delete(cfg.Services, name)
	return cfg, nil
}

// RemoveClient removes a client.
func RemoveClient(cfg config.Config, name string) (config.Config, error) {
	if _, ok := cfg.Clients[name]; !ok {
		return cfg, fmt.Errorf("client %q not found", name)
	}
	delete(cfg.Clients, name)
	return cfg, nil
}

// Render renders the whole config, or a single client when clientName is set.
func Render(cfg config.Config, clientName string) (string, error) {
	if clientName == "" {
		return routeros.RenderConfig(cfg)
	}
	res, err := cfg.Resolve(clientName)
	if err != nil {
		return "", err
	}
	return routeros.Render(res)
}

// Summary is a structured, secret-free view of a config for CLI/JSON output.
type Summary struct {
	Router   config.Router    `json:"router"`
	Defaults config.Defaults  `json:"defaults"`
	Services []ServiceSummary `json:"services"`
	Clients  []ClientSummary  `json:"clients"`
}

type ServiceSummary struct {
	Name          string `json:"name"`
	ServiceName   string `json:"service_name"`
	Stage1Port    int    `json:"stage1_port"`
	Stage2Port    int    `json:"stage2_port"`
	TokenPort     int    `json:"token_port"`
	AllowedList   string `json:"allowed_list"`
	NATEnabled    bool   `json:"nat_enabled"`
	NATDstPort    int    `json:"nat_dst_port"`
	NATToAddress  string `json:"nat_to_address"`
	NATToPort     int    `json:"nat_to_port"`
	NotifyEnabled bool   `json:"notify_enabled"`
	NotifyChannel string `json:"notify_channel"`
}

type ClientSummary struct {
	Name     string `json:"name"`
	ClientID string `json:"client_id"`
	Service  string `json:"service"`
	PSKSet   bool   `json:"psk_set"`
}

// Summarize builds a secret-free summary with deterministic ordering.
func Summarize(cfg config.Config) Summary {
	s := Summary{Router: cfg.Router, Defaults: cfg.Defaults}
	for _, name := range sortedKeys(cfg.Services) {
		svc := cfg.Services[name]
		s.Services = append(s.Services, ServiceSummary{
			Name:          name,
			ServiceName:   svc.ServiceName,
			Stage1Port:    svc.Stage1Port,
			Stage2Port:    svc.Stage2Port,
			TokenPort:     svc.TokenPort,
			AllowedList:   svc.AllowedList,
			NATEnabled:    svc.NAT.Enabled,
			NATDstPort:    svc.NAT.DstPort,
			NATToAddress:  svc.NAT.ToAddress,
			NATToPort:     svc.NAT.ToPort,
			NotifyEnabled: svc.Notify.Enabled,
			NotifyChannel: svc.Notify.Channel,
		})
	}
	for _, name := range sortedKeys(cfg.Clients) {
		c := cfg.Clients[name]
		s.Clients = append(s.Clients, ClientSummary{
			Name: name, ClientID: c.ClientID, Service: c.Service, PSKSet: c.PSK != "",
		})
	}
	return s
}

func sortedKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
