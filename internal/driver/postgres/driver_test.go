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

// TestBuildDSN_QuotesSpecialValues is the regression for libpq keyword escaping:
// a password containing spaces/quotes/backslashes must be single-quoted (with '
// and \ escaped) so pgx parses it back to the exact original, not split tokens.
func TestBuildDSN_QuotesSpecialValues(t *testing.T) {
	for _, pw := range []string{"p@ss word", `quote'd`, `back\slash`, "a b c"} {
		dsn := buildDSN(driver.Profile{
			Host: "localhost", Port: 5432, User: "u", Password: pw, Database: "d", SSLMode: "disable",
		})
		cfg, err := pgx.ParseConfig(dsn)
		if err != nil {
			t.Fatalf("password %q: ParseConfig(%q): %v", pw, dsn, err)
		}
		if cfg.Password != pw {
			t.Fatalf("password %q round-tripped as %q via DSN %q", pw, cfg.Password, dsn)
		}
		if cfg.Database != "d" {
			t.Fatalf("password %q broke dbname: got %q", pw, cfg.Database)
		}
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
