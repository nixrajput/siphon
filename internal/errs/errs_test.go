package errs

import (
	"errors"
	"strings"
	"testing"
)

func TestSentinels_AreDistinct(t *testing.T) {
	sentinels := []error{
		ErrProfileNotFound,
		ErrProfileInvalid,
		ErrSecretUnresolved,
		ErrDriverUnsupported,
		ErrToolMissing,
		ErrConnectionFailed,
		ErrChecksumMismatch,
		ErrDumpCorrupt,
		ErrIncompatibleEngine,
		Err2FARequired,
		ErrCancelled,
	}
	for i, a := range sentinels {
		for j, b := range sentinels {
			if i != j && a == b {
				t.Fatalf("sentinels %d and %d are the same error", i, j)
			}
		}
	}
}

func TestError_Unwrap_FindsSentinel(t *testing.T) {
	cause := ErrConnectionFailed
	wrapped := &Error{
		Op:    "backup",
		Code:  CodeSystem,
		Cause: cause,
		Hint:  "check the host:port",
	}

	if !errors.Is(wrapped, ErrConnectionFailed) {
		t.Fatalf("errors.Is failed to find ErrConnectionFailed through Error")
	}
}

func TestError_Message_IncludesOpAndCause(t *testing.T) {
	err := &Error{Op: "backup", Code: CodeSystem, Cause: ErrToolMissing, Hint: "install postgresql"}
	got := err.Error()
	if !strings.Contains(got, "backup") || !strings.Contains(got, "required external tool not found") {
		t.Fatalf("Error() = %q; want it to contain 'backup' and the cause text", got)
	}
}

func TestError_Message_IncludesHint_WhenPresent(t *testing.T) {
	err := &Error{Op: "backup", Code: CodeSystem, Cause: ErrToolMissing, Hint: "install postgresql"}
	got := err.Error()
	if !strings.Contains(got, "install postgresql") {
		t.Fatalf("Error() = %q; want it to contain the hint", got)
	}
}

func TestError_Message_NoHint_NoParens(t *testing.T) {
	err := &Error{Op: "backup", Code: CodeSystem, Cause: ErrToolMissing}
	got := err.Error()
	if strings.Contains(got, "(") {
		t.Fatalf("Error() = %q; want no parentheses when hint is empty", got)
	}
}

func TestCode_ExitCode_MapsToSpec(t *testing.T) {
	cases := map[Code]int{
		CodeOK:         0,
		CodeUser:       1,
		CodeSystem:     2,
		CodeIntegrity:  3,
		CodePartial:    4,
		CodeCancelled:  130,
		CodeTerminated: 143,
	}
	for code, want := range cases {
		if got := code.ExitCode(); got != want {
			t.Fatalf("Code(%d).ExitCode() = %d; want %d", code, got, want)
		}
	}
}
