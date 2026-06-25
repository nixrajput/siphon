package app

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/nixrajput/siphon/internal/config"
	"github.com/nixrajput/siphon/internal/driver"
	"github.com/nixrajput/siphon/internal/dumps"
	"github.com/nixrajput/siphon/internal/jobs"
	"github.com/nixrajput/siphon/internal/profile"
	"github.com/nixrajput/siphon/internal/secrets"
)

// rtFakeConn writes payload on Backup and captures whatever Restore reads, so a
// test can assert the bytes survive the catalog file round-trip.
type rtFakeConn struct {
	payload  string
	restored []byte
}

func (c *rtFakeConn) Inspect(_ context.Context) (*driver.Schema, error) {
	return &driver.Schema{}, nil
}

func (c *rtFakeConn) Backup(_ context.Context, _ driver.BackupOpts, w io.Writer) error {
	_, err := io.WriteString(w, c.payload)
	return err
}

func (c *rtFakeConn) Restore(_ context.Context, _ driver.RestoreOpts, r io.Reader) error {
	b, err := io.ReadAll(r)
	c.restored = b
	return err
}

func (c *rtFakeConn) Verify(_ context.Context, _ io.Reader) (*driver.VerifyReport, error) {
	return &driver.VerifyReport{OK: true}, nil
}

func (c *rtFakeConn) Close() error { return nil }

// rtFakeDriver always hands back the same rtFakeConn so the test can inspect it.
type rtFakeDriver struct{ conn driver.Conn }

func (d *rtFakeDriver) Name() string { return "fake" }
func (d *rtFakeDriver) Capabilities() driver.Capabilities {
	return driver.Capabilities{NativeStream: true}
}
func (d *rtFakeDriver) Connect(_ context.Context, _ driver.Profile) (driver.Conn, error) {
	return d.conn, nil
}

// TestBackupRestoreVerify_Roundtrip proves the catalog path-convention contract
// between Backup, Verify, and Restore without Docker: Backup writes a dump +
// meta, Verify recomputes the checksum and matches the stored meta.Checksum, and
// Restore reads back the exact bytes Backup wrote.
func TestBackupRestoreVerify_Roundtrip(t *testing.T) {
	const payload = "roundtrip-payload"

	dir := t.TempDir()
	t.Setenv("SIPHON_CONFIG_HOME", t.TempDir())

	cfg := &config.Config{Profiles: map[string]config.ProfileConfig{
		"test": {Driver: "fake", Host: "h", User: "u", Database: "d", Password: "p"},
	}}
	res := secrets.NewResolver(secrets.Passthrough{})
	ps := profile.New(cfg, res, func(*config.Config) error { return nil })
	cat, _ := dumps.NewCatalog(dir)
	runner := jobs.NewRunner()

	conn := &rtFakeConn{payload: payload}
	deps := Deps{
		Profiles: ps,
		Dumps:    cat,
		Runner:   runner,
		Drivers:  fakeGetter{d: &rtFakeDriver{conn: conn}},
	}

	ctx := context.Background()

	// 1. Backup.
	bch, _, err := Backup(ctx, deps, BackupOpts{Profile: "test"})
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}
	drain(t, bch)

	entries, err := cat.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("catalog.List() = %d entries; want 1", len(entries))
	}
	id := entries[0].ID

	// 2. Verify — file on disk must match the meta.Checksum Backup wrote.
	report, err := Verify(ctx, deps, id)
	if err != nil {
		t.Fatalf("Verify: unexpected error: %v", err)
	}
	if report == nil || !report.OK {
		t.Fatalf("Verify: report=%+v; want non-nil OK report", report)
	}

	// 3. Restore — must open the same path Backup wrote.
	rch, _, err := Restore(ctx, deps, RestoreOpts{Profile: "test", DumpID: id})
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	drain(t, rch)

	// 4. Byte-level round-trip through the catalog file.
	if got := string(conn.restored); got != payload {
		t.Fatalf("Restore read %q; want %q", got, payload)
	}
}

// drain consumes a job event channel with a hard timeout.
func drain(t *testing.T, ch <-chan jobs.Event) {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return
			}
		case <-timer.C:
			t.Fatal("job did not complete within 5 s")
		}
	}
}
