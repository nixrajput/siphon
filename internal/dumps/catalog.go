package dumps

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/nixrajput/siphon/internal/errs"
	"github.com/nixrajput/siphon/internal/storage"
)

// Catalog is a dump store addressed by dump ID. It holds a storage.Store
// substrate (local directory or object store) and maps each ID to two keys:
// "<id>.dump" (the dump body, envelope-prefixed) and "<id>.meta.json" (the
// sidecar metadata). Callers never see storage paths — only IDs.
type Catalog struct {
	store storage.Store
}

const (
	dumpSuffix = ".dump"
	metaSuffix = ".meta.json"
)

// New returns a Catalog over the given storage backend.
func New(store storage.Store) *Catalog {
	return &Catalog{store: store}
}

// NewCatalog returns a Catalog backed by a local directory, creating it if
// missing. Retained for callers (and tests) that want the default local
// backend without constructing a storage.Store themselves.
func NewCatalog(root string) (*Catalog, error) {
	st, err := storage.NewLocal(root)
	if err != nil {
		return nil, err
	}
	return &Catalog{store: st}, nil
}

// validID rejects IDs that could escape the storage key space. Dump IDs are
// ULIDs, but restore/inspect/verify take the ID from user input, so a crafted
// value like "../../etc/passwd" must not traverse out.
func validID(id string) error {
	if id == "" || id == "." || id == ".." ||
		strings.ContainsAny(id, `/\`) || strings.Contains(id, "..") {
		return &errs.Error{Op: "dumps.id", Code: errs.CodeUser, Cause: errors.New("invalid dump id: " + id), Hint: "dump ids contain no path separators"}
	}
	return nil
}

func dumpKey(id string) string { return id + dumpSuffix }
func metaKey(id string) string { return id + metaSuffix }

// PutDump streams r (the envelope-prefixed dump body) to the store under the
// dump key for id. The store guarantees the object is published atomically, so a
// failed or cancelled write never leaves a partial dump addressable by id.
func (c *Catalog) PutDump(ctx context.Context, id string, r io.Reader) error {
	if err := validID(id); err != nil {
		return err
	}
	return c.store.Put(ctx, dumpKey(id), r)
}

// OpenDump opens the dump body for id as a one-shot forward stream. The caller
// must Close it. A missing dump maps to a CodeUser error.
func (c *Catalog) OpenDump(ctx context.Context, id string) (io.ReadCloser, error) {
	if err := validID(id); err != nil {
		return nil, err
	}
	rc, err := c.store.Get(ctx, dumpKey(id))
	if errors.Is(err, storage.ErrNotFound) {
		return nil, &errs.Error{Op: "dumps.open", Code: errs.CodeUser, Cause: errs.ErrDumpCorrupt, Hint: "no dump body for " + id}
	}
	if err != nil {
		return nil, &errs.Error{Op: "dumps.open", Code: errs.CodeSystem, Cause: err}
	}
	return rc, nil
}

// WriteMeta serializes m and writes it under the meta key. Write meta LAST when
// publishing a dump: the catalog enumerates by meta, so a dump body without its
// meta is an invisible (prunable) orphan, whereas a meta without its body would
// be a dangling catalog entry.
func (c *Catalog) WriteMeta(ctx context.Context, m *Meta) error {
	if err := validID(m.ID); err != nil {
		return err
	}
	body, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return c.store.Put(ctx, metaKey(m.ID), strings.NewReader(string(body)))
}

// ReadMeta loads and decodes the sidecar metadata for id.
func (c *Catalog) ReadMeta(ctx context.Context, id string) (*Meta, error) {
	if err := validID(id); err != nil {
		return nil, err
	}
	rc, err := c.store.Get(ctx, metaKey(id))
	if errors.Is(err, storage.ErrNotFound) {
		return nil, &errs.Error{Op: "dumps.read_meta", Code: errs.CodeUser, Cause: errs.ErrDumpCorrupt, Hint: "no metadata for " + id}
	}
	if err != nil {
		return nil, &errs.Error{Op: "dumps.read_meta", Code: errs.CodeSystem, Cause: err}
	}
	defer func() { _ = rc.Close() }()
	body, err := io.ReadAll(rc)
	if err != nil {
		return nil, &errs.Error{Op: "dumps.read_meta", Code: errs.CodeSystem, Cause: err}
	}
	m := &Meta{}
	if err := json.Unmarshal(body, m); err != nil {
		return nil, &errs.Error{Op: "dumps.read_meta", Code: errs.CodeUser, Cause: errs.ErrDumpCorrupt, Hint: "metadata for " + id + " is corrupt"}
	}
	return m, nil
}

// List returns metadata for every dump in the catalog, sorted newest first. It
// enumerates meta keys, reading each; a corrupt entry is skipped rather than
// failing the whole listing.
func (c *Catalog) List(ctx context.Context) ([]Meta, error) {
	keys, err := c.store.List(ctx)
	if err != nil {
		return nil, &errs.Error{Op: "dumps.list", Code: errs.CodeSystem, Cause: err}
	}
	var out []Meta
	for _, k := range keys {
		if !strings.HasSuffix(k, metaSuffix) {
			continue
		}
		id := strings.TrimSuffix(k, metaSuffix)
		m, err := c.ReadMeta(ctx, id)
		if err != nil {
			continue // skip corrupt entries; user can prune later
		}
		out = append(out, *m)
	}
	sortByCreatedDesc(out)
	return out, nil
}

// Delete removes both the dump body and its sidecar. Delete is idempotent in the
// store, so a missing object is not an error.
func (c *Catalog) Delete(ctx context.Context, id string) error {
	if err := validID(id); err != nil {
		return err
	}
	if err := c.store.Delete(ctx, dumpKey(id)); err != nil {
		return &errs.Error{Op: "dumps.delete", Code: errs.CodeSystem, Cause: err}
	}
	if err := c.store.Delete(ctx, metaKey(id)); err != nil {
		return &errs.Error{Op: "dumps.delete", Code: errs.CodeSystem, Cause: err}
	}
	return nil
}

func sortByCreatedDesc(m []Meta) {
	// inline insertion sort: catalogs are typically small (< 1000 entries)
	for i := 1; i < len(m); i++ {
		j := i
		for j > 0 && m[j-1].Created.Before(m[j].Created) {
			m[j-1], m[j] = m[j], m[j-1]
			j--
		}
	}
}
