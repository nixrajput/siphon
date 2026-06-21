//go:build integration

package postgres

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/nixrajput/siphon/internal/driver"
)

// TestIncremental_SlotAndLSNCapture verifies the slot lifecycle + LSN capture
// against a real Postgres container: create slot, take a base backup, record
// start/end LSN, then drop the slot. (WAL streaming via pg_receivewal needs a
// wal_level>=replica server and is exercised separately.)
func TestIncremental_SlotAndLSNCapture(t *testing.T) {
	prof, cleanup, _ := startPostgres(t)
	t.Cleanup(cleanup)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	conn, err := Driver{}.Connect(ctx, prof)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = conn.Close() }()
	pg := conn.(*Conn)

	info, err := pg.CreateBaseSlot(ctx)
	if err != nil {
		t.Fatalf("CreateBaseSlot: %v", err)
	}
	defer func() { _ = pg.DropSlot(ctx, info.SlotName) }()

	var base bytes.Buffer
	if err := conn.Backup(ctx, driver.BackupOpts{}, &base); err != nil {
		t.Fatalf("Backup base: %v", err)
	}
	if err := pg.CaptureBaseEnd(ctx, info); err != nil {
		t.Fatalf("CaptureBaseEnd: %v", err)
	}
	if info.WALStart == "" || info.WALEnd == "" {
		t.Fatalf("expected non-empty WAL LSNs, got %+v", info)
	}
	if info.SlotName == "" {
		t.Fatalf("expected slot name")
	}
}
