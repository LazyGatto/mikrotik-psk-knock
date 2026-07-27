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
	"mikrotik-psk-knock/client/internal/invite"
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
				Target:      config.Target{Type: config.TargetForward, Protocol: "tcp", Comment: "mkpk-tt target " + o.ServiceName, Port: 2222, ToAddress: "192.0.2.10", ToPort: 22},
				Notify:      config.Notify{Channel: "webhook"},
			},
		},
		Clients: map[string]config.Client{
			o.ClientName: {ClientID: o.ClientName, Services: []string{o.ServiceName}, PSK: psk},
		},
	}
	return config.Config{Routers: map[string]config.Router{o.RouterName: router}}, nil
}

// RouterOptions describes a router to create or update: its address plus the
// SSH deploy credentials that live on the router.
type RouterOptions struct {
	Name    string
	Address string
	Deploy  config.Deploy
}

// SetRouter creates the router when absent (default timeouts, empty services and
// clients) or updates its address and deploy credentials when present, keeping
// its services and clients. This is the single add/edit entry point.
func SetRouter(cfg config.Config, o RouterOptions) (config.Config, error) {
	if o.Name == "" || o.Address == "" {
		return cfg, fmt.Errorf("router name and address are required")
	}
	if o.Deploy.Port != 0 {
		if o.Deploy.Port < 1 || o.Deploy.Port > 65535 {
			return cfg, fmt.Errorf("deploy.port must be between 1 and 65535")
		}
	}
	r, ok := cfg.Routers[o.Name]
	if !ok {
		r = config.Router{
			Defaults: defaultDefaults(),
			Services: map[string]config.Service{},
			Clients:  map[string]config.Client{},
		}
	}
	r.Address = o.Address
	r.Deploy = o.Deploy
	return putRouter(cfg, o.Name, r), nil
}

// RemoveRouter removes a router entirely.
func RemoveRouter(cfg config.Config, name string) (config.Config, error) {
	if _, ok := cfg.Routers[name]; !ok {
		return cfg, fmt.Errorf("router %q not found", name)
	}
	delete(cfg.Routers, name)
	return cfg, nil
}

