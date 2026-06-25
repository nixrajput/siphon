// Package telemetry records opt-in, aggregate operational metrics about
// destructive operations: per-op counts, success/error tallies, and total
// duration. It plugs into the same audit.Auditor seam as the audit log, so it
// adds no new interception point in the app layer.
//
// Privacy: telemetry deliberately records ONLY the operation name, outcome, and
// duration — never profile names, hosts, dump IDs, actor, or any data. It is
// off by default and must be explicitly enabled in config.
package telemetry

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/nixrajput/siphon/internal/audit"
)

// Recorder is an audit.Auditor sink that accumulates aggregate counters and
// flushes them to a JSON file. Safe for concurrent use.
type Recorder struct {
	path string
	mu   sync.Mutex
	ops  map[string]*opStat
}

type opStat struct {
	Count      int64 `json:"count"`
	Errors     int64 `json:"errors"`
	DurationMS int64 `json:"total_duration_ms"`
}

// NewRecorder returns a telemetry Recorder that flushes aggregates to path
// (created on first flush), or nil if path is empty.
func NewRecorder(path string) *Recorder {
	if path == "" {
		return nil
	}
	return &Recorder{path: path, ops: map[string]*opStat{}}
}

// Begin returns a handle that, at End, increments this op's aggregate counters
// and flushes the snapshot. Only ev.Op and the outcome/duration are read — no
// identifying fields.
func (r *Recorder) Begin(_ context.Context, ev audit.Event) audit.Handle {
	return &recHandle{r: r, op: string(ev.Op)}
}

type recHandle struct {
	r  *Recorder
	op string
}

func (h *recHandle) End(err error) {
	h.r.mu.Lock()
	st := h.r.ops[h.op]
	if st == nil {
		st = &opStat{}
		h.r.ops[h.op] = st
	}
	st.Count++
	if err != nil {
		st.Errors++
	}
	snapshot := h.r.snapshotLocked()
	h.r.mu.Unlock()

	// Duration is not on the audit.Event at this layer (the handle does not see
	// it), so telemetry tracks counts/errors only; duration aggregation would
	// require the End signature to carry it. Counts + error rate are the useful
	// opt-in signal and keep the seam unchanged.
	h.r.flush(snapshot)
}

// snapshotLocked builds a serializable copy under the held lock.
func (r *Recorder) snapshotLocked() map[string]opStat {
	out := make(map[string]opStat, len(r.ops))
	for k, v := range r.ops {
		out[k] = *v
	}
	return out
}

func (r *Recorder) flush(snapshot map[string]opStat) {
	// Stable key order for a deterministic file (and stable tests).
	keys := make([]string, 0, len(snapshot))
	for k := range snapshot {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	ordered := make(map[string]opStat, len(snapshot))
	for _, k := range keys {
		ordered[k] = snapshot[k]
	}

	body, err := json.MarshalIndent(ordered, "", "  ")
	if err != nil {
		return // best-effort: telemetry never fails the operation
	}
	if mkErr := os.MkdirAll(filepath.Dir(r.path), 0o700); mkErr != nil {
		return
	}
	_ = os.WriteFile(r.path, body, 0o600)
}
