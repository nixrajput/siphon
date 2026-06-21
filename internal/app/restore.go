package app

import (
	"context"
	"os"

	"github.com/nixrajput/siphon/internal/driver"
	"github.com/nixrajput/siphon/internal/dumps"
	"github.com/nixrajput/siphon/internal/errs"
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
	UpTo         string // optional: stop applying the chain after this dump ID
}

// Restore resolves the dump's chain (base + incrementals) and applies it in
// order into the target profile. For a plain (non-incremental) dump the chain
// is a single element. --up-to stops the chain early at the named dump.
func Restore(parent context.Context, d Deps, opt RestoreOpts) (<-chan jobs.Event, string, error) {
	resolved, err := d.Profiles.Resolve(opt.Profile)
	if err != nil {
		return nil, "", err
	}
	drv, err := d.Drivers.Get(resolved.Driver)
	if err != nil {
		return nil, "", err
	}

	chain, err := d.Dumps.ResolveChain(opt.DumpID)
	if err != nil {
		return nil, "", err
	}
	if opt.UpTo != "" {
		chain, err = truncateChain(chain, opt.UpTo)
		if err != nil {
			return nil, "", err
		}
	}

	return d.Runner.Run(parent, jobs.Job{
		Stage: "restore",
		Func: func(ctx context.Context, emit func(jobs.Event)) error {
			conn, err := drv.Connect(ctx, resolved)
			if err != nil {
				return err
			}
			defer func() { _ = conn.Close() }()

			for i, m := range chain {
				emit(jobs.Event{Phase: jobs.PhaseProgress, Message: "applying " + m.ID})
				f, err := os.Open(d.Dumps.Path(m.ID))
				if err != nil {
					return err
				}
				_, body, err := dumps.ReadEnvelope(f)
				if err != nil {
					_ = f.Close()
					return err
				}
				rOpts := driver.RestoreOpts{
					TargetTables: opt.TargetTables,
					SchemaOnly:   opt.SchemaOnly,
					DataOnly:     opt.DataOnly,
					Clean:        opt.Clean && i == 0, // clean once, before the base only
				}
				if err := conn.Restore(ctx, rOpts, body); err != nil {
					_ = f.Close()
					return err
				}
				_ = f.Close()
			}
			return nil
		},
	})
}

// truncateChain returns chain up to and including the dump named upTo. Unlike
// silently applying the full chain, an unknown upTo is an error so a typo'd
// --up-to surfaces instead of restoring more than the user asked for.
func truncateChain(chain []dumps.Meta, upTo string) ([]dumps.Meta, error) {
	for i, m := range chain {
		if m.ID == upTo {
			return chain[:i+1], nil
		}
	}
	return nil, &errs.Error{
		Op:   "restore",
		Code: errs.CodeUser,
		Hint: "--up-to " + upTo + " is not in the restore chain for the target dump",
	}
}
