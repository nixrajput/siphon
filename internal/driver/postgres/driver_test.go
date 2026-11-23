package postgres

import (
	"context"
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

	if strings.Contains(dsn, "secret") {
		t.Fatalf("DSN must not contain the password, got %q", dsn)
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
// a value containing spaces/quotes/backslashes must be single-quoted (with '
// and \ escaped) so pgx parses it back to the exact original, not split tokens.
func TestBuildDSN_QuotesSpecialValues(t *testing.T) {
	for _, v := range []string{"p@ss word", `quote'd`, `back\slash`, "a b c"} {
		dsn := buildDSN(driver.Profile{
			Host: "localhost", Port: 5432, User: v, Database: v, SSLMode: "disable",
		})
		cfg, err := pgx.ParseConfig(dsn)
		if err != nil {
			t.Fatalf("value %q: ParseConfig(%q): %v", v, dsn, err)
		}
		if cfg.User != v || cfg.Database != v {
			t.Fatalf("value %q round-tripped as user %q, dbname %q via DSN %q", v, cfg.User, cfg.Database, dsn)
		}
	}
}

// TestParseConfig_SetsPassword checks that a password with DSN-special
// characters reaches the parsed config unchanged.
func TestParseConfig_SetsPassword(t *testing.T) {
	for _, pw := range []string{"p@ss word", `quote'd`, `back\slash`, "a b c", "password=x"} {
		cfg, err := parseConfig(driver.Profile{
			Host: "localhost", Port: 5432, User: "u", Password: pw, Database: "d", SSLMode: "disable",
		})
		if err != nil {
			t.Fatalf("password %q: %v", pw, err)
		}
		if cfg.Password != pw {
			t.Fatalf("password %q arrived as %q", pw, cfg.Password)
		}
		if cfg.Database != "d" {
			t.Fatalf("password %q broke dbname: got %q", pw, cfg.Database)
		}
	}
}

// TestConnectErrors_HidePassword is the regression for a password leak: pgx put
// the DSN in its parse error, and its redaction left the part of a quoted
// password after the quote. The audit log stores that error text.
func TestConnectErrors_HidePassword(t *testing.T) {
	p := driver.Profile{
		Host: "localhost", Port: 5432, User: "u", Database: "d",
		Password: "pa'ss word-TAIL", SSLMode: "bogus", // an invalid sslmode fails the parse
	}
	_, connErr := Driver{}.Connect(context.Background(), p)
	_, replErr := (&Conn{p: p}).connectReplication(context.Background())
	for name, err := range map[string]error{"connect": connErr, "replication": replErr} {
		if err == nil {
			t.Fatalf("%s: want a parse error for sslmode=bogus", name)
		}
		for _, frag := range []string{"pa'ss", "word", "TAIL"} {
			if strings.Contains(err.Error(), frag) {
				t.Fatalf("%s error leaks password fragment %q: %v", name, frag, err)
			}
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
