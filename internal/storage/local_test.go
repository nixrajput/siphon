package storage

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalStore_Contract(t *testing.T) {
	RunStoreSuite(t, func(t *testing.T) Store {
		s, err := NewLocal(t.TempDir())
		if err != nil {
			t.Fatalf("NewLocal: %v", err)
		}
		return s
	})
}

// TestLocalStore_AtomicPublish proves Put never leaves a partial object under
// the final key: a reader that errors mid-stream must abort before the rename,
// so the key does not resolve.
func TestLocalStore_AtomicPublish(t *testing.T) {
	dir := t.TempDir()
	s, err := NewLocal(dir)
	if err != nil {
		t.Fatalf("NewLocal: %v", err)
	}
	err = s.Put(context.Background(), "x.dump", &failingReader{after: 4})
	if err == nil {
		t.Fatal("Put with failing reader returned nil, want error")
	}
	if _, exists, _ := s.Stat(context.Background(), "x.dump"); exists {
		t.Error("partial object visible under final key after failed Put")
	}
	// And no leftover temp file should linger.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 0 {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("dir not clean after failed Put: %v", names)
	}
}

// TestLocalStore_KeyLayout pins the on-disk layout: a key maps verbatim to a
// file of that name under root, so pre-Phase-G local catalogs keep working.
func TestLocalStore_KeyLayout(t *testing.T) {
	dir := t.TempDir()
	s, err := NewLocal(dir)
	if err != nil {
		t.Fatalf("NewLocal: %v", err)
	}
	if err := s.Put(context.Background(), "01H.dump", strings.NewReader("body")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "01H.dump")); err != nil {
		t.Errorf("expected file %s on disk: %v", filepath.Join(dir, "01H.dump"), err)
	}
}

// TestAtomicRename_PreservesDstOnUnrelatedError proves the narrow guard: when a
// rename fails for a reason OTHER than "destination exists" (here, a missing
// source → ErrNotExist), atomicRename must NOT delete the existing destination.
// The earlier broad "remove dst on any error" would have destroyed the last
// good copy of a dump on a transient glitch.
func TestAtomicRename_PreservesDstOnUnrelatedError(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "good.dump")
	if err := os.WriteFile(dst, []byte("the only good copy"), 0o600); err != nil {
		t.Fatalf("seed dst: %v", err)
	}
	missingSrc := filepath.Join(dir, "does-not-exist.tmp")

	if err := atomicRename(missingSrc, dst); err == nil {
		t.Fatal("atomicRename with missing source returned nil, want error")
	}
	// The existing destination must still be intact.
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("dst was destroyed by a failed rename: %v", err)
	}
	if string(got) != "the only good copy" {
		t.Errorf("dst content = %q, want it untouched", got)
	}
}

// failingReader returns `after` bytes then a non-EOF error, simulating a body
// source that dies mid-stream.
type failingReader struct {
	after int
	n     int
}

func (r *failingReader) Read(p []byte) (int, error) {
	if r.n >= r.after {
		return 0, errReadFailed
	}
	p[0] = 'x'
	r.n++
	return 1, nil
}

var errReadFailed = osErr("simulated read failure")

type osErr string

func (e osErr) Error() string { return string(e) }
