package postgres

import (
	"context"
	"io"

	"github.com/jackc/pglogrepl"

	"github.com/nixrajput/siphon/internal/canonical"
	"github.com/nixrajput/siphon/internal/driver"
	"github.com/nixrajput/siphon/internal/errs"
)

var _ driver.IncrementalBackuper = (*Conn)(nil)
var _ driver.BasePositioner = (*Conn)(nil)

// CurrentPosition returns the server's current WAL position via
// pg_current_wal_lsn(). app.Backup calls this right after a full backup so the
// base dump's Envelope records where the first incremental should resume from.
func (c *Conn) CurrentPosition(ctx context.Context) (canonical.Position, error) {
	var lsn string
	if err := c.db.QueryRowContext(ctx, "SELECT pg_current_wal_lsn()::text").Scan(&lsn); err != nil {
		return canonical.Position{}, &errs.Error{Op: "postgres.current_position", Code: errs.CodeSystem, Cause: err}
	}
	return canonical.Position{LSN: lsn}, nil
}

// BackupIncremental captures the BOUNDED change set from `since` to the server's
// current end LSN, serializing each CanonicalChange to w as JSONL, and returns
// the end Position reached.
//
// Bounding mechanism: the end LSN is captured up front via pg_current_wal_lsn()
// and passed to the shared pgoutput decode loop as a stop target. The loop
// advances its client position only AFTER decoding+emitting each XLogData
// message, so it returns cleanly at the first message boundary where the client
// position reaches or passes the captured end LSN — every change committed at or
// before that LSN has been emitted, and none past it. (Once the live stream is
// caught up, a server keepalive carries ServerWALEnd, which crosses the bound and
// triggers the stop.) This reuses StreamChanges' decode machinery rather than
// streaming raw WAL bytes, so the incremental body is engine-neutral JSONL that
// the restore path replays via ApplyChange.
//
// Before streaming, orphaned siphon replication slots are swept (best-effort) to
// keep WAL retention bounded.
//
// This path is exercised against a live wal_level=logical server only in CI (see
// incremental_integration_test.go); it is not validated locally (no Docker here).
func (c *Conn) BackupIncremental(ctx context.Context, since canonical.Position, w io.Writer) (canonical.Position, error) {
	// Best-effort orphan-slot sweep before we (re)use the logical slot. A failure
	// here must not abort the backup, so the count/err is logged-by-return only
	// where the caller surfaces it; here we ignore a sweep error and proceed.
	_, _ = c.SweepOrphanSlots(ctx)

	var endLSNText string
	if err := c.db.QueryRowContext(ctx, "SELECT pg_current_wal_lsn()::text").Scan(&endLSNText); err != nil {
		return canonical.Position{}, &errs.Error{Op: "postgres.backup_incremental", Code: errs.CodeSystem, Cause: err}
	}
	stopLSN, err := pglogrepl.ParseLSN(endLSNText)
	if err != nil {
		return canonical.Position{}, &errs.Error{Op: "postgres.backup_incremental", Code: errs.CodeSystem, Cause: err}
	}

	emit := func(ch canonical.CanonicalChange) error {
		return canonical.WriteJSONL(w, ch)
	}
	return c.streamWithStop(ctx, since, stopLSN, emit)
}

// SweepOrphanSlots drops inactive siphon-owned PHYSICAL base slots and returns
// the count dropped.
//
// Policy: a base backup creates a non-temporary physical slot named
// siphon_<ulid> (see CreateBaseSlot) to pin WAL for a future incremental; that
// slot must be dropped when the chain is sealed. An active such slot is in use by
// a running backup, but an inactive one is orphaned — a normal run drops its own
// slot, so any inactive siphon_<ulid> left behind belongs to a crashed/aborted
// run and is only pinning WAL. We drop every inactive slot matching the prefix,
// EXCEPT the persistent logical CDC slot (siphonSlot), which is the resume anchor
// for change streaming and is legitimately inactive between runs — sweeping it
// would discard the resume position. Each drop is best-effort: a concurrent run
// that re-activates a slot between the scan and the drop makes
// pg_drop_replication_slot fail with "in use", which we skip.
func (c *Conn) SweepOrphanSlots(ctx context.Context) (int, error) {
	rows, err := c.db.QueryContext(ctx,
		`SELECT slot_name FROM pg_replication_slots
		 WHERE slot_name LIKE 'siphon\_%' AND active = false AND slot_name <> $1`, siphonSlot)
	if err != nil {
		return 0, &errs.Error{Op: "postgres.sweep_slots", Code: errs.CodeSystem, Cause: err}
	}
	var names []string
	for rows.Next() {
		var name string
		if scanErr := rows.Scan(&name); scanErr != nil {
			_ = rows.Close()
			return 0, &errs.Error{Op: "postgres.sweep_slots", Code: errs.CodeSystem, Cause: scanErr}
		}
		names = append(names, name)
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return 0, &errs.Error{Op: "postgres.sweep_slots", Code: errs.CodeSystem, Cause: err}
	}

	dropped := 0
	for _, name := range names {
		if _, err := c.db.ExecContext(ctx, "SELECT pg_drop_replication_slot($1)", name); err != nil {
			continue // raced with a re-activation, or already gone: skip
		}
		dropped++
	}
	return dropped, nil
}
