package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/oklog/ulid/v2"

	"github.com/nixrajput/siphon/internal/errs"
)

// IncrementalBaseInfo records what a base backup captured so a later
// incremental can resume from the correct WAL position. It is serialized into
// the dump Envelope (WALStart/WALEnd) and the slot is dropped when the chain
// is sealed (or via an orphan scan).
type IncrementalBaseInfo struct {
	WALStart string // server LSN when the base backup began
	WALEnd   string // server LSN when the base backup finished
	SlotName string // temporary physical replication slot anchoring WAL retention
}

// CreateBaseSlot creates a temporary physical replication slot and records the
// start LSN. Call this immediately before taking a base backup; the slot
// prevents the server from recycling WAL the future incremental will need.
func (c *Conn) CreateBaseSlot(ctx context.Context) (*IncrementalBaseInfo, error) {
	// Postgres replication slot names allow only [a-z0-9_]; ULIDs are Crockford
	// base32 (uppercase), so lowercase it or pg_create_physical_replication_slot
	// rejects the name with SQLSTATE 42602 ("contains invalid character").
	slot := "siphon_" + strings.ToLower(ulid.Make().String())
	// temporary=false so the slot survives this session (the incremental runs
	// in a later session); we drop it explicitly via DropSlot.
	if _, err := c.db.ExecContext(ctx,
		"SELECT pg_create_physical_replication_slot($1, false, false)", slot); err != nil {
		return nil, &errs.Error{Op: "postgres.create_slot", Code: errs.CodeSystem, Cause: err}
	}
	info := &IncrementalBaseInfo{SlotName: slot}
	if err := c.db.QueryRowContext(ctx, "SELECT pg_current_wal_lsn()::text").Scan(&info.WALStart); err != nil {
		_, _ = c.db.ExecContext(ctx, "SELECT pg_drop_replication_slot($1)", slot) // best-effort cleanup
		return nil, &errs.Error{Op: "postgres.capture_lsn", Code: errs.CodeSystem, Cause: err}
	}
	return info, nil
}

// CaptureBaseEnd records the end-of-base LSN into info.
func (c *Conn) CaptureBaseEnd(ctx context.Context, info *IncrementalBaseInfo) error {
	if err := c.db.QueryRowContext(ctx, "SELECT pg_current_wal_lsn()::text").Scan(&info.WALEnd); err != nil {
		return &errs.Error{Op: "postgres.capture_lsn", Code: errs.CodeSystem, Cause: err}
	}
	return nil
}

// DropSlot removes the replication slot once a chain is sealed (or via an
// orphan scan on startup). Safe to call best-effort.
func (c *Conn) DropSlot(ctx context.Context, slotName string) error {
	if _, err := c.db.ExecContext(ctx, "SELECT pg_drop_replication_slot($1)", slotName); err != nil {
		return fmt.Errorf("drop slot %s: %w", slotName, err)
	}
	return nil
}
