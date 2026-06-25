package audit

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// FileAuditor appends audit events as JSONL to a single log file. Writes are
// serialized by a mutex so concurrent verbs (e.g. parallel jobs) don't interleave
// partial lines. A write error is swallowed: auditing must never fail the
// operation it records (a backup that succeeded must not report failure because
// the audit line couldn't be written) — best-effort, append-only.
type FileAuditor struct {
	path string
	mu   sync.Mutex
	now  func() time.Time // injectable for tests
}

// NewFileAuditor returns an Auditor appending to path, creating its parent
// directory. now defaults to time.Now when nil.
func NewFileAuditor(path string, now func() time.Time) (*FileAuditor, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	if now == nil {
		now = time.Now
	}
	return &FileAuditor{path: path, now: now}, nil
}

func (a *FileAuditor) Begin(_ context.Context, ev Event) Handle {
	ev.Time = a.now()
	return &fileHandle{a: a, ev: ev, start: ev.Time}
}

func (a *FileAuditor) append(ev Event) {
	line, err := json.Marshal(ev)
	if err != nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	f, err := os.OpenFile(a.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return // best-effort: never fail the audited operation
	}
	defer func() { _ = f.Close() }()
	_, _ = f.Write(append(line, '\n'))
}

type fileHandle struct {
	a     *FileAuditor
	ev    Event
	start time.Time
}

func (h *fileHandle) End(err error) {
	h.ev.DurationMS = h.a.now().Sub(h.start).Milliseconds()
	if err != nil {
		h.ev.Outcome = "error"
		h.ev.Err = err.Error()
	} else {
		h.ev.Outcome = "ok"
	}
	h.a.append(h.ev)
}
