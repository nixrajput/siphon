// Package drivertesting holds the shared driver test harness. Imported by
// each driver's integration test under a //go:build integration tag.
package drivertesting

import (
	"context"
	"database/sql"

	"github.com/nixrajput/siphon/internal/driver"
)

// Fixtures describes everything a driver needs to be exercised by the
// shared RunDriverSuite. Drivers populate this struct from their own
// integration setup (e.g. testcontainers).
type Fixtures struct {
	// Profile points at a freshly-started, empty test database.
	Profile driver.Profile

	// Seed runs arbitrary SQL/script to populate the test database with a
	// known fixture. Called before Backup.
	Seed func(ctx context.Context, db *sql.DB) error

	// VerifyRestore asserts the database state matches what Seed produced.
	// Called after Restore round-trips through Backup.
	VerifyRestore func(ctx context.Context, db *sql.DB) error

	// Cleanup tears down the test database. Called via t.Cleanup.
	Cleanup func()

	// SQLOpener returns a *sql.DB connected to the same database. Used by
	// Seed and VerifyRestore (they can't go through driver.Conn because
	// it doesn't expose raw SQL).
	SQLOpener func() (*sql.DB, error)
}
