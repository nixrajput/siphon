package jobs

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRunner_Run_StartedAndDoneEmitted(t *testing.T) {
	r := NewRunner()
	ch, _, err := r.Run(context.Background(), Job{
		Stage: "noop",
		Func:  func(context.Context, func(Event)) error { return nil },
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	phases := drainPhases(ch)
	wantPhases := []Phase{PhaseStarted, PhaseDone}
	if !equalPhases(phases, wantPhases) {
		t.Fatalf("phases = %v; want %v", phases, wantPhases)
	}
}

func TestRunner_Run_ErrorPropagates(t *testing.T) {
	r := NewRunner()
	boom := errors.New("boom")
	ch, _, _ := r.Run(context.Background(), Job{
		Stage: "fail",
		Func:  func(context.Context, func(Event)) error { return boom },
	})

	phases := drainPhases(ch)
	if phases[len(phases)-1] != PhaseError {
		t.Fatalf("last phase = %v; want PhaseError", phases[len(phases)-1])
	}
}

func TestRunner_Cancel_PropagatesToFunc(t *testing.T) {
	r := NewRunner()
	started := make(chan struct{})

	ch, id, _ := r.Run(context.Background(), Job{
		Stage: "sleep",
		Func: func(ctx context.Context, _ func(Event)) error {
			close(started)
			<-ctx.Done()
			return ctx.Err()
		},
	})

	<-started
	r.Cancel(id)

	phases := drainPhases(ch)
	if phases[len(phases)-1] != PhaseCancelled {
		t.Fatalf("last phase = %v; want PhaseCancelled", phases[len(phases)-1])
	}
}

func drain(ch <-chan Event) []Event {
	var out []Event
	timeout := time.After(2 * time.Second)
	for {
		select {
		case e, ok := <-ch:
			if !ok {
				return out
			}
			out = append(out, e)
		case <-timeout:
			return out
		}
	}
}

func drainPhases(ch <-chan Event) []Phase {
	events := drain(ch)
	out := make([]Phase, len(events))
	for i, e := range events {
		out[i] = e.Phase
	}
	return out
}

func equalPhases(a, b []Phase) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
