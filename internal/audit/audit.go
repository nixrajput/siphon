// Package audit records an append-only trail of destructive operations
// (backup, restore, sync, prune) — who ran what, against which profile, when,
// and whether it succeeded. It is a stdlib-only leaf: the app layer holds an
// Auditor and calls Begin before a destructive verb and End after it.
//
// The same Begin/End seam is the interception point reused by other cross-
// cutting concerns (2FA gating runs as a pre-check before the verb; telemetry
// records timing/outcome) — they are layered on top rather than each re-wrapping
// every verb.
package audit

import (
	"context"
	"time"
)

// Op names a destructive operation, used as the audit record's action.
type Op string

const (
	OpBackup  Op = "backup"
	OpRestore Op = "restore"
	OpSync    Op = "sync"
	OpPrune   Op = "prune"
)

// Event is one audit record. Outcome and DurationMS are filled in at End; the
// rest are known at Begin.
type Event struct {
	Time       time.Time `json:"time"`
	Op         Op        `json:"op"`
	Profile    string    `json:"profile,omitempty"`
	Target     string    `json:"target,omitempty"` // e.g. sync destination, dump id
	Actor      string    `json:"actor"`            // OS user who ran the command
	Outcome    string    `json:"outcome"`          // "ok" | "error" (set at End)
	Err        string    `json:"error,omitempty"`  // error text when Outcome == "error"
	DurationMS int64     `json:"duration_ms"`      // wall time Begin→End
}

// Auditor records audit events. A nil Auditor is valid and is a no-op, so the
// app layer can call Begin/End unconditionally without nil checks.
type Auditor interface {
	// Begin records the start of op and returns a handle to End. Implementations
	// may persist a "started" record or defer all writes to End; the app layer
	// does not care which.
	Begin(ctx context.Context, ev Event) Handle
}

// Handle finalizes one operation's audit record.
type Handle interface {
	// End records the operation's outcome. err == nil means success. It is safe
	// to call End on a zero/nil Handle (no-op).
	End(err error)
}

// Record runs Begin, returns a function to defer that calls End with the
// verb's error. Usage in an app verb:
//
//	done := audit.Record(ctx, d.Auditor, audit.Event{Op: audit.OpPrune, Profile: p, Actor: actor})
//	defer func() { done(retErr) }()
//
// A nil auditor yields a no-op done func, so callers need no nil check.
func Record(ctx context.Context, a Auditor, ev Event) func(error) {
	if a == nil {
		return func(error) {}
	}
	h := a.Begin(ctx, ev)
	if h == nil {
		return func(error) {}
	}
	return h.End
}
