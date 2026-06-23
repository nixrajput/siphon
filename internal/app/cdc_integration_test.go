//go:build integration

package app

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	tc "github.com/testcontainers/testcontainers-go"
	pgctr "github.com/testcontainers/testcontainers-go/modules/postgres"

	_ "github.com/jackc/pgx/v5/stdlib"

	mysqlcommon "github.com/nixrajput/siphon/internal/driver/_mysqlcommon"

	"github.com/nixrajput/siphon/internal/config"
	"github.com/nixrajput/siphon/internal/driver"
	"github.com/nixrajput/siphon/internal/dumps"
	"github.com/nixrajput/siphon/internal/jobs"
	"github.com/nixrajput/siphon/internal/profile"
	"github.com/nixrajput/siphon/internal/secrets"
)

// startLogicalPG starts a Postgres 16 container configured for logical decoding
// (wal_level=logical), the prerequisite for CDC StreamChanges.
func startLogicalPG(t *testing.T) (host string, port int, cleanup func()) {
	t.Helper()
	ctx := context.Background()
	c, err := pgctr.Run(ctx, "postgres:16-alpine",
		pgctr.WithDatabase("test"),
		pgctr.WithUsername("postgres"),
		pgctr.WithPassword("postgres"),
		pgctr.BasicWaitStrategies(),
		tc.WithCmdArgs("-c", "wal_level=logical", "-c", "max_wal_senders=10", "-c", "max_replication_slots=10"),
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

func openPG(t *testing.T, host string, port int) *sql.DB {
	t.Helper()
	dsn := fmt.Sprintf("host=%s port=%d user=postgres password=postgres dbname=test sslmode=disable", host, port)
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	return db
}

// pollUntil polls fn every 250ms until it returns true or the timeout elapses.
func pollUntil(t *testing.T, timeout time.Duration, fn func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return true
		}
		time.Sleep(250 * time.Millisecond)
	}
	return fn()
}

// drainForError drains the job event channel in the background and fails the
// test on any Error event. It does not block — CDC runs unbounded.
func drainForError(t *testing.T, ch <-chan jobs.Event) {
	t.Helper()
	go func() {
		for ev := range ch {
			if ev.Err != nil {
				t.Errorf("CDC job error: %v", ev.Err)
			}
		}
	}()
}

// TestCDC_SameEngine_PostgresToPostgres is the solid, primary CDC test: it runs
// RunCDC PG→PG, then performs INSERT/UPDATE/DELETE on the source and asserts each
// change is applied to the target within a timeout, then cancels (clean stop).
func TestCDC_SameEngine_PostgresToPostgres(t *testing.T) {
	t.Setenv("SIPHON_STATE_HOME", t.TempDir())

	srcHost, srcPort, srcCleanup := startLogicalPG(t)
	defer srcCleanup()
	dstHost, dstPort, dstCleanup := startLogicalPG(t)
	defer dstCleanup()

	srcDB := openPG(t, srcHost, srcPort)
	defer func() { _ = srcDB.Close() }()
	dstDB := openPG(t, dstHost, dstPort)
	defer func() { _ = dstDB.Close() }()

	ctx := context.Background()
	// Seed source + target with the same empty schema so the snapshot + apply
	// have a target table to write into.
	for _, db := range []*sql.DB{srcDB, dstDB} {
		if _, err := db.ExecContext(ctx,
			`CREATE TABLE widgets(id int primary key, name text);`); err != nil {
			t.Fatalf("create table: %v", err)
		}
	}
	if _, err := srcDB.ExecContext(ctx, `INSERT INTO widgets VALUES (1,'seed')`); err != nil {
		t.Fatalf("seed insert: %v", err)
	}

	deps := cdcDeps(t, pgProfile(srcHost, srcPort), pgProfile(dstHost, dstPort))

	runCtx, cancel := context.WithCancel(ctx)
	ch, _, err := RunCDC(runCtx, deps, SyncOpts{From: "src", To: "dst"})
	if err != nil {
		t.Fatalf("RunCDC setup: %v", err)
	}
	drainForError(t, ch)

	// The snapshot row should land on the target.
	if !pollUntil(t, 60*time.Second, func() bool {
		var n int
		_ = dstDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM widgets WHERE id=1").Scan(&n)
		return n == 1
	}) {
		cancel()
		t.Fatal("snapshot row id=1 never appeared on target")
	}

	// Give the streamer a moment to establish its slot, then apply DML.
	time.Sleep(2 * time.Second)
	if _, err := srcDB.ExecContext(ctx, `INSERT INTO widgets VALUES (2,'wrench')`); err != nil {
		t.Fatalf("stream insert: %v", err)
	}
	if !pollUntil(t, 30*time.Second, func() bool {
		var name string
		_ = dstDB.QueryRowContext(ctx, "SELECT name FROM widgets WHERE id=2").Scan(&name)
		return name == "wrench"
	}) {
		cancel()
		t.Fatal("streamed INSERT id=2 never appeared on target")
	}

	if _, err := srcDB.ExecContext(ctx, `UPDATE widgets SET name='spanner' WHERE id=2`); err != nil {
		t.Fatalf("stream update: %v", err)
	}
	if !pollUntil(t, 30*time.Second, func() bool {
		var name string
		_ = dstDB.QueryRowContext(ctx, "SELECT name FROM widgets WHERE id=2").Scan(&name)
		return name == "spanner"
	}) {
		cancel()
		t.Fatal("streamed UPDATE id=2 never propagated")
	}

	if _, err := srcDB.ExecContext(ctx, `DELETE FROM widgets WHERE id=2`); err != nil {
		t.Fatalf("stream delete: %v", err)
	}
	if !pollUntil(t, 30*time.Second, func() bool {
		var n int
		_ = dstDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM widgets WHERE id=2").Scan(&n)
		return n == 0
	}) {
		cancel()
		t.Fatal("streamed DELETE id=2 never propagated")
	}

	cancel() // clean stop — ctx cancel is the normal CDC termination
}

