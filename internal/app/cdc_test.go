package app

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nixrajput/siphon/internal/driver"
	"github.com/nixrajput/siphon/internal/errs"
	"github.com/nixrajput/siphon/internal/jobs"
)

func TestCDCState_SaveLoad_Roundtrip(t *testing.T) {
	t.Setenv("SIPHON_STATE_HOME", t.TempDir())

	in := &CDCState{JobID: "job1", Source: "a", Target: "b", LastAppliedLSN: "0/16B3748"}
	if err := saveCDCState(in); err != nil {
		t.Fatalf("saveCDCState: %v", err)
	}

	out, err := loadCDCState("job1")
	if err != nil {
		t.Fatalf("loadCDCState: %v", err)
	}
	if out.Source != "a" || out.Target != "b" {
		t.Fatalf("Source/Target = %q/%q; want a/b", out.Source, out.Target)
	}
	if out.LastAppliedLSN != "0/16B3748" {
		t.Fatalf("LastAppliedLSN = %q; want 0/16B3748", out.LastAppliedLSN)
	}
	if out.UpdatedAt.IsZero() {
		t.Fatal("UpdatedAt is zero; saveCDCState should stamp it")
	}
}

func TestCDCStateDir_HonorsEnv(t *testing.T) {
	t.Run("SIPHON_STATE_HOME", func(t *testing.T) {
		tmp := t.TempDir()
		t.Setenv("SIPHON_STATE_HOME", tmp)
		t.Setenv("XDG_STATE_HOME", "")
		if got, want := CDCStateDir(), filepath.Join(tmp, "cdc"); got != want {
			t.Fatalf("CDCStateDir() = %q; want %q", got, want)
		}
	})

	t.Run("XDG_STATE_HOME", func(t *testing.T) {
		tmp := t.TempDir()
		t.Setenv("SIPHON_STATE_HOME", "")
		t.Setenv("XDG_STATE_HOME", tmp)
		got := CDCStateDir()
		if want := filepath.Join(tmp, "siphon", "cdc"); got != want {
			t.Fatalf("CDCStateDir() = %q; want %q", got, want)
		}
		if !strings.HasSuffix(got, filepath.Join("siphon", "cdc")) {
			t.Fatalf("CDCStateDir() = %q; want suffix siphon/cdc", got)
		}
	})
}

func TestRunCDC_RejectsWithoutCapability(t *testing.T) {
	// A driver with CDC:false (and a Runner so the call is well-formed even
	// though the cap gate rejects before the job ever runs).
	deps := capDeps(driver.Capabilities{CDC: false})
	deps.Runner = jobs.NewRunner()

	_, _, err := RunCDC(context.Background(), deps, SyncOpts{From: "p", To: "p"})
	if err == nil {
		t.Fatal("RunCDC = nil; want error (no driver advertises CDC)")
	}
	if !errors.Is(err, errs.ErrDriverUnsupported) {
		t.Fatalf("errors.Is(err, ErrDriverUnsupported) = false; err = %v", err)
	}
}
