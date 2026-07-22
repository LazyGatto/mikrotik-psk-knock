package config

import (
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
	Name    string `yaml:"name"`
	Address string `yaml:"address"`
}

type Defaults struct {
	BucketSeconds   int64  `yaml:"bucket_seconds"`
	StageTimeout    string `yaml:"stage_timeout"`
	TokenHitTimeout string `yaml:"token_hit_timeout"`
	AllowedTimeout  string `yaml:"allowed_timeout"`
	UsedTimeout     string `yaml:"used_timeout"`
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
	Enabled   bool   `yaml:"enabled"`
	Comment   string `yaml:"comment"`
	DstPort   int    `yaml:"dst_port"`
	ToAddress string `yaml:"to_address"`
	ToPort    int    `yaml:"to_port"`
}

type Notify struct {
	Enabled bool   `yaml:"enabled"`
	URL     string `yaml:"url"`
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
		c.Defaults.UsedTimeout = "35s"
	}
	for name, svc := range c.Services {
		if svc.ServiceName == "" {
			svc.ServiceName = name
		}
		if svc.AllowedList == "" {
			svc.AllowedList = "mkpk-tt-allowed"
		}
		if svc.NAT.Comment == "" {
			svc.NAT.Comment = "mkpk-tt dst-nat " + name
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
	if _, err := time.ParseDuration(c.Defaults.UsedTimeout); err != nil {
		return fmt.Errorf("defaults.used_timeout: %w", err)
	}
	for name, svc := range c.Services {
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
	}
	for name, client := range c.Clients {
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

func (c Config) Resolve(clientName string) (Resolved, error) {
	client, ok := c.Clients[clientName]
	if !ok {
		return Resolved{}, fmt.Errorf("unknown client %q", clientName)
	}
	service := c.Services[client.Service]
	return Resolved{Config: c, Client: client, Service: service}, nil
}

func validatePort(name string, port int) error {
	if port <= 0 || port > 65535 {
		return fmt.Errorf("%s must be between 1 and 65535", name)
	}
	return nil
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
