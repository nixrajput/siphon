// Package app implements siphon's verbs over the domain layer.
// CLI and TUI both call into this package; neither cares about the other.
package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/nixrajput/siphon/internal/audit"
	"github.com/nixrajput/siphon/internal/canonical"
	"github.com/nixrajput/siphon/internal/driver"
	"github.com/nixrajput/siphon/internal/dumps"
	"github.com/nixrajput/siphon/internal/errs"
	"github.com/nixrajput/siphon/internal/jobs"
	"github.com/nixrajput/siphon/internal/profile"
)

// Deps bundles every dependency the app verbs need. CLI and TUI build
// one Deps at startup and pass it to every verb. Makes mocking trivial.
type Deps struct {
	Profiles *profile.Store
	Dumps    *dumps.Catalog
	Runner   *jobs.Runner
	Drivers  DriverGetter
	// Auditor records destructive operations; nil is a no-op. It is also the
	// interception seam for 2FA gating (a pre-check before the verb) and
	// telemetry (timing/outcome), so those reuse these call sites.
	Auditor audit.Auditor
	// Gate, if set, is consulted before a destructive verb runs and can block it
	// (e.g. require 2FA / destructive confirmation for a profile's group).
	Gate Gate
	// Actor is the OS user attributed in audit records.
	Actor string
}

// Gate authorizes a destructive operation before it runs. A nil Gate allows
// everything. Returning a non-nil error blocks the verb.
type Gate interface {
	Authorize(ctx context.Context, op audit.Op, profile string) error
}

// DriverGetter is satisfied by internal/driver.Get. Wrapped to allow mocking.
type DriverGetter interface {
	Get(name string) (driver.Driver, error)
}

// BackupOpts configures the Backup verb.
type BackupOpts struct {
	Profile          string
	IncludeTables    []string
	ExcludeTables    []string
	ExcludeDataFrom  []string
	SchemaOnly       bool
	DataOnly         bool
	CompressionLevel int
	Parallel         int
	Incremental      bool
	BaseID           string
}

// Backup dumps the source profile to a new entry in the catalog.
// Returns the running job's Event channel and ID.
func Backup(parent context.Context, d Deps, opt BackupOpts) (<-chan jobs.Event, string, error) {
	resolved, err := d.Profiles.Resolve(opt.Profile)
	if err != nil {
		return nil, "", err
	}
	drv, err := d.Drivers.Get(resolved.Driver)
	if err != nil {
		return nil, "", err
	}
	// Gate (2FA/confirmation) runs synchronously at launch — a block aborts
	// before the job starts. The audit record is finalized when the job ends.
	done, err := guardedOp(parent, d, audit.OpBackup, opt.Profile, "")
	if err != nil {
		return nil, "", err
	}

	return launchGuarded(d.Runner, parent, done, jobs.Job{
		Stage: "backup",
		Func: func(ctx context.Context, emit func(jobs.Event)) (retErr error) {
			defer func() { done(retErr) }()
			conn, err := drv.Connect(ctx, resolved)
			if err != nil {
				return err
			}
			defer func() { _ = conn.Close() }()

			if opt.Incremental {
				return runIncrementalBackup(ctx, d, opt, resolved, conn, emit)
			}

			// Stream the dump body to a temp file FIRST, then capture the
			// engine's change-stream position, then assemble the final dump as
			// envelope(position) + body — mirroring runIncrementalBackup.
			//
			// Why this ordering: a full base may later be used as `--base` for an
			// incremental, whose `since` is read from this envelope. If the
			// envelope carried no position, the first incremental would start
			// from "now" and silently drop every change committed between the
			// base dump and the incremental run. We therefore record a position.
			//
			// Consistent point — capture AFTER Backup returns: a consistent dump
			// reflects the DB as of its snapshot; pg_current_wal_lsn() (binlog
			// pos for MySQL/MariaDB) read just after the dump is at-or-after that
			// snapshot, so the incremental captures every post-base change from
			// there forward — no under-capture / no data loss. The only risk is a
			// change landing right at the boundary being captured in BOTH base
			// and incremental; ApplyChange's INSERT is idempotent (ON CONFLICT DO
			// NOTHING / INSERT IGNORE) so such a re-apply is harmless. Capturing
			// after (not before) the dump avoids the inverse hazard of streaming
			// pre-snapshot inserts that would conflict on PK.
			bodyTmp, err := os.CreateTemp("", "siphon-base-body-*")
			if err != nil {
				return err
			}
			bodyPath := bodyTmp.Name()
			defer func() { _ = os.Remove(bodyPath) }()

			emit(jobs.Event{Phase: jobs.PhaseProgress, Message: "dumping"})

			backupErr := conn.Backup(ctx, driver.BackupOpts{
				IncludeTables:    opt.IncludeTables,
				ExcludeTables:    opt.ExcludeTables,
				ExcludeDataFrom:  opt.ExcludeDataFrom,
				SchemaOnly:       opt.SchemaOnly,
				DataOnly:         opt.DataOnly,
				CompressionLevel: opt.CompressionLevel,
				Parallel:         opt.Parallel,
			}, bodyTmp)

			// Close the body file explicitly and check the error: for a file
			// being WRITTEN, Close() is where buffered data is flushed and where
			// late I/O failures (ENOSPC, quota, disk error) surface. The pg_dump
			// error takes precedence if both occurred.
			bodyCloseErr := bodyTmp.Close()
			if backupErr != nil {
				return backupErr
			}
			if bodyCloseErr != nil {
				return &errs.Error{Op: "app.backup", Code: errs.CodeSystem, Cause: bodyCloseErr, Hint: "failed to flush dump to disk (out of space?)"}
			}

			// Capture the base position now (just after a consistent dump). Only
			// drivers that support incremental implement BasePositioner; for the
			// rest the envelope simply carries no position, as before.
			var basePos canonical.Position
			if bp, ok := conn.(driver.BasePositioner); ok {
				pos, posErr := bp.CurrentPosition(ctx)
				if posErr != nil {
					return posErr
				}
				basePos = pos
			}

			id := ulid.Make().String()

			env := &dumps.Envelope{
				Type:    dumps.EnvelopeBase,
				Driver:  resolved.Driver,
				WALEnd:  basePos.LSN,
				Created: time.Now().UTC(),
			}
			if basePos.BinlogFile != "" {
				env.BinlogFile = basePos.BinlogFile
				env.BinlogEnd = basePos.BinlogPos
			}

			// Assemble the dump as envelope ++ body and stream it through the
			// catalog's store under id, teeing through sha256 as it flows. The
			// store publishes the object atomically, so a failed/cancelled Put
			// leaves nothing addressable by id. Hashing stays here (app-side), so
			// integrity is identical regardless of the storage backend.
			size, checksum, err := putDump(ctx, d.Dumps, id, env, bodyPath)
			if err != nil {
				return err
			}

			meta := &dumps.Meta{
				ID:         id,
				Profile:    opt.Profile,
				Driver:     resolved.Driver,
				SizeBytes:  size,
				Checksum:   checksum,
				Created:    time.Now(),
				DumpFormat: "custom",
			}
			// Write meta LAST: the catalog enumerates by sidecar metadata, so a
			// dump body without its meta is an invisible (prunable) orphan, never
			// a dangling catalog entry. On meta failure, best-effort remove the
			// orphaned body.
			if writeErr := d.Dumps.WriteMeta(ctx, meta); writeErr != nil {
				_ = d.Dumps.Delete(ctx, id)
				return writeErr
			}

			emit(jobs.Event{Phase: jobs.PhaseProgress, Message: "wrote dump " + id})
			return nil
		},
	})
}

