//go:build integration

package mariadb

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	mariadbctr "github.com/testcontainers/testcontainers-go/modules/mariadb"

	"github.com/nixrajput/siphon/internal/driver"
	mysqlcommon "github.com/nixrajput/siphon/internal/driver/_mysqlcommon"
	drivertesting "github.com/nixrajput/siphon/internal/driver/_testing"
)

func startMariaDB(t *testing.T) (driver.Profile, func(), func() (*sql.DB, error)) {
	t.Helper()
	ctx := context.Background()
	c, err := mariadbctr.Run(ctx, "mariadb:11",
		mariadbctr.WithDatabase("test"),
		mariadbctr.WithUsername("root"),
		mariadbctr.WithPassword("rootpass"),
	)
	if err != nil {
		t.Fatalf("start mariadb container: %v", err)
	}

	host, err := c.Host(ctx)
	if err != nil {
		t.Fatalf("container host: %v", err)
	}
	port, err := c.MappedPort(ctx, "3306/tcp")
	if err != nil {
		t.Fatalf("container mapped port: %v", err)
	}

	prof := driver.Profile{
		Driver:   "mariadb",
		Host:     host,
		Port:     int(port.Num()),
		User:     "root",
		Password: "rootpass",
		Database: "test",
		SSLMode:  "disable",
	}
	cleanup := func() { _ = c.Terminate(ctx) }
	opener := func() (*sql.DB, error) { return mysqlcommon.Open(prof) }
	return prof, cleanup, opener
}

func TestSuite_MariaDB(t *testing.T) {
	prof, cleanup, opener := startMariaDB(t)
	drivertesting.RunDriverSuite(t, func() driver.Driver { return Driver{} },
		drivertesting.Fixtures{
			Profile:   prof,
			Cleanup:   cleanup,
			SQLOpener: opener,
			Seed: func(ctx context.Context, db *sql.DB) error {
				stmts := []string{
					`DROP TABLE IF EXISTS widgets`,
					`CREATE TABLE widgets(id INT PRIMARY KEY, name VARCHAR(64))`,
					`INSERT INTO widgets VALUES (1,'wrench'),(2,'hammer')`,
				}
				for _, s := range stmts {
					if _, err := db.ExecContext(ctx, s); err != nil {
						return err
					}
				}
				return nil
			},
			VerifyRestore: func(ctx context.Context, db *sql.DB) error {
				var n int
				if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM widgets`).Scan(&n); err != nil {
					return err
				}
				if n != 2 {
					return errors.New("expected 2 widgets after restore")
				}
				return nil
			},
		})
}
