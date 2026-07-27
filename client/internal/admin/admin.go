// Package admin holds the provisioning operations shared by the mkpk-provision
// CLI and the local web UI: build and mutate the multi-router config, summarize
// it, render a router and deploy over SSH. Frontends stay thin — they parse
// input, call these functions, and present the results. Mutating operations are
// router-scoped: they take a router name and act within that router.
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

func getRouter(cfg config.Config, name string) (config.Router, error) {
	if name == "" {
		return config.Router{}, fmt.Errorf("router name is required")
	}
	r, ok := cfg.Routers[name]
	if !ok {
		return config.Router{}, fmt.Errorf("unknown router %q", name)
	}
	return r, nil
}

func putRouter(cfg config.Config, name string, r config.Router) config.Config {
	if cfg.Routers == nil {
		cfg.Routers = map[string]config.Router{}
	}
	cfg.Routers[name] = r
	return cfg
}

func defaultDefaults() config.Defaults {
	return config.Defaults{
		BucketSeconds:   30,
		StageTimeout:    "5s",
		TokenHitTimeout: "2s",
		AllowedTimeout:  "3m",
		UsedTimeout:     "65s",
	}
}

// InitOptions parameterizes a starter config.
type InitOptions struct {
	RouterName    string
	RouterAddress string
	ServiceName   string
	ClientName    string
}

// InitConfig builds a starter config: one router with a demo service and a user
// with a generated PSK.
func InitConfig(o InitOptions) (config.Config, error) {
	if o.RouterName == "" || o.RouterAddress == "" {
		return config.Config{}, fmt.Errorf("router name and address are required")
	}
	if o.ServiceName == "" || o.ClientName == "" {
		return config.Config{}, fmt.Errorf("service and client names are required")
	}
	psk, err := GenerateSecret(32)
	if err != nil {
		return config.Config{}, err
	}
	router := config.Router{
		Address:  o.RouterAddress,
		Defaults: defaultDefaults(),
		Services: map[string]config.Service{
			o.ServiceName: {
				ServiceName: o.ServiceName,
				Stage1Port:  41001, Stage2Port: 41002, TokenPort: 41003,
				AllowedList: "mkpk-tt-allowed-" + o.ServiceName,
				NAT:         config.NAT{Comment: "mkpk-tt dst-nat " + o.ServiceName, DstPort: 2222, ToAddress: "192.0.2.10", ToPort: 22},
				Notify:      config.Notify{Channel: "webhook"},
			},
		},
		Clients: map[string]config.Client{
			o.ClientName: {ClientID: o.ClientName, Services: []string{o.ServiceName}, PSK: psk},
		},
	}
	return config.Config{Routers: map[string]config.Router{o.RouterName: router}}, nil
}

// AddRouter adds an empty router with default timeouts.
func AddRouter(cfg config.Config, name, address string) (config.Config, error) {
	if name == "" || address == "" {
		return cfg, fmt.Errorf("router name and address are required")
	}
	if _, ok := cfg.Routers[name]; ok {
		return cfg, fmt.Errorf("router %q already exists", name)
	}
	return putRouter(cfg, name, config.Router{
		Address:  address,
		Defaults: defaultDefaults(),
		Services: map[string]config.Service{},
		Clients:  map[string]config.Client{},
	}), nil
}

// RemoveRouter removes a router entirely.
func RemoveRouter(cfg config.Config, name string) (config.Config, error) {
	if _, ok := cfg.Routers[name]; !ok {
		return cfg, fmt.Errorf("router %q not found", name)
	}
	delete(cfg.Routers, name)
	return cfg, nil
}

// ServiceOptions describes a service to add. Zero AllowedList / NAT.Comment and
// email port/tls get sensible per-name defaults.
type ServiceOptions struct {
	Name        string
	ServiceName string
	Disabled    bool
	Stage1Port  int
	Stage2Port  int
	TokenPort   int
	AllowedList string
	NAT         config.NAT
	Notify      config.Notify
	Force       bool
}

