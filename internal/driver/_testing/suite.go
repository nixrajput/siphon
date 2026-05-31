package drivertesting

import (
	"bytes"
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/nixrajput/siphon/internal/driver"
	"github.com/nixrajput/siphon/internal/errs"
)

// seedCancelLoad bulk-loads a table of wide rows via the fixture's raw SQL
// connection so a subsequent Backup has enough data to stay in-flight past the
// cancel delay. Uses only portable SQL (CREATE TABLE + parameterized INSERT in
// one transaction), so it works for any engine the harness is pointed at.
func seedCancelLoad(t *testing.T, fx Fixtures) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, err := fx.SQLOpener()
	if err != nil {
		t.Fatalf("seedCancelLoad: SQLOpener: %v", err)
	}
	defer func() { _ = db.Close() }()

	if _, err := db.ExecContext(ctx,
		`CREATE TABLE cancel_load (id integer primary key, payload text)`); err != nil {
		t.Fatalf("seedCancelLoad: create table: %v", err)
	}

	// ~5000 rows of ~1 KiB each ≈ 5 MiB — large enough that the dump can't
	// complete within the 150ms cancel window, small enough to load quickly.
	// The payload is a fixed safe literal (no user input), so it's inlined
	// rather than parameterized — this keeps the SQL placeholder-agnostic
	// ($1 vs ? differs by engine) so the harness stays portable across drivers.
	payload := strings.Repeat("x", 1024)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("seedCancelLoad: begin: %v", err)
	}
	for i := 0; i < 5000; i++ {
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO cancel_load (id, payload) VALUES ("+strconv.Itoa(i)+", '"+payload+"')"); err != nil {
			_ = tx.Rollback()
			t.Fatalf("seedCancelLoad: insert %d: %v", i, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("seedCancelLoad: commit: %v", err)
	}
}

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
		// Inflate the database so the dump takes long enough that cancel()
		// reliably lands while pg_dump (or the engine's equivalent) is still
		// streaming. With only the round-trip subtest's tiny fixture, the dump
		// can finish in well under the cancel delay on a fast host, making
		// Backup return a clean nil and the assertion below spuriously fail.
		seedCancelLoad(t, fx)

		conn, err := d.Connect(context.Background(), fx.Profile)
		if err != nil {
			t.Fatalf("Connect: %v", err)
		}
		defer func() { _ = conn.Close() }()

		ctx, cancel := context.WithCancel(context.Background())
		ch := make(chan error, 1)
		buf := &bytes.Buffer{}
		go func() { ch <- conn.Backup(ctx, driver.BackupOpts{}, buf) }()

		time.Sleep(150 * time.Millisecond)
		cancel()

		select {
		case err := <-ch:
			if err == nil {
				t.Fatal("Backup returned nil after cancel; want non-nil error")
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
