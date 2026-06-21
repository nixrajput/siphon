package app

import (
	"context"
	"os"

	"github.com/nixrajput/siphon/internal/driver"
	"github.com/nixrajput/siphon/internal/dumps"
	"github.com/nixrajput/siphon/internal/jobs"
)

// RestoreOpts configures the Restore verb.
type RestoreOpts struct {
	Profile      string
	DumpID       string
	TargetTables []string
	SchemaOnly   bool
	DataOnly     bool
	Clean        bool
}

// Restore replays a dump from the catalog into the target profile.
func Restore(parent context.Context, d Deps, opt RestoreOpts) (<-chan jobs.Event, string, error) {
	resolved, err := d.Profiles.Resolve(opt.Profile)
	if err != nil {
		return nil, "", err
	}
	drv, err := d.Drivers.Get(resolved.Driver)
	if err != nil {
		return nil, "", err
	}

	return d.Runner.Run(parent, jobs.Job{
		Stage: "restore",
		Func: func(ctx context.Context, emit func(jobs.Event)) error {
			conn, err := drv.Connect(ctx, resolved)
			if err != nil {
				return err
			}
			defer func() { _ = conn.Close() }()

			f, err := os.Open(d.Dumps.Path(opt.DumpID))
			if err != nil {
				return err
			}
			defer func() { _ = f.Close() }()

			_, body, err := dumps.ReadEnvelope(f)
			if err != nil {
				return err
			}

			emit(jobs.Event{Phase: jobs.PhaseProgress, Message: "restoring " + opt.DumpID})

			return conn.Restore(ctx, driver.RestoreOpts{
				TargetTables: opt.TargetTables,
				SchemaOnly:   opt.SchemaOnly,
				DataOnly:     opt.DataOnly,
				Clean:        opt.Clean,
			}, body)
		},
	})
}
