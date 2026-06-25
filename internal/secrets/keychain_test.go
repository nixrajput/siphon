package secrets

import (
	"errors"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/nixrajput/siphon/internal/errs"
)

func TestKeychain_Resolve(t *testing.T) {
	keyring.MockInit() // in-memory keyring, no real OS store touched
	if err := keyring.Set("siphon", "prod-db", "s3cr3t"); err != nil {
		t.Fatalf("seed keyring: %v", err)
	}
	if err := keyring.Set("custom-svc", "acct", "other"); err != nil {
		t.Fatalf("seed keyring: %v", err)
	}

	kc := Keychain{}

	// Short form: keychain://<account> → service "siphon".
	if got, err := kc.Resolve("keychain://prod-db"); err != nil || got != "s3cr3t" {
		t.Errorf("short form = (%q, %v), want (s3cr3t, nil)", got, err)
	}
	// Two-segment form: keychain://<service>/<account>.
	if got, err := kc.Resolve("keychain://custom-svc/acct"); err != nil || got != "other" {
		t.Errorf("two-segment form = (%q, %v), want (other, nil)", got, err)
	}
}

func TestKeychain_NotFoundIsUserError(t *testing.T) {
	keyring.MockInit()
	_, err := (Keychain{}).Resolve("keychain://missing")
	var e *errs.Error
	if !errors.As(err, &e) || e.Code != errs.CodeUser {
		t.Fatalf("missing key err = %v, want CodeUser", err)
	}
}

func TestKeychain_BadRef(t *testing.T) {
	keyring.MockInit()
	// keychain:///acct has an empty service and must be rejected, not silently
	// looked up under service "".
	for _, ref := range []string{"keychain://", "keychain://svc/", "keychain:///acct"} {
		if _, err := (Keychain{}).Resolve(ref); err == nil {
			t.Errorf("Resolve(%q) = nil, want error", ref)
		}
	}
}

func TestKeychain_Scheme(t *testing.T) {
	if (Keychain{}).Scheme() != "keychain" {
		t.Errorf("Scheme() = %q, want keychain", (Keychain{}).Scheme())
	}
}
