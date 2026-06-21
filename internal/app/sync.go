package app

import (
	"context"

	"github.com/nixrajput/siphon/internal/driver"
	"github.com/nixrajput/siphon/internal/errs"
	"github.com/nixrajput/siphon/internal/jobs"
)

// SyncOpts configures the Sync verb.
type SyncOpts struct {
	From   string
	To     string
	Stream bool
	Tables []string
	// CrossEngine routes through the canonical-schema path (e.g. postgres→mysql)
	// instead of the native homogeneous stream. Gated on driver capability.
	CrossEngine bool
	// Continuous requests CDC follow mode. Not wired here — see `siphon cdc`
	// (Phase F Task 10). Setting it returns a clear CodeUser error.
	Continuous bool
}

// Sync backs up From and restores into To in a single pass. The native
// (homogeneous) path streams the dump through a bounded jobs.Stream — no temp
// file, no catalog entry — so backpressure is observable via FillPercent while
// a backup failure still propagates to Restore as a read error (via CloseErr),
// preventing a truncated dump from being committed as if clean.
//
// When opt.CrossEngine is set the work routes through runCrossEngineSync, which
// is capability-gated and today returns a clear unsupported error (no driver
// declares cross-engine source/target yet).
func Sync(parent context.Context, d Deps, opt SyncOpts) (<-chan jobs.Event, string, error) {
	if opt.Continuous {
		return nil, "", &errs.Error{
			Op:    "sync.continuous",
			Code:  errs.CodeUser,
			Cause: errs.ErrDriverUnsupported,
			Hint:  "continuous CDC sync is not wired here; use `siphon cdc` (Phase F Task 10)",
		}
	}

	src, err := d.Profiles.Resolve(opt.From)
	if err != nil {
		return nil, "", err
	}
	dst, err := d.Profiles.Resolve(opt.To)
	if err != nil {
		return nil, "", err
	}
	srcDrv, err := d.Drivers.Get(src.Driver)
	if err != nil {
		return nil, "", err
	}
	dstDrv, err := d.Drivers.Get(dst.Driver)
	if err != nil {
		return nil, "", err
	}

	if opt.CrossEngine {
		return runCrossEngineSync(parent, d, opt)
	}

	return d.Runner.Run(parent, jobs.Job{
		Stage: "sync",
		Func: func(ctx context.Context, emit func(jobs.Event)) error {
			srcConn, err := srcDrv.Connect(ctx, src)
			if err != nil {
				return err
			}
			defer func() { _ = srcConn.Close() }()

			dstConn, err := dstDrv.Connect(ctx, dst)
			if err != nil {
				return err
			}
			defer func() { _ = dstConn.Close() }()

			// Stream is both the backup writer and the restore reader (it is
			// io.Reader+io.Writer): the backup goroutine writes, Restore reads.
			stream := jobs.NewStream(64)
			errCh := make(chan error, 1)

			go func() {
				bErr := srcConn.Backup(ctx, driver.BackupOpts{IncludeTables: opt.Tables}, stream)
				// CloseErr propagates a backup failure to the reader as a read
				// error instead of a clean io.EOF (the bounded-buffer analogue of
				// io.PipeWriter.CloseWithError). Without this, a truncated dump
				// looks complete to Restore — which, with Clean:true, has already
				// dropped the target and would commit partial data. A nil bErr
				// behaves like Close → clean EOF.
				_ = stream.CloseErr(bErr)
				errCh <- bErr
			}()

			restoreErr := dstConn.Restore(ctx, driver.RestoreOpts{TargetTables: opt.Tables, Clean: true}, stream)
			_ = stream.Close() // unblock the backup goroutine's Write immediately if Restore returned early
			backupErr := <-errCh

			if backupErr != nil {
				return backupErr
			}
			return restoreErr
		},
	})
}

// runCrossEngineSync handles heterogeneous sync (e.g. postgres→mysql) by routing
// through the canonical-schema machinery (driver EmitCanonical/ConsumeCanonical).
//
// It is HONESTLY gated: it checks CapCrossEngineSource on the source driver and
// CapCrossEngineTarget on the target. No driver declares either today (all
// return false — see each driver.go), because cross-engine translation needs a
// typed CanonicalSchema and driver.Inspect carries no column types yet. So the
// capability gate rejects every cross-engine request with ErrDriverUnsupported.
//
// This is deliberate scaffolding: the canonical emit/consume machinery (Task 8)
// exists and is unit-tested, but wiring it requires typed schema introspection
// (a future task). We do NOT fabricate a CanonicalSchema here. Once Inspect
// emits column types and a driver flips its cross-engine caps to true, this
// function gains the real emit→translate→consume pipeline.
func runCrossEngineSync(parent context.Context, d Deps, opt SyncOpts) (<-chan jobs.Event, string, error) {
	if err := RequireCapability(d, opt.From, CapCrossEngineSource); err != nil {
		return nil, "", err
	}
	if err := RequireCapability(d, opt.To, CapCrossEngineTarget); err != nil {
		return nil, "", err
	}
	// Unreachable today (no driver advertises cross-engine caps), but kept as a
	// clear, honest backstop should a cap ever flip true before the pipeline is
	// actually wired.
	return nil, "", &errs.Error{
		Op:    "sync.cross_engine",
		Code:  errs.CodeUser,
		Cause: errs.ErrDriverUnsupported,
		Hint:  "cross-engine sync requires typed schema introspection, not yet available",
	}
}
