//go:build integration

package app

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	mysqlctr "github.com/testcontainers/testcontainers-go/modules/mysql"
	pgctr "github.com/testcontainers/testcontainers-go/modules/postgres"

	mysqlcommon "github.com/nixrajput/siphon/internal/driver/_mysqlcommon"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/nixrajput/siphon/internal/config"
	"github.com/nixrajput/siphon/internal/driver"
	"github.com/nixrajput/siphon/internal/dumps"
	"github.com/nixrajput/siphon/internal/jobs"
	"github.com/nixrajput/siphon/internal/profile"
	"github.com/nixrajput/siphon/internal/secrets"
)

// startIntegPG starts a Postgres 16-alpine testcontainer and returns its
// host, mapped port, and a cleanup function.
func startIntegPG(t *testing.T) (host string, port int, cleanup func()) {
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
	h, err := c.Host(ctx)
	if err != nil {
		_ = c.Terminate(ctx)
		t.Fatalf("postgres container host: %v", err)
	}
	p, err := c.MappedPort(ctx, "5432/tcp")
	if err != nil {
		_ = c.Terminate(ctx)
		t.Fatalf("postgres container port: %v", err)
	}
	return h, int(p.Num()), func() { _ = c.Terminate(ctx) }
}

// startIntegMySQL starts a MySQL 8.0 testcontainer and returns its host,
// mapped port, and a cleanup function.
func startIntegMySQL(t *testing.T) (host string, port int, cleanup func()) {
	t.Helper()
	ctx := context.Background()
	c, err := mysqlctr.Run(ctx, "mysql:8.0",
		mysqlctr.WithDatabase("test"),
		mysqlctr.WithUsername("root"),
		mysqlctr.WithPassword("rootpass"),
	)
	if err != nil {
		t.Fatalf("start mysql container: %v", err)
	}
	h, err := c.Host(ctx)
	if err != nil {
		_ = c.Terminate(ctx)
		t.Fatalf("mysql container host: %v", err)
	}
	p, err := c.MappedPort(ctx, "3306/tcp")
	if err != nil {
		_ = c.Terminate(ctx)
		t.Fatalf("mysql container port: %v", err)
	}
	return h, int(p.Num()), func() { _ = c.Terminate(ctx) }
}

// TestCrossEngineSync_PostgresToMySQL starts real Postgres and MySQL
// testcontainers, seeds a primary-keyed table on the Postgres side, runs
// app.Sync with CrossEngine:true, and asserts the rows arrive in MySQL.
func TestCrossEngineSync_PostgresToMySQL(t *testing.T) {
	pgHost, pgPort, pgCleanup := startIntegPG(t)
	defer pgCleanup()

	myHost, myPort, myCleanup := startIntegMySQL(t)
	defer myCleanup()

	// Seed the Postgres source table.
	pgDSN := fmt.Sprintf("host=%s port=%d user=postgres password=postgres dbname=test sslmode=disable", pgHost, pgPort)
	pgDB, err := sql.Open("pgx", pgDSN)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	defer func() { _ = pgDB.Close() }()

	ctx := context.Background()
	if _, err := pgDB.ExecContext(ctx,
		`DROP TABLE IF EXISTS widgets;
		 CREATE TABLE widgets(id int PRIMARY KEY, name text);
		 INSERT INTO widgets VALUES (1,'wrench'),(2,'hammer'),(3,'pliers');`); err != nil {
		t.Fatalf("seed postgres: %v", err)
	}

	// Build Deps with both profiles pointing at the containers.
	cfg := &config.Config{Profiles: map[string]config.ProfileConfig{
		"pg": {Driver: "postgres", Host: pgHost, Port: pgPort, User: "postgres", Password: "postgres", Database: "test", SSLMode: "disable"},
		"my": {Driver: "mysql", Host: myHost, Port: myPort, User: "root", Password: "rootpass", Database: "test"},
	}}
	res := secrets.NewResolver(secrets.Passthrough{})
	ps := profile.New(cfg, res, func(*config.Config) error { return nil })
	cat, err := dumps.NewCatalog(t.TempDir())
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	runner := jobs.NewRunner()
	deps := Deps{
		Profiles: ps,
		Dumps:    cat,
		Runner:   runner,
		Drivers:  DefaultDrivers(),
	}

	// Run the cross-engine sync.
	ch, _, err := Sync(ctx, deps, SyncOpts{From: "pg", To: "my", CrossEngine: true})
	if err != nil {
		t.Fatalf("Sync setup: %v", err)
	}

	// Drain the event channel. Any Error event is a test failure.
	timer := time.NewTimer(120 * time.Second)
	defer timer.Stop()
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				goto done
			}
			if ev.Err != nil {
				t.Fatalf("Sync job error: %v", ev.Err)
			}
		case <-timer.C:
			t.Fatal("Sync did not complete within 120 s")
		}
	}
done:

	// Open MySQL and count the transferred rows.
	myProf := driver.Profile{
		Driver:   "mysql",
		Host:     myHost,
		Port:     myPort,
		User:     "root",
		Password: "rootpass",
		Database: "test",
		SSLMode:  "disable",
	}
	myDB, err := mysqlcommon.Open(myProf)
	if err != nil {
		t.Fatalf("open mysql: %v", err)
	}
	defer func() { _ = myDB.Close() }()

	var count int
	if err := myDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM widgets").Scan(&count); err != nil {
		t.Fatalf("count mysql widgets: %v", err)
	}
	if count != 3 {
		t.Errorf("mysql widgets: got %d rows, want 3", count)
	}
}