// TestCDC_Resume runs CDC, lets a streamed change land, cancels, then restarts
// RunCDC with the same (src,dst) — which derives the same jobID — and applies
// more changes. It asserts the resume picks up new changes (and at-least-once
// re-apply of any tail is harmless because ApplyChange is idempotent).
func TestCDC_Resume(t *testing.T) {
	t.Setenv("SIPHON_STATE_HOME", t.TempDir())

	srcHost, srcPort, srcCleanup := startLogicalPG(t)
	defer srcCleanup()
	dstHost, dstPort, dstCleanup := startLogicalPG(t)
	defer dstCleanup()

	srcDB := openPG(t, srcHost, srcPort)
	defer func() { _ = srcDB.Close() }()
	dstDB := openPG(t, dstHost, dstPort)
	defer func() { _ = dstDB.Close() }()

	ctx := context.Background()
	for _, db := range []*sql.DB{srcDB, dstDB} {
		if _, err := db.ExecContext(ctx,
			`CREATE TABLE widgets(id int primary key, name text);`); err != nil {
			t.Fatalf("create table: %v", err)
		}
	}

	deps := cdcDeps(t, pgProfile(srcHost, srcPort), pgProfile(dstHost, dstPort))

	// First run: stream one change, then cancel.
	runCtx1, cancel1 := context.WithCancel(ctx)
	ch1, _, err := RunCDC(runCtx1, deps, SyncOpts{From: "src", To: "dst"})
	if err != nil {
		t.Fatalf("RunCDC (1) setup: %v", err)
	}
	drainForError(t, ch1)

	time.Sleep(3 * time.Second) // let the slot establish
	if _, err := srcDB.ExecContext(ctx, `INSERT INTO widgets VALUES (1,'first')`); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	if !pollUntil(t, 30*time.Second, func() bool {
		var n int
		_ = dstDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM widgets WHERE id=1").Scan(&n)
		return n == 1
	}) {
		cancel1()
		t.Fatal("first run: id=1 never propagated")
	}
	cancel1()
	time.Sleep(2 * time.Second) // allow clean shutdown + state persist

	// Second run: same profiles → same jobID → resume. Apply another change.
	runCtx2, cancel2 := context.WithCancel(ctx)
	defer cancel2()
	ch2, _, err := RunCDC(runCtx2, deps, SyncOpts{From: "src", To: "dst"})
	if err != nil {
		t.Fatalf("RunCDC (2) setup: %v", err)
	}
	drainForError(t, ch2)

	time.Sleep(3 * time.Second)
	if _, err := srcDB.ExecContext(ctx, `INSERT INTO widgets VALUES (2,'second')`); err != nil {
		t.Fatalf("second insert: %v", err)
	}
	if !pollUntil(t, 30*time.Second, func() bool {
		var n int
		_ = dstDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM widgets WHERE id=2").Scan(&n)
		return n == 1
	}) {
		t.Fatal("resume: id=2 never propagated after restart")
	}

	// No gap/dup: id=1 must still be present exactly once (idempotent apply).
	var n1 int
	if err := dstDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM widgets WHERE id=1").Scan(&n1); err != nil {
		t.Fatalf("count id=1: %v", err)
	}
	if n1 != 1 {
		t.Errorf("id=1 count after resume = %d, want 1 (no gap, no dup)", n1)
	}
}

