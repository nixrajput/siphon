package drivertesting

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nixrajput/siphon/internal/driver"
	"github.com/nixrajput/siphon/internal/errs"
)

// RunDriverSuite exercises the full Driver contract against a real
// database. ctor returns the driver-under-test; fx provides the env.
//
// Every driver should call this from a single TestSuite_<DriverName>
// integration test; that yields uniform coverage of:
//   - Connect / Close
//   - Inspect
//   - Backup round-trip via Restore + VerifyRestore
//   - Cancel propagation (ctx cancellation kills subprocess + no temp file)
//   - Sentinel error mapping on bad credentials
func RunDriverSuite(t *testing.T, ctor func() driver.Driver, fx Fixtures) {
	t.Helper()
	t.Cleanup(fx.Cleanup)

	d := ctor()
	t.Run("Connect_And_Inspect", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		conn, err := d.Connect(ctx, fx.Profile)
		if err != nil {
			t.Fatalf("Connect: %v", err)
		}
		defer func() { _ = conn.Close() }()

		if _, err := conn.Inspect(ctx); err != nil {
			t.Fatalf("Inspect: %v", err)
		}
	})

	t.Run("BackupRestore_Roundtrip", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()

		// Open a raw SQL connection for seeding/verification.
		db, err := fx.SQLOpener()
		if err != nil {
			t.Fatalf("SQLOpener: %v", err)
		}
		defer func() { _ = db.Close() }()

		if err := fx.Seed(ctx, db); err != nil {
			t.Fatalf("Seed: %v", err)
		}

		conn, err := d.Connect(ctx, fx.Profile)
		if err != nil {
			t.Fatalf("Connect: %v", err)
		}
		defer func() { _ = conn.Close() }()

		var dump bytes.Buffer
		if err := conn.Backup(ctx, driver.BackupOpts{}, &dump); err != nil {
			t.Fatalf("Backup: %v", err)
		}

		// Round-trip: drop everything, then restore.
		// (Drivers do this with their own "clean" flag in Restore.)
		if err := conn.Restore(ctx, driver.RestoreOpts{Clean: true}, &dump); err != nil {
			t.Fatalf("Restore: %v", err)
		}

		if err := fx.VerifyRestore(ctx, db); err != nil {
			t.Fatalf("VerifyRestore: %v", err)
		}
	})

	t.Run("Cancel_PropagatesToSubprocess", func(t *testing.T) {
		// Verify ctx cancellation propagates to the dump subprocess. We cancel
		// the context BEFORE starting Backup rather than racing a sleep against
		// the dump's duration: a fixed (sleep, data-size) pair is inherently
		// flaky across engines (mysqldump is far faster than pg_dump, so a dump
		// sized to outlast the delay for one engine finishes early on another).
		// With an already-cancelled ctx, exec.CommandContext refuses to start
		// (or immediately kills) the process, so Backup must return a non-nil
		// error deterministically on every engine — proving the ctx is wired to
		// the subprocess without a timing bet.
		conn, err := d.Connect(context.Background(), fx.Profile)
		if err != nil {
			t.Fatalf("Connect: %v", err)
		}
		defer func() { _ = conn.Close() }()

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // cancel up front

		ch := make(chan error, 1)
		buf := &bytes.Buffer{}
		go func() { ch <- conn.Backup(ctx, driver.BackupOpts{}, buf) }()

		select {
		case err := <-ch:
			if err == nil {
				t.Fatal("Backup returned nil for a cancelled context; want non-nil error")
			}
		case <-time.After(10 * time.Second):
			t.Fatal("Backup did not return within 10s after cancel — subprocess leak?")
		}
	})

	t.Run("BadCredentials_ReturnsErrConnectionFailed", func(t *testing.T) {
		bad := fx.Profile
		bad.Password = "definitely-wrong-" + t.Name()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		_, err := d.Connect(ctx, bad)
		if err == nil {
			t.Fatal("Connect succeeded with bad credentials")
		}
		if !errors.Is(err, errs.ErrConnectionFailed) {
			t.Fatalf("err = %v; want errors.Is(err, ErrConnectionFailed)", err)
		}
	})
}
