package cli

import (
	"errors"
	"io"
	"testing"

	"github.com/nixrajput/siphon/internal/errs"
	"github.com/nixrajput/siphon/internal/jobs"
)

func TestHeartbeat_ReturnsNilOnSuccess(t *testing.T) {
	ch := make(chan jobs.Event, 8)
	ch <- jobs.Event{Stage: "backup", Phase: jobs.PhaseStarted}
	ch <- jobs.Event{Stage: "backup", Phase: jobs.PhaseDone}
	close(ch)

	if err := Heartbeat(io.Discard, ch); err != nil {
		t.Fatalf("Heartbeat() = %v; want nil", err)
	}
}

func TestHeartbeat_ReturnsErrorOnFailure(t *testing.T) {
	jobErr := &errs.Error{Op: "backup", Code: errs.CodeSystem, Cause: errs.ErrConnectionFailed}
	ch := make(chan jobs.Event, 8)
	ch <- jobs.Event{Stage: "backup", Phase: jobs.PhaseStarted}
	ch <- jobs.Event{Stage: "backup", Phase: jobs.PhaseError, Err: jobErr}
	close(ch)

	err := Heartbeat(io.Discard, ch)
	if err == nil {
		t.Fatal("Heartbeat() = nil; want error")
	}
	var e *errs.Error
	if !errors.As(err, &e) {
		t.Fatalf("Heartbeat() error = %T; want *errs.Error", err)
	}
	if e.Code.ExitCode() != 2 {
		t.Fatalf("ExitCode() = %d; want 2", e.Code.ExitCode())
	}
}

func TestHeartbeat_ReturnsCancelledCode(t *testing.T) {
	ch := make(chan jobs.Event, 8)
	ch <- jobs.Event{Stage: "backup", Phase: jobs.PhaseStarted}
	ch <- jobs.Event{Stage: "backup", Phase: jobs.PhaseCancelled}
	close(ch)

	err := Heartbeat(io.Discard, ch)
	if err == nil {
		t.Fatal("Heartbeat() = nil; want error")
	}
	var e *errs.Error
	if !errors.As(err, &e) {
		t.Fatalf("Heartbeat() error = %T; want *errs.Error", err)
	}
	if e.Code.ExitCode() != 130 {
		t.Fatalf("ExitCode() = %d; want 130", e.Code.ExitCode())
	}
}
