package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad_MissingFile_ReturnsEmptyConfig(t *testing.T) {
	t.Setenv("SIPHON_CONFIG_HOME", t.TempDir())
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if cfg == nil || len(cfg.Profiles) != 0 {
		t.Fatalf("Load() returned %+v; want empty config", cfg)
	}
}

func TestLoad_ValidYAML_Parses(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SIPHON_CONFIG_HOME", dir)
	t.Setenv("PROD_PASS", "hunter2")

	body := `version: 1
defaults:
  dump_dir: ~/dumps
  jobs: 4
profiles:
  prod:
    driver: postgres
    host: db.example.com
    port: 5432
    user: app
    password: ${PROD_PASS}
    database: app_prod
    sslmode: require
`
	mustWrite(t, filepath.Join(dir, "config.yaml"), body)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	prod, ok := cfg.Profiles["prod"]
	if !ok {
		t.Fatalf("expected profile 'prod' in %+v", cfg.Profiles)
	}
	if prod.Password != "hunter2" {
		t.Fatalf("password = %q; want 'hunter2' (env interpolation)", prod.Password)
	}
	if cfg.Defaults.Jobs != 4 {
		t.Fatalf("Defaults.Jobs = %d; want 4", cfg.Defaults.Jobs)
	}
	if prod.Name != "prod" {
		t.Fatalf("Name = %q; want 'prod' (populated from map key)", prod.Name)
	}
}

func TestLoad_InvalidYAML_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SIPHON_CONFIG_HOME", dir)
	mustWrite(t, filepath.Join(dir, "config.yaml"), "this is: : not yaml")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() returned no error on invalid YAML")
	}
	if !strings.Contains(err.Error(), "yaml") {
		t.Fatalf("error %q does not mention yaml", err)
	}
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}
