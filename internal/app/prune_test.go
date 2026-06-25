package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/nixrajput/siphon/internal/audit"
	"github.com/nixrajput/siphon/internal/dumps"
	"github.com/nixrajput/siphon/internal/jobs"
)

// recordingStore is an in-memory storage.Store that records the order of Delete
// calls, so prune's leaf-inward deletion order can be asserted. A key in
// failKeys returns an error from Delete (to test collected-failure handling).
type recordingStore struct {
	objects  map[string][]byte
	deletes  []string // keys passed to Delete, in call order
	failKeys map[string]bool
}

func newRecordingStore() *recordingStore {
	return &recordingStore{objects: map[string][]byte{}, failKeys: map[string]bool{}}
}

func (s *recordingStore) Put(_ context.Context, key string, r io.Reader) error {
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	s.objects[key] = b
	return nil
}

func (s *recordingStore) Get(_ context.Context, key string) (io.ReadCloser, error) {
	b, ok := s.objects[key]
	if !ok {
		return nil, errors.New("not found")
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}

func (s *recordingStore) Delete(_ context.Context, key string) error {
	s.deletes = append(s.deletes, key)
	if s.failKeys[key] {
		return errors.New("simulated delete failure")
	}
	delete(s.objects, key)
	return nil
}

func (s *recordingStore) List(_ context.Context) ([]string, error) {
	keys := make([]string, 0, len(s.objects))
	for k := range s.objects {
		keys = append(keys, k)
	}
	return keys, nil
}

func (s *recordingStore) Stat(_ context.Context, key string) (int64, bool, error) {
	b, ok := s.objects[key]
	return int64(len(b)), ok, nil
}

// seedDump writes a dump body + meta directly into the catalog over the store.
func seedDump(t *testing.T, cat *dumps.Catalog, id, profile string, created time.Time, baseID, parentID string) {
	t.Helper()
	ctx := context.Background()
	if err := cat.PutDump(ctx, id, strings.NewReader("body-"+id)); err != nil {
		t.Fatalf("PutDump %s: %v", id, err)
	}
	if err := cat.WriteMeta(ctx, &dumps.Meta{
		ID: id, Profile: profile, Driver: "fake", Created: created,
		BaseID: baseID, ParentID: parentID, SizeBytes: 10,
	}); err != nil {
		t.Fatalf("WriteMeta %s: %v", id, err)
	}
}

func pruneDeps(store *recordingStore) Deps {
	return Deps{Dumps: dumps.New(store), Runner: jobs.NewRunner()}
}

func TestPrune_DryRunDeletesNothing(t *testing.T) {
	store := newRecordingStore()
	cat := dumps.New(store)
	now := time.Now()
	seedDump(t, cat, "new", "p", now, "", "")
	seedDump(t, cat, "old", "p", now.AddDate(0, 0, -40), "", "")

	res, err := Prune(context.Background(), pruneDeps(store), PruneOpts{
		Policy: dumps.RetentionPolicy{KeepLast: 1}, Apply: false,
	})
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if len(store.deletes) != 0 {
		t.Errorf("dry-run performed %d deletes, want 0: %v", len(store.deletes), store.deletes)
	}
	// The plan still reports one chain as pruned.
	var prunedChains int
	for _, oc := range res.Outcomes {
		if oc.Pruned {
			prunedChains++
		}
	}
	if prunedChains != 1 {
		t.Errorf("plan pruned %d chains, want 1", prunedChains)
	}
}

func TestPrune_ApplyDeletesLeafInward(t *testing.T) {
	store := newRecordingStore()
	cat := dumps.New(store)
	now := time.Now()
	// Keep this fresh single chain.
	seedDump(t, cat, "keep", "p", now, "", "")
	// Prune this old base + 2 incrementals; deletion must be inc-before-base.
	seedDump(t, cat, "base", "p", now.AddDate(0, 0, -40), "base", "")
	seedDump(t, cat, "inc1", "p", now.AddDate(0, 0, -39), "base", "base")
	seedDump(t, cat, "inc2", "p", now.AddDate(0, 0, -38), "base", "inc1")

	res, err := Prune(context.Background(), pruneDeps(store), PruneOpts{
		Policy: dumps.RetentionPolicy{KeepLast: 1}, Apply: true,
	})
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if res.Failed != 0 {
		t.Fatalf("Failed = %d, want 0", res.Failed)
	}
	// Order: the chain's members delete newest-first, i.e. inc2.dump, inc2.meta,
	// inc1.dump, inc1.meta, base.dump, base.meta. The base's keys must come AFTER
	// both incrementals' keys.
	lastBase := lastIndexWithPrefix(store.deletes, "base")
	firstInc := firstIndexWithPrefix(store.deletes, "inc")
	if lastBase < 0 || firstInc < 0 {
		t.Fatalf("missing deletes: %v", store.deletes)
	}
	if !lastBeforeFirst(store.deletes, "inc2", "inc1") || !lastBeforeFirst(store.deletes, "inc1", "base") {
		t.Errorf("deletion order not leaf-inward: %v", store.deletes)
	}
	// "keep" must be untouched.
	for _, k := range store.deletes {
		if strings.HasPrefix(k, "keep") {
			t.Errorf("kept chain's dump was deleted: %s", k)
		}
	}
}

func TestPrune_ProfileScope(t *testing.T) {
	store := newRecordingStore()
	cat := dumps.New(store)
	old := time.Now().AddDate(0, 0, -40)
	seedDump(t, cat, "p1old", "p1", old, "", "")
	seedDump(t, cat, "p2old", "p2", old, "", "")

	// Prune p1 only, keep-last 0 + max-age tiny so the old chain is pruned.
	_, err := Prune(context.Background(), pruneDeps(store), PruneOpts{
		Profile: "p1", Policy: dumps.RetentionPolicy{MaxAge: time.Hour}, Apply: true,
	})
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	for _, k := range store.deletes {
		if strings.HasPrefix(k, "p2") {
			t.Errorf("prune of profile p1 deleted a p2 dump: %s", k)
		}
	}
	if firstIndexWithPrefix(store.deletes, "p1old") < 0 {
		t.Errorf("p1's old chain was not pruned: %v", store.deletes)
	}
}

func TestPrune_CollectsDeletionFailures(t *testing.T) {
	store := newRecordingStore()
	cat := dumps.New(store)
	old := time.Now().AddDate(0, 0, -40)
	seedDump(t, cat, "old", "p", old, "", "")
	store.failKeys["old.dump"] = true // the body delete will fail

	res, err := Prune(context.Background(), pruneDeps(store), PruneOpts{
		Policy: dumps.RetentionPolicy{MaxAge: time.Hour}, Apply: true,
	})
	if err != nil {
		t.Fatalf("Prune returned error, want collected-failure result: %v", err)
	}
	if res.Failed != 1 {
		t.Errorf("Failed = %d, want 1", res.Failed)
	}
}

// TestPrune_StopsChainOnFirstFailure proves the orphan-prevention invariant:
// when deleting the leaf incremental of a pruned chain fails, the prune must NOT
// proceed to delete the base — otherwise the surviving leaf is orphaned.
func TestPrune_StopsChainOnFirstFailure(t *testing.T) {
	store := newRecordingStore()
	cat := dumps.New(store)
	old := time.Now().AddDate(0, 0, -40)
	seedDump(t, cat, "base", "p", old, "base", "")
	seedDump(t, cat, "inc", "p", old.Add(time.Hour), "base", "base")
	// The leaf (inc) is deleted first (leaf-inward). Make its meta delete fail —
	// Catalog.Delete removes meta first, so this is the first Delete call.
	store.failKeys["inc.meta.json"] = true

	res, err := Prune(context.Background(), pruneDeps(store), PruneOpts{
		Policy: dumps.RetentionPolicy{MaxAge: time.Hour}, Apply: true,
	})
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if res.Failed != 1 {
		t.Errorf("Failed = %d, want 1", res.Failed)
	}
	// The base must NOT have been deleted — neither its meta nor its body.
	for _, k := range store.deletes {
		if strings.HasPrefix(k, "base") {
			t.Errorf("base was deleted after leaf failure (orphans the leaf): deletes=%v", store.deletes)
		}
	}
}

// fakeAuditor records the events it sees, and the outcome passed to End.
type fakeAuditor struct {
	begun []audit.Event
	ended []error
}

func (a *fakeAuditor) Begin(_ context.Context, ev audit.Event) audit.Handle {
	a.begun = append(a.begun, ev)
	return &fakeHandle{a: a}
}

type fakeHandle struct{ a *fakeAuditor }

func (h *fakeHandle) End(err error) { h.a.ended = append(h.a.ended, err) }

// blockGate denies every operation.
type blockGate struct{ err error }

func (g blockGate) Authorize(_ context.Context, _ audit.Op, _ string) error { return g.err }

func TestPrune_AuditsTheOperation(t *testing.T) {
	store := newRecordingStore()
	cat := dumps.New(store)
	seedDump(t, cat, "a", "p", time.Now(), "", "")
	a := &fakeAuditor{}
	deps := Deps{Dumps: cat, Runner: jobs.NewRunner(), Auditor: a, Actor: "alice"}

	if _, err := Prune(context.Background(), deps, PruneOpts{Policy: dumps.RetentionPolicy{KeepLast: 1}}); err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if len(a.begun) != 1 || a.begun[0].Op != audit.OpPrune || a.begun[0].Actor != "alice" {
		t.Fatalf("audit Begin = %+v, want one prune by alice", a.begun)
	}
	if len(a.ended) != 1 || a.ended[0] != nil {
		t.Errorf("audit End = %v, want one nil (success)", a.ended)
	}
}

func TestPrune_GateBlocksAndIsNotAudited(t *testing.T) {
	store := newRecordingStore()
	cat := dumps.New(store)
	seedDump(t, cat, "a", "p", time.Now(), "", "")
	a := &fakeAuditor{}
	denied := errors.New("2FA required")
	deps := Deps{Dumps: cat, Runner: jobs.NewRunner(), Auditor: a, Gate: blockGate{err: denied}}

	_, err := Prune(context.Background(), deps, PruneOpts{Policy: dumps.RetentionPolicy{KeepLast: 1}, Apply: true})
	if !errors.Is(err, denied) {
		t.Fatalf("Prune err = %v, want the gate's denial", err)
	}
	// Blocked before running: nothing deleted, and no audit Begin (the op never started).
	if len(store.deletes) != 0 {
		t.Errorf("gate-blocked prune still deleted: %v", store.deletes)
	}
	if len(a.begun) != 0 {
		t.Errorf("gate-blocked op was audited as begun: %+v", a.begun)
	}
}

// helpers

func firstIndexWithPrefix(s []string, p string) int {
	for i, v := range s {
		if strings.HasPrefix(v, p) {
			return i
		}
	}
	return -1
}

func lastIndexWithPrefix(s []string, p string) int {
	idx := -1
	for i, v := range s {
		if strings.HasPrefix(v, p) {
			idx = i
		}
	}
	return idx
}

// lastBeforeFirst reports whether the last delete of prefix a precedes the first
// delete of prefix b — i.e. all of a's keys come before any of b's.
func lastBeforeFirst(s []string, a, b string) bool {
	return lastIndexWithPrefix(s, a) < firstIndexWithPrefix(s, b)
}
