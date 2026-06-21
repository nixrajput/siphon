package app

import (
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
// Backup writes a payload large enough to overflow the bounded jobs.Stream
// buffer so that Write blocks indefinitely if the stream is never closed after
// Restore returns early.
type syncFakeConn struct {
	restoreErr error
	backupData []byte
}

func (c *syncFakeConn) Inspect(_ context.Context) (*driver.Schema, error) {
	return &driver.Schema{}, nil
}

func (c *syncFakeConn) Backup(_ context.Context, _ driver.BackupOpts, w io.Writer) error {
	// Write in many small chunks rather than io.Copy(w, bytes.NewReader(...)):
	// bytes.Reader implements io.WriterTo, so io.Copy would collapse the whole
	// payload into a SINGLE w.Write call, which a bounded jobs.Stream accepts as
	// one buffered chunk and never blocks on. Looping forces ≥64 separate Writes
	// so the bounded buffer actually fills and the producer blocks when the
	// reader (Restore) stops draining — which is the leak scenario under test.
	const chunk = 64 * 1024
	for off := 0; off < len(c.backupData); off += chunk {
		end := off + chunk
		if end > len(c.backupData) {
			end = len(c.backupData)
		}
		if _, err := w.Write(c.backupData[off:end]); err != nil {
			return err
		}
	}
	return nil
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
	// The native sync path streams through a bounded jobs.Stream (64 chunks).
	// io.Copy feeds it in ~32 KiB chunks, so the buffer holds ~2 MiB before a
	// Write blocks. Use 8 MiB so the producer is guaranteed to block once the
	// 64-chunk buffer fills — then if Restore returns early and the stream is
	// NOT closed, the backup goroutine stays parked on Write forever (leak).
	// Sync must close the stream after Restore returns to unblock it.
	bigPayload := make([]byte, 8*1024*1024)

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
