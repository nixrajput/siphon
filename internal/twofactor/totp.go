// Package twofactor implements RFC 6238 TOTP verification with the standard
// library only (HMAC-SHA1, 30-second steps, 6 digits — the parameters every
// authenticator app uses by default). It is a pure leaf: no I/O, no clock
// dependency beyond an injected "now", so verification is fully unit-testable.
//
// siphon uses this to gate destructive operations for profile groups that set
// require_2fa: the group's base32 TOTP secret (a secret-ref, never plaintext)
// is shared with the operator's authenticator app, and the CLI prompts for the
// current 6-digit code before running the operation.
package twofactor

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"strings"
	"time"
)

const (
	period = 30 // seconds per TOTP step (RFC 6238 default)
	digits = 6
)

// Verify reports whether code is a valid TOTP for secret at time now. It accepts
// the code for the current 30s step and the immediately adjacent steps (±1), the
// standard skew tolerance for clock drift between the operator's device and this
// host. secret is base32 (the format authenticator apps display/scan).
func Verify(secret, code string, now time.Time) (bool, error) {
	key, err := decodeSecret(secret)
	if err != nil {
		return false, err
	}
	code = strings.TrimSpace(code)
	step := now.Unix() / period
	for _, s := range []int64{step, step - 1, step + 1} {
		if generate(key, s) == code {
			return true, nil
		}
	}
	return false, nil
}

// Generate returns the current 6-digit TOTP code for secret at time now. It is
// the counterpart to Verify (useful for provisioning/round-trip checks).
func Generate(secret string, now time.Time) (string, error) {
	key, err := decodeSecret(secret)
	if err != nil {
		return "", err
	}
	return generate(key, now.Unix()/period), nil
}

// decodeSecret base32-decodes a TOTP secret, tolerating lowercase and the
// padding-stripped form authenticator apps commonly show.
func decodeSecret(secret string) ([]byte, error) {
	s := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(secret), " ", ""))
	enc := base32.StdEncoding.WithPadding(base32.NoPadding)
	key, err := enc.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("twofactor: invalid base32 TOTP secret: %w", err)
	}
	return key, nil
}

// generate computes the RFC 6238 / RFC 4226 HOTP value for the given counter.
func generate(key []byte, counter int64) string {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], uint64(counter))
	mac := hmac.New(sha1.New, key)
	mac.Write(buf[:])
	sum := mac.Sum(nil)

	offset := sum[len(sum)-1] & 0x0f
	value := (uint32(sum[offset]&0x7f) << 24) |
		(uint32(sum[offset+1]) << 16) |
		(uint32(sum[offset+2]) << 8) |
		uint32(sum[offset+3])
	mod := value % 1_000_000 // 10^digits
	return fmt.Sprintf("%0*d", digits, mod)
}
