// Package postgres implements siphon's Postgres driver. Backup/Restore
// shell out to pg_dump/pg_restore for correctness; Inspect uses pgx for
// fast schema reads.
package postgres

import (
	"context"
	"database/sql"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/nixrajput/siphon/internal/driver"
	"github.com/nixrajput/siphon/internal/errs"
	"github.com/nixrajput/siphon/internal/jobs"
)

func init() { driver.Register(&Driver{}) }

type Driver struct{}

func (Driver) Name() string { return "postgres" }

func (Driver) Capabilities() driver.Capabilities {
	return driver.Capabilities{
		Incremental:        true, // Phase F: bounded change capture via logical decoding
		NativeStream:       true,
		PerTable:           true,
		SchemaOnly:         true,
		DataOnly:           true,
		Parallel:           true, // capability exists in the engine, but not wired in Phase B: pg_dump -j needs -Fd (see backup.go) and RestoreOpts carries no Parallel field yet — both land in a later phase
		Compression:        true,
		BinaryFormat:       true,
		CrossEngineSource:  true,
		CrossEngineTarget:  true,
		CDC:                true,
		NativeBackpressure: true,
	}
}

func (Driver) Connect(ctx context.Context, p driver.Profile) (driver.Conn, error) {
	db, err := openDB(p)
	if err != nil {
		return nil, wrapConnErr(err)
	}

	// Probe the connection with bounded retry (spec §4.3: 3 attempts,
	// exponential backoff) so a briefly-unavailable server isn't a hard fail.
	if err := jobs.Retry(ctx, 3, func() error { return db.PingContext(ctx) }); err != nil {
		_ = db.Close()
		return nil, wrapConnErr(err)
	}
	return &Conn{db: db, p: p}, nil
}

// openDB returns a database/sql handle for p. It does not connect.
func openDB(p driver.Profile) (*sql.DB, error) {
	cfg, err := parseConfig(p)
	if err != nil {
		return nil, err
	}
	return stdlib.OpenDB(*cfg), nil
}

// parseConfig parses p's DSN and then sets the password on the parsed config.
// pgx echoes a DSN that fails to parse, and its redaction misses a password
// that contains a quote, so the password never goes into the DSN.
func parseConfig(p driver.Profile) (*pgx.ConnConfig, error) {
	cfg, err := pgx.ParseConfig(buildDSN(p))
	if err != nil {
		return nil, err
	}
	if p.Password != "" {
		cfg.Password = p.Password
	}
	return cfg, nil
}

// buildDSN assembles a libpq keyword DSN from only the non-empty profile
// fields, without the password (see parseConfig). Emitting an empty value
// (e.g. dbname=) makes pgx's keyword parser swallow the following token. Port
// is always emitted since it is an int default.
func buildDSN(p driver.Profile) string {
	parts := make([]string, 0, 6)
	if p.Host != "" {
		parts = append(parts, kv("host", p.Host))
	}
	if p.Port != 0 {
		parts = append(parts, kv("port", strconv.Itoa(p.Port)))
	}
	if p.User != "" {
		parts = append(parts, kv("user", p.User))
	}
	if p.Database != "" {
		parts = append(parts, kv("dbname", p.Database))
	}
	parts = append(parts, kv("sslmode", defaultSSL(p.SSLMode)))
	return strings.Join(parts, " ")
}

// kv formats one libpq keyword DSN pair. Values containing spaces, quotes, or
// backslashes must be single-quoted with ' and \ escaped (per libpq rules),
// otherwise a value like "p@ss word" would be split across tokens and misparse.
func kv(key, val string) string {
	if strings.ContainsAny(val, " '\\") {
		val = "'" + strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(val) + "'"
	}
	return key + "=" + val
}

func defaultSSL(s string) string {
	if s == "" {
		return "prefer"
	}
	return s
}

func wrapConnErr(err error) error {
	return &errs.Error{
		Op:    "postgres.connect",
		Code:  errs.CodeSystem,
		Cause: errs.ErrConnectionFailed,
		Hint:  err.Error(),
	}
}

// Conn is a live Postgres connection.
type Conn struct {
	db *sql.DB
	p  driver.Profile
}

var _ driver.Conn = (*Conn)(nil)
var _ driver.SchemaInspector = (*Conn)(nil)
var _ driver.CanonicalTransfer = (*Conn)(nil)

func (c *Conn) Close() error { return c.db.Close() }
