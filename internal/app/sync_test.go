package app

import (
	"bytes"
	"context"
	"errors"
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

// syncFakeConn is a fakeConn variant whose Restore behaviour is configurable.
// Backup writes a payload large enough (>64 KiB) to fill the kernel pipe buffer
// so that pw.Write would block indefinitely if pr is never closed.
type syncFakeConn struct {
	restoreErr error
	backupData []byte
}

func (c *syncFakeConn) Inspect(_ context.Context) (*driver.Schema, error) {
	return &driver.Schema{}, nil
}

func (c *syncFakeConn) Backup(_ context.Context, _ driver.BackupOpts, w io.Writer) error {
	_, err := io.Copy(w, bytes.NewReader(c.backupData))
	return err
}

func (c *syncFakeConn) Restore(_ context.Context, _ driver.RestoreOpts, _ io.Reader) error {
	// Return immediately without reading the pipe — simulates an early error.
	return c.restoreErr
}

func (c *syncFakeConn) Verify(_ context.Context, _ io.Reader) (*driver.VerifyReport, error) {
	return &driver.VerifyReport{OK: true}, nil
}

func (c *syncFakeConn) Close() error { return nil }

// syncFakeDriver always returns the same syncFakeConn for every Connect call.
type syncFakeDriver struct {
	conn driver.Conn
}

func (d *syncFakeDriver) Name() string { return "syncfake" }
func (d *syncFakeDriver) Capabilities() driver.Capabilities {
	return driver.Capabilities{NativeStream: true}
}
func (d *syncFakeDriver) Connect(_ context.Context, _ driver.Profile) (driver.Conn, error) {
	return d.conn, nil
}

// syncDualGetter returns the src driver for "src" and the dst driver for "dst".
type syncDualGetter struct {
	src driver.Driver
	dst driver.Driver
}

func (g syncDualGetter) Get(name string) (driver.Driver, error) {
	if name == "dst" {
		return g.dst, nil
	}
	return g.src, nil
}

// TestSync_PipeReaderClosed_NoGoroutineLeak verifies that when Restore returns
// an error immediately without draining the pipe, Sync still completes (the job
// channel closes) within a short timeout.  Before the pr.Close() fix the backup
// goroutine's pw.Write would block forever, causing Sync to hang.
func TestSync_PipeReaderClosed_NoGoroutineLeak(t *testing.T) {
	// 128 KiB > typical pipe buffer (64 KiB on Linux/macOS), so pw.Write
	// blocks if pr is not closed after Restore returns early.
	bigPayload := make([]byte, 128*1024)

	restoreErr := errors.New("restore failed intentionally")

	srcConn := &syncFakeConn{backupData: bigPayload}
	dstConn := &syncFakeConn{restoreErr: restoreErr}

	dir := t.TempDir()
	t.Setenv("SIPHON_CONFIG_HOME", t.TempDir())

	cfg := &config.Config{Profiles: map[string]config.ProfileConfig{
		"src": {Driver: "src", Host: "h", User: "u", Database: "d", Password: "p"},
		"dst": {Driver: "dst", Host: "h", User: "u", Database: "d", Password: "p"},
	}}
	res := secrets.NewResolver(secrets.Passthrough{})
	ps := profile.New(cfg, res, func(*config.Config) error { return nil })
	cat, _ := dumps.NewCatalog(dir)
	runner := jobs.NewRunner()

	deps := Deps{
		Profiles: ps,
		Dumps:    cat,
		Runner:   runner,
		Drivers: syncDualGetter{
			src: &syncFakeDriver{conn: srcConn},
			dst: &syncFakeDriver{conn: dstConn},
		},
	}

	ch, _, err := Sync(context.Background(), deps, SyncOpts{From: "src", To: "dst"})
	if err != nil {
		t.Fatalf("Sync setup: %v", err)
	}

	// Drain the event channel with a hard timeout.  If pr.Close() is absent,
	// the backup goroutine blocks on pw.Write and the job never finishes.
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()

	for {
		select {
		case _, ok := <-ch:
			if !ok {
				// Channel closed — job finished; goroutine did not leak.
				return
			}
		case <-timer.C:
			t.Fatal("Sync did not complete within 5 s — probable goroutine leak (pr not closed)")
		}
	}
}
