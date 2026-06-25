package app

import (
	"context"

	"github.com/nixrajput/siphon/internal/audit"
	"github.com/nixrajput/siphon/internal/jobs"
)

// guardedOp is the single interception point wrapping every destructive verb.
// It (1) runs the authorization Gate (2FA / confirmation) before the operation
// and (2) opens an audit record. It returns a done(err) the caller defers to
// finalize the audit outcome. If the Gate blocks, it returns a non-nil error and
// a no-op done — the caller must not run the operation.
//
// Layering all destructive concerns here means each verb gains one guard call
// at entry and one deferred done(err); audit, 2FA gating, and (later) telemetry
// hook in here rather than each re-wrapping every verb.
func guardedOp(ctx context.Context, d Deps, op audit.Op, profile, target string) (done func(error), err error) {
	if d.Gate != nil {
		if gErr := d.Gate.Authorize(ctx, op, profile); gErr != nil {
			return func(error) {}, gErr
		}
	}
	rec := audit.Record(ctx, d.Auditor, audit.Event{
		Op:      op,
		Profile: profile,
		Target:  target,
		Actor:   d.Actor,
	})
	return rec, nil
}

// launchGuarded runs a job whose Func already defers done(retErr) on completion,
// and finalizes the audit record if the launch ITSELF fails synchronously
// (Runner.Run cannot today, but the signature allows it, and an open record must
// not leak). On a launch error it calls done and returns the error.
func launchGuarded(r *jobs.Runner, parent context.Context, done func(error), j jobs.Job) (<-chan jobs.Event, string, error) {
	ch, id, err := r.Run(parent, j)
	if err != nil {
		done(err)
		return nil, "", err
	}
	return ch, id, nil
}
