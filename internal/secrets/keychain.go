package secrets

import (
	"errors"
	"strings"

	"github.com/zalando/go-keyring"

	"github.com/nixrajput/siphon/internal/errs"
)

// Keychain resolves "keychain://..." refs from the OS credential store —
// macOS Keychain, Windows Credential Manager, or the Linux Secret Service —
// via go-keyring, which abstracts all three behind one call.
//
// Ref shapes:
//   - keychain://<service>/<account>  → keyring.Get(service, account)
//   - keychain://<account>            → keyring.Get("siphon", account)
//
// The two-segment form addresses an arbitrary stored credential; the short form
// is the common case (a siphon-owned secret named <account>).
type Keychain struct{}

// defaultService is the keyring "service" for short keychain://<account> refs.
const defaultService = "siphon"

func (Keychain) Scheme() string { return "keychain" }

func (Keychain) Resolve(ref string) (string, error) {
	rest := strings.TrimPrefix(ref, "keychain://")
	if rest == "" || rest == ref { // ref wasn't keychain://… or had no body
		return "", &errs.Error{
			Op:    "secrets.keychain.resolve",
			Code:  errs.CodeUser,
			Cause: errors.New("keychain: ref missing name"),
			Hint:  "use keychain://<account> or keychain://<service>/<account>",
		}
	}
	service, account := defaultService, rest
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		service, account = rest[:i], rest[i+1:]
	}
	if account == "" {
		return "", &errs.Error{
			Op:    "secrets.keychain.resolve",
			Code:  errs.CodeUser,
			Cause: errors.New("keychain: ref missing account"),
			Hint:  "use keychain://<service>/<account>",
		}
	}

	val, err := keyring.Get(service, account)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", &errs.Error{
			Op:    "secrets.keychain.resolve",
			Code:  errs.CodeUser,
			Cause: errs.ErrSecretUnresolved,
			Hint:  "no keychain entry for service=" + service + " account=" + account,
		}
	}
	if err != nil {
		return "", &errs.Error{
			Op:    "secrets.keychain.resolve",
			Code:  errs.CodeSystem,
			Cause: err,
			Hint:  "could not read from the OS credential store",
		}
	}
	return val, nil
}
