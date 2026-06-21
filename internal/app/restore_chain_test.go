package app

import (
	"context"
	"errors"
	"io"
	"os"
	"testing"
	"time"

	"github.com/nixrajput/siphon/internal/config"
	"github.com/nixrajput/siphon/internal/driver"
	"github.com/nixrajput/siphon/internal/dumps"
	"github.com/nixrajput/siphon/internal/errs"
	"github.com/nixrajput/siphon/internal/jobs"
	"github.com/nixrajput/siphon/internal/profile"
	"github.com/nixrajput/siphon/internal/secrets"
)

// chainRestoreCall records one Restore invocation: the native body the driver
// received (envelope already stripped) and whether Clean was requested.
type chainRestoreCall struct {
	body  string
	clean bool
}

// chainFakeConn records every Restore call so a test can assert chain order,
// per-dump envelope stripping, and that Clean was set only for the base.
type chainFakeConn struct {
	calls []chainRestoreCall
}

func (c *chainFakeConn) Inspect(_ context.Context) (*driver.Schema, error) {
	return &driver.Schema{}, nil
}

func (c *chainFakeConn) Backup(_ context.Context, _ driver.BackupOpts, _ io.Writer) error {
	return nil
}

func (c *chainFakeConn) Restore(_ context.Context, opts driver.RestoreOpts, r io.Reader) error {
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	c.calls = append(c.calls, chainRestoreCall{body: string(b), clean: opts.Clean})
	return nil
}

func (c *chainFakeConn) Verify(_ context.Context, _ io.Reader) (*driver.VerifyReport, error) {
	return &driver.VerifyReport{OK: true}, nil
}

func (c *chainFakeConn) Close() error { return nil }

// chainFakeDriver always returns the same chainFakeConn so the test inspects it.
type chainFakeDriver struct{ conn driver.Conn }

func (d *chainFakeDriver) Name() string { return "fake" }
func (d *chainFakeDriver) Capabilities() driver.Capabilities {
	return driver.Capabilities{NativeStream: true}
}
func (d *chainFakeDriver) Connect(_ context.Context, _ driver.Profile) (driver.Conn, error) {
	return d.conn, nil
}

// writeChainDump writes a real catalog dump file (envelope header + native
// payload) at cat.Path(id) plus its sidecar Meta with the given lineage.
func writeChainDump(t *testing.T, cat *dumps.Catalog, id, baseID, parentID, payload string) {
	t.Helper()
	f, err := os.Create(cat.Path(id))
	if err != nil {
		t.Fatalf("create dump %s: %v", id, err)
	}
	typ := dumps.EnvelopeIncremental
	if parentID == "" {
		typ = dumps.EnvelopeBase
	}
	if _, err := dumps.WriteEnvelope(f, &dumps.Envelope{
		Type:     typ,
		Driver:   "fake",
		BaseID:   baseID,
		ParentID: parentID,
	}); err != nil {
		_ = f.Close()
		t.Fatalf("write envelope %s: %v", id, err)
	}
	if _, err := io.WriteString(f, payload); err != nil {
		_ = f.Close()
		t.Fatalf("write payload %s: %v", id, err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close dump %s: %v", id, err)
	}
	if err := cat.WriteMeta(&dumps.Meta{
		ID:       id,
		Profile:  "test",
		Driver:   "fake",
		BaseID:   baseID,
		ParentID: parentID,
	}); err != nil {
		t.Fatalf("write meta %s: %v", id, err)
	}
}

// chainDeps builds a Deps backed by a real catalog at dir and the given
// recording conn, with a single profile.
func chainDeps(t *testing.T, dir string, conn driver.Conn) Deps {
	t.Helper()
	t.Setenv("SIPHON_CONFIG_HOME", t.TempDir())
	cfg := &config.Config{Profiles: map[string]config.ProfileConfig{
		"test": {Driver: "fake", Host: "h", User: "u", Database: "d", Password: "p"},
	}}
	res := secrets.NewResolver(secrets.Passthrough{})
	ps := profile.New(cfg, res, func(*config.Config) error { return nil })
	cat, err := dumps.NewCatalog(dir)
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}
	return Deps{
		Profiles: ps,
		Dumps:    cat,
		Runner:   jobs.NewRunner(),
		Drivers:  fakeGetter{d: &chainFakeDriver{conn: conn}},
	}
}

