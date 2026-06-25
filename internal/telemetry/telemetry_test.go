package telemetry

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/nixrajput/siphon/internal/audit"
)

func TestRecorder_AggregatesCountsAndErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "telemetry.json")
	r := NewRecorder(path)
	ctx := context.Background()

	// 2 backups (1 ok, 1 error), 1 prune ok.
	r.Begin(ctx, audit.Event{Op: audit.OpBackup}).End(nil)
	r.Begin(ctx, audit.Event{Op: audit.OpBackup}).End(errors.New("boom"))
	r.Begin(ctx, audit.Event{Op: audit.OpPrune}).End(nil)

	stats := readStats(t, path)
	if stats["backup"].Count != 2 || stats["backup"].Errors != 1 {
		t.Errorf("backup stats = %+v, want count=2 errors=1", stats["backup"])
	}
	if stats["prune"].Count != 1 || stats["prune"].Errors != 0 {
		t.Errorf("prune stats = %+v, want count=1 errors=0", stats["prune"])
	}
}

func TestRecorder_RecordsOnlyOpNotIdentifyingFields(t *testing.T) {
	// Telemetry must not persist profile/actor/target. The on-disk JSON is keyed
	// by op only; assert no identifying string leaks into the file.
	path := filepath.Join(t.TempDir(), "telemetry.json")
	r := NewRecorder(path)
	r.Begin(context.Background(), audit.Event{
		Op: audit.OpRestore, Profile: "prod-secret", Actor: "alice", Target: "dump-xyz",
	}).End(nil)

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read telemetry: %v", err)
	}
	for _, leak := range []string{"prod-secret", "alice", "dump-xyz"} {
		if contains(string(body), leak) {
			t.Errorf("telemetry leaked identifying field %q: %s", leak, body)
		}
	}
}

func TestNewRecorder_EmptyPathIsNil(t *testing.T) {
	if NewRecorder("") != nil {
		t.Error("empty path should yield a nil recorder (disabled)")
	}
}

func TestMulti_FansOutToAllSinks(t *testing.T) {
	a := &countingAuditor{}
	b := &countingAuditor{}
	m := audit.NewMulti(a, nil, b) // nil skipped
	if m == nil {
		t.Fatal("NewMulti of live auditors returned nil")
	}
	m.Begin(context.Background(), audit.Event{Op: audit.OpSync}).End(nil)
	if a.begins != 1 || a.ends != 1 || b.begins != 1 || b.ends != 1 {
		t.Errorf("fan-out = a(%d,%d) b(%d,%d), want each (1,1)", a.begins, a.ends, b.begins, b.ends)
	}
}

func TestMulti_AllNilIsNil(t *testing.T) {
	if audit.NewMulti(nil, nil) != nil {
		t.Error("NewMulti of only nils should be nil (no-op)")
	}
}

type countingAuditor struct{ begins, ends int }

func (c *countingAuditor) Begin(_ context.Context, _ audit.Event) audit.Handle {
	c.begins++
	return &countingHandle{c: c}
}

type countingHandle struct{ c *countingAuditor }

func (h *countingHandle) End(error) { h.c.ends++ }

func readStats(t *testing.T, path string) map[string]opStat {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read telemetry: %v", err)
	}
	var m map[string]opStat
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("bad telemetry JSON: %v", err)
	}
	return m
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