// TestCDC_CrossEngine_PostgresToMySQL runs CDC PG→MySQL: an INSERT on Postgres
// must arrive on MySQL via canonical apply.
func TestCDC_CrossEngine_PostgresToMySQL(t *testing.T) {
	t.Setenv("SIPHON_STATE_HOME", t.TempDir())

	pgHost, pgPort, pgCleanup := startLogicalPG(t)
	defer pgCleanup()
	myHost, myPort, myCleanup := startIntegMySQL(t)
	defer myCleanup()

	pgDB := openPG(t, pgHost, pgPort)
	defer func() { _ = pgDB.Close() }()

	ctx := context.Background()
	if _, err := pgDB.ExecContext(ctx,
		`CREATE TABLE widgets(id int primary key, name text);`); err != nil {
		t.Fatalf("pg create table: %v", err)
	}
	if _, err := pgDB.ExecContext(ctx, `INSERT INTO widgets VALUES (1,'seed')`); err != nil {
		t.Fatalf("pg seed: %v", err)
	}

	deps := cdcDeps(t, pgProfile(pgHost, pgPort),
		config.ProfileConfig{Driver: "mysql", Host: myHost, Port: myPort, User: "root", Password: "rootpass", Database: "test"})

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	ch, _, err := RunCDC(runCtx, deps, SyncOpts{From: "src", To: "dst"})
	if err != nil {
		t.Fatalf("RunCDC setup: %v", err)
	}
	drainForError(t, ch)

	myProf := driver.Profile{Driver: "mysql", Host: myHost, Port: myPort, User: "root", Password: "rootpass", Database: "test", SSLMode: "disable"}
	myDB, err := mysqlcommon.Open(myProf)
	if err != nil {
		t.Fatalf("open mysql: %v", err)
	}
	defer func() { _ = myDB.Close() }()

	// Snapshot row must land on MySQL.
	if !pollUntil(t, 90*time.Second, func() bool {
		var n int
		_ = myDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM widgets WHERE id=1").Scan(&n)
		return n == 1
	}) {
		t.Fatal("snapshot row id=1 never appeared on mysql")
	}

	time.Sleep(2 * time.Second)
	if _, err := pgDB.ExecContext(ctx, `INSERT INTO widgets VALUES (2,'wrench')`); err != nil {
		t.Fatalf("pg stream insert: %v", err)
	}
	if !pollUntil(t, 30*time.Second, func() bool {
		var name string
		_ = myDB.QueryRowContext(ctx, "SELECT name FROM widgets WHERE id=2").Scan(&name)
		return name == "wrench"
	}) {
		t.Fatal("streamed INSERT id=2 never crossed to mysql")
	}
}

// --- helpers ---

func pgProfile(host string, port int) config.ProfileConfig {
	return config.ProfileConfig{Driver: "postgres", Host: host, Port: port, User: "postgres", Password: "postgres", Database: "test", SSLMode: "disable"}
}

// cdcDeps builds Deps with two profiles "src" and "dst".
func cdcDeps(t *testing.T, src, dst config.ProfileConfig) Deps {
	t.Helper()
	cfg := &config.Config{Profiles: map[string]config.ProfileConfig{"src": src, "dst": dst}}
	res := secrets.NewResolver(secrets.Passthrough{})
	ps := profile.New(cfg, res, func(*config.Config) error { return nil })
	cat, err := dumps.NewCatalog(t.TempDir())
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	return Deps{
		Profiles: ps,
		Dumps:    cat,
		Runner:   jobs.NewRunner(),
		Drivers:  DefaultDrivers(),
	}
}
