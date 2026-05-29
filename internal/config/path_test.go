package config

import (
	"path/filepath"
	"testing"
)

// TestPath_AlwaysAbsolute is the regression for the ignored os.UserHomeDir
// error: even when SIPHON_CONFIG_HOME / XDG_CONFIG_HOME are unset and home
// resolution must fall back, Path() must return an absolute path — never a
// cwd-relative one like ".config/siphon/config.yaml".
func TestPath_AlwaysAbsolute(t *testing.T) {
	t.Setenv("SIPHON_CONFIG_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	got := Path()
	if !filepath.IsAbs(got) {
		t.Fatalf("Path() = %q; want an absolute path", got)
	}
}

// TestHomeDir_FallsBackWhenHomeUnset verifies homeDir never returns "" even
// when the platform home env var is cleared — it falls back to os.TempDir().
func TestHomeDir_FallsBackWhenHomeUnset(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")

	got := homeDir()
	if got == "" {
		t.Fatal("homeDir() = \"\"; want a non-empty absolute path")
	}
	if !filepath.IsAbs(got) {
		t.Fatalf("homeDir() = %q; want an absolute path", got)
	}
}