// runIncrementalBackup captures the bounded change set since the base dump's end
// position and writes a new incremental dump linked to that base.
//
// Envelope write ordering: the incremental envelope must carry the END position
// of THIS capture (so the next incremental resumes from here), but that position
// is only known AFTER streaming. We therefore stream the JSONL change body into a
// temp file first, learn the end Position, then assemble the final dump as
// envelope(end position) followed by the temp body — checksumming the whole file
// as it is written, mirroring the full-backup tmp→rename + sha + WriteMeta flow.
func runIncrementalBackup(ctx context.Context, d Deps, opt BackupOpts, resolved driver.Profile, conn driver.Conn, emit func(jobs.Event)) error {
	base, err := d.Dumps.ReadMeta(ctx, opt.BaseID)
	if err != nil {
		return &errs.Error{
			Op:    "app.backup.incremental",
			Code:  errs.CodeUser,
			Cause: err,
			Hint:  "base dump " + opt.BaseID + " not found in the catalog",
		}
	}

	// Reject a base that does not belong to this profile/driver: an incremental
	// chain is only meaningful against the same engine and source, and a
	// cross-driver base would produce changes the target cannot replay.
	if base.Driver != resolved.Driver || base.Profile != opt.Profile {
		return &errs.Error{
			Op:    "app.backup.incremental",
			Code:  errs.CodeUser,
			Cause: errs.ErrIncompatibleEngine,
			Hint:  "base dump " + opt.BaseID + " was created for profile " + base.Profile + " (" + base.Driver + "), not " + opt.Profile + " (" + resolved.Driver + ")",
		}
	}

	since, err := basePosition(ctx, d, opt.BaseID)
	if err != nil {
		return err
	}
	// A base with no recorded change-stream position cannot anchor an incremental;
	// capturing from a zero cursor would silently start at "now" and drop every
	// change committed since the base. Require a real position.
	if since.LSN == "" && since.BinlogFile == "" {
		return &errs.Error{
			Op:   "app.backup.incremental",
			Code: errs.CodeUser,
			Hint: "base dump " + opt.BaseID + " has no recorded change-stream position; take a fresh full backup and use that as --base",
		}
	}

	inc, ok := conn.(driver.IncrementalBackuper)
	if !ok {
		return &errs.Error{
			Op:    "app.backup.incremental",
			Code:  errs.CodeUser,
			Cause: errs.ErrDriverUnsupported,
			Hint:  resolved.Driver + " does not support incremental backup",
		}
	}

	// Stream the change body to a local temp file so we learn the end Position
	// before writing the envelope (which must carry that end Position). The body
	// is staged locally regardless of the catalog's storage backend, then
	// published in one streamed Put.
	bodyTmp, err := os.CreateTemp("", "siphon-inc-body-*")
	if err != nil {
		return err
	}
	bodyPath := bodyTmp.Name()
	defer func() { _ = os.Remove(bodyPath) }()

	emit(jobs.Event{Phase: jobs.PhaseProgress, Message: "capturing changes since base"})
	endPos, capErr := inc.BackupIncremental(ctx, since, bodyTmp)
	closeErr := bodyTmp.Close()
	if capErr != nil {
		return capErr
	}
	if closeErr != nil {
		return &errs.Error{Op: "app.backup.incremental", Code: errs.CodeSystem, Cause: closeErr, Hint: "failed to flush change body to disk"}
	}

	id := ulid.Make().String()

	// The chain root: if the base is itself an incremental, inherit its BaseID;
	// otherwise the base IS the root.
	root := base.BaseID
	if root == "" {
		root = base.ID
	}
	env := &dumps.Envelope{
		Type:     dumps.EnvelopeIncremental,
		Driver:   resolved.Driver,
		BaseID:   root,
		ParentID: opt.BaseID,
		WALEnd:   endPos.LSN,
		Created:  time.Now().UTC(),
	}
	if endPos.BinlogFile != "" {
		env.BinlogFile = endPos.BinlogFile
		env.BinlogEnd = endPos.BinlogPos
	}

	size, checksum, err := putDump(ctx, d.Dumps, id, env, bodyPath)
	if err != nil {
		return err
	}

	meta := &dumps.Meta{
		ID:         id,
		Profile:    opt.Profile,
		Driver:     resolved.Driver,
		SizeBytes:  size,
		Checksum:   checksum,
		Created:    time.Now(),
		DumpFormat: "jsonl-changes",
		BaseID:     root,
		ParentID:   opt.BaseID,
	}
	if writeErr := d.Dumps.WriteMeta(ctx, meta); writeErr != nil {
		_ = d.Dumps.Delete(ctx, id)
		return writeErr
	}

	emit(jobs.Event{Phase: jobs.PhaseProgress, Message: "wrote dump " + id})
	return nil
}

