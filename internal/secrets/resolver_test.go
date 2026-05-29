package secrets

import (
	"errors"
	"testing"

	"github.com/nixrajput/siphon/internal/errs"
)

func TestResolver_Passthrough_LiteralValue(t *testing.T) {
	r := NewResolver(Passthrough{}, Env{})
	got, err := r.Resolve("hunter2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "hunter2" {
		t.Fatalf("got %q; want 'hunter2'", got)
	}
}

func TestResolver_EnvBackend_Resolves(t *testing.T) {
	t.Setenv("MY_SECRET", "shh")
	r := NewResolver(Env{}, Passthrough{})
	got, err := r.Resolve("env:MY_SECRET")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "shh" {
		t.Fatalf("got %q; want 'shh'", got)
	}
}

func TestResolver_EnvMissing_ReturnsErrSecretUnresolved(t *testing.T) {
	r := NewResolver(Env{}, Passthrough{})
	_, err := r.Resolve("env:DEFINITELY_NOT_SET_" + t.Name())
	if !errors.Is(err, errs.ErrSecretUnresolved) {
		t.Fatalf("got %v; want ErrSecretUnresolved", err)
	}
}

// With a passthrough backend present, an unknown scheme falls back to the
// literal value rather than erroring — there's no way to tell "meant the vault
// backend" from "literal value that looks like vault:foo", so passthrough wins.
func TestResolver_UnknownScheme_FallsBackToPassthrough(t *testing.T) {
	r := NewResolver(Passthrough{}, Env{})
	got, err := r.Resolve("vault:foo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "vault:foo" {
		t.Fatalf("got %q; want literal 'vault:foo' via passthrough", got)
	}
}

// Without a passthrough backend, an unknown scheme still errors (and the error
// names the available backends).
func TestResolver_UnknownScheme_NoPassthrough_Errors(t *testing.T) {
	r := NewResolver(Env{})
	_, err := r.Resolve("vault:foo")
	if err == nil {
		t.Fatal("expected error for unknown scheme when no passthrough backend")
	}
}

// TestResolver_LiteralWithColon is the regression: a literal password
// containing a colon (valid in Postgres) must resolve as-is, not be misread as
// the unknown scheme before the colon.
func TestResolver_LiteralWithColon(t *testing.T) {
	r := NewResolver(Env{}, Passthrough{})
	got, err := r.Resolve("p@ss:w0rd")
	if err != nil {
		t.Fatalf("literal with colon errored: %v", err)
	}
	if got != "p@ss:w0rd" {
		t.Fatalf("got %q; want literal 'p@ss:w0rd'", got)
	}
}
