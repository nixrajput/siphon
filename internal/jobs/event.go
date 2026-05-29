// Package jobs runs long-running siphon operations and streams progress
// events to subscribers. Job IDs are ULIDs.
package jobs

import "time"

type Phase int

const (
	PhaseStarted Phase = iota
	PhaseProgress
	PhaseWarn
	PhaseDone
	PhaseError
	PhaseCancelled
)

func (p Phase) String() string {
	return [...]string{"started", "progress", "warn", "done", "error", "cancelled"}[p]
}

// Event is one step in a job's lifecycle. Drivers + the runner emit Events
// on a shared channel that the CLI heartbeat and TUI job panel consume.
type Event struct {
	JobID    string
	Stage    string // "dump", "verify", "restore", etc.
	Phase    Phase
	Progress *Progress
	Message  string
	Err      error
	At       time.Time
}

// Progress is the live counters block. All fields are optional; populate
// what your stage can measure.
type Progress struct {
	BytesDone, BytesTotal     int64
	RowsDone, RowsTotal       int64
	TableActive, TablesDone   string
	TablesTotal, TablesPassed int
}
