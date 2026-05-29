// Package secrets resolves SecretRef values (e.g. "${VAR}", "keychain://name",
// "vault://...") to their cleartext form via pluggable backends.
package secrets

import (
	"errors"
	"strings"

	"github.com/nixrajput/siphon/internal/errs"
)

// Backend resolves SecretRef values it claims via Scheme().
type Backend interface {
	Scheme() string // "env", "keychain", "vault", "" for passthrough
	Resolve(ref string) (string, error)
}

// Resolver dispatches refs to the matching backend.
type Resolver struct {
	backends []Backend
}

// NewResolver builds a Resolver from the given backends. Order matters:
// the first backend whose Scheme() matches the ref handles it.
func NewResolver(backends ...Backend) *Resolver {
	return &Resolver{backends: backends}
}

// Resolve dispatches ref to a matching backend. Refs use a "scheme:value"
// or "scheme://value" shape; values with no scheme go to passthrough.
//
// ${VAR} env interpolation is handled at config-load time (see internal/config),
// not here — this function operates on already-loaded ProfileConfig.Password
// values that may carry an explicit prefix.
func (r *Resolver) Resolve(ref string) (string, error) {
	if ref == "" {
		return "", nil
	}
	scheme := parseScheme(ref)
	for _, b := range r.backends {
		if b.Scheme() == scheme {
			return b.Resolve(ref)
		}
	}
	// No backend claims this scheme. Fall back to the passthrough ("") backend
	// rather than erroring: a literal value can legitimately contain a colon
	// before any "/" or space (e.g. a Postgres password "p@ss:w0rd"), which
	// parseScheme would otherwise read as the unknown scheme "p@ss".
	for _, b := range r.backends {
		if b.Scheme() == "" {
			return b.Resolve(ref)
		}
	}
	return "", &errs.Error{
		Op:    "secrets.resolve",
		Code:  errs.CodeUser,
		Cause: errors.New("no backend matches scheme " + scheme),
		Hint:  "available backends: " + strings.Join(r.schemes(), ", "),
	}
}

func (r *Resolver) schemes() []string {
	out := make([]string, 0, len(r.backends))
	for _, b := range r.backends {
		out = append(out, b.Scheme())
	}
	return out
}

func parseScheme(ref string) string {
	for i := 0; i < len(ref); i++ {
		switch ref[i] {
		case ':':
			return ref[:i]
		case '/', ' ':
			return "" // looks like a literal value
		}
	}
	return ""
}
