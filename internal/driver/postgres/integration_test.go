//go:build integration

package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	pgctr "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/nixrajput/siphon/internal/driver"
	drivertesting "github.com/nixrajput/siphon/internal/driver/_testing"
)

func startPostgres(t *testing.T) (driver.Profile, func(), func() (*sql.DB, error)) {
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

	host, err := c.Host(ctx)
	if err != nil {
		t.Fatalf("container host: %v", err)
	}
	port, err := c.MappedPort(ctx, "5432/tcp")
	if err != nil {
		t.Fatalf("container mapped port: %v", err)
	}

	prof := driver.Profile{
		Driver:   "postgres",
		Host:     host,
		Port:     int(port.Num()),
		User:     "postgres",
		Password: "postgres",
		Database: "test",
		SSLMode:  "disable",
	}
	cleanup := func() { _ = c.Terminate(ctx) }
	opener := func() (*sql.DB, error) {
		dsn := fmt.Sprintf("host=%s port=%d user=postgres password=postgres dbname=test sslmode=disable", prof.Host, prof.Port)
		return sql.Open("pgx", dsn)
	}
	return prof, cleanup, opener
}

func TestSuite_Postgres(t *testing.T) {
	prof, cleanup, opener := startPostgres(t)
	drivertesting.RunDriverSuite(t, func() driver.Driver { return Driver{} },
		drivertesting.Fixtures{
			Profile:   prof,
			Cleanup:   cleanup,
			SQLOpener: opener,
			Seed: func(ctx context.Context, db *sql.DB) error {
				_, err := db.ExecContext(ctx,
					`DROP TABLE IF EXISTS widgets;
					 CREATE TABLE widgets(id int primary key, name text);
					 INSERT INTO widgets VALUES (1,'wrench'),(2,'hammer');`)
				return err
			},
			VerifyRestore: func(ctx context.Context, db *sql.DB) error {
				var count int
				if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM widgets`).Scan(&count); err != nil {
					return err
				}
				if count != 2 {
					return fmt.Errorf("expected 2 widgets after restore, got %d", count)
				}
				return nil
			},
		})
}
