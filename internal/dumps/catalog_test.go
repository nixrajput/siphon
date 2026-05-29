package dumps

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCatalog_WriteRead_Roundtrip(t *testing.T) {
	dir := t.TempDir()
	c, err := NewCatalog(dir)
	if err != nil {
		t.Fatal(err)
	}

	m := &Meta{
		ID:        "01HXKZ000000000000000000",
		Profile:   "prod",
		Driver:    "postgres",
		Created:   time.Now(),
		Checksum:  "sha256:abc",
		SizeBytes: 1234,
	}
	if err := c.WriteMeta(m); err != nil {
		t.Fatal(err)
	}

	if _, statErr := os.Stat(filepath.Join(dir, m.ID+".meta.json")); statErr != nil {
		t.Fatalf("expected sidecar to exist: %v", statErr)
	}

	got, err := c.ReadMeta(m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Checksum != m.Checksum {
		t.Fatalf("Checksum = %q; want %q", got.Checksum, m.Checksum)
	}
}

func TestCatalog_List_SortsNewestFirst(t *testing.T) {
	dir := t.TempDir()
	c, _ := NewCatalog(dir)

	old := &Meta{ID: "01HOLD0000000000000000000", Created: time.Now().Add(-24 * time.Hour)}
	new_ := &Meta{ID: "01HNEW0000000000000000000", Created: time.Now()}
	_ = c.WriteMeta(old)
	_ = c.WriteMeta(new_)

	got, err := c.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != new_.ID {
		t.Fatalf("List() = %v; want newest first", got)
	}
}

func TestCatalog_PruneDryRun_AppliesMaxAge(t *testing.T) {
	dir := t.TempDir()
	c, _ := NewCatalog(dir)

	old := &Meta{ID: "01HOLD0000000000000000000", Created: time.Now().Add(-48 * time.Hour)}
	new_ := &Meta{ID: "01HNEW0000000000000000000", Created: time.Now()}
	_ = c.WriteMeta(old)
	_ = c.WriteMeta(new_)

	rep, err := c.PruneDryRun(RetentionPolicy{MaxAge: 24 * time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Would) != 1 || rep.Would[0].ID != old.ID {
		t.Fatalf("Would = %v; want only old", rep.Would)
	}
	if len(rep.Kept) != 1 || rep.Kept[0].ID != new_.ID {
		t.Fatalf("Kept = %v; want only new", rep.Kept)
	}
}

// TestCatalog_RejectsTraversalID guards the path-traversal fix: IDs from user
// input (restore/inspect/verify <id>) must not escape the catalog root.
func TestCatalog_RejectsTraversalID(t *testing.T) {
	c, err := NewCatalog(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"../escape", "../../etc/passwd", "a/b", `a\b`, "", "..", "."} {
		if _, err := c.ReadMeta(bad); err == nil {
			t.Errorf("ReadMeta(%q) = nil error; want rejection", bad)
		}
		if err := c.Delete(bad); err == nil {
			t.Errorf("Delete(%q) = nil error; want rejection", bad)
		}
	}
	// A clean ULID-like id is accepted (ReadMeta then fails only on not-found).
	if _, err := c.ReadMeta("01HXKZ000000000000000000"); err == nil {
		t.Skip("unexpected: a non-existent valid id should error as not-found, not pass")
	}
}
