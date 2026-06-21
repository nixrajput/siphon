package postgres

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

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

// incrementalArgs builds the pg_receivewal argv to stream WAL from the slot
// into a destination DIRECTORY. pg_receivewal requires -D <dir>; it does NOT
// support "-D -" (stdout). The slot (created at base time) anchors retention,
// so streaming from it captures WAL accumulated since the base. --endpos stops
// the stream at a defined LSN so the invocation terminates deterministically;
// --no-loop controls connection-RETRY behavior (don't loop reconnecting on a
// dropped connection), NOT WAL-end termination. This path needs validation
// against a live wal_level>=replica server (see incremental_test.go) — it is
// structurally complete but unproven locally (no Docker in this environment).
func incrementalArgs(p driver.Profile, slotName, dir, endpos string) []string {
	return []string{
		"-h", p.Host,
		"-p", strconv.Itoa(p.Port),
		"-U", p.User,
		"-D", dir, // pg_receivewal writes WAL segments into this directory
		"--slot=" + slotName,
		"--endpos=" + endpos, // stop at this LSN instead of streaming forever
		"--synchronous",
		"--no-loop", // do not retry the connection if it drops
		"--verbose",
	}
}

// BackupIncremental streams WAL from the base's slot into a temp directory up
// to the server's current end LSN, then concatenates the resulting WAL segment
// files into w in name order. The caller prepends the dump Envelope at the app
// layer.
//
// pg_receivewal cannot stream to stdout (no "-D -"), so it must write segment
// files to a directory; we capture the current end LSN via pg_current_wal_lsn()
// and pass it as --endpos so the stream terminates at a defined point. This
// streaming path is exercised only in CI / against a live server — it is not
// validated locally (no Docker in this environment).
func (c *Conn) BackupIncremental(ctx context.Context, info IncrementalBaseInfo, w io.Writer) error {
	var endpos string
	if err := c.db.QueryRowContext(ctx, "SELECT pg_current_wal_lsn()::text").Scan(&endpos); err != nil {
		return &errs.Error{Op: "postgres.backup_incremental", Code: errs.CodeSystem, Cause: err}
	}

	tmpDir, err := os.MkdirTemp("", "siphon-wal-*")
	if err != nil {
		return &errs.Error{Op: "postgres.backup_incremental", Code: errs.CodeSystem, Cause: err}
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	cmd := exec.CommandContext(ctx, "pg_receivewal", incrementalArgs(c.p, info.SlotName, tmpDir, endpos)...)
	cmd.Env = append(os.Environ(), "PGPASSWORD="+c.p.Password)
	cmd.Stderr = os.Stderr // surface pg_receivewal diagnostics (matches restore.go convention)
	if err := cmd.Run(); err != nil {
		return &errs.Error{Op: "postgres.backup_incremental", Code: errs.CodeSystem, Cause: err}
	}

	// Copy the captured WAL segments to w in name order. Segment file names are
	// fixed-width hex, so lexical sort is chronological order.
	segments, err := filepath.Glob(filepath.Join(tmpDir, "*"))
	if err != nil {
		return &errs.Error{Op: "postgres.backup_incremental", Code: errs.CodeSystem, Cause: err}
	}
	sort.Strings(segments)
	for _, seg := range segments {
		f, err := os.Open(seg)
		if err != nil {
			return &errs.Error{Op: "postgres.backup_incremental", Code: errs.CodeSystem, Cause: err}
		}
		if _, err := io.Copy(w, f); err != nil {
			_ = f.Close()
			return &errs.Error{Op: "postgres.backup_incremental", Code: errs.CodeSystem, Cause: err}
		}
		_ = f.Close()
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
