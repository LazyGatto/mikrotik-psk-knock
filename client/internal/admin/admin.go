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
	"math/big"
	"os"
	"slices"
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
			},
		},
	}
	return config.Config{
		Routers: map[string]config.Router{o.RouterName: router},
		Users: map[string]config.User{
			o.ClientName: {
				ClientID: o.ClientName,
				Access: map[string]config.UserAccess{
					o.RouterName: {Services: []string{o.ServiceName}, PSK: psk},
				},
			},
		},
	}, nil
}

// RouterOptions describes a router to create or update: its address, the SSH
// deploy credentials, and the per-router notification config.
type RouterOptions struct {
	Name    string
	Address string        // public knock address (also default SSH target)
	Deploy  config.Deploy // Deploy.Address optionally overrides the SSH target
	Notify  config.Notify
}

// SetRouter creates the router when absent (default timeouts, empty services) or
// updates its address, deploy credentials and notification config when present,
// keeping its services. This is the single add/edit entry point.
func SetRouter(cfg config.Config, o RouterOptions) (config.Config, error) {
	if o.Name == "" || o.Address == "" {
		return cfg, fmt.Errorf("router name and address are required")
	}
	if o.Deploy.Port != 0 {
		if o.Deploy.Port < 1 || o.Deploy.Port > 65535 {
			return cfg, fmt.Errorf("deploy.port must be between 1 and 65535")
		}
	}
	notify := o.Notify
	if notify.Email.Enabled {
		if notify.Email.Port == 0 {
			notify.Email.Port = 587
		}
		if notify.Email.TLS == "" {
			notify.Email.TLS = "starttls"
		}
	}
	r, ok := cfg.Routers[o.Name]
	if !ok {
		r = config.Router{
			Defaults: defaultDefaults(),
			Services: map[string]config.Service{},
		}
	}
	r.Address = o.Address
	r.Deploy = o.Deploy
	r.Notify = notify
	return putRouter(cfg, o.Name, r), nil
}

// RemoveRouter removes a router entirely, along with every user's access to it
// (access to a deleted router would otherwise dangle).
func RemoveRouter(cfg config.Config, name string) (config.Config, error) {
	if _, ok := cfg.Routers[name]; !ok {
		return cfg, fmt.Errorf("router %q not found", name)
	}
	delete(cfg.Routers, name)
	for un, u := range cfg.Users {
		if _, ok := u.Access[name]; ok {
			delete(u.Access, name)
			cfg.Users[un] = u
		}
	}
	return cfg, nil
}

// ServiceOptions describes a service to add. Zero AllowedList / Target.Comment /
// Target.Protocol get sensible defaults. Notifications are per router, not here.
type ServiceOptions struct {
	Name        string
	ServiceName string
	Disabled    bool
	Stage1Port  int
	Stage2Port  int
	TokenPort   int
	AllowedList string
	Target      config.Target
	Force       bool
}

// SuggestPorts returns n distinct free ports for a router — not used by any of
// its services' knock or target ports — drawn from the 40000-59999 range. It
// backs the UI "randomize knock ports" button and the CLI --random-ports flag.
func SuggestPorts(cfg config.Config, routerName string, n int) ([]int, error) {
	r, err := getRouter(cfg, routerName)
	if err != nil {
		return nil, err
	}
	if n <= 0 || n > 16 {
		return nil, fmt.Errorf("count must be between 1 and 16")
	}
	used := r.UsedPorts()
	const lo, hi = 40000, 60000
	out := make([]int, 0, n)
	picked := map[int]bool{}
	for attempts := 0; len(out) < n && attempts < 100000; attempts++ {
		p, err := randPort(lo, hi)
		if err != nil {
			return nil, err
		}
		if used[p] || picked[p] {
			continue
		}
		picked[p] = true
		out = append(out, p)
	}
	if len(out) < n {
		return nil, fmt.Errorf("could not find %d free ports in %d-%d", n, lo, hi)
	}
	return out, nil
}

