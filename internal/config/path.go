package config

import (
	"os"
	"path/filepath"
	"runtime"
)

// Path returns the absolute path to siphon's config file. Honors
// $SIPHON_CONFIG_HOME first, then $XDG_CONFIG_HOME, then the OS default.
func Path() string {
	if v := os.Getenv("SIPHON_CONFIG_HOME"); v != "" {
		return filepath.Join(v, "config.yaml")
	}
	if v := os.Getenv("XDG_CONFIG_HOME"); v != "" {
		return filepath.Join(v, "siphon", "config.yaml")
	}
	home := homeDir()
	switch runtime.GOOS {
	case "windows":
		if v := os.Getenv("APPDATA"); v != "" {
			return filepath.Join(v, "siphon", "config.yaml")
		}
		return filepath.Join(home, "AppData", "Roaming", "siphon", "config.yaml")
	default:
		return filepath.Join(home, ".config", "siphon", "config.yaml")
	}
}

// homeDir resolves the user's home directory deterministically. os.UserHomeDir
// only fails when the platform's home env var is unset; if that happens we fall
// back to that env var directly and finally to os.TempDir(), so the result is
// always an absolute path — never "" (which would yield a cwd-relative config
// path silently written/read in an unexpected location).
func homeDir() string {
	if h, err := os.UserHomeDir(); err == nil && h != "" {
		return h
	}
	envVar := "HOME"
	if runtime.GOOS == "windows" {
		envVar = "USERPROFILE"
	}
	if h := os.Getenv(envVar); h != "" {
		return h
	}
	return os.TempDir()
}
