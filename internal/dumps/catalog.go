package dumps

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/nixrajput/siphon/internal/errs"
)

// Catalog is a filesystem-backed dump store. Phase G replaces the
// filesystem path with a Storage abstraction; the API does not change.
type Catalog struct {
	root string
}

// NewCatalog creates the directory if missing and returns a Catalog rooted there.
func NewCatalog(root string) (*Catalog, error) {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}
	return &Catalog{root: root}, nil
}

// Path returns the dump file path for the given ID.
func (c *Catalog) Path(id string) string { return filepath.Join(c.root, id+".dump") }

// MetaPath returns the sidecar JSON path for the given ID.
func (c *Catalog) MetaPath(id string) string { return filepath.Join(c.root, id+".meta.json") }

// Root returns the catalog's root directory.
func (c *Catalog) Root() string { return c.root }

// WriteMeta serializes m and writes it to <id>.meta.json.
func (c *Catalog) WriteMeta(m *Meta) error {
	body, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.MetaPath(m.ID), body, 0o600)
}

// ReadMeta loads <id>.meta.json from disk.
func (c *Catalog) ReadMeta(id string) (*Meta, error) {
	body, err := os.ReadFile(c.MetaPath(id))
	if errors.Is(err, os.ErrNotExist) {
		return nil, &errs.Error{Op: "dumps.read_meta", Code: errs.CodeUser, Cause: errs.ErrDumpCorrupt, Hint: "no metadata for " + id}
	}
	if err != nil {
		return nil, err
	}
	m := &Meta{}
	return m, json.Unmarshal(body, m)
}

// List returns metadata for every dump in the catalog, sorted newest first.
func (c *Catalog) List() ([]Meta, error) {
	entries, err := os.ReadDir(c.root)
	if err != nil {
		return nil, err
	}
	var out []Meta
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".meta.json") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".meta.json")
		m, err := c.ReadMeta(id)
		if err != nil {
			continue // skip corrupt entries; user can `dumps prune --orphans` later
		}
		out = append(out, *m)
	}
	// sort newest first
	sortByCreatedDesc(out)
	return out, nil
}

// Delete removes both the dump file and its sidecar.
func (c *Catalog) Delete(id string) error {
	_ = os.Remove(c.Path(id))
	return os.Remove(c.MetaPath(id))
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