// ServiceOptions describes a service to add. Zero AllowedList / Target.Comment /
// Target.Protocol and email port/tls get sensible defaults.
type ServiceOptions struct {
	Name        string
	ServiceName string
	Disabled    bool
	Stage1Port  int
	Stage2Port  int
	TokenPort   int
	AllowedList string
	Target      config.Target
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
	target := o.Target
	if target.Type == "" {
		target.Type = config.TargetForward
	}
	if target.Protocol == "" {
		target.Protocol = "tcp"
	}
	if target.Comment == "" {
		target.Comment = "mkpk-tt target " + o.Name
	}
	if target.Type == config.TargetLocal {
		target.ToAddress, target.ToPort = "", 0
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
	svc := config.Service{
		ServiceName: id,
		Disabled:    o.Disabled,
		Stage1Port:  o.Stage1Port,
		Stage2Port:  o.Stage2Port,
		TokenPort:   o.TokenPort,
		AllowedList: allowed,
		Target:      target,
		Notify:      notify,
	}
	if err := validateService(o.Name, svc); err != nil {
		return cfg, err
	}
	r.Services[o.Name] = svc
	return putRouter(cfg, routerName, r), nil
}

// validateService checks a single service in isolation (target coherence).
// Full config validation, including cross-service checks, runs in SaveConfig.
func validateService(name string, svc config.Service) error {
	switch svc.Target.Type {
	case config.TargetForward:
		if svc.Target.Port == 0 || svc.Target.ToPort == 0 || svc.Target.ToAddress == "" {
			return fmt.Errorf("service %q forward target needs port, to_address and to_port", name)
		}
	case config.TargetLocal:
		if svc.Target.Port == 0 {
			return fmt.Errorf("service %q local target needs a port", name)
		}
	default:
		return fmt.Errorf("service %q target.type must be %q or %q", name, config.TargetForward, config.TargetLocal)
	}
	return nil
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

// ExportUser builds a per-user invite blob for a user on a router: the router
// address, bucket seconds, that user's client_id and PSK, and the ports of the
// enabled services they are assigned. It never includes other users' secrets.
func ExportUser(cfg config.Config, routerName, userName string) (string, error) {
	r, err := getRouter(cfg, routerName)
	if err != nil {
		return "", err
	}
	c, ok := r.Clients[userName]
	if !ok {
		return "", fmt.Errorf("unknown user %q", userName)
	}
	b := invite.Blob{
		Version:       invite.Version,
		Router:        r.Address,
		BucketSeconds: r.Defaults.BucketSeconds,
		ClientID:      c.ClientID,
		PSK:           c.PSK,
	}
	for _, sn := range c.Services {
		s, ok := r.Services[sn]
		if !ok || !s.Enabled() {
			continue
		}
		b.Services = append(b.Services, invite.Service{
			Name:      s.ServiceName,
			Stage1:    s.Stage1Port,
			Stage2:    s.Stage2Port,
			Token:     s.TokenPort,
			CheckPort: s.Target.Port,
		})
	}
	if len(b.Services) == 0 {
		return "", fmt.Errorf("user %q has no enabled services to export", userName)
	}
	return invite.Encode(b)
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
	Deploy   DeploySummary    `json:"deploy"`
	Defaults config.Defaults  `json:"defaults"`
	Services []ServiceSummary `json:"services"`
	Clients  []ClientSummary  `json:"clients"`
}

// DeploySummary is a secret-free view of a router's SSH deploy credentials:
// non-secret connection params plus booleans for whether secrets are set.
// Configured reports whether there is enough to attempt a connection.
type DeploySummary struct {
	Port        int    `json:"port"`
	User        string `json:"user"`
	KeyPath     string `json:"key_path"`
	UseAgent    bool   `json:"use_agent"`
	PasswordSet bool   `json:"password_set"`
	KeyPassSet  bool   `json:"key_pass_set"`
	Configured  bool   `json:"configured"`
}

type ServiceSummary struct {
	Name            string `json:"name"`
	ServiceName     string `json:"service_name"`
	Enabled         bool   `json:"enabled"`
	Stage1Port      int    `json:"stage1_port"`
	Stage2Port      int    `json:"stage2_port"`
	TokenPort       int    `json:"token_port"`
	AllowedList     string `json:"allowed_list"`
	TargetType      string `json:"target_type"`
	TargetProtocol  string `json:"target_protocol"`
	TargetPort      int    `json:"target_port"`
	TargetToAddress string `json:"target_to_address"`
	TargetToPort    int    `json:"target_to_port"`
	NotifyEnabled   bool   `json:"notify_enabled"`
	NotifyChannel   string `json:"notify_channel"`
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
		rs := RouterSummary{Name: rn, Address: r.Address, Hash: r.Hash(), Deploy: deploySummary(r), Defaults: r.Defaults}
		for _, name := range sortedKeys(r.Services) {
			svc := r.Services[name]
			rs.Services = append(rs.Services, ServiceSummary{
				Name:            name,
				ServiceName:     svc.ServiceName,
				Enabled:         svc.Enabled(),
				Stage1Port:      svc.Stage1Port,
				Stage2Port:      svc.Stage2Port,
				TokenPort:       svc.TokenPort,
				AllowedList:     svc.AllowedList,
				TargetType:      svc.Target.Type,
				TargetProtocol:  svc.Target.Protocol,
				TargetPort:      svc.Target.Port,
				TargetToAddress: svc.Target.ToAddress,
				TargetToPort:    svc.Target.ToPort,
				NotifyEnabled:   svc.Notify.Enabled,
				NotifyChannel:   svc.Notify.Channel,
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

func deploySummary(r config.Router) DeploySummary {
	d := r.Deploy
	return DeploySummary{
		Port:        d.Port,
		User:        d.User,
		KeyPath:     d.KeyPath,
		UseAgent:    d.UseAgent,
		PasswordSet: d.Password != "",
		KeyPassSet:  d.KeyPass != "",
		// Enough to attempt a connection: an address (router-level) and any auth.
		Configured: r.Address != "" && (d.KeyPath != "" || d.UseAgent || d.Password != ""),
	}
}

func sortedKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
