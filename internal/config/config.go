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
	Storage  StorageConfig            `yaml:"storage"`
	Profiles map[string]ProfileConfig `yaml:"profiles"`
	Groups   map[string]GroupConfig   `yaml:"groups"`
}

type Defaults struct {
	DumpDir       string `yaml:"dump_dir"`
	Jobs          int    `yaml:"jobs"`
	Compression   int    `yaml:"compression"`
	SecretBackend string `yaml:"secret_backend"`
}

// StorageConfig selects where the dump catalog physically lives. Type "local"
// (or empty, the default) uses Defaults.DumpDir on the local filesystem. Type
// "s3" stores dumps in an S3 or S3-compatible bucket. Credentials are NOT held
// here — the S3 SDK resolves them from the standard chain (env vars, shared
// config, instance role), so the config file stays free of secrets.
type StorageConfig struct {
	Type     string `yaml:"type"`               // "local" | "s3" (default "local")
	Bucket   string `yaml:"bucket,omitempty"`   // s3: target bucket
	Prefix   string `yaml:"prefix,omitempty"`   // s3: optional key prefix within the bucket
	Region   string `yaml:"region,omitempty"`   // s3: AWS region
	Endpoint string `yaml:"endpoint,omitempty"` // s3: custom endpoint for S3-compatible services (MinIO, R2)
}

// Validate checks the storage block for internal consistency. It is called by
// Load so a malformed storage config fails fast with a clear message rather
// than surfacing as an obscure runtime error on first backup.
func (s StorageConfig) Validate() error {
	switch s.Type {
	case "", "local":
		return nil
	case "s3":
		if s.Bucket == "" {
			return errors.New("storage.type is s3 but storage.bucket is empty")
		}
		return nil
	default:
		return fmt.Errorf("unknown storage.type %q (want \"local\" or \"s3\")", s.Type)
	}
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

	if err := cfg.Storage.Validate(); err != nil {
		return nil, fmt.Errorf("config storage: %w", err)
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
