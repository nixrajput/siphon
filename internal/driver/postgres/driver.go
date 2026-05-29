// Package postgres implements siphon's Postgres driver. Backup/Restore
// shell out to pg_dump/pg_restore for correctness; Inspect uses pgx for
// fast schema reads.
package postgres

import (
	"context"
	"database/sql"
	"strconv"
	"strings"

	"github.com/nixrajput/siphon/internal/driver"
	"github.com/nixrajput/siphon/internal/errs"

	_ "github.com/jackc/pgx/v5/stdlib" // register pgx as a database/sql driver
)

func init() { driver.Register(&Driver{}) }

type Driver struct{}

func (Driver) Name() string { return "postgres" }

func (Driver) Capabilities() driver.Capabilities {
	return driver.Capabilities{
		Incremental:        false, // arrives in Phase F (WAL)
		NativeStream:       true,
		PerTable:           true,
		SchemaOnly:         true,
		DataOnly:           true,
		Parallel:           true, // pg_restore supports -j; pg_dump parallelism needs -Fd (see backup.go), so backups are single-stream in Phase B
		Compression:        true,
		BinaryFormat:       true,
		CrossEngineSource:  false, // Phase F
		CrossEngineTarget:  false, // Phase F
		CDC:                false, // Phase F
		NativeBackpressure: true,
	}
}

func (Driver) Connect(ctx context.Context, p driver.Profile) (driver.Conn, error) {
	dsn := buildDSN(p)

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, wrapConnErr(err)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, wrapConnErr(err)
	}
	return &Conn{db: db, p: p}, nil
}

// buildDSN assembles a libpq keyword DSN from only the non-empty profile
// fields. Emitting an empty value (e.g. password=) makes pgx's keyword parser
// swallow the following token, which previously dropped dbname when the
// password was empty. Port is always emitted since it is an int default.
func buildDSN(p driver.Profile) string {
	parts := make([]string, 0, 6)
	if p.Host != "" {
		parts = append(parts, "host="+p.Host)
	}
	if p.Port != 0 {
		parts = append(parts, "port="+strconv.Itoa(p.Port))
	}
	if p.User != "" {
		parts = append(parts, "user="+p.User)
	}
	if p.Password != "" {
		parts = append(parts, "password="+p.Password)
	}
	if p.Database != "" {
		parts = append(parts, "dbname="+p.Database)
	}
	parts = append(parts, "sslmode="+defaultSSL(p.SSLMode))
	return strings.Join(parts, " ")
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

func (c *Conn) Close() error { return c.db.Close() }
