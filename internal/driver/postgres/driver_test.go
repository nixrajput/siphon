package postgres

import (
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/nixrajput/siphon/internal/driver"
)

// TestBuildDSN_OmitsEmptyFields is the exact regression: an empty password
// must not be emitted, and dbname must survive parsing.
func TestBuildDSN_OmitsEmptyFields(t *testing.T) {
	dsn := buildDSN(driver.Profile{
		Host:     "localhost",
		Port:     5432,
		User:     "u",
		Database: "d",
		SSLMode:  "disable",
	})

	if strings.Contains(dsn, "password=") {
		t.Fatalf("DSN must not contain password= when password is empty, got %q", dsn)
	}
	if !strings.Contains(dsn, "dbname=d") {
		t.Fatalf("DSN must contain dbname=d, got %q", dsn)
	}

	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("pgx.ParseConfig(%q): %v", dsn, err)
	}
	if cfg.Database != "d" {
		t.Fatalf("parsed Database = %q, want %q (empty password ate dbname)", cfg.Database, "d")
	}
}

func TestBuildDSN_FullProfile(t *testing.T) {
	dsn := buildDSN(driver.Profile{
		Host:     "localhost",
		Port:     5432,
		User:     "u",
		Password: "secret",
		Database: "d",
		SSLMode:  "disable",
	})

	if !strings.Contains(dsn, "password=secret") {
		t.Fatalf("DSN must contain password=secret, got %q", dsn)
	}
	if !strings.Contains(dsn, "dbname=d") {
		t.Fatalf("DSN must contain dbname=d, got %q", dsn)
	}

	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("pgx.ParseConfig(%q): %v", dsn, err)
	}
	if cfg.Database != "d" {
		t.Fatalf("parsed Database = %q, want %q", cfg.Database, "d")
	}
	if cfg.User != "u" {
		t.Fatalf("parsed User = %q, want %q", cfg.User, "u")
	}
}

func TestBuildDSN_DefaultSSL(t *testing.T) {
	dsn := buildDSN(driver.Profile{
		Host:     "localhost",
		Port:     5432,
		User:     "u",
		Database: "d",
	})

	if !strings.Contains(dsn, "sslmode=prefer") {
		t.Fatalf("DSN must contain sslmode=prefer for empty SSLMode, got %q", dsn)
	}
}