// seedThreeDumpChain writes base -> inc1 -> inc2 into a fresh catalog at dir and
// returns the Deps wired to conn.
func seedThreeDumpChain(t *testing.T, dir string, conn driver.Conn) Deps {
	t.Helper()
	deps := chainDeps(t, dir, conn)
	cat := deps.Dumps
	writeChainDump(t, cat, "base", "base", "", "base-data")
	writeChainDump(t, cat, "inc1", "base", "base", "inc1-data")
	writeChainDump(t, cat, "inc2", "base", "inc1", "inc2-data")
	return deps
}

// TestRestoreChain_AppliesInOrder proves the chain is applied base->inc1->inc2
// and that each recorded body is the native payload with the 4 KB envelope
// stripped per dump (not the raw file bytes).
func TestRestoreChain_AppliesInOrder(t *testing.T) {
	conn := &chainFakeConn{}
	deps := seedThreeDumpChain(t, t.TempDir(), conn)

	ch, _, err := Restore(context.Background(), deps, RestoreOpts{Profile: "test", DumpID: "inc2"})
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	drain(t, ch)

	want := []string{"base-data", "inc1-data", "inc2-data"}
	if len(conn.calls) != len(want) {
		t.Fatalf("Restore made %d calls; want %d", len(conn.calls), len(want))
	}
	for i, w := range want {
		if conn.calls[i].body != w {
			t.Fatalf("call %d body = %q; want %q (envelope must be stripped per dump)", i, conn.calls[i].body, w)
		}
	}
}

// TestRestoreChain_CleanOnlyBeforeBase proves Clean is set on the base restore
// only and false for every incremental, when Restore is called with Clean:true.
func TestRestoreChain_CleanOnlyBeforeBase(t *testing.T) {
	conn := &chainFakeConn{}
	deps := seedThreeDumpChain(t, t.TempDir(), conn)

	ch, _, err := Restore(context.Background(), deps, RestoreOpts{Profile: "test", DumpID: "inc2", Clean: true})
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	drain(t, ch)

	if len(conn.calls) != 3 {
		t.Fatalf("Restore made %d calls; want 3", len(conn.calls))
	}
	if !conn.calls[0].clean {
		t.Fatalf("base call clean = false; want true")
	}
	for i := 1; i < len(conn.calls); i++ {
		if conn.calls[i].clean {
			t.Fatalf("incremental call %d clean = true; want false (clean once, base only)", i)
		}
	}
}

// TestRestoreChain_UpToTruncates proves --up-to stops the chain early, applying
// only base and inc1 when targeting inc2 with UpTo=inc1.
func TestRestoreChain_UpToTruncates(t *testing.T) {
	conn := &chainFakeConn{}
	deps := seedThreeDumpChain(t, t.TempDir(), conn)

	ch, _, err := Restore(context.Background(), deps, RestoreOpts{Profile: "test", DumpID: "inc2", UpTo: "inc1"})
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	drain(t, ch)

	want := []string{"base-data", "inc1-data"}
	if len(conn.calls) != len(want) {
		t.Fatalf("Restore made %d calls; want %d", len(conn.calls), len(want))
	}
	for i, w := range want {
		if conn.calls[i].body != w {
			t.Fatalf("call %d body = %q; want %q", i, conn.calls[i].body, w)
		}
	}
}

