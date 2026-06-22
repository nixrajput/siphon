//go:build integration

package postgres

import (
	"context"
	"database/sql"
	"sync"
	"testing"
	"time"

	tc "github.com/testcontainers/testcontainers-go"
	pgctr "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/nixrajput/siphon/internal/canonical"
	"github.com/nixrajput/siphon/internal/driver"
)

// startLogicalPostgres starts a Postgres container configured for logical
// decoding (wal_level=logical), the prerequisite for StreamChanges.
func startLogicalPostgres(t *testing.T) (driver.Profile, func()) {
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
	return prof, func() { _ = c.Terminate(ctx) }
}

// TestStreamChanges_Bounded seeds a table, captures the current LSN, performs
// insert/update/delete, then streams changes bounded to those three events
// (emit cancels ctx after the third). It asserts op + table + key/values.
func TestStreamChanges_Bounded(t *testing.T) {
	prof, cleanup := startLogicalPostgres(t)
	defer cleanup()

	ctx := context.Background()
	db, err := sql.Open("pgx", buildDSN(prof))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()

	if _, err := db.ExecContext(ctx,
		`CREATE TABLE widgets(id int primary key, name text);`); err != nil {
		t.Fatalf("create table: %v", err)
	}

	conn := &Conn{db: db, p: prof}

	// Capture the start position before any DML so the stream sees exactly our
	// changes. CurrentPosition also establishes the logical slot — a slot only
	// retains WAL produced after its creation, so it must exist before the DML.
	startPos, err := conn.CurrentPosition(ctx)
	if err != nil {
		t.Fatalf("capture start position: %v", err)
	}

	// Apply the three operations.
	if _, err := db.ExecContext(ctx, `INSERT INTO widgets VALUES (1,'wrench')`); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE widgets SET name='spanner' WHERE id=1`); err != nil {
		t.Fatalf("update: %v", err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM widgets WHERE id=1`); err != nil {
		t.Fatalf("delete: %v", err)
	}

	streamCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	var mu sync.Mutex
	var got []canonical.CanonicalChange
	emit := func(ch canonical.CanonicalChange) error {
		mu.Lock()
		got = append(got, ch)
		n := len(got)
		mu.Unlock()
		if n >= 3 {
			cancel() // bounded: stop after the three expected changes
		}
		return nil
	}

	if _, err := conn.StreamChanges(streamCtx, startPos, emit); err != nil {
		t.Fatalf("StreamChanges: %v", err)
	}

	if len(got) < 3 {
		t.Fatalf("expected at least 3 changes, got %d: %+v", len(got), got)
	}

	ins, upd, del := got[0], got[1], got[2]
	if ins.Op != canonical.OpInsert || ins.Table != "widgets" {
		t.Errorf("change[0] = %+v, want insert into widgets", ins)
	}
	if ins.Values["name"] != "wrench" {
		t.Errorf("insert values name = %v, want wrench", ins.Values["name"])
	}
	if upd.Op != canonical.OpUpdate || upd.Values["name"] != "spanner" {
		t.Errorf("change[1] = %+v, want update name=spanner", upd)
	}
	if del.Op != canonical.OpDelete {
		t.Errorf("change[2] = %+v, want delete", del)
	}
	// id is the PK; every op must carry it in Key.
	for i, ch := range []canonical.CanonicalChange{ins, upd, del} {
		if _, ok := ch.Key["id"]; !ok {
			t.Errorf("change[%d] key missing id: %+v", i, ch.Key)
		}
	}
}
