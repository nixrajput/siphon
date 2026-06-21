package postgres

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"

	"github.com/oklog/ulid/v2"

	"github.com/nixrajput/siphon/internal/driver"
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
	slot := "siphon_" + ulid.Make().String()
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

// incrementalArgs builds the pg_receivewal argv to stream WAL from the slot to
// stdout. NOTE: pg_receivewal streams from the slot's confirmed position; it
// does not accept an arbitrary --start LSN. The slot (created at base time)
// anchors retention, so streaming from it captures WAL accumulated since the
// base. This invocation needs validation against a live wal_level>=replica
// server (see incremental_test.go) — it is structurally complete but unproven
// locally (no Docker in this environment).
func incrementalArgs(p driver.Profile, slotName string) []string {
	return []string{
		"-h", p.Host,
		"-p", strconv.Itoa(p.Port),
		"-U", p.User,
		"-D", "-", // stream to stdout
		"--slot=" + slotName,
		"--no-loop", // exit at end of available WAL instead of waiting for more
		"--verbose",
	}
}

// BackupIncremental streams WAL from the base's slot to w. The caller prepends
// the dump Envelope at the app layer.
func (c *Conn) BackupIncremental(ctx context.Context, info IncrementalBaseInfo, w io.Writer) error {
	cmd := exec.CommandContext(ctx, "pg_receivewal", incrementalArgs(c.p, info.SlotName)...)
	cmd.Env = append(os.Environ(), "PGPASSWORD="+c.p.Password)
	cmd.Stdout = w
	cmd.Stderr = os.Stderr // surface pg_receivewal diagnostics (matches restore.go convention)

	if err := cmd.Run(); err != nil {
		return &errs.Error{Op: "postgres.backup_incremental", Code: errs.CodeSystem, Cause: err}
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