// putDump assembles a dump as the 4 KB envelope followed by the staged body
// file at bodyPath, streams it into the catalog under id, and returns the total
// byte count and the sha256 checksum (computed over the same envelope++body
// stream). The store publishes atomically, so a failed Put leaves nothing
// addressable by id. Counting size and hashing inline keeps both backend-
// agnostic — neither depends on the object being a local file afterward.
func putDump(ctx context.Context, cat *dumps.Catalog, id string, env *dumps.Envelope, bodyPath string) (int64, string, error) {
	var hdr bytes.Buffer
	if _, err := dumps.WriteEnvelope(&hdr, env); err != nil {
		return 0, "", err
	}
	body, err := os.Open(bodyPath)
	if err != nil {
		return 0, "", err
	}
	defer func() { _ = body.Close() }()

	h := sha256.New()
	counter := &countWriter{}
	// Reader order: envelope header bytes, then the body file. Tee through the
	// hash and a byte counter as the store consumes the stream.
	src := io.TeeReader(io.MultiReader(&hdr, body), io.MultiWriter(h, counter))

	if err := cat.PutDump(ctx, id, src); err != nil {
		return 0, "", err
	}
	return counter.n, "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

// countWriter is an io.Writer that only counts the bytes written to it, so the
// dump's size can be recorded as the assembled stream flows to the store
// (which may be remote, so os.Stat on the result is not an option).
type countWriter struct{ n int64 }

func (c *countWriter) Write(p []byte) (int, error) {
	c.n += int64(len(p))
	return len(p), nil
}

// basePosition reads the end Position recorded in the base dump's envelope. The
// next incremental resumes from here: WALEnd for Postgres, BinlogFile+BinlogEnd
// for MySQL/MariaDB.
func basePosition(ctx context.Context, d Deps, baseID string) (canonical.Position, error) {
	rc, err := d.Dumps.OpenDump(ctx, baseID)
	if err != nil {
		return canonical.Position{}, err
	}
	defer func() { _ = rc.Close() }()
	env, _, err := dumps.ReadEnvelope(rc)
	if err != nil {
		return canonical.Position{}, err
	}
	return canonical.Position{
		LSN:        env.WALEnd,
		BinlogFile: env.BinlogFile,
		BinlogPos:  env.BinlogEnd,
	}, nil
}
