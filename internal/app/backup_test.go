package app

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nixrajput/siphon/internal/canonical"
	"github.com/nixrajput/siphon/internal/config"
	"github.com/nixrajput/siphon/internal/driver"
	"github.com/nixrajput/siphon/internal/dumps"
	"github.com/nixrajput/siphon/internal/jobs"
	"github.com/nixrajput/siphon/internal/profile"
	"github.com/nixrajput/siphon/internal/secrets"
)

type fakeDriver struct {
	name    string
	payload string
}

func (f *fakeDriver) Name() string { return f.name }
func (f *fakeDriver) Capabilities() driver.Capabilities {
	return driver.Capabilities{NativeStream: true}
}
func (f *fakeDriver) Connect(_ context.Context, _ driver.Profile) (driver.Conn, error) {
	return &fakeConn{payload: f.payload}, nil
}

type fakeConn struct{ payload string }

func (c *fakeConn) Inspect(_ context.Context) (*driver.Schema, error) { return &driver.Schema{}, nil }
func (c *fakeConn) Backup(_ context.Context, _ driver.BackupOpts, w io.Writer) error {
	_, err := io.WriteString(w, c.payload)
	return err
}
func (c *fakeConn) Restore(_ context.Context, _ driver.RestoreOpts, _ io.Reader) error {
	return nil
}
func (c *fakeConn) Verify(_ context.Context, _ io.Reader) (*driver.VerifyReport, error) {
	return &driver.VerifyReport{OK: true}, nil
}
func (c *fakeConn) Close() error { return nil }

type fakeGetter struct{ d driver.Driver }

func (f fakeGetter) Get(_ string) (driver.Driver, error) { return f.d, nil }

func TestBackup_WritesDumpAndMeta(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SIPHON_CONFIG_HOME", t.TempDir())

	cfg := &config.Config{Profiles: map[string]config.ProfileConfig{
		"test": {Driver: "fake", Host: "h", User: "u", Database: "d", Password: "p"},
	}}
	res := secrets.NewResolver(secrets.Passthrough{})
	ps := profile.New(cfg, res, func(*config.Config) error { return nil })
	cat, _ := dumps.NewCatalog(dir)
	runner := jobs.NewRunner()

	deps := Deps{
		Profiles: ps,
		Dumps:    cat,
		Runner:   runner,
		Drivers:  fakeGetter{d: &fakeDriver{name: "fake", payload: "hello-dump"}},
	}

	ch, _, err := Backup(context.Background(), deps, BackupOpts{Profile: "test"})
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}
	for range ch { /* drain */
	}

	got, err := cat.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("catalog.List() = %d entries; want 1", len(got))
	}
	if !strings.HasPrefix(got[0].Checksum, "sha256:") {
		t.Fatalf("Checksum = %q; want sha256: prefix", got[0].Checksum)
	}
}

// posConn is a fakeConn that also implements driver.BasePositioner, so a full
// backup stamps the returned position into the base envelope.
type posConn struct {
	fakeConn
	pos canonical.Position
}

func (c *posConn) CurrentPosition(_ context.Context) (canonical.Position, error) {
	return c.pos, nil
}

type posDriver struct {
	payload string
	pos     canonical.Position
}

func (posDriver) Name() string                      { return "fake" }
func (posDriver) Capabilities() driver.Capabilities { return driver.Capabilities{Incremental: true} }
func (d posDriver) Connect(_ context.Context, _ driver.Profile) (driver.Conn, error) {
	return &posConn{fakeConn: fakeConn{payload: d.payload}, pos: d.pos}, nil
}

// TestBackup_FullStampsBasePosition asserts the gap is closed: a full backup
// whose driver implements BasePositioner records the engine position in the base
// dump's envelope, so a later incremental's basePosition() resumes from a real
// position instead of the zero value (which would silently drop changes between
// the base dump and the first incremental).
func TestBackup_FullStampsBasePosition(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SIPHON_CONFIG_HOME", t.TempDir())

	cfg := &config.Config{Profiles: map[string]config.ProfileConfig{
		"test": {Driver: "fake", Host: "h", User: "u", Database: "d", Password: "p"},
	}}
	res := secrets.NewResolver(secrets.Passthrough{})
	ps := profile.New(cfg, res, func(*config.Config) error { return nil })
	cat, _ := dumps.NewCatalog(dir)

	deps := Deps{
		Profiles: ps,
		Dumps:    cat,
		Runner:   jobs.NewRunner(),
		Drivers:  fakeGetter{d: posDriver{payload: "hello-dump", pos: canonical.Position{LSN: "0/16B6358"}}},
	}

	ch, _, err := Backup(context.Background(), deps, BackupOpts{Profile: "test"})
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}
	for range ch { /* drain */
	}

	got, err := cat.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("catalog.List() = %d entries; want 1", len(got))
	}

	pos, err := basePosition(deps, got[0].ID)
	if err != nil {
		t.Fatalf("basePosition: %v", err)
	}
	if pos.LSN != "0/16B6358" {
		t.Fatalf("base envelope WALEnd = %q; want 0/16B6358 (position not stamped on full backup)", pos.LSN)
	}
}

// failBackupConn fails mid-dump, exercising the backup-error branch that must
// remove the temp file and create no catalog entry.
type failBackupConn struct{ fakeConn }

func (failBackupConn) Backup(_ context.Context, _ driver.BackupOpts, _ io.Writer) error {
	return errors.New("pg_dump exploded")
}

type failBackupDriver struct{}

func (failBackupDriver) Name() string                      { return "fake" }
func (failBackupDriver) Capabilities() driver.Capabilities { return driver.Capabilities{} }
func (failBackupDriver) Connect(_ context.Context, _ driver.Profile) (driver.Conn, error) {
	return &failBackupConn{}, nil
}

func TestBackup_DumpError_CleansTmpAndWritesNoCatalogEntry(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SIPHON_CONFIG_HOME", t.TempDir())

	cfg := &config.Config{Profiles: map[string]config.ProfileConfig{
		"test": {Driver: "fake", Host: "h", User: "u", Database: "d", Password: "p"},
	}}
	res := secrets.NewResolver(secrets.Passthrough{})
	ps := profile.New(cfg, res, func(*config.Config) error { return nil })
	cat, _ := dumps.NewCatalog(dir)

	deps := Deps{
		Profiles: ps,
		Dumps:    cat,
		Runner:   jobs.NewRunner(),
		Drivers:  fakeGetter{d: failBackupDriver{}},
	}

	ch, _, err := Backup(context.Background(), deps, BackupOpts{Profile: "test"})
	if err != nil {
		t.Fatalf("Backup setup: %v", err)
	}
	var lastErr error
	for e := range ch {
		if e.Err != nil {
			lastErr = e.Err
		}
	}
	if lastErr == nil {
		t.Fatal("expected a failure event carrying the backup error")
	}

	// No catalog entry should have been recorded.
	got, err := cat.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("catalog.List() = %d; want 0 on backup failure", len(got))
	}

	// No leftover .dump or .dump.tmp files should remain in the catalog dir.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, de := range entries {
		name := de.Name()
		if strings.HasSuffix(name, ".dump") || strings.HasSuffix(name, ".dump.tmp") {
			t.Fatalf("leftover artifact after failed backup: %s", filepath.Join(dir, name))
		}
	}
}
