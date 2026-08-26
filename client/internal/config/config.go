package config

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// DefaultPath is the default config location for a local install:
// $XDG_CONFIG_HOME/mkpk/mkpk.yaml, else ~/.config/mkpk/mkpk.yaml. Falls back to
// ./mkpk.yaml only if the home directory can't be resolved.
func DefaultPath() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "mkpk.yaml"
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "mkpk", "mkpk.yaml")
}

// Config is the top-level admin config. Routers and users are both top-level:
// a user is a person (one client_id) who can be granted access on several
// routers. A router owns its services; who may reach them is expressed by the
// users' per-router access.
type Config struct {
	Routers map[string]Router `yaml:"routers" json:"routers"`
	Users   map[string]User   `yaml:"users" json:"users"`
}

// Router holds everything provisioned onto one MikroTik. It owns its services;
// the users allowed to reach them live at the top level and reference it.
// Notifications are per router: one channel that fires on every successful knock
// (the message carries which service/user). The alert content already names the
// service, so routing per service is unnecessary.
type Router struct {
	// Address is the public address end users knock (from untrusted networks). It
	// is the only address that goes into a client invite — no fallback. It is also
	// the default SSH deploy target unless Deploy.Address overrides it.
	Address  string             `yaml:"address" json:"address"`
	Deploy   Deploy             `yaml:"deploy,omitempty" json:"deploy"`
	Notify   Notify             `yaml:"notify,omitempty" json:"notify"`
	Defaults Defaults           `yaml:"defaults" json:"defaults"`
	Services map[string]Service `yaml:"services" json:"services"`
	// Note is free-form operator commentary, stored only in this local config —
	// never rendered to the router or included in an invite.
	Note string `yaml:"note,omitempty" json:"note"`
}

// User is a person: one client_id (a stable identity across all routers) plus
// per-router access. The PSK is per (user, router) — the router's rendered
// config contains the raw PSK, so a distinct secret per router keeps a
// single-router compromise from cascading.
type User struct {
	ClientID string                `yaml:"client_id" json:"client_id"`
	Access   map[string]UserAccess `yaml:"access" json:"access"` // router name → access
	// Note is free-form operator commentary, stored only in this local config.
	Note string `yaml:"note,omitempty" json:"note"`
}

// UserAccess is a user's grant on one router: the services they may open and
// the PSK used to derive tokens on that router.
type UserAccess struct {
	Services []string `yaml:"services" json:"services"`
	PSK      string   `yaml:"psk" json:"psk"`
}

// Deploy holds the SSH connection parameters used to provision this router.
// They belong to the router (set once when it is added/edited), not to each
// deploy action. Secrets (key_pass, password) live here alongside the config's
// other secrets; key or ssh-agent auth is preferred and stores no secret.
type Deploy struct {
	// Address optionally overrides the SSH target — e.g. deploy over a
	// management/LAN address while the public Router.Address is what clients
	// knock. Empty → deploy uses Router.Address.
	Address  string `yaml:"address,omitempty" json:"address"`
	Port     int    `yaml:"port,omitempty" json:"port"`
	User     string `yaml:"user,omitempty" json:"user"`
	KeyPath  string `yaml:"key_path,omitempty" json:"key_path"`
	KeyPass  string `yaml:"key_pass,omitempty" json:"key_pass"`
	UseAgent bool   `yaml:"use_agent,omitempty" json:"use_agent"`
	Password string `yaml:"password,omitempty" json:"password"`
}

type Defaults struct {
	BucketSeconds   int64  `yaml:"bucket_seconds" json:"bucket_seconds"`
	StageTimeout    string `yaml:"stage_timeout" json:"stage_timeout"`
	TokenHitTimeout string `yaml:"token_hit_timeout" json:"token_hit_timeout"`
	AllowedTimeout  string `yaml:"allowed_timeout" json:"allowed_timeout"`
	UsedTimeout     string `yaml:"used_timeout" json:"used_timeout"`
}

