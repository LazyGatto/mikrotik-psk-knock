package config

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Router   Router             `yaml:"router"`
	Defaults Defaults           `yaml:"defaults"`
	Services map[string]Service `yaml:"services"`
	Clients  map[string]Client  `yaml:"clients"`
}

type Router struct {
	Name    string `yaml:"name" json:"name"`
	Address string `yaml:"address" json:"address"`
}

type Defaults struct {
	BucketSeconds   int64  `yaml:"bucket_seconds" json:"bucket_seconds"`
	StageTimeout    string `yaml:"stage_timeout" json:"stage_timeout"`
	TokenHitTimeout string `yaml:"token_hit_timeout" json:"token_hit_timeout"`
	AllowedTimeout  string `yaml:"allowed_timeout" json:"allowed_timeout"`
	UsedTimeout     string `yaml:"used_timeout" json:"used_timeout"`
}

type Service struct {
	ServiceName string `yaml:"service_name"`
	Stage1Port  int    `yaml:"stage1_port"`
	Stage2Port  int    `yaml:"stage2_port"`
	TokenPort   int    `yaml:"token_port"`
	AllowedList string `yaml:"allowed_list"`
	NAT         NAT    `yaml:"nat"`
	Notify      Notify `yaml:"notify"`
}

type NAT struct {
	Enabled   bool   `yaml:"enabled" json:"enabled"`
	Comment   string `yaml:"comment" json:"comment"`
	DstPort   int    `yaml:"dst_port" json:"dst_port"`
	ToAddress string `yaml:"to_address" json:"to_address"`
	ToPort    int    `yaml:"to_port" json:"to_port"`
}

type Notify struct {
	Enabled  bool           `yaml:"enabled" json:"enabled"`
	Channel  string         `yaml:"channel" json:"channel"` // "webhook" | "telegram" | "email"
	URL      string         `yaml:"url" json:"url"`         // webhook
	Telegram NotifyTelegram `yaml:"telegram" json:"telegram"`
	Email    NotifyEmail    `yaml:"email" json:"email"`
}

type NotifyTelegram struct {
	BotToken string `yaml:"bot_token" json:"bot_token"`
	ChatID   string `yaml:"chat_id" json:"chat_id"`
}

type NotifyEmail struct {
	To       string `yaml:"to" json:"to"`
	From     string `yaml:"from" json:"from"`
	Server   string `yaml:"server" json:"server"`
	Port     int    `yaml:"port" json:"port"`
	TLS      string `yaml:"tls" json:"tls"` // "no" | "yes" | "starttls"
	User     string `yaml:"user" json:"user"`
	Password string `yaml:"password" json:"password"`
}

type Client struct {
	ClientID string `yaml:"client_id"`
	Service  string `yaml:"service"`
	PSK      string `yaml:"psk"`
}

type Resolved struct {
	Config  Config
	Client  Client
	Service Service
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
	if c.Defaults.BucketSeconds == 0 {
		c.Defaults.BucketSeconds = 30
	}
	if c.Defaults.StageTimeout == "" {
		c.Defaults.StageTimeout = "5s"
	}
	if c.Defaults.TokenHitTimeout == "" {
		c.Defaults.TokenHitTimeout = "2s"
	}
	if c.Defaults.AllowedTimeout == "" {
		c.Defaults.AllowedTimeout = "3m"
	}
	if c.Defaults.UsedTimeout == "" {
		c.Defaults.UsedTimeout = "65s"
	}
	for name, svc := range c.Services {
		if svc.ServiceName == "" {
			svc.ServiceName = name
		}
		if svc.AllowedList == "" {
			svc.AllowedList = "mkpk-tt-allowed-" + name
		}
		if svc.NAT.Comment == "" {
			svc.NAT.Comment = "mkpk-tt dst-nat " + name
		}
		if svc.Notify.Channel == "" {
			svc.Notify.Channel = "webhook"
		}
		if svc.Notify.Channel == "email" {
			if svc.Notify.Email.Port == 0 {
				svc.Notify.Email.Port = 587
			}
			if svc.Notify.Email.TLS == "" {
				svc.Notify.Email.TLS = "starttls"
			}
		}
		c.Services[name] = svc
	}
	for name, client := range c.Clients {
		if client.ClientID == "" {
			client.ClientID = name
		}
		c.Clients[name] = client
	}
}

