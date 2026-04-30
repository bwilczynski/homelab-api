package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// BackendType identifies the kind of backend.
type BackendType string

const (
	BackendTypeSynology BackendType = "synology"
	BackendTypeUniFi    BackendType = "unifi"
)

// Backend describes a single backend target.
type Backend struct {
	Name        string      `yaml:"name"`
	Type        BackendType `yaml:"type"`
	Host        string      `yaml:"host"`
	Username    string      `yaml:"username"`
	Password    string      `yaml:"password"`
	AuthVersion string      `yaml:"auth_version"` // optional; Synology only — overrides the auto-discovered SYNO.API.Auth version
	InsecureTLS bool        `yaml:"insecure_tls"` // optional; skip TLS certificate verification (defaults to false)
}

// Auth holds JWT/JWKS authorization settings.
type Auth struct {
	Enabled  bool   `yaml:"enabled"`
	Issuer   string `yaml:"issuer"`
	JWKSURL  string `yaml:"jwks_url"`
	Audience string `yaml:"audience"`
}

// ImageSourceConfig maps a container image (without tag) to its GitHub release source.
// Containers with version tags are discovered automatically from running backends;
// this config provides the GitHub repo to use when checking for newer versions.
type ImageSourceConfig struct {
	Image  string `yaml:"image"`  // image reference without tag, e.g. "ghcr.io/dani-garcia/vaultwarden"
	Source string `yaml:"source"` // GitHub repo "owner/repo", e.g. "dani-garcia/vaultwarden"
}

// UpdatesConfig holds configuration for the software update tracking feature.
type UpdatesConfig struct {
	Sources       []ImageSourceConfig `yaml:"sources"`        // image → GitHub source mappings
	CheckInterval Duration            `yaml:"check_interval"` // how often to refresh from upstream (default: 1h)
}

// Duration wraps time.Duration to support YAML unmarshalling from strings like "30m" or "2h".
type Duration struct {
	time.Duration
}

func (d *Duration) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}
	dur, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", s, err)
	}
	d.Duration = dur
	return nil
}

// Config is the top-level configuration.
type Config struct {
	Auth     Auth          `yaml:"auth"`
	Backends []Backend     `yaml:"backends"`
	Updates  UpdatesConfig `yaml:"updates"`
}

// Load reads and parses a YAML config file. Values containing ${VAR}
// references are expanded from environment variables.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	expanded := os.ExpandEnv(string(data))

	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &cfg, nil
}

func (c *Config) validate() error {
	if c.Auth.Enabled {
		if c.Auth.Issuer == "" {
			return fmt.Errorf("auth.issuer is required when auth is enabled")
		}
		if c.Auth.JWKSURL == "" {
			return fmt.Errorf("auth.jwks_url is required when auth is enabled")
		}
	}

	if len(c.Backends) == 0 {
		return fmt.Errorf("no backends configured")
	}

	seen := make(map[string]bool, len(c.Backends))
	for i, b := range c.Backends {
		if b.Name == "" {
			return fmt.Errorf("backend[%d]: name is required", i)
		}
		if seen[b.Name] {
			return fmt.Errorf("backend[%d]: duplicate name %q", i, b.Name)
		}
		seen[b.Name] = true

		switch b.Type {
		case BackendTypeSynology, BackendTypeUniFi:
		default:
			return fmt.Errorf("backend %q: unknown type %q", b.Name, b.Type)
		}

		if b.Host == "" {
			return fmt.Errorf("backend %q: host is required", b.Name)
		}
		if b.Username == "" {
			return fmt.Errorf("backend %q: username is required", b.Name)
		}
		if b.Password == "" {
			return fmt.Errorf("backend %q: password is required", b.Name)
		}
	}

	return nil
}

// ByType returns all backends of the given type.
func (c *Config) ByType(t BackendType) []Backend {
	var result []Backend
	for _, b := range c.Backends {
		if b.Type == t {
			result = append(result, b)
		}
	}
	return result
}