type Service struct {
	ServiceName string `yaml:"service_name" json:"service_name"`
	Disabled    bool   `yaml:"disabled" json:"disabled"` // absent → false → enabled
	Stage1Port  int    `yaml:"stage1_port" json:"stage1_port"`
	Stage2Port  int    `yaml:"stage2_port" json:"stage2_port"`
	TokenPort   int    `yaml:"token_port" json:"token_port"`
	AllowedList string `yaml:"allowed_list" json:"allowed_list"`
	// AllowedTimeout overrides how long a client stays in this service's allowed
	// list after a knock. Empty → inherit the router's defaults.allowed_timeout.
	AllowedTimeout string `yaml:"allowed_timeout,omitempty" json:"allowed_timeout"`
	Target         Target `yaml:"target" json:"target"`
	// Launch tells the GUI clients which kind of app to open after this service
	// opens ("rdp", "ssh", "http", "https", "vnc"; empty → nothing). It travels
	// in the invite as a kind, never as a command line — see invite.Service.
	Launch string `yaml:"launch,omitempty" json:"launch"`
	// Note is free-form operator commentary, stored only in this local config.
	Note string `yaml:"note,omitempty" json:"note"`
}

// LaunchKinds are the app kinds a service may ask clients to open. The client
// maps a kind to a platform-specific invocation; unknown kinds are ignored.
var LaunchKinds = []string{"rdp", "ssh", "http", "https", "vnc"}

// ValidLaunch reports whether v is empty or a known launch kind.
func ValidLaunch(v string) bool {
	if v == "" {
		return true
	}
	for _, k := range LaunchKinds {
		if v == k {
			return true
		}
	}
	return false
}

// EffectiveAllowedTimeout returns the service's allowed timeout, falling back to
// the router default when the service does not set one.
func (s Service) EffectiveAllowedTimeout(def Defaults) string {
	if s.AllowedTimeout != "" {
		return s.AllowedTimeout
	}
	return def.AllowedTimeout
}

// Target types.
const (
	TargetForward = "forward" // dst-nat to an internal host:port
	TargetLocal   = "local"   // input accept to a port on the router itself
)

// Target is what a service gates: knock adds the client to the service's
// allowed-list, and the target is the single rule that consumes that list. It is
// always present — a service without a target would gate nothing.
type Target struct {
	Type      string `yaml:"type" json:"type"`                       // "forward" | "local"
	Protocol  string `yaml:"protocol" json:"protocol"`               // "tcp" | "udp"
	Port      int    `yaml:"port" json:"port"`                       // dst-port on the router the client reaches
	ToAddress string `yaml:"to_address,omitempty" json:"to_address"` // forward only: internal host
	ToPort    int    `yaml:"to_port,omitempty" json:"to_port"`       // forward only: internal port
	Comment   string `yaml:"comment,omitempty" json:"comment"`       // stable RouterOS rule comment
}

// Notify is a router's notification config. The channels are independent — any
// combination can be enabled at once, and a successful knock fires every enabled
// one.
type Notify struct {
	Webhook  NotifyWebhook  `yaml:"webhook,omitempty" json:"webhook"`
	Telegram NotifyTelegram `yaml:"telegram,omitempty" json:"telegram"`
	Email    NotifyEmail    `yaml:"email,omitempty" json:"email"`
}

// Active reports whether any channel is enabled.
func (n Notify) Active() bool { return n.Webhook.Enabled || n.Telegram.Enabled || n.Email.Enabled }

type NotifyWebhook struct {
	Enabled bool   `yaml:"enabled" json:"enabled"`
	URL     string `yaml:"url" json:"url"`
}

type NotifyTelegram struct {
	Enabled  bool   `yaml:"enabled" json:"enabled"`
	BotToken string `yaml:"bot_token" json:"bot_token"`
	ChatID   string `yaml:"chat_id" json:"chat_id"`
	// ThreadID targets a forum-supergroup topic (Bot API message_thread_id).
	// Empty → the group's General topic / a plain chat.
	ThreadID string `yaml:"thread_id,omitempty" json:"thread_id"`
}

