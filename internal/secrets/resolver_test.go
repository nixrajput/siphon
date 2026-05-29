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

func TestResolver_UnknownScheme_ErrorMentionsBackends(t *testing.T) {
	r := NewResolver(Passthrough{}, Env{})
	_, err := r.Resolve("vault:foo")
	if err == nil {
		t.Fatal("expected error for unknown scheme")
	}
}
