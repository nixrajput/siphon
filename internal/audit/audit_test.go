package audit

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRecord_NilAuditorIsNoOp(t *testing.T) {
	// Record with a nil Auditor must return a usable no-op done func (the app
	// layer calls it unconditionally).
	done := Record(context.Background(), nil, Event{Op: OpBackup})
	done(nil)
	done(errors.New("x")) // must not panic
}

func TestFileAuditor_WritesJSONLOnEnd(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")
	clock := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	a, err := NewFileAuditor(path, func() time.Time { return clock })
	if err != nil {
		t.Fatalf("NewFileAuditor: %v", err)
	}

	// One successful op, one failed op.
	Record(context.Background(), a, Event{Op: OpBackup, Profile: "prod", Actor: "alice"})(nil)
	Record(context.Background(), a, Event{Op: OpRestore, Profile: "prod", Target: "dump1", Actor: "alice"})(errors.New("boom"))

	events := readAudit(t, path)
	if len(events) != 2 {
		t.Fatalf("got %d audit lines, want 2", len(events))
	}
	if events[0].Op != OpBackup || events[0].Outcome != "ok" || events[0].Err != "" {
		t.Errorf("event[0] = %+v, want backup/ok/no-err", events[0])
	}
	if events[0].Actor != "alice" || events[0].Profile != "prod" {
		t.Errorf("event[0] attribution = %+v, want actor=alice profile=prod", events[0])
	}
	if events[1].Op != OpRestore || events[1].Outcome != "error" || events[1].Err != "boom" {
		t.Errorf("event[1] = %+v, want restore/error/boom", events[1])
	}
	if events[1].Target != "dump1" {
		t.Errorf("event[1].Target = %q, want dump1", events[1].Target)
	}
}

func TestFileAuditor_AppendsAcrossCalls(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")
	a, _ := NewFileAuditor(path, nil)
	for i := 0; i < 3; i++ {
		Record(context.Background(), a, Event{Op: OpPrune})(nil)
	}
	if got := len(readAudit(t, path)); got != 3 {
		t.Errorf("append: got %d lines, want 3", got)
	}
}

func readAudit(t *testing.T, path string) []Event {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open audit log: %v", err)
	}
	defer func() { _ = f.Close() }()
	var out []Event
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if len(sc.Bytes()) == 0 {
			continue
		}
		var ev Event
		if err := json.Unmarshal(sc.Bytes(), &ev); err != nil {
			t.Fatalf("bad audit JSON %q: %v", sc.Text(), err)
		}
		out = append(out, ev)
	}
	return out
}