type NotifyEmail struct {
	Enabled  bool   `yaml:"enabled" json:"enabled"`
	To       string `yaml:"to" json:"to"`
	From     string `yaml:"from" json:"from"`
	Server   string `yaml:"server" json:"server"`
	Port     int    `yaml:"port" json:"port"`
	TLS      string `yaml:"tls" json:"tls"` // "no" | "yes" | "starttls"
	User     string `yaml:"user" json:"user"`
	Password string `yaml:"password" json:"password"`
}

// RenderClient is the per-router projection of a user used by the renderer and
// the router hash: the identity, the PSK for this router, and the services the
// user may open on it.
type RenderClient struct {
	Name     string   `yaml:"name"`      // user map key, used for RouterOS object names
	ClientID string   `yaml:"client_id"` // stable identity in the token
	PSK      string   `yaml:"psk"`
	Services []string `yaml:"services"` // service names on this router
}

// Resolved is a single (router, user, service) triple used by token/knock.
type Resolved struct {
	RouterName  string
	Router      Router
	UserName    string
	ClientID    string
	PSK         string
	ServiceName string
	Service     Service
}

// Enabled reports whether the service is active (rendered/deployed).
func (s Service) Enabled() bool { return !s.Disabled }

// LoadOrEmpty behaves like Load, except a missing file yields an empty (valid)
// config instead of an error. This is the first-run state for the web/desktop
// UI: it shows onboarding until the first router is added and the file is written.
func LoadOrEmpty(path string) (Config, error) {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return Config{Routers: map[string]Router{}, Users: map[string]User{}}, nil
	}
	return Load(path)
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	cfg.applyDefaults()
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c *Config) applyDefaults() {
	for name, r := range c.Routers {
		r.applyDefaults()
		c.Routers[name] = r
	}
	c.applyUserDefaults()
}

func (r *Router) applyDefaults() {
	if r.Defaults.BucketSeconds == 0 {
		r.Defaults.BucketSeconds = 30
	}
	if r.Defaults.StageTimeout == "" {
		r.Defaults.StageTimeout = "5s"
	}
	if r.Defaults.TokenHitTimeout == "" {
		r.Defaults.TokenHitTimeout = "2s"
	}
	if r.Defaults.AllowedTimeout == "" {
		r.Defaults.AllowedTimeout = "3m"
	}
	if r.Defaults.UsedTimeout == "" {
		r.Defaults.UsedTimeout = "65s"
	}
	if r.Notify.Email.Enabled {
		if r.Notify.Email.Port == 0 {
			r.Notify.Email.Port = 587
		}
		if r.Notify.Email.TLS == "" {
			r.Notify.Email.TLS = "starttls"
		}
	}
	for name, svc := range r.Services {
		if svc.ServiceName == "" {
			svc.ServiceName = name
		}
		if svc.AllowedList == "" {
			svc.AllowedList = "mkpk-tt-allowed-" + name
		}
		if svc.Target.Type == "" {
			svc.Target.Type = TargetForward
		}
		if svc.Target.Protocol == "" {
			svc.Target.Protocol = "tcp"
		}
		if svc.Target.Comment == "" {
			svc.Target.Comment = "mkpk-tt target " + name
		}
		r.Services[name] = svc
	}
}

func (c *Config) applyUserDefaults() {
	for name, u := range c.Users {
		if u.ClientID == "" {
			u.ClientID = name
		}
		c.Users[name] = u
	}
}

func (c Config) Validate() error {
	// An empty config is a legitimate state: a fresh install has no routers, and
	// removing the last one must be allowed. Commands that need a router say so
	// themselves (pickRouter, render, deploy).

	for name, r := range c.Routers {
		if !isSafeName(name) {
			return fmt.Errorf("router key %q must match ^[A-Za-z0-9][A-Za-z0-9_-]*$", name)
		}
		if err := r.Validate(); err != nil {
			return fmt.Errorf("router %q %w", name, err)
		}
	}
	return c.validateUsers()
}