func randPort(lo, hi int) (int, error) {
	nBig, err := rand.Int(rand.Reader, big.NewInt(int64(hi-lo)))
	if err != nil {
		return 0, err
	}
	return lo + int(nBig.Int64()), nil
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
	svc := config.Service{
		ServiceName: id,
		Disabled:    o.Disabled,
		Stage1Port:  o.Stage1Port,
		Stage2Port:  o.Stage2Port,
		TokenPort:   o.TokenPort,
		AllowedList: allowed,
		Target:      target,
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
	for un, u := range cfg.Users {
		access, ok := u.Access[routerName]
		if !ok {
			continue
		}
		if slices.Contains(access.Services, name) {
			refs = append(refs, un)
		}
	}
	if len(refs) > 0 {
		sort.Strings(refs)
		return cfg, fmt.Errorf("service %q is referenced by users: %s", name, strings.Join(refs, ", "))
	}
	delete(r.Services, name)
	return putRouter(cfg, routerName, r), nil
}

// UserOptions describes granting a user access on one router: the user identity
// (common client_id) plus the services and per-router PSK for that router. Empty
// PSK is generated for a new grant, or kept for an existing one.
type UserOptions struct {
	Name     string
	ClientID string
	Services []string
	PSK      string
	Force    bool
}

// AddUserResult carries the updated config and where the PSK came from.
type AddUserResult struct {
	Config    config.Config
	PSKSource string // "provided", "generated" or "kept"
}

// AddUser creates the user if needed and grants (or updates) its access on the
// router: the services it may open there and the PSK used on that router. Other
// routers' access is preserved.
func AddUser(cfg config.Config, routerName string, o UserOptions) (AddUserResult, error) {
	r, err := getRouter(cfg, routerName)
	if err != nil {
		return AddUserResult{}, err
	}
	if o.Name == "" {
		return AddUserResult{}, fmt.Errorf("user name is required")
	}
	for _, s := range o.Services {
		if _, ok := r.Services[s]; !ok {
			return AddUserResult{}, fmt.Errorf("unknown service %q", s)
		}
	}
	if cfg.Users == nil {
		cfg.Users = map[string]config.User{}
	}
	u, existed := cfg.Users[o.Name]
	if u.Access == nil {
		u.Access = map[string]config.UserAccess{}
	}
	if _, granted := u.Access[routerName]; existed && granted && !o.Force {
		return AddUserResult{}, fmt.Errorf("user %q already has access to router %q; use force to replace", o.Name, routerName)
	}
	if o.ClientID != "" {
		u.ClientID = o.ClientID
	} else if u.ClientID == "" {
		u.ClientID = o.Name
	}
	psk := o.PSK
	source := "provided"
	if psk == "" {
		if prev, granted := u.Access[routerName]; granted && prev.PSK != "" {
			psk, source = prev.PSK, "kept"
		} else {
			if psk, err = GenerateSecret(32); err != nil {
				return AddUserResult{}, err
			}
			source = "generated"
		}
	}
	u.Access[routerName] = config.UserAccess{Services: o.Services, PSK: psk}
	cfg.Users[o.Name] = u
	return AddUserResult{Config: cfg, PSKSource: source}, nil
}

// CreateUser creates a top-level user with no access yet. clientID defaults to
// the name. It is the "add user" entry point; access is granted later via
// AddUser / the access matrix.
func CreateUser(cfg config.Config, name, clientID string) (config.Config, error) {
	if name == "" {
		return cfg, fmt.Errorf("user name is required")
	}
	if cfg.Users == nil {
		cfg.Users = map[string]config.User{}
	}
	if _, ok := cfg.Users[name]; ok {
		return cfg, fmt.Errorf("user %q already exists", name)
	}
	if clientID == "" {
		clientID = name
	}
	cfg.Users[name] = config.User{ClientID: clientID, Access: map[string]config.UserAccess{}}
	return cfg, nil
}

// RenameUser renames a user, moving its access and updating its client_id to the
// new name. Because the client_id is part of the token, this invalidates every
// invite already handed out for the user.
func RenameUser(cfg config.Config, oldName, newName string) (config.Config, error) {
	if newName == "" {
		return cfg, fmt.Errorf("new user name is required")
	}
	u, ok := cfg.Users[oldName]
	if !ok {
		return cfg, fmt.Errorf("user %q not found", oldName)
	}
	if oldName == newName {
		return cfg, nil
	}
	if _, ok := cfg.Users[newName]; ok {
		return cfg, fmt.Errorf("user %q already exists", newName)
	}
	u.ClientID = newName
	delete(cfg.Users, oldName)
	cfg.Users[newName] = u
	return cfg, nil
}

// RotateUserPSK generates a fresh PSK for one (user, router) pair. Deploying the
// router and re-issuing the user's invite are required for it to take effect.
func RotateUserPSK(cfg config.Config, userName, routerName string) (config.Config, error) {
	u, ok := cfg.Users[userName]
	if !ok {
		return cfg, fmt.Errorf("user %q not found", userName)
	}
	access, ok := u.Access[routerName]
	if !ok {
		return cfg, fmt.Errorf("user %q has no access to router %q", userName, routerName)
	}
	psk, err := GenerateSecret(32)
	if err != nil {
		return cfg, err
	}
	access.PSK = psk
	u.Access[routerName] = access
	cfg.Users[userName] = u
	return cfg, nil
}

// RemoveUserAccess revokes a user's access on one router. The user entity
// remains (users are top-level); use RemoveUser to delete it entirely.
func RemoveUserAccess(cfg config.Config, routerName, name string) (config.Config, error) {
	u, ok := cfg.Users[name]
	if !ok {
		return cfg, fmt.Errorf("user %q not found", name)
	}
	if _, ok := u.Access[routerName]; !ok {
		return cfg, fmt.Errorf("user %q has no access to router %q", name, routerName)
	}
	delete(u.Access, routerName)
	cfg.Users[name] = u
	return cfg, nil
}

// RemoveUser removes a user entirely, across all routers.
func RemoveUser(cfg config.Config, name string) (config.Config, error) {
	if _, ok := cfg.Users[name]; !ok {
		return cfg, fmt.Errorf("user %q not found", name)
	}
	delete(cfg.Users, name)
	return cfg, nil
}

// ExportUser builds an invite blob for a user. When routerName is empty the blob
// bundles every router the user can reach; otherwise it carries just that one.
// Each router entry has its own address, bucket seconds, PSK and enabled
// services. The common client_id is shared. It never includes other users'
// secrets.
func ExportUser(cfg config.Config, userName, routerName string) (string, error) {
	u, ok := cfg.Users[userName]
	if !ok {
		return "", fmt.Errorf("unknown user %q", userName)
	}
	b := invite.Blob{Version: invite.Version, ClientID: u.ClientID}
	routerNames := sortedKeys(u.Access)
	if routerName != "" {
		if _, ok := u.Access[routerName]; !ok {
			return "", fmt.Errorf("user %q has no access to router %q", userName, routerName)
		}
		routerNames = []string{routerName}
	}
	for _, rn := range routerNames {
		r, ok := cfg.Routers[rn]
		if !ok {
			continue
		}
		access := u.Access[rn]
		rb := invite.Router{
			Router:        r.Address, // the public knock address — no fallback
			BucketSeconds: r.Defaults.BucketSeconds,
			PSK:           access.PSK,
		}
		for _, sn := range access.Services {
			s, ok := r.Services[sn]
			if !ok || !s.Enabled() {
				continue
			}
			rb.Services = append(rb.Services, invite.Service{
				Name:      s.ServiceName,
				Stage1:    s.Stage1Port,
				Stage2:    s.Stage2Port,
				Token:     s.TokenPort,
				CheckPort: s.Target.Port,
			})
		}
		if len(rb.Services) > 0 {
			b.Routers = append(b.Routers, rb)
		}
	}
	if len(b.Routers) == 0 {
		return "", fmt.Errorf("user %q has no enabled services to export", userName)
	}
	return invite.Encode(b)
}

// Render renders one router (its services and the users granted access) into
// RouterOS script.
func Render(cfg config.Config, routerName string) (string, error) {
	r, err := getRouter(cfg, routerName)
	if err != nil {
		return "", err
	}
	return routeros.RenderConfig(r, cfg.RenderClients(routerName))
}

// Summary is a structured, secret-free view of the config for CLI/JSON output.
type Summary struct {
	Routers []RouterSummary `json:"routers"`
	Users   []UserSummary   `json:"users"`
}

// UserSummary is the top-level, secret-free view of a user and its per-router
// access.
type UserSummary struct {
	Name     string          `json:"name"`
	ClientID string          `json:"client_id"`
	Access   []AccessSummary `json:"access"`
}

type AccessSummary struct {
	Router   string   `json:"router"`
	Services []string `json:"services"`
	PSKSet   bool     `json:"psk_set"`
}

type RouterSummary struct {
	Name     string           `json:"name"`
	Address  string           `json:"address"`
	Hash     string           `json:"hash"`
	Deploy   DeploySummary    `json:"deploy"`
	Notify   NotifySummary    `json:"notify"`
	Defaults config.Defaults  `json:"defaults"`
	Services []ServiceSummary `json:"services"`
	Clients  []ClientSummary  `json:"clients"`
}

// NotifySummary is a secret-free view of a router's notification config: each
// channel's non-secret fields plus booleans for whether its secrets are set.
// Channels are independent — any combination may be enabled.
type NotifySummary struct {
	Active          bool   `json:"active"`
	WebhookEnabled  bool   `json:"webhook_enabled"`
	URL             string `json:"url"`
	TelegramEnabled bool   `json:"telegram_enabled"`
	TelegramChat    string `json:"telegram_chat_id"`
	BotTokenSet     bool   `json:"bot_token_set"`
	EmailEnabled    bool   `json:"email_enabled"`
	EmailTo         string `json:"email_to"`
	EmailFrom       string `json:"email_from"`
	EmailServer     string `json:"email_server"`
	EmailPort       int    `json:"email_port"`
	EmailTLS        string `json:"email_tls"`
	EmailUser       string `json:"email_user"`
	EmailPassSet    bool   `json:"email_password_set"`
}

// DeploySummary is a secret-free view of a router's SSH deploy credentials:
// non-secret connection params plus booleans for whether secrets are set.
// Configured reports whether there is enough to attempt a connection.
type DeploySummary struct {
	Address     string `json:"address"` // optional SSH override; "" → use the router address
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
		// Non-nil slices so the JSON always carries [] (never null) for the frontend.
		rs := RouterSummary{Name: rn, Address: r.Address, Hash: cfg.RouterHash(rn), Deploy: deploySummary(r), Notify: notifySummary(r.Notify), Defaults: r.Defaults, Services: []ServiceSummary{}, Clients: []ClientSummary{}}
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
			})
		}
		for _, un := range sortedKeys(cfg.Users) {
			u := cfg.Users[un]
			access, ok := u.Access[rn]
			if !ok {
				continue
			}
			rs.Clients = append(rs.Clients, ClientSummary{
				Name: un, ClientID: u.ClientID, Services: access.Services, PSKSet: access.PSK != "",
			})
		}
		s.Routers = append(s.Routers, rs)
	}
	for _, un := range sortedKeys(cfg.Users) {
		u := cfg.Users[un]
		us := UserSummary{Name: un, ClientID: u.ClientID, Access: []AccessSummary{}}
		for _, rn := range sortedKeys(u.Access) {
			access := u.Access[rn]
			svcs := access.Services
			if svcs == nil {
				svcs = []string{}
			}
			us.Access = append(us.Access, AccessSummary{Router: rn, Services: svcs, PSKSet: access.PSK != ""})
		}
		s.Users = append(s.Users, us)
	}
	return s
}

func notifySummary(n config.Notify) NotifySummary {
	return NotifySummary{
		Active:          n.Active(),
		WebhookEnabled:  n.Webhook.Enabled,
		URL:             n.Webhook.URL,
		TelegramEnabled: n.Telegram.Enabled,
		TelegramChat:    n.Telegram.ChatID,
		BotTokenSet:     n.Telegram.BotToken != "",
		EmailEnabled:    n.Email.Enabled,
		EmailTo:         n.Email.To,
		EmailFrom:       n.Email.From,
		EmailServer:     n.Email.Server,
		EmailPort:       n.Email.Port,
		EmailTLS:        n.Email.TLS,
		EmailUser:       n.Email.User,
		EmailPassSet:    n.Email.Password != "",
	}
}

func deploySummary(r config.Router) DeploySummary {
	d := r.Deploy
	return DeploySummary{
		Address:     d.Address,
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