// TestRestoreChain_UpToUnknownErrors proves an --up-to that names a dump not in
// the chain is a synchronous CodeUser error (returned before the job runs), not
// a silent full-chain restore.
func TestRestoreChain_UpToUnknownErrors(t *testing.T) {
	conn := &chainFakeConn{}
	deps := seedThreeDumpChain(t, t.TempDir(), conn)

	_, _, err := Restore(context.Background(), deps, RestoreOpts{Profile: "test", DumpID: "inc2", UpTo: "nonexistent"})
	if err == nil {
		t.Fatal("Restore with unknown --up-to returned nil error; want CodeUser error")
	}
	var e *errs.Error
	if !errors.As(err, &e) {
		t.Fatalf("error type = %T; want *errs.Error", err)
	}
	if e.Code != errs.CodeUser {
		t.Fatalf("error Code = %d; want CodeUser (%d)", e.Code, errs.CodeUser)
	}
	if len(conn.calls) != 0 {
		t.Fatalf("driver received %d Restore calls; want 0 (error must be synchronous)", len(conn.calls))
	}
}

// writeMismatchDump writes a single base dump whose envelope.Driver differs
// from the target profile's driver ("fake"), so Restore must reject it before
// any destructive Clean runs.
func writeMismatchDump(t *testing.T, cat *dumps.Catalog, id, envDriver string) {
	t.Helper()
	f, err := os.Create(cat.Path(id))
	if err != nil {
		t.Fatalf("create dump %s: %v", id, err)
	}
	if _, err := dumps.WriteEnvelope(f, &dumps.Envelope{
		Type:   dumps.EnvelopeBase,
		Driver: envDriver,
		BaseID: id,
	}); err != nil {
		_ = f.Close()
		t.Fatalf("write envelope %s: %v", id, err)
	}
	if _, err := io.WriteString(f, "payload"); err != nil {
		_ = f.Close()
		t.Fatalf("write payload %s: %v", id, err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close dump %s: %v", id, err)
	}
	if err := cat.WriteMeta(&dumps.Meta{
		ID:      id,
		Profile: "test",
		Driver:  envDriver,
		BaseID:  id,
	}); err != nil {
		t.Fatalf("write meta %s: %v", id, err)
	}
}

// TestRestoreChain_DriverMismatchRejectsBeforeClean proves a dump whose
// envelope.Driver differs from the target profile's driver makes Restore return
// a CodeUser/ErrIncompatibleEngine error and NEVER calls the driver's Restore —
// so a destructive Clean cannot wipe the target before the mismatch is found.
func TestRestoreChain_DriverMismatchRejectsBeforeClean(t *testing.T) {
	conn := &chainFakeConn{}
	dir := t.TempDir()
	deps := chainDeps(t, dir, conn)
	// Target profile driver is "fake"; this dump claims "postgres".
	writeMismatchDump(t, deps.Dumps, "base", "postgres")

	ch, _, err := Restore(context.Background(), deps, RestoreOpts{Profile: "test", DumpID: "base", Clean: true})
	if err != nil {
		t.Fatalf("Restore setup: %v", err)
	}
	gotErr := drainErr(t, ch)
	if gotErr == nil {
		t.Fatal("Restore with mismatched driver returned nil error; want incompatible-engine error")
	}
	var e *errs.Error
	if !errors.As(gotErr, &e) {
		t.Fatalf("error type = %T; want *errs.Error", gotErr)
	}
	if e.Code != errs.CodeUser {
		t.Fatalf("error Code = %d; want CodeUser (%d)", e.Code, errs.CodeUser)
	}
	if !errors.Is(gotErr, errs.ErrIncompatibleEngine) {
		t.Fatalf("error = %v; want ErrIncompatibleEngine", gotErr)
	}
	if len(conn.calls) != 0 {
		t.Fatalf("driver Restore called %d times; want 0 (no destructive Clean on mismatch)", len(conn.calls))
	}
}

// drainErr consumes a job event channel and returns the error from a
// PhaseError event (or nil if the job completed cleanly), with a hard timeout.
func drainErr(t *testing.T, ch <-chan jobs.Event) error {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	var jobErr error
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return jobErr
			}
			if ev.Phase == jobs.PhaseError && ev.Err != nil {
				jobErr = ev.Err
			}
		case <-timer.C:
			t.Fatal("job did not complete within 5 s")
			return nil
		}
	}
}
