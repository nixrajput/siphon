package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// localStore is a Store backed by a single local directory. Keys map directly to
// file names within root. Put is made atomic with the classic write-temp +
// rename dance, so a reader that fails mid-stream never leaves a partial object
// visible under its final name.
type localStore struct {
	root string
}

// NewLocal returns a Store backed by the directory root, creating it (0700) if
// absent. This preserves siphon's pre-Phase-G on-disk layout: keys are written
// verbatim as files under root, so an existing local catalog keeps working with
// no migration.
func NewLocal(root string) (Store, error) {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("storage.local: mkdir %s: %w", root, err)
	}
	return &localStore{root: root}, nil
}

// safeName rejects keys that could escape root once joined. Keys are
// siphon-internal ("<id>.dump"), but defense-in-depth here mirrors the catalog's
// own validID guard.
func (s *localStore) path(key string) (string, error) {
	if key == "" || strings.ContainsAny(key, `/\`) || strings.Contains(key, "..") {
		return "", fmt.Errorf("storage.local: invalid key %q", key)
	}
	return filepath.Join(s.root, key), nil
}

func (s *localStore) Put(ctx context.Context, key string, r io.Reader) error {
	p, err := s.path(key)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.root, ".tmp-"+key+"-*")
	if err != nil {
		return fmt.Errorf("storage.local: temp for %s: %w", key, err)
	}
	tmpName := tmp.Name()
	// Copy with cancellation: a cancelled context aborts before the rename, so no
	// partial object becomes visible under the final key.
	if _, err := copyCtx(ctx, tmp, r); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("storage.local: write %s: %w", key, err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("storage.local: flush %s: %w", key, err)
	}
	if err := os.Rename(tmpName, p); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("storage.local: publish %s: %w", key, err)
	}
	return nil
}

func (s *localStore) Get(_ context.Context, key string) (io.ReadCloser, error) {
	p, err := s.path(key)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(p)
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("storage.local: %s: %w", key, ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("storage.local: open %s: %w", key, err)
	}
	return f, nil
}

func (s *localStore) Delete(_ context.Context, key string) error {
	p, err := s.path(key)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("storage.local: delete %s: %w", key, err)
	}
	return nil // idempotent: a missing key is not an error
}

func (s *localStore) List(_ context.Context) ([]string, error) {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil, fmt.Errorf("storage.local: list %s: %w", s.root, err)
	}
	var keys []string
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".tmp-") {
			continue // skip subdirs and in-flight temp files
		}
		keys = append(keys, e.Name())
	}
	return keys, nil
}

func (s *localStore) Stat(_ context.Context, key string) (int64, bool, error) {
	p, err := s.path(key)
	if err != nil {
		return 0, false, err
	}
	fi, err := os.Stat(p)
	if os.IsNotExist(err) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("storage.local: stat %s: %w", key, err)
	}
	return fi.Size(), true, nil
}

// copyCtx copies r→w like io.Copy but aborts when ctx is cancelled. It checks
// the context between chunks so a long upload/download stops promptly.
func copyCtx(ctx context.Context, w io.Writer, r io.Reader) (int64, error) {
	buf := make([]byte, 32*1024)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		n, rerr := r.Read(buf)
		if n > 0 {
			wn, werr := w.Write(buf[:n])
			total += int64(wn)
			if werr != nil {
				return total, werr
			}
		}
		if rerr == io.EOF {
			return total, nil
		}
		if rerr != nil {
			return total, rerr
		}
	}
}
