// Package config loads and validates siphon's config file.
package config

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Version  int                      `yaml:"version"`
	Defaults Defaults                 `yaml:"defaults"`
	Profiles map[string]ProfileConfig `yaml:"profiles"`
	Groups   map[string]GroupConfig   `yaml:"groups"`
}

type Defaults struct {
	DumpDir       string `yaml:"dump_dir"`
	Jobs          int    `yaml:"jobs"`
	Compression   int    `yaml:"compression"`
	SecretBackend string `yaml:"secret_backend"`
}

// ProfileConfig is the unresolved on-disk form of a connection profile.
// Name is NOT read from YAML — it is populated by Load() from the map key
// in Config.Profiles so callers don't need to thread the name separately.
type ProfileConfig struct {
	Name     string `yaml:"-"`
	Driver   string `yaml:"driver"`
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"` // may be a SecretRef like ${VAR} or keychain://...
	Database string `yaml:"database"`
	SSLMode  string `yaml:"sslmode"`
	Group    string `yaml:"group"`
}

type GroupConfig struct {
	Color              string `yaml:"color"`
	Require2FA         bool   `yaml:"require_2fa"`
	ConfirmDestructive bool   `yaml:"confirm_destructive"`
}

// Load reads and parses the config file. Returns an empty Config if the
// file does not exist (first-run case). Env-var interpolation (${VAR}) is
// performed BEFORE YAML parsing so values resolve to their interpolated
// form in the typed struct.
func Load() (*Config, error) {
	path := Path()
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &Config{
			Version:  1,
			Profiles: map[string]ProfileConfig{},
			Groups:   map[string]GroupConfig{},
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	expanded := os.ExpandEnv(string(raw))

	cfg := &Config{
		Profiles: map[string]ProfileConfig{},
		Groups:   map[string]GroupConfig{},
	}
	if err := yaml.Unmarshal([]byte(expanded), cfg); err != nil {
		return nil, fmt.Errorf("yaml parse: %w", err)
	}

	// Populate Name on each ProfileConfig from its map key. This lets
	// downstream code (ProfileStore.Resolve, etc.) work with ProfileConfig
	// values without losing the name when they leave the map.
	for name, p := range cfg.Profiles {
		p.Name = name
		cfg.Profiles[name] = p
	}

	return cfg, nil
}

// Save writes cfg to the configured Path, creating directories as needed.
// Note: Name is yaml:"-" so it is not serialized — the map key is the
// canonical source of a profile's name on disk.
func Save(cfg *Config) error {
	path := Path()
	if err := os.MkdirAll(parentDir(path), 0o700); err != nil {
		return fmt.Errorf("mkdir config: %w", err)
	}
	body, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("yaml marshal: %w", err)
	}
	return os.WriteFile(path, body, 0o600)
}

func parentDir(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			return path[:i]
		}
	}
	return "."
}
