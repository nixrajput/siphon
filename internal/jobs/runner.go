package jobs

import (
	"context"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
)

// Job is the unit a Runner executes. Real work lives in Func; the runner
// provides ID, cancellation, and event-channel plumbing.
type Job struct {
	Stage string // "dump", "restore", "sync"
	Func  func(ctx context.Context, emit func(Event)) error
}

type JobStatus struct {
	ID      string
	Stage   string
	Started time.Time
	Cancel  context.CancelFunc
	Done    bool
}

// Runner orchestrates concurrent jobs. Methods are safe for concurrent use.
type Runner struct {
	mu     sync.Mutex
	active map[string]*JobStatus
}

func NewRunner() *Runner {
	return &Runner{active: map[string]*JobStatus{}}
}

// Run starts j in a goroutine and returns a buffered channel of Events.
// The channel is closed when the job ends (success, error, or cancel).
func (r *Runner) Run(parent context.Context, j Job) (<-chan Event, string, error) {
	id := ulid.Make().String()
	ctx, cancel := context.WithCancel(parent)

	r.mu.Lock()
	r.active[id] = &JobStatus{ID: id, Stage: j.Stage, Started: time.Now(), Cancel: cancel}
	r.mu.Unlock()

	ch := make(chan Event, 128)
	stamp := func(e Event) Event {
		e.JobID = id
		e.Stage = j.Stage
		if e.At.IsZero() {
			e.At = time.Now()
		}
		return e
	}
	emit := func(e Event) {
		select {
		case ch <- stamp(e):
		case <-ctx.Done():
		}
	}
	// send always delivers even after ctx is cancelled (used for terminal events).
	send := func(e Event) { ch <- stamp(e) }

	go func() {
		defer close(ch)
		defer r.markDone(id)

		emit(Event{Phase: PhaseStarted})
		err := j.Func(ctx, emit)
		switch {
		case err == nil:
			send(Event{Phase: PhaseDone})
		case ctx.Err() != nil:
			send(Event{Phase: PhaseCancelled, Err: ctx.Err()})
		default:
			send(Event{Phase: PhaseError, Err: err})
		}
	}()

	return ch, id, nil
}

func (r *Runner) markDone(id string) {
	r.mu.Lock()
	if s, ok := r.active[id]; ok {
		s.Done = true
	}
	r.mu.Unlock()
}

// Active returns a snapshot of in-flight jobs.
func (r *Runner) Active() []JobStatus {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]JobStatus, 0, len(r.active))
	for _, s := range r.active {
		if !s.Done {
			out = append(out, *s)
		}
	}
	return out
}

// Cancel signals the named job's context to finish.
func (r *Runner) Cancel(id string) {
	r.mu.Lock()
	if s, ok := r.active[id]; ok && s.Cancel != nil {
		s.Cancel()
	}
	r.mu.Unlock()
}