func (c Config) Validate() error {
	if len(c.Services) == 0 {
		return fmt.Errorf("services must not be empty")
	}
	if len(c.Clients) == 0 {
		return fmt.Errorf("clients must not be empty")
	}
	if c.Defaults.BucketSeconds <= 0 {
		return fmt.Errorf("defaults.bucket_seconds must be positive")
	}
	if _, err := time.ParseDuration(c.Defaults.StageTimeout); err != nil {
		return fmt.Errorf("defaults.stage_timeout: %w", err)
	}
	if _, err := time.ParseDuration(c.Defaults.TokenHitTimeout); err != nil {
		return fmt.Errorf("defaults.token_hit_timeout: %w", err)
	}
	if _, err := time.ParseDuration(c.Defaults.AllowedTimeout); err != nil {
		return fmt.Errorf("defaults.allowed_timeout: %w", err)
	}
	usedTimeout, err := time.ParseDuration(c.Defaults.UsedTimeout)
	if err != nil {
		return fmt.Errorf("defaults.used_timeout: %w", err)
	}
	minUsedTimeout := 2 * time.Duration(c.Defaults.BucketSeconds) * time.Second
	if usedTimeout < minUsedTimeout {
		return fmt.Errorf("defaults.used_timeout must be at least %s to cover current and previous token buckets", minUsedTimeout)
	}
	for name, svc := range c.Services {
		if !isSafeName(name) {
			return fmt.Errorf("service key %q must match ^[A-Za-z0-9][A-Za-z0-9_-]*$", name)
		}
		if !isSafeName(svc.AllowedList) {
			return fmt.Errorf("service %q allowed_list %q must match ^[A-Za-z0-9][A-Za-z0-9_-]*$", name, svc.AllowedList)
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
		if err := validatePort("nat.dst_port", svc.NAT.DstPort); err != nil {
			return fmt.Errorf("service %q %w", name, err)
		}
		if err := validatePort("nat.to_port", svc.NAT.ToPort); err != nil {
			return fmt.Errorf("service %q %w", name, err)
		}
		if svc.NAT.ToAddress == "" {
			return fmt.Errorf("service %q nat.to_address is required", name)
		}
		if err := validateNotify(svc.Notify); err != nil {
			return fmt.Errorf("service %q %w", name, err)
		}
	}
	for name, client := range c.Clients {
		if !isSafeName(name) {
			return fmt.Errorf("client key %q must match ^[A-Za-z0-9][A-Za-z0-9_-]*$", name)
		}
		if client.Service == "" {
			return fmt.Errorf("client %q service is required", name)
		}
		if client.PSK == "" {
			return fmt.Errorf("client %q psk is required", name)
		}
		if !isSafePSK(client.PSK) {
			return fmt.Errorf("client %q psk must use only base64url-safe characters: A-Z, a-z, 0-9, - and _", name)
		}
		if _, ok := c.Services[client.Service]; !ok {
			return fmt.Errorf("client %q references unknown service %q", name, client.Service)
		}
	}
	return nil
}

// Hash returns a stable fingerprint of the configuration, used to detect whether
// the RouterOS side is up to date. The rendered .rsc is a deterministic function
// of the config, so hashing the marshaled config detects any drift.
func (c Config) Hash() string {
	data, err := yaml.Marshal(c)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (c Config) Resolve(clientName string) (Resolved, error) {
	client, ok := c.Clients[clientName]
	if !ok {
		return Resolved{}, fmt.Errorf("unknown client %q", clientName)
	}
	service := c.Services[client.Service]
	return Resolved{Config: c, Client: client, Service: service}, nil
}

func validateNotify(n Notify) error {
	if !n.Enabled {
		return nil
	}
	switch n.Channel {
	case "", "webhook":
		if n.URL == "" {
			return fmt.Errorf("notify.url is required for webhook channel")
		}
	case "telegram":
		if !isTelegramToken(n.Telegram.BotToken) {
			return fmt.Errorf("notify.telegram.bot_token must match ^[0-9]+:[A-Za-z0-9_-]+$")
		}
		if !isChatID(n.Telegram.ChatID) {
			return fmt.Errorf("notify.telegram.chat_id must be an integer id")
		}
	case "email":
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
	default:
		return fmt.Errorf("notify.channel %q must be webhook, telegram or email", n.Channel)
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

func validatePort(name string, port int) error {
	if port <= 0 || port > 65535 {
		return fmt.Errorf("%s must be between 1 and 65535", name)
	}
	return nil
}

func isSafeName(v string) bool {
	if v == "" {
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
