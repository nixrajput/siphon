package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sort"
	"testing"
)

// RunStoreSuite exercises the Store contract against a backend. Every Store
// implementation runs this same table, so correctness is a property of
// implementing the interface — not a per-backend afterthought. newStore returns
// a fresh, empty Store for each subtest (e.g. a t.TempDir-rooted local store, or
// a uniquely-prefixed bucket view).
//
// It lives in a non-test file so both the local unit test and the S3 integration
// test (separate build tags) can call it.
func RunStoreSuite(t *testing.T, newStore func(t *testing.T) Store) {
	t.Helper()
	ctx := context.Background()

	t.Run("put then get round-trips bytes", func(t *testing.T) {
		s := newStore(t)
		want := []byte("envelope+body payload\x00with NUL and \xff bytes")
		if err := s.Put(ctx, "a.dump", bytes.NewReader(want)); err != nil {
			t.Fatalf("Put: %v", err)
		}
		rc, err := s.Get(ctx, "a.dump")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		defer func() { _ = rc.Close() }()
		got, err := io.ReadAll(rc)
		if err != nil {
			t.Fatalf("ReadAll: %v", err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("round-trip mismatch: got %q want %q", got, want)
		}
	})

	t.Run("get missing key wraps ErrNotFound", func(t *testing.T) {
		s := newStore(t)
		_, err := s.Get(ctx, "nope.dump")
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("Get missing: err = %v, want wrapping ErrNotFound", err)
		}
	})

	t.Run("stat reports size and existence", func(t *testing.T) {
		s := newStore(t)
		body := []byte("0123456789")
		if err := s.Put(ctx, "sz.dump", bytes.NewReader(body)); err != nil {
			t.Fatalf("Put: %v", err)
		}
		size, exists, err := s.Stat(ctx, "sz.dump")
		if err != nil {
			t.Fatalf("Stat: %v", err)
		}
		if !exists || size != int64(len(body)) {
			t.Errorf("Stat = (%d, %v), want (%d, true)", size, exists, len(body))
		}
		// Missing key: (0, false, nil) — absence is not an error for Stat.
		_, exists, err = s.Stat(ctx, "missing.dump")
		if err != nil || exists {
			t.Errorf("Stat missing = (exists %v, err %v), want (false, nil)", exists, err)
		}
	})

	t.Run("delete is idempotent", func(t *testing.T) {
		s := newStore(t)
		if err := s.Put(ctx, "d.dump", bytes.NewReader([]byte("x"))); err != nil {
			t.Fatalf("Put: %v", err)
		}
		if err := s.Delete(ctx, "d.dump"); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if _, exists, _ := s.Stat(ctx, "d.dump"); exists {
			t.Error("key still present after Delete")
		}
		// Deleting an absent key is not an error.
		if err := s.Delete(ctx, "d.dump"); err != nil {
			t.Errorf("Delete of absent key: %v, want nil", err)
		}
	})

	t.Run("list returns present keys", func(t *testing.T) {
		s := newStore(t)
		for _, k := range []string{"one.dump", "one.meta.json", "two.dump"} {
			if err := s.Put(ctx, k, bytes.NewReader([]byte("x"))); err != nil {
				t.Fatalf("Put %s: %v", k, err)
			}
		}
		keys, err := s.List(ctx)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		sort.Strings(keys)
		want := []string{"one.dump", "one.meta.json", "two.dump"}
		if len(keys) != len(want) {
			t.Fatalf("List = %v, want %v", keys, want)
		}
		for i := range want {
			if keys[i] != want[i] {
				t.Errorf("List[%d] = %q, want %q", i, keys[i], want[i])
			}
		}
	})

	t.Run("put overwrites existing key", func(t *testing.T) {
		s := newStore(t)
		if err := s.Put(ctx, "o.dump", bytes.NewReader([]byte("first"))); err != nil {
			t.Fatalf("Put 1: %v", err)
		}
		if err := s.Put(ctx, "o.dump", bytes.NewReader([]byte("second"))); err != nil {
			t.Fatalf("Put 2: %v", err)
		}
		rc, err := s.Get(ctx, "o.dump")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		defer func() { _ = rc.Close() }()
		got, _ := io.ReadAll(rc)
		if string(got) != "second" {
			t.Errorf("after overwrite got %q, want %q", got, "second")
		}
	})

	t.Run("cancelled context aborts put without publishing", func(t *testing.T) {
		s := newStore(t)
		cctx, cancel := context.WithCancel(ctx)
		cancel() // already cancelled
		err := s.Put(cctx, "cancel.dump", bytes.NewReader([]byte("data")))
		if err == nil {
			t.Fatal("Put with cancelled context returned nil, want error")
		}
		// The key must NOT be visible — a cancelled Put leaves no partial object.
		if _, exists, _ := s.Stat(ctx, "cancel.dump"); exists {
			t.Error("cancelled Put left a partial object visible")
		}
	})
}
