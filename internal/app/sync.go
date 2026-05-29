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

// Sync backs up From and restores into To. If Stream==true and both
// drivers report NativeStream, the data flows through an in-memory pipe
// (no temp file). Otherwise it falls back to Backup-then-Restore via
// the catalog (Phase B implementation).
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
				errCh <- srcConn.Backup(ctx, driver.BackupOpts{IncludeTables: opt.Tables}, pw)
				_ = pw.Close()
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
