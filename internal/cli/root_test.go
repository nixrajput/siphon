package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/nixrajput/siphon/internal/errs"
)

func TestRoot_HelpListsAllVerbs(t *testing.T) {
	var out, errb bytes.Buffer
	root := NewRoot(&out, &errb)
	root.SetArgs([]string{"--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute(--help) returned error: %v", err)
	}

	help := out.String() + errb.String()
	wantVerbs := []string{
		"backup", "restore", "sync", "profile", "dumps",
		"inspect", "verify", "config", "schedule", "tunnel",
	}
	for _, v := range wantVerbs {
		if !strings.Contains(help, v) {
			t.Fatalf("--help output missing %q\n\nfull output:\n%s", v, help)
		}
	}
}

func TestRoot_VersionFlagPrintsVersion(t *testing.T) {
	var out, errb bytes.Buffer
	root := NewRoot(&out, &errb)
	root.SetArgs([]string{"--version"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute(--version) returned error: %v", err)
	}

	combined := out.String() + errb.String()
	if !strings.Contains(combined, Version) {
		t.Fatalf("--version output %q does not contain %q", combined, Version)
	}
}

func TestExecute_ErrsErrorExitCode_RoutesThroughCode(t *testing.T) {
	// We can't easily test Execute() directly (it writes to os.Stdout/os.Stderr),
	// so verify the routing logic is reachable: a typed *errs.Error wrapped in
	// cobra's RunE return should have its Code extractable via errors.As.
	typedErr := &errs.Error{Op: "test", Code: errs.CodeIntegrity, Cause: errs.ErrChecksumMismatch}
	var e *errs.Error
	if !errors.As(typedErr, &e) {
		t.Fatal("errors.As failed to extract *errs.Error")
	}
	if e.Code.ExitCode() != 3 {
		t.Fatalf("ExitCode = %d; want 3 (CodeIntegrity)", e.Code.ExitCode())
	}
}

func TestRoot_StubSubcommand_ReturnsNotImplementedError(t *testing.T) {
	var out, errb bytes.Buffer
	root := NewRoot(&out, &errb)
	root.SetArgs([]string{"schedule"})

	err := root.Execute()
	if err == nil {
		t.Fatal("Execute(schedule) returned nil; want 'not implemented' error")
	}
	if !strings.Contains(err.Error(), "not implemented") {
		t.Fatalf("error = %q; want it to contain 'not implemented'", err.Error())
	}
}