func (r Router) Validate() error {
	if len(r.Note) > maxNoteLen {
		return fmt.Errorf("note must be at most %d characters", maxNoteLen)
	}
	if r.Address == "" {
		return fmt.Errorf("address is required")
	}
	if !isHostOrIP(r.Address) {
		return fmt.Errorf("address %q must be an IPv4 address or a hostname", r.Address)
	}
	if r.Deploy.Address != "" && !isHostOrIP(r.Deploy.Address) {
		return fmt.Errorf("deploy.address %q must be an IPv4 address or a hostname", r.Deploy.Address)
	}
	if r.Defaults.BucketSeconds <= 0 {
		return fmt.Errorf("defaults.bucket_seconds must be positive")
	}
	if _, err := time.ParseDuration(r.Defaults.StageTimeout); err != nil {
		return fmt.Errorf("defaults.stage_timeout: %w", err)
	}
	if _, err := time.ParseDuration(r.Defaults.TokenHitTimeout); err != nil {
		return fmt.Errorf("defaults.token_hit_timeout: %w", err)
	}
	if _, err := time.ParseDuration(r.Defaults.AllowedTimeout); err != nil {
		return fmt.Errorf("defaults.allowed_timeout: %w", err)
	}
	usedTimeout, err := time.ParseDuration(r.Defaults.UsedTimeout)
	if err != nil {
		return fmt.Errorf("defaults.used_timeout: %w", err)
	}
	minUsedTimeout := 2 * time.Duration(r.Defaults.BucketSeconds) * time.Second
	if usedTimeout < minUsedTimeout {
		return fmt.Errorf("defaults.used_timeout must be at least %s to cover current and previous token buckets", minUsedTimeout)
	}
	if r.Deploy.Port != 0 {
		if err := validatePort("deploy.port", r.Deploy.Port); err != nil {
			return err
		}
	}
	if err := validateNotify(r.Notify); err != nil {
		return err
	}
	for name, svc := range r.Services {
		if !isSafeName(name) {
			return fmt.Errorf("service name %q must match ^[A-Za-z0-9][A-Za-z0-9_-]*$ (max %d chars)", name, maxNameLen)
		}
		if !isSafeName(svc.AllowedList) {
			return fmt.Errorf("service %q allowed_list %q must match ^[A-Za-z0-9][A-Za-z0-9_-]*$", name, svc.AllowedList)
		}
		if len(svc.Note) > maxNoteLen {
			return fmt.Errorf("service %q note must be at most %d characters", name, maxNoteLen)
		}
		if !ValidLaunch(svc.Launch) {
			return fmt.Errorf("service %q launch %q must be empty or one of %v", name, svc.Launch, LaunchKinds)
		}
		if svc.AllowedTimeout != "" {
			if _, err := time.ParseDuration(svc.AllowedTimeout); err != nil {
				return fmt.Errorf("service %q allowed_timeout: %w", name, err)
			}
		}
		if err := validatePort("stage1_port", svc.Stage1Port); err != nil {
			return fmt.Errorf("service %q %w", name, err)
		}
		if err := validatePort("stage2_port", svc.Stage2Port); err != nil {
			return fmt.Errorf("service %q %w", name, err)
		}
		if err := validatePort("token_port", svc.TokenPort); err != nil {
			return fmt.Errorf("service %q %w", name, err)
		}
		if svc.Stage1Port == svc.Stage2Port || svc.Stage1Port == svc.TokenPort || svc.Stage2Port == svc.TokenPort {
			return fmt.Errorf("service %q stage1_port, stage2_port and token_port must be distinct", name)
		}
		if err := validateTarget(svc.Target); err != nil {
			return fmt.Errorf("service %q %w", name, err)
		}
	}
	if err := r.checkPortCollisions(); err != nil {
		return err
	}
	return nil
}

