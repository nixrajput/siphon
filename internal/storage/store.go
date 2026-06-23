// Package storage abstracts where dump objects and their sidecar metadata
// physically live. The dump catalog (internal/dumps) holds a Store and addresses
// objects by opaque keys (e.g. "<id>.dump", "<id>.meta.json") rather than
// filesystem paths, so the same catalog logic works over a local directory or an
// object store (S3 and S3-compatible services such as MinIO, R2).
//
// This package is a stdlib-only leaf: it defines the Store interface plus a
// shared not-found sentinel. Concrete backends live in sibling files (local.go,
// s3.go) and may import third-party SDKs; the interface itself does not.
package storage

import (
	"context"
	"errors"
	"io"
)

// ErrNotFound is returned by Get/Stat/Delete when the requested key is absent.
// Backends MUST wrap their native "no such object" error with this sentinel
// (via fmt.Errorf("...: %w", ErrNotFound) or by returning it directly) so the
// catalog and app layers can distinguish "you asked for something that isn't
// here" (a user error) from a transient transport failure (a system error).
var ErrNotFound = errors.New("storage: object not found")

// Store is the durable key→bytes substrate behind the dump catalog.
//
// Keys are opaque, caller-chosen strings. A backend may map a key onto a file
// name or an object key, but callers never depend on that mapping.
//
// All methods take a context: object I/O is network I/O for remote backends and
// must be cancellable. A cancelled context aborts the operation.
type Store interface {
	// Put writes the full contents of r under key, durably and atomically: the
	// key either resolves to the complete object or does not resolve at all — a
	// reader that fails mid-stream, or a cancelled context, must not leave a
	// partial object visible under key. Overwriting an existing key is allowed
	// and replaces it. Put reads r to EOF.
	Put(ctx context.Context, key string, r io.Reader) error

	// Get opens key for reading. The returned ReadCloser is a one-shot forward
	// stream — callers must not assume it is seekable — and must be closed. A
	// missing key returns an error wrapping ErrNotFound.
	Get(ctx context.Context, key string) (io.ReadCloser, error)

	// Delete removes key. Deleting a key that does not exist is NOT an error
	// (delete is idempotent), so callers can prune without racing existence.
	Delete(ctx context.Context, key string) error

	// List returns every key currently present in the store, in no guaranteed
	// order.
	List(ctx context.Context) ([]string, error)

	// Stat reports the size in bytes and existence of key. A missing key returns
	// (0, false, nil) — absence is not an error for Stat. A transport failure
	// returns a non-nil error.
	Stat(ctx context.Context, key string) (size int64, exists bool, err error)
}
