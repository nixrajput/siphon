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

			id := ulid.Make().String()
			tmpPath := filepath.Join(d.Dumps.Root(), id+".dump.tmp")
			finalPath := d.Dumps.Path(id)

			f, err := os.Create(tmpPath)
			if err != nil {
				return err
			}
			h := sha256.New()
			tee := io.MultiWriter(f, h)

			emit(jobs.Event{Phase: jobs.PhaseProgress, Message: "dumping"})

			backupErr := conn.Backup(ctx, driver.BackupOpts{
				IncludeTables:    opt.IncludeTables,
				ExcludeTables:    opt.ExcludeTables,
				ExcludeDataFrom:  opt.ExcludeDataFrom,
				SchemaOnly:       opt.SchemaOnly,
				DataOnly:         opt.DataOnly,
				CompressionLevel: opt.CompressionLevel,
				Parallel:         opt.Parallel,
			}, tee)

			// Close the dump file explicitly and check the error: for a file
			// being WRITTEN, Close() is where buffered data is flushed and where
			// late I/O failures (ENOSPC, quota, disk error) surface. Ignoring it
			// could rename a truncated dump into the catalog with a checksum that
			// matches the truncated bytes — corrupt-but-looks-valid. The pg_dump
			// error takes precedence if both occurred.
			closeErr := f.Close()
			if backupErr != nil {
				_ = os.Remove(tmpPath)
				return backupErr
			}
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
