//go:build integration

package postgres

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	pgctr "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/nixrajput/siphon/internal/driver"
)

func startPostgres(t *testing.T) (driver.Profile, func()) {
	t.Helper()
	ctx := context.Background()
	c, err := pgctr.Run(ctx, "postgres:16-alpine",
		pgctr.WithDatabase("test"),
		pgctr.WithUsername("postgres"),
		pgctr.WithPassword("postgres"),
		pgctr.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}

	host, _ := c.Host(ctx)
	port, _ := c.MappedPort(ctx, "5432/tcp")

	return driver.Profile{
			Driver:   "postgres",
			Host:     host,
			Port:     int(port.Num()),
			User:     "postgres",
			Password: "postgres",
			Database: "test",
			SSLMode:  "disable",
		}, func() {
			_ = c.Terminate(ctx)
		}
}

func TestIntegration_Connect_And_Inspect(t *testing.T) {
	p, cleanup := startPostgres(t)
	t.Cleanup(cleanup)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	conn, err := Driver{}.Connect(ctx, p)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.Inspect(ctx); err != nil {
		t.Fatalf("Inspect: %v", err)
	}
}

func TestIntegration_BackupRestore_Roundtrip(t *testing.T) {
	p, cleanup := startPostgres(t)
	t.Cleanup(cleanup)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	conn, err := Driver{}.Connect(ctx, p)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = conn.Close() }()

	// Seed
	if _, execErr := conn.(*Conn).db.ExecContext(ctx,
		`CREATE TABLE widgets(id int primary key, name text); INSERT INTO widgets VALUES (1,'wrench');`,
	); execErr != nil {
		t.Fatalf("seed: %v", execErr)
	}

	var buf bytes.Buffer
	if err := conn.Backup(ctx, driver.BackupOpts{}, &buf); err != nil {
		t.Fatalf("Backup: %v", err)
	}

	// Drop the table
	if _, execErr := conn.(*Conn).db.ExecContext(ctx, `DROP TABLE widgets;`); execErr != nil {
		t.Fatalf("drop: %v", execErr)
	}

	if err := conn.Restore(ctx, driver.RestoreOpts{}, io.NopCloser(&buf)); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	var count int
	row := conn.(*Conn).db.QueryRowContext(ctx, `SELECT COUNT(*) FROM widgets`)
	if err := row.Scan(&count); err != nil {
		t.Fatalf("post-restore SELECT: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 widget after restore; got %d", count)
	}
}
