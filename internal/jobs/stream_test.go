package jobs

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"
)

// TestStream_CloseErr_PropagatesError locks in the failure-propagation contract
// the streaming sync relies on: after CloseErr(err), Read must serve every
// buffered chunk and then return that exact err (not io.EOF). A consumer
// (Restore) thus sees a read error on a truncated stream instead of mistaking
// it for a clean end — the bounded-buffer analogue of pipe CloseWithError.
func TestStream_CloseErr_PropagatesError(t *testing.T) {
	wantErr := errors.New("backup failed mid-dump")
	s := NewStream(8)
	const n = 3
	for i := 0; i < n; i++ {
		if _, err := s.Write([]byte("data")); err != nil {
			t.Fatalf("Write %d: %v", i, err)
		}
	}
	_ = s.CloseErr(wantErr)

	// Drain: buffered chunks first (each a clean read), then the final error.
	buf := make([]byte, 64)
	var got int
	var finalErr error
	for {
		nRead, err := s.Read(buf)
		got += nRead
		if err != nil {
			finalErr = err
			break
		}
	}
	if want := n * len("data"); got != want {
		t.Fatalf("drained %d bytes before error; want %d (chunks dropped)", got, want)
	}
	if !errors.Is(finalErr, wantErr) {
		t.Fatalf("final Read error = %v; want %v", finalErr, wantErr)
	}
	if errors.Is(finalErr, io.EOF) {
		t.Fatalf("final Read error must not be io.EOF on a failed stream")
	}
}

// TestStream_CloseNilErr_YieldsEOF confirms a clean Close (and CloseErr(nil))
// still ends with io.EOF — a nil error is not a failure.
func TestStream_CloseNilErr_YieldsEOF(t *testing.T) {
	s := NewStream(4)
	if _, err := s.Write([]byte("ok")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	_ = s.CloseErr(nil) // nil err behaves like Close

	out, err := io.ReadAll(s) // ReadAll treats io.EOF as success
	if err != nil {
		t.Fatalf("ReadAll after CloseErr(nil): %v; want clean EOF", err)
	}
	if string(out) != "ok" {
		t.Fatalf("got %q; want %q", out, "ok")
	}
}

func TestStream_WriteRead_Roundtrip(t *testing.T) {
	s := NewStream(4)
	payload := []byte("hello-buffered-world")
	go func() {
		_, _ = s.Write(payload)
		_ = s.Close()
	}()
	out, err := io.ReadAll(s)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != string(payload) {
		t.Fatalf("got %q; want %q", out, payload)
	}
}

// TestStream_DrainsAllChunksBeforeEOF locks in the critical invariant: when
// Close fires while chunks are still buffered, every buffered chunk must be
// delivered before io.EOF. A naive select{ch; done} in Read would randomly
// return EOF and drop the tail — silently truncating a streamed dump.
func TestStream_DrainsAllChunksBeforeEOF(t *testing.T) {
	const n = 50
	s := NewStream(64) // cap > n so all writes buffer without a reader
	for i := 0; i < n; i++ {
		if _, err := s.Write([]byte("chunk")); err != nil {
			t.Fatalf("Write %d: %v", i, err)
		}
	}
	_ = s.Close() // close with all n chunks still buffered

	out, err := io.ReadAll(s)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if got, want := len(out), n*len("chunk"); got != want {
		t.Fatalf("drained %d bytes after Close; want %d (chunks dropped before EOF)", got, want)
	}
}

func TestStream_FillPercent_TracksWrites(t *testing.T) {
	s := NewStream(4)
	// Fill without a reader so chunks accumulate (cap 4, write 3 -> ~75%).
	for i := 0; i < 3; i++ {
		if _, err := s.Write([]byte("chunk")); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}
	got := s.FillPercent()
	if got <= 0 || got > 100 {
		t.Fatalf("FillPercent = %d; want 1..100 after 3/4 writes", got)
	}
}

func TestStream_FillPercent_ClampedAndEmpty(t *testing.T) {
	s := NewStream(2)
	if got := s.FillPercent(); got != 0 {
		t.Fatalf("FillPercent on empty = %d; want 0", got)
	}
	for i := 0; i < 2; i++ {
		if _, err := s.Write([]byte("x")); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}
	if got := s.FillPercent(); got != 100 {
		t.Fatalf("FillPercent full = %d; want 100", got)
	}
}

func TestStream_WriteAfterClose_ReturnsErrClosedPipe(t *testing.T) {
	s := NewStream(2)
	_ = s.Close()
	if _, err := s.Write([]byte("x")); err != io.ErrClosedPipe {
		t.Fatalf("err = %v; want io.ErrClosedPipe", err)
	}
}

func TestStream_CloseIsIdempotent(t *testing.T) {
	s := NewStream(2)
	if err := s.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := s.Close(); err != nil { // must not panic on double close
		t.Fatalf("second Close: %v", err)
	}
}

func TestStream_PartialRead_Overflow(t *testing.T) {
	s := NewStream(2)
	if _, err := s.Write([]byte("abcdef")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	_ = s.Close()
	buf := make([]byte, 4)
	n, err := s.Read(buf)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if string(buf[:n]) != "abcd" {
		t.Fatalf("first read = %q; want %q", buf[:n], "abcd")
	}
	rest, err := io.ReadAll(s)
	if err != nil {
		t.Fatalf("ReadAll rest: %v", err)
	}
	if string(rest) != "ef" {
		t.Fatalf("rest = %q; want %q", rest, "ef")
	}
}

func TestStream_ConcurrentWriteCloseNoPanic(t *testing.T) {
	// Stress the Write/Close race directly (run under -race). A concurrent
	// Close must never panic the writer; Write must return without sending
	// on a closed channel.
	for i := 0; i < 200; i++ {
		s := NewStream(1)
		go func() { _ = s.Close() }()
		go func() { _, _ = s.Write([]byte("x")) }()
		go func() {
			buf := make([]byte, 8)
			_, _ = s.Read(buf)
		}()
	}
}

func TestStream_CloseOnCtx(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	s := NewStream(64)
	s.CloseOnCtx(ctx)
	cancel()

	// Poll (bounded) until CloseOnCtx has closed the stream after cancel.
	// Use a fresh single Write each iteration; cap 64 means a Write before
	// close lands buffers without blocking.
	deadline := time.Now().Add(time.Second)
	for {
		if _, err := s.Write([]byte("x")); err == io.ErrClosedPipe {
			return // CloseOnCtx closed the stream after cancel.
		}
		if time.Now().After(deadline) {
			t.Fatal("stream not closed within 1s of ctx cancel")
		}
		time.Sleep(time.Millisecond)
	}
}
