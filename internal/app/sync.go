package app

import (
	"context"

	"github.com/nixrajput/siphon/internal/audit"
	"github.com/nixrajput/siphon/internal/canonical"
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
	// Continuous requests CDC follow mode: routes to RunCDC, which tails the
	// source change stream and applies each change to the target (same-engine
	// and cross-engine), resumable via the CDC state file.
	Continuous bool
}

// Sync backs up From and restores into To in a single pass. The native
// (homogeneous) path streams the dump through a bounded jobs.Stream — no temp
// file, no catalog entry — so backpressure is observable via FillPercent while
// a backup failure still propagates to Restore as a read error (via CloseErr),
// preventing a truncated dump from being committed as if clean.
//
// When opt.CrossEngine is set the work routes through runCrossEngineSync, which
// is capability-gated and uses driver.SchemaInspector + driver.CanonicalTransfer
// for typed cross-engine snapshot transfer.
func Sync(parent context.Context, d Deps, opt SyncOpts) (<-chan jobs.Event, string, error) {
	// CDC is a standalone verb (also invoked directly by the cdc command); it
	// runs the guard itself, so route to it before guarding here to avoid a
	// double gate/audit.
	if opt.Continuous {
		return RunCDC(parent, d, opt)
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

	// Guard BEFORE routing to the cross-engine path, so every non-CDC sync
	// variant (native and cross-engine — both destructive) is gated and audited.
	done, err := guardedOp(parent, d, audit.OpSync, opt.From, opt.To)
	if err != nil {
		return nil, "", err
	}

	if opt.CrossEngine {
		return runCrossEngineSync(parent, d, opt, done)
	}

	return launchGuarded(d.Runner, parent, done, jobs.Job{
		Stage: "sync",
		Func: func(ctx context.Context, emit func(jobs.Event)) (retErr error) {
			defer func() { done(retErr) }()
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

// resolveCrossEngine checks cross-engine capability and resolves both profiles
// and their drivers. Bundled so runCrossEngineSync has a single launch-failure
// path (which finalizes the audit record once).
func resolveCrossEngine(d Deps, opt SyncOpts) (src, dst driver.Profile, srcDrv, dstDrv driver.Driver, err error) {
	if err = RequireCapability(d, opt.From, CapCrossEngineSource); err != nil {
		return
	}
	if err = RequireCapability(d, opt.To, CapCrossEngineTarget); err != nil {
		return
	}
	if src, err = d.Profiles.Resolve(opt.From); err != nil {
		return
	}
	if dst, err = d.Profiles.Resolve(opt.To); err != nil {
		return
	}
	if srcDrv, err = d.Drivers.Get(src.Driver); err != nil {
		return
	}
	dstDrv, err = d.Drivers.Get(dst.Driver)
	return
}

// runCrossEngineSync handles heterogeneous sync (e.g. postgres→mysql) via the
// canonical-schema machinery: InspectSchema builds a CanonicalSchema,
// EmitCanonical streams it as JSONL, ConsumeCanonical replays it on the target.
// Both ends must implement SchemaInspector and CanonicalTransfer. The audit
// record opened by the caller's guard is finalized via done.
func runCrossEngineSync(parent context.Context, d Deps, opt SyncOpts, done func(error)) (<-chan jobs.Event, string, error) {
	// Resolve everything needed to launch; any failure here finalizes the audit
	// record (the op was authorized but could not start) and aborts.
	src, dst, srcDrv, dstDrv, launchErr := resolveCrossEngine(d, opt)
	if launchErr != nil {
		done(launchErr)
		return nil, "", launchErr
	}

	return launchGuarded(d.Runner, parent, done, jobs.Job{
		Stage: "sync.cross-engine",
		Func: func(ctx context.Context, emit func(jobs.Event)) (retErr error) {
			defer func() { done(retErr) }()
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

			srcInsp, ok := srcConn.(driver.SchemaInspector)
			if !ok {
				return &errs.Error{
					Op:    "sync.cross_engine",
					Code:  errs.CodeUser,
					Cause: errs.ErrDriverUnsupported,
					Hint:  src.Driver + " driver does not implement SchemaInspector",
				}
			}
			srcXfer, ok := srcConn.(driver.CanonicalTransfer)
			if !ok {
				return &errs.Error{
					Op:    "sync.cross_engine",
					Code:  errs.CodeUser,
					Cause: errs.ErrDriverUnsupported,
					Hint:  src.Driver + " driver does not implement CanonicalTransfer",
				}
			}
			dstXfer, ok := dstConn.(driver.CanonicalTransfer)
			if !ok {
				return &errs.Error{
					Op:    "sync.cross_engine",
					Code:  errs.CodeUser,
					Cause: errs.ErrDriverUnsupported,
					Hint:  dst.Driver + " driver does not implement CanonicalTransfer",
				}
			}

			schema, err := srcInsp.InspectSchema(ctx)
			if err != nil {
				return err
			}
			// Honor --table: native sync passes opt.Tables into Backup, so the
			// cross-engine path must filter the canonical schema the same way or it
			// would copy every table regardless of the requested subset.
			schema = filterSchemaTables(schema, opt.Tables)

			stream := jobs.NewStream(64)
			errCh := make(chan error, 1)

			go func() {
				emitErr := srcXfer.EmitCanonical(ctx, schema, stream)
				_ = stream.CloseErr(emitErr)
				errCh <- emitErr
			}()

			consumeErr := dstXfer.ConsumeCanonical(ctx, stream)
			_ = stream.Close()
			emitErr := <-errCh

			if emitErr != nil {
				return emitErr
			}
			return consumeErr
		},
	})
}

// tableAllowed reports whether table t passes a --table filter. An empty filter
// (the default) allows every table.
func tableAllowed(t string, filter []string) bool {
	if len(filter) == 0 {
		return true
	}
	for _, want := range filter {
		if want == t {
			return true
		}
	}
	return false
}

// filterSchemaTables returns a schema containing only the tables that pass the
// --table filter. An empty filter returns the schema unchanged.
func filterSchemaTables(schema *canonical.CanonicalSchema, filter []string) *canonical.CanonicalSchema {
	if len(filter) == 0 {
		return schema
	}
	out := &canonical.CanonicalSchema{}
	for _, t := range schema.Tables {
		if tableAllowed(t.Name, filter) {
			out.Tables = append(out.Tables, t)
		}
	}
	return out
}
