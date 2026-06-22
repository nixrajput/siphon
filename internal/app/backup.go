// Package app implements siphon's verbs over the domain layer.
// CLI and TUI both call into this package; neither cares about the other.
package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/oklog/ulid/v2"

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

	return d.Runner.Run(parent, jobs.Job{
		Stage: "backup",
		Func: func(ctx context.Context, emit func(jobs.Event)) error {
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
			bodyTmp, err := os.CreateTemp(d.Dumps.Root(), "siphon-base-body-*")
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
			tmpPath := filepath.Join(d.Dumps.Root(), id+".dump.tmp")
			finalPath := d.Dumps.Path(id)

			f, err := os.Create(tmpPath)
			if err != nil {
				return err
			}
			h := sha256.New()
			tee := io.MultiWriter(f, h)

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
			if _, err := dumps.WriteEnvelope(tee, env); err != nil {
				_ = f.Close()
				_ = os.Remove(tmpPath)
				return err
			}

			body, err := os.Open(bodyPath)
			if err != nil {
				_ = f.Close()
				_ = os.Remove(tmpPath)
				return err
			}
			if _, err := io.Copy(tee, body); err != nil {
				_ = body.Close()
				_ = f.Close()
				_ = os.Remove(tmpPath)
				return err
			}
			_ = body.Close()

			closeErr := f.Close()
			if closeErr != nil {
				_ = os.Remove(tmpPath)
				return &errs.Error{Op: "app.backup", Code: errs.CodeSystem, Cause: closeErr, Hint: "failed to flush dump to disk (out of space?)"}
			}

			if err := os.Rename(tmpPath, finalPath); err != nil {
				_ = os.Remove(tmpPath)
				return err
			}

			st, _ := os.Stat(finalPath)
			size := int64(0)
			if st != nil {
				size = st.Size()
			}

			meta := &dumps.Meta{
				ID:         id,
				Profile:    opt.Profile,
				Driver:     resolved.Driver,
				SizeBytes:  size,
				Checksum:   "sha256:" + hex.EncodeToString(h.Sum(nil)),
				Created:    time.Now(),
				DumpFormat: "custom",
			}
			if writeErr := d.Dumps.WriteMeta(meta); writeErr != nil {
				// The catalog enumerates by sidecar metadata, so a dump without
				// its meta would be an invisible orphan that never gets pruned.
				_ = os.Remove(finalPath)
				return writeErr
			}

			emit(jobs.Event{Phase: jobs.PhaseProgress, Message: "wrote " + finalPath})
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
	base, err := d.Dumps.ReadMeta(opt.BaseID)
	if err != nil {
		return &errs.Error{
			Op:    "app.backup.incremental",
			Code:  errs.CodeUser,
			Cause: err,
			Hint:  "base dump " + opt.BaseID + " not found in the catalog",
		}
	}

	since, err := basePosition(d, opt.BaseID)
	if err != nil {
		return err
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

	// Stream the change body to a temp file so we learn the end Position before
	// writing the envelope (which must carry that end Position).
	bodyTmp, err := os.CreateTemp(d.Dumps.Root(), "siphon-inc-body-*")
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

	// Assemble the final dump: envelope(end position) + change body.
	id := ulid.Make().String()
	tmpPath := filepath.Join(d.Dumps.Root(), id+".dump.tmp")
	finalPath := d.Dumps.Path(id)

	f, err := os.Create(tmpPath)
	if err != nil {
		return err
	}
	h := sha256.New()
	tee := io.MultiWriter(f, h)

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
	if _, err := dumps.WriteEnvelope(tee, env); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return err
	}

	body, err := os.Open(bodyPath)
	if err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if _, err := io.Copy(tee, body); err != nil {
		_ = body.Close()
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	_ = body.Close()

	closeErr = f.Close()
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return &errs.Error{Op: "app.backup.incremental", Code: errs.CodeSystem, Cause: closeErr, Hint: "failed to flush dump to disk (out of space?)"}
	}
	if err := os.Rename(tmpPath, finalPath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	st, _ := os.Stat(finalPath)
	size := int64(0)
	if st != nil {
		size = st.Size()
	}
	meta := &dumps.Meta{
		ID:         id,
		Profile:    opt.Profile,
		Driver:     resolved.Driver,
		SizeBytes:  size,
		Checksum:   "sha256:" + hex.EncodeToString(h.Sum(nil)),
		Created:    time.Now(),
		DumpFormat: "jsonl-changes",
		BaseID:     root,
		ParentID:   opt.BaseID,
	}
	if writeErr := d.Dumps.WriteMeta(meta); writeErr != nil {
		_ = os.Remove(finalPath)
		return writeErr
	}

	emit(jobs.Event{Phase: jobs.PhaseProgress, Message: "wrote " + finalPath})
	return nil
}

// basePosition reads the end Position recorded in the base dump's envelope. The
// next incremental resumes from here: WALEnd for Postgres, BinlogFile+BinlogEnd
// for MySQL/MariaDB.
func basePosition(d Deps, baseID string) (canonical.Position, error) {
	f, err := os.Open(d.Dumps.Path(baseID))
	if err != nil {
		return canonical.Position{}, err
	}
	defer func() { _ = f.Close() }()
	env, _, err := dumps.ReadEnvelope(f)
	if err != nil {
		return canonical.Position{}, err
	}
	return canonical.Position{
		LSN:        env.WALEnd,
		BinlogFile: env.BinlogFile,
		BinlogPos:  env.BinlogEnd,
	}, nil
}