// AddService adds or replaces a service on the router.
func AddService(cfg config.Config, routerName string, o ServiceOptions) (config.Config, error) {
	r, err := getRouter(cfg, routerName)
	if err != nil {
		return cfg, err
	}
	if o.Name == "" {
		return cfg, fmt.Errorf("service name is required")
	}
	if o.Stage1Port == 0 || o.Stage2Port == 0 || o.TokenPort == 0 {
		return cfg, fmt.Errorf("stage1, stage2 and token ports are required")
	}
	if o.NAT.DstPort == 0 || o.NAT.ToPort == 0 || o.NAT.ToAddress == "" {
		return cfg, fmt.Errorf("nat dst_port, to_address and to_port are required")
	}
	if r.Services == nil {
		r.Services = map[string]config.Service{}
	}
	if _, ok := r.Services[o.Name]; ok && !o.Force {
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
	r.Services[o.Name] = config.Service{
		ServiceName: id,
		Disabled:    o.Disabled,
		Stage1Port:  o.Stage1Port,
		Stage2Port:  o.Stage2Port,
		TokenPort:   o.TokenPort,
		AllowedList: allowed,
		NAT:         nat,
		Notify:      notify,
	}
	return putRouter(cfg, routerName, r), nil
}

// SetServiceEnabled toggles a service on the router.
func SetServiceEnabled(cfg config.Config, routerName, name string, enabled bool) (config.Config, error) {
	r, err := getRouter(cfg, routerName)
	if err != nil {
		return cfg, err
	}
	svc, ok := r.Services[name]
	if !ok {
		return cfg, fmt.Errorf("service %q not found", name)
	}
	svc.Disabled = !enabled
	r.Services[name] = svc
	return putRouter(cfg, routerName, r), nil
}

// RemoveService removes a service. It refuses if a user still references it.
func RemoveService(cfg config.Config, routerName, name string) (config.Config, error) {
	r, err := getRouter(cfg, routerName)
	if err != nil {
		return cfg, err
	}
	if _, ok := r.Services[name]; !ok {
		return cfg, fmt.Errorf("service %q not found", name)
	}
	var refs []string
	for cn, c := range r.Clients {
		for _, s := range c.Services {
			if s == name {
				refs = append(refs, cn)
			}
		}
	}
	if len(refs) > 0 {
		sort.Strings(refs)
		return cfg, fmt.Errorf("service %q is referenced by users: %s", name, strings.Join(refs, ", "))
	}
	delete(r.Services, name)
	return putRouter(cfg, routerName, r), nil
}

// ClientOptions describes a user to add. Empty PSK is generated.
type ClientOptions struct {
	Name     string
	ClientID string
	Services []string
	PSK      string
	Force    bool
}

// AddClientResult carries the updated config and where the PSK came from.
type AddClientResult struct {
	Config    config.Config
	PSKSource string // "provided" or "generated"
}

// AddClient adds or replaces a user on the router.
func AddClient(cfg config.Config, routerName string, o ClientOptions) (AddClientResult, error) {
	r, err := getRouter(cfg, routerName)
	if err != nil {
		return AddClientResult{}, err
	}
	if o.Name == "" {
		return AddClientResult{}, fmt.Errorf("user name is required")
	}
	for _, s := range o.Services {
		if _, ok := r.Services[s]; !ok {
			return AddClientResult{}, fmt.Errorf("unknown service %q", s)
		}
	}
	if r.Clients == nil {
		r.Clients = map[string]config.Client{}
	}
	if _, ok := r.Clients[o.Name]; ok && !o.Force {
		return AddClientResult{}, fmt.Errorf("user %q already exists; use force to replace", o.Name)
	}
	id := o.ClientID
	if id == "" {
		id = o.Name
	}
	psk := o.PSK
	source := "provided"
	if psk == "" {
		psk, err = GenerateSecret(32)
		if err != nil {
			return AddClientResult{}, err
		}
		source = "generated"
	}
	r.Clients[o.Name] = config.Client{ClientID: id, Services: o.Services, PSK: psk}
	return AddClientResult{Config: putRouter(cfg, routerName, r), PSKSource: source}, nil
}

// RemoveClient removes a user from the router.
func RemoveClient(cfg config.Config, routerName, name string) (config.Config, error) {
	r, err := getRouter(cfg, routerName)
	if err != nil {
		return cfg, err
	}
	if _, ok := r.Clients[name]; !ok {
		return cfg, fmt.Errorf("user %q not found", name)
	}
	delete(r.Clients, name)
	return putRouter(cfg, routerName, r), nil
}

// Render renders one router into RouterOS script.
func Render(cfg config.Config, routerName string) (string, error) {
	r, err := getRouter(cfg, routerName)
	if err != nil {
		return "", err
	}
	return routeros.RenderConfig(r)
}

// Summary is a structured, secret-free view of the config for CLI/JSON output.
type Summary struct {
	Routers []RouterSummary `json:"routers"`
}

type RouterSummary struct {
	Name     string           `json:"name"`
	Address  string           `json:"address"`
	Hash     string           `json:"hash"`
	Defaults config.Defaults  `json:"defaults"`
	Services []ServiceSummary `json:"services"`
	Clients  []ClientSummary  `json:"clients"`
}

type ServiceSummary struct {
	Name          string `json:"name"`
	ServiceName   string `json:"service_name"`
	Enabled       bool   `json:"enabled"`
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
	Name     string   `json:"name"`
	ClientID string   `json:"client_id"`
	Services []string `json:"services"`
	PSKSet   bool     `json:"psk_set"`
}

// Summarize builds a secret-free summary with deterministic ordering.
func Summarize(cfg config.Config) Summary {
	var s Summary
	for _, rn := range sortedKeys(cfg.Routers) {
		r := cfg.Routers[rn]
		rs := RouterSummary{Name: rn, Address: r.Address, Hash: r.Hash(), Defaults: r.Defaults}
		for _, name := range sortedKeys(r.Services) {
			svc := r.Services[name]
			rs.Services = append(rs.Services, ServiceSummary{
				Name:          name,
				ServiceName:   svc.ServiceName,
				Enabled:       svc.Enabled(),
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
		for _, name := range sortedKeys(r.Clients) {
			c := r.Clients[name]
			rs.Clients = append(rs.Clients, ClientSummary{
				Name: name, ClientID: c.ClientID, Services: c.Services, PSKSet: c.PSK != "",
			})
		}
		s.Routers = append(s.Routers, rs)
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
