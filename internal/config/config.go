// Package config loads and validates siphon's config file.
package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Version   int                      `yaml:"version"`
	Defaults  Defaults                 `yaml:"defaults"`
	Storage   StorageConfig            `yaml:"storage"`
	Audit     AuditConfig              `yaml:"audit"`
	Telemetry TelemetryConfig          `yaml:"telemetry"`
	Secrets   SecretsConfig            `yaml:"secrets"`
	Profiles  map[string]ProfileConfig `yaml:"profiles"`
	Groups    map[string]GroupConfig   `yaml:"groups"`
}

// AuditConfig controls the append-only audit log of destructive operations.
// Disabled by default; Path defaults to <state>/siphon/audit.log when empty.
type AuditConfig struct {
	Enabled bool   `yaml:"enabled"`
	Path    string `yaml:"path,omitempty"`
}

// SecretsConfig enables optional secret backends. The keychain:// backend is
// always available (no config, no network). AWS Secrets Manager (awssm://) is
// gated by AWSSM because constructing it loads AWS config; off by default so a
// machine without AWS creds doesn't pay that cost or fail at startup.
type SecretsConfig struct {
	AWSSM       bool   `yaml:"awssm"`                  // enable the awssm:// backend
	AWSSMRegion string `yaml:"awssm_region,omitempty"` // optional; defaults to the AWS chain's region
}

// TelemetryConfig controls opt-in aggregate operational metrics (per-op counts
// and error tallies — never identifying data). Disabled by default; Path
// defaults to <state>/siphon/telemetry.json when empty.
type TelemetryConfig struct {
	Enabled bool   `yaml:"enabled"`
	Path    string `yaml:"path,omitempty"`
}

type Defaults struct {
	DumpDir       string           `yaml:"dump_dir"`
	Jobs          int              `yaml:"jobs"`
	Compression   int              `yaml:"compression"`
	SecretBackend string           `yaml:"secret_backend"`
	Retention     *RetentionConfig `yaml:"retention,omitempty"`
}

// RetentionConfig is the YAML shape of a retention policy. It lives in config
// (not dumps) so config stays a leaf; the app layer maps it to a
// dumps.RetentionPolicy. A nil pointer (block omitted) means "keep everything".
// A profile's retention block REPLACES the defaults block wholesale — it is not
// field-merged — so a profile's policy is read as a single coherent rule set.
type RetentionConfig struct {
	KeepLast int       `yaml:"keep_last,omitempty"` // keep the N newest chains
	MaxAge   string    `yaml:"max_age,omitempty"`   // Go duration string, e.g. "720h"
	GFS      GFSConfig `yaml:"gfs,omitempty"`       // grandfather-father-son tiers
}

// GFSConfig is the YAML shape of a grandfather-father-son rule.
type GFSConfig struct {
	Daily   int `yaml:"daily,omitempty"`
	Weekly  int `yaml:"weekly,omitempty"`
	Monthly int `yaml:"monthly,omitempty"`
}

// Validate rejects nonsensical retention settings (negative counts, an
// unparseable duration). An all-zero policy is valid and means "keep
// everything", so an empty or misconfigured block can never wipe the catalog.
func (r *RetentionConfig) Validate() error {
	if r == nil {
		return nil
	}
	if r.KeepLast < 0 {
		return fmt.Errorf("retention.keep_last must be >= 0, got %d", r.KeepLast)
	}
	if r.GFS.Daily < 0 || r.GFS.Weekly < 0 || r.GFS.Monthly < 0 {
		return errors.New("retention.gfs tiers must be >= 0")
	}
	if r.MaxAge != "" {
		d, err := time.ParseDuration(r.MaxAge)
		if err != nil {
			return fmt.Errorf("retention.max_age %q is not a valid duration: %w", r.MaxAge, err)
		}
		// time.ParseDuration accepts signed inputs ("-1h"); a negative max-age
		// would push the cutoff into the future and make the whole catalog
		// eligible for deletion, so reject it.
		if d < 0 {
			return fmt.Errorf("retention.max_age must be >= 0, got %q", r.MaxAge)
		}
	}
	return nil
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
	t := strings.TrimSpace(s.Type)
	switch t {
	case "", "local":
		return nil
	case "s3":
		if strings.TrimSpace(s.Bucket) == "" {
			return errors.New("storage.type is s3 but storage.bucket is empty")
		}
		return nil
	default:
		return fmt.Errorf("unknown storage.type %q (want \"local\" or \"s3\")", t)
	}
}

// ProfileConfig is the unresolved on-disk form of a connection profile.
// Name is NOT read from YAML — it is populated by Load() from the map key
// in Config.Profiles so callers don't need to thread the name separately.
type ProfileConfig struct {
	Name      string           `yaml:"-"`
	Driver    string           `yaml:"driver"`
	Host      string           `yaml:"host"`
	Port      int              `yaml:"port"`
	User      string           `yaml:"user"`
	Password  string           `yaml:"password"` // may be a SecretRef like ${VAR} or keychain://...
	Database  string           `yaml:"database"`
	SSLMode   string           `yaml:"sslmode"`
	Group     string           `yaml:"group"`
	Retention *RetentionConfig `yaml:"retention,omitempty"` // overrides Defaults.Retention wholesale
	Tunnel    *TunnelConfig    `yaml:"tunnel,omitempty"`    // optional SSH bastion for reaching this DB
}

// TunnelConfig describes an SSH bastion through which this profile's database is
// reached. `siphon tunnel <profile>` opens an `ssh -L` local forward from
// LocalPort to the profile's Host:Port via Bastion, using the system ssh client
// (so the user's ssh config, keys, and agent apply). It is delegation, not a
// reimplementation of SSH.
type TunnelConfig struct {
	Bastion   string `yaml:"bastion"`              // [user@]host[:port] of the SSH jump host
	LocalPort int    `yaml:"local_port,omitempty"` // local forward port (defaults to the DB port)
}

type GroupConfig struct {
	Color              string `yaml:"color"`
	Require2FA         bool   `yaml:"require_2fa"`
	ConfirmDestructive bool   `yaml:"confirm_destructive"`
	// TOTPSecret is the base32 RFC-6238 secret shared with the operator's
	// authenticator app, consulted when Require2FA is set. It is a secret-ref
	// (e.g. env:SIPHON_PROD_TOTP), so the plaintext secret never lives in config.
	TOTPSecret string `yaml:"totp_secret,omitempty"`
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
	if err := cfg.Defaults.Retention.Validate(); err != nil {
		return nil, fmt.Errorf("config defaults.retention: %w", err)
	}
	for name, p := range cfg.Profiles {
		if err := p.Retention.Validate(); err != nil {
			return nil, fmt.Errorf("config profile %q retention: %w", name, err)
		}
	}

	return cfg, nil
}

// EffectiveRetention returns the RetentionConfig for a profile: its own block if
// present, else the defaults block, else nil (keep everything). The profile
// block replaces the defaults wholesale — it is not merged.
func (c *Config) EffectiveRetention(profile string) *RetentionConfig {
	if p, ok := c.Profiles[profile]; ok && p.Retention != nil {
		return p.Retention
	}
	return c.Defaults.Retention
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
