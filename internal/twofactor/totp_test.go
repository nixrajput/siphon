package twofactor

import (
	"encoding/base32"
	"testing"
	"time"
)

// secret "12345678901234567890" (the RFC 4226/6238 test key) in base32.
var rfcSecret = base32.StdEncoding.WithPadding(base32.NoPadding).
	EncodeToString([]byte("12345678901234567890"))

func TestVerify_AcceptsCurrentStep(t *testing.T) {
	now := time.Unix(59, 0) // RFC 6238 first test vector time
	code := generate(mustDecode(t, rfcSecret), now.Unix()/period)
	ok, err := Verify(rfcSecret, code, now)
	if err != nil || !ok {
		t.Fatalf("Verify current step = (%v, %v), want (true, nil)", ok, err)
	}
}

func TestVerify_AcceptsAdjacentStepsForSkew(t *testing.T) {
	now := time.Unix(10_000, 0)
	prev := generate(mustDecode(t, rfcSecret), now.Unix()/period-1)
	next := generate(mustDecode(t, rfcSecret), now.Unix()/period+1)
	for name, code := range map[string]string{"prev": prev, "next": next} {
		ok, err := Verify(rfcSecret, code, now)
		if err != nil || !ok {
			t.Errorf("%s-step code rejected: (%v, %v), want accepted", name, ok, err)
		}
	}
}

func TestVerify_RejectsWrongAndStaleCode(t *testing.T) {
	now := time.Unix(10_000, 0)
	if ok, _ := Verify(rfcSecret, "000000", now); ok {
		t.Error("clearly-wrong code accepted")
	}
	// A code two steps away is outside the ±1 skew window.
	stale := generate(mustDecode(t, rfcSecret), now.Unix()/period-2)
	if ok, _ := Verify(rfcSecret, stale, now); ok {
		t.Error("stale code (2 steps old) accepted; skew window should be ±1")
	}
}

func TestVerify_TolerantOfFormatting(t *testing.T) {
	now := time.Unix(10_000, 0)
	code := generate(mustDecode(t, rfcSecret), now.Unix()/period)
	// Surrounding whitespace/newline (as ReadString would leave) must be tolerated.
	if ok, err := Verify(rfcSecret, " "+code+"\n", now); err != nil || !ok {
		t.Errorf("formatted code rejected: (%v, %v)", ok, err)
	}
	// Lowercase / spaced secret must still decode.
	if ok, err := Verify(lower(rfcSecret), code, now); err != nil || !ok {
		t.Errorf("lowercase secret rejected: (%v, %v)", ok, err)
	}
}

func TestVerify_BadSecret(t *testing.T) {
	if _, err := Verify("not!base32", "123456", time.Unix(0, 0)); err == nil {
		t.Error("invalid base32 secret should error")
	}
}

func mustDecode(t *testing.T, s string) []byte {
	t.Helper()
	k, err := decodeSecret(s)
	if err != nil {
		t.Fatalf("decodeSecret: %v", err)
	}
	return k
}

func lower(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 'a' - 'A'
		}
	}
	return string(b)
}
