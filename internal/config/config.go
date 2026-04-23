package config

import (
	"fmt"
	"os"

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
	Disable     []string    `yaml:"disable"`      // optional list of capabilities to disable, e.g. ["docker"]
}

// Disabled reports whether the named capability is disabled for this backend.
func (b Backend) Disabled(capability string) bool {
	for _, d := range b.Disable {
		if d == capability {
			return true
		}
	}
	return false
}

// Config is the top-level configuration.
type Config struct {
	Backends []Backend `yaml:"backends"`
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
