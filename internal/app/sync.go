package app

import (
	"context"
	"io"

	"github.com/nixrajput/siphon/internal/driver"
	"github.com/nixrajput/siphon/internal/jobs"
)

// SyncOpts configures the Sync verb.
type SyncOpts struct {
	From   string
	To     string
	Stream bool
	Tables []string
}

// Sync backs up From and restores into To. In Phase B the data always flows
// through an in-memory io.Pipe (no temp file, no catalog entry). The opt.Stream
// flag and Capabilities().NativeStream gating — and a catalog fallback for
// drivers that can't stream — are planned for Phase F; today Stream is unused.
func Sync(parent context.Context, d Deps, opt SyncOpts) (<-chan jobs.Event, string, error) {
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

			pr, pw := io.Pipe()
			errCh := make(chan error, 1)

			go func() {
				bErr := srcConn.Backup(ctx, driver.BackupOpts{IncludeTables: opt.Tables}, pw)
				// CloseWithError propagates a backup failure to the reader as a
				// read error instead of a clean io.EOF. Without this, a truncated
				// dump looks complete to Restore — which, with Clean:true, has
				// already dropped the target and would commit partial data.
				_ = pw.CloseWithError(bErr)
				errCh <- bErr
			}()

			restoreErr := dstConn.Restore(ctx, driver.RestoreOpts{TargetTables: opt.Tables, Clean: true}, pr)
			_ = pr.Close() // unblock the backup goroutine's pw.Write immediately if Restore returned early
			backupErr := <-errCh

			if backupErr != nil {
				return backupErr
			}
			return restoreErr
		},
	})
}
