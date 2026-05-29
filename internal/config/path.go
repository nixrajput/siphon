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
	home, _ := os.UserHomeDir()
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