// validateUsers checks the top-level users and their per-router access against
// the routers and services they reference.
func (c Config) validateUsers() error {
	for name, u := range c.Users {
		if !isSafeName(name) {
			return fmt.Errorf("user key %q must match ^[A-Za-z0-9][A-Za-z0-9_-]*$", name)
		}
		if !isSafeName(u.ClientID) {
			return fmt.Errorf("user %q client_id %q must match ^[A-Za-z0-9][A-Za-z0-9_-]*$", name, u.ClientID)
		}
		if len(u.Note) > maxNoteLen {
			return fmt.Errorf("user %q note must be at most %d characters", name, maxNoteLen)
		}
		for rn, access := range u.Access {
			router, ok := c.Routers[rn]
			if !ok {
				return fmt.Errorf("user %q references unknown router %q", name, rn)
			}
			if access.PSK == "" {
				return fmt.Errorf("user %q access to router %q requires a psk", name, rn)
			}
			if !isSafePSK(access.PSK) {
				return fmt.Errorf("user %q psk for router %q must use only base64url-safe characters", name, rn)
			}
			for _, svc := range access.Services {
				if _, ok := router.Services[svc]; !ok {
					return fmt.Errorf("user %q references unknown service %q on router %q", name, svc, rn)
				}
			}
		}
	}
	return nil
}

// RenderClients returns, for a router, the per-router projection of every user
// with access to it: one RenderClient per (user × router) grant. Deterministic
// order by user name.
func (c Config) RenderClients(routerName string) []RenderClient {
	var out []RenderClient
	for _, name := range sortedStringKeys(c.Users) {
		u := c.Users[name]
		access, ok := u.Access[routerName]
		if !ok || len(access.Services) == 0 {
			continue
		}
		out = append(out, RenderClient{
			Name:     name,
			ClientID: u.ClientID,
			PSK:      access.PSK,
			Services: append([]string(nil), access.Services...),
		})
	}
	return out
}

