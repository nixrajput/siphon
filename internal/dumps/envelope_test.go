package dumps

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestEnvelope_Roundtrip(t *testing.T) {
	in := &Envelope{
		Siphon:  "1.0",
		Type:    EnvelopeBase,
		Driver:  "postgres",
		Created: time.Now().UTC().Truncate(time.Second),
	}
	var buf bytes.Buffer
	n, err := WriteEnvelope(&buf, in)
	if err != nil {
		t.Fatalf("WriteEnvelope: %v", err)
	}
	if n != EnvelopeSize {
		t.Fatalf("wrote %d bytes; want %d", n, EnvelopeSize)
	}
	buf.WriteString("native-dump-bytes")

	out, body, err := ReadEnvelope(&buf)
	if err != nil {
		t.Fatalf("ReadEnvelope: %v", err)
	}
	if out.Driver != "postgres" || out.Type != EnvelopeBase {
		t.Fatalf("Envelope round-trip mismatch: %+v", out)
	}

	rest := make([]byte, 64)
	n2, _ := body.Read(rest)
	if !strings.HasPrefix(string(rest[:n2]), "native-dump-bytes") {
		t.Fatalf("body reader misaligned; got %q", rest[:n2])
	}
}

func TestEnvelope_MissingMagic(t *testing.T) {
	junk := make([]byte, EnvelopeSize)
	copy(junk, []byte("NOPE"))
	_, _, err := ReadEnvelope(bytes.NewReader(junk))
	if err == nil {
		t.Fatal("expected error on bad magic")
	}
}

func TestEnvelope_OversizedJSON(t *testing.T) {
	huge := strings.Repeat("x", EnvelopeSize)
	e := &Envelope{Siphon: "1.0", Driver: "postgres", EngineVersion: huge}
	_, err := WriteEnvelope(&bytes.Buffer{}, e)
	if err == nil {
		t.Fatal("expected error on oversized envelope")
	}
}