// RenderHash fingerprints exactly what a router's .rsc depends on: the router
// (minus SSH deploy creds) and the per-router user projection. Both the renderer
// and drift detection use it, so a change to a user's access or PSK on this
// router is detected as drift.
func RenderHash(r Router, clients []RenderClient) string {
	// Connection/identity metadata does not affect the rendered .rsc, so changing
	// it must not read as drift: exclude SSH creds, the address and local notes.
	r.Deploy = Deploy{}
	r.Address = ""
	r.Note = ""
	// Service notes are local-only too; rebuild the map (can't mutate map values
	// in place) with notes cleared so a note edit is not seen as drift.
	if r.Services != nil {
		svcs := make(map[string]Service, len(r.Services))
		for k, s := range r.Services {
			s.Note = ""
			svcs[k] = s
		}
		r.Services = svcs
	}
	payload := struct {
		Router  Router
		Clients []RenderClient
	}{r, clients}
	data, err := yaml.Marshal(payload)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// RouterHash is the RenderHash of a router within this config.
func (c Config) RouterHash(routerName string) string {
	return RenderHash(c.Routers[routerName], c.RenderClients(routerName))
}

// Router returns the named router.
func (c Config) Router(name string) (Router, bool) {
	r, ok := c.Routers[name]
	return r, ok
}

// Resolve locates a (user, router, service) triple. serviceName may be empty
// when the user has exactly one service on that router.
func (c Config) Resolve(userName, routerName, serviceName string) (Resolved, error) {
	u, ok := c.Users[userName]
	if !ok {
		return Resolved{}, fmt.Errorf("unknown user %q", userName)
	}
	router, ok := c.Routers[routerName]
	if !ok {
		return Resolved{}, fmt.Errorf("unknown router %q", routerName)
	}
	access, ok := u.Access[routerName]
	if !ok {
		return Resolved{}, fmt.Errorf("user %q has no access to router %q", userName, routerName)
	}
	if serviceName == "" {
		switch len(access.Services) {
		case 1:
			serviceName = access.Services[0]
		case 0:
			return Resolved{}, fmt.Errorf("user %q has no services on router %q", userName, routerName)
		default:
			return Resolved{}, fmt.Errorf("user %q has multiple services on router %q; specify one of %v", userName, routerName, access.Services)
		}
	}
	if !slices.Contains(access.Services, serviceName) {
		return Resolved{}, fmt.Errorf("user %q is not assigned service %q on router %q", userName, serviceName, routerName)
	}
	svc, ok := router.Services[serviceName]
	if !ok {
		return Resolved{}, fmt.Errorf("unknown service %q on router %q", serviceName, routerName)
	}
	return Resolved{
		RouterName: routerName, Router: router,
		UserName: userName, ClientID: u.ClientID, PSK: access.PSK,
		ServiceName: serviceName, Service: svc,
	}, nil
}

func sortedStringKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// ports returns every dst-port a service occupies: its three knock ports and
// its target port. Zero values (unset) are included and filtered by callers.
func (s Service) ports() []int {
	return []int{s.Stage1Port, s.Stage2Port, s.TokenPort, s.Target.Port}
}

// UsedPorts is the set of all non-zero ports occupied by the router's services.
// It is the basis for suggesting free knock ports.
func (r Router) UsedPorts() map[int]bool {
	used := map[int]bool{}
	for _, svc := range r.Services {
		for _, p := range svc.ports() {
			if p != 0 {
				used[p] = true
			}
		}
	}
	return used
}

// checkPortCollisions rejects any port shared by two services: knock ports and
// target ports must all be distinct within a router.
func (r Router) checkPortCollisions() error {
	type owner struct{ svc, field string }
	seen := map[int]owner{}
	names := make([]string, 0, len(r.Services))
	for name := range r.Services {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		svc := r.Services[name]
		fields := []struct {
			field string
			port  int
		}{
			{"stage1_port", svc.Stage1Port},
			{"stage2_port", svc.Stage2Port},
			{"token_port", svc.TokenPort},
			{"target.port", svc.Target.Port},
		}
		for _, f := range fields {
			if f.port == 0 {
				continue
			}
			if prev, dup := seen[f.port]; dup {
				return fmt.Errorf("port %d is used by both service %q (%s) and service %q (%s)", f.port, prev.svc, prev.field, name, f.field)
			}
			seen[f.port] = owner{name, f.field}
		}
	}
	return nil
}

func validateTarget(t Target) error {
	switch t.Protocol {
	case "tcp", "udp":
	default:
		return fmt.Errorf("target.protocol %q must be tcp or udp", t.Protocol)
	}
	if err := validatePort("target.port", t.Port); err != nil {
		return err
	}
	switch t.Type {
	case TargetForward:
		if t.ToAddress == "" {
			return fmt.Errorf("target.to_address is required for a forward target")
		}
		if ip := net.ParseIP(t.ToAddress); ip == nil || ip.To4() == nil {
			return fmt.Errorf("target.to_address %q must be a literal IPv4 address (RouterOS dst-nat)", t.ToAddress)
		}
		if err := validatePort("target.to_port", t.ToPort); err != nil {
			return err
		}
	case TargetLocal:
		if t.ToAddress != "" || t.ToPort != 0 {
			return fmt.Errorf("target.to_address/to_port must be empty for a local target")
		}
	default:
		return fmt.Errorf("target.type %q must be %q or %q", t.Type, TargetForward, TargetLocal)
	}
	return nil
}

// validateNotify checks each enabled channel independently — several may be on
// at once.
func validateNotify(n Notify) error {
	if n.Webhook.Enabled && n.Webhook.URL == "" {
		return fmt.Errorf("notify.webhook.url is required when the webhook channel is enabled")
	}
	if n.Telegram.Enabled {
		if !isTelegramToken(n.Telegram.BotToken) {
			return fmt.Errorf("notify.telegram.bot_token must match ^[0-9]+:[A-Za-z0-9_-]+$")
		}
		if !isChatID(n.Telegram.ChatID) {
			return fmt.Errorf("notify.telegram.chat_id must be an integer id")
		}
		if n.Telegram.ThreadID != "" && !isDigits(n.Telegram.ThreadID) {
			return fmt.Errorf("notify.telegram.thread_id must be a positive integer (forum topic id)")
		}
	}
	if n.Email.Enabled {
		if !isEmailAddr(n.Email.To) {
			return fmt.Errorf("notify.email.to must be an email address")
		}
		if !isEmailAddr(n.Email.From) {
			return fmt.Errorf("notify.email.from must be an email address")
		}
		if n.Email.Server == "" {
			return fmt.Errorf("notify.email.server is required")
		}
		if n.Email.Port <= 0 || n.Email.Port > 65535 {
			return fmt.Errorf("notify.email.port must be between 1 and 65535")
		}
		switch n.Email.TLS {
		case "no", "yes", "starttls":
		default:
			return fmt.Errorf("notify.email.tls must be no, yes or starttls")
		}
	}
	return nil
}

func isEmailAddr(v string) bool {
	at := -1
	for i, r := range v {
		if r == '@' {
			if at != -1 {
				return false
			}
			at = i
		}
		if r == ' ' || r == '"' || r == '\n' || r == '\r' {
			return false
		}
	}
	return at > 0 && at < len(v)-1
}

func isTelegramToken(v string) bool {
	colon := -1
	for i, r := range v {
		if r == ':' {
			colon = i
			break
		}
		if r < '0' || r > '9' {
			return false
		}
	}
	if colon <= 0 || colon == len(v)-1 {
		return false
	}
	for _, r := range v[colon+1:] {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		return false
	}
	return true
}

func isDigits(v string) bool {
	for _, r := range v {
		if r < '0' || r > '9' {
			return false
		}
	}
	return v != ""
}

func isChatID(v string) bool {
	if v == "" {
		return false
	}
	for i, r := range v {
		if r == '-' && i == 0 {
			continue
		}
		if r < '0' || r > '9' {
			return false
		}
	}
	return v != "-"
}

// isHostOrIP reports whether s is an IP literal or a syntactically valid
// hostname — what a router's public/deploy address may be.
func isHostOrIP(s string) bool {
	if net.ParseIP(s) != nil {
		return true
	}
	return isHostname(s)
}

// isHostname validates a DNS hostname: 1..253 chars of dot-separated labels,
// each 1..63 chars of [A-Za-z0-9-] not starting or ending with a hyphen.
func isHostname(s string) bool {
	if len(s) == 0 || len(s) > 253 {
		return false
	}
	for label := range strings.SplitSeq(s, ".") {
		n := len(label)
		if n == 0 || n > 63 || label[0] == '-' || label[n-1] == '-' {
			return false
		}
		for i := range n {
			c := label[i]
			if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-') {
				return false
			}
		}
	}
	return true
}

func validatePort(name string, port int) error {
	if port <= 0 || port > 65535 {
		return fmt.Errorf("%s must be between 1 and 65535", name)
	}
	return nil
}

// maxNameLen caps router/service/user names. They compose into RouterOS object
// names (e.g. mkpk-tt-hit-now-<user>-<service>), so keep each part short.
const maxNameLen = 32

// maxNoteLen caps the free-form local note on any entity.
const maxNoteLen = 1000

func isSafeName(v string) bool {
	if v == "" || len(v) > maxNameLen {
		return false
	}
	for i, r := range v {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			continue
		case (r == '-' || r == '_') && i > 0:
			continue
		default:
			return false
		}
	}
	return true
}

func isSafePSK(v string) bool {
	for _, r := range v {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		return false
	}
	return true
}
