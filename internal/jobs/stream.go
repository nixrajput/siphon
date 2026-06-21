package jobs

import (
	"context"
	"io"
	"sync"
	"sync/atomic"
)

// Stream is a bounded in-memory pipe satisfying io.Reader, io.Writer, and
// io.Closer. It replaces io.Pipe for streaming sync so backpressure is
// observable: FillPercent reports how full the buffer is (a metric a job-panel
// view can surface; not yet rendered in the TUI). Safe for one writer goroutine
// + one reader goroutine, and Close may be called from any goroutine (e.g.
// CloseOnCtx on cancel).
//
// Close never closes the buffer channel s.ch — closing a channel concurrently
// with a send is itself a data race (and a send on a closed channel panics).
// Instead Close closes only the s.done signal; Write selects on done so it can
// never send after close, and Read selects on both ch and done so it drains
// remaining chunks and then reports EOF. This keeps Stream race-free even when
// Write (writer goroutine) and Close (CloseOnCtx goroutine) run concurrently.
type Stream struct {
	ch        chan []byte
	capChunks int
	used      atomic.Int64
	done      chan struct{}         // closed by Close/CloseErr; signals writer-done
	closeErr  atomic.Pointer[error] // set before done is closed by CloseErr; nil means clean EOF
	closeOnce sync.Once
	overflow  []byte // leftover from the previous Read
}

// NewStream creates a Stream buffering up to capChunks chunks (each an
// independent []byte). capChunks <= 0 defaults to 64 (~64MB at 1MB/chunk):
// enough for a laptop, small enough that a slow target shows visible pressure.
func NewStream(capChunks int) *Stream {
	if capChunks <= 0 {
		capChunks = 64
	}
	return &Stream{
		ch:        make(chan []byte, capChunks),
		capChunks: capChunks,
		done:      make(chan struct{}),
	}
}

// Write enqueues a copy of p. Blocks when the buffer is full. Returns
// io.ErrClosedPipe if the stream is closed. Because s.ch is never closed,
// the send can never panic; the select simply abandons the send and returns
// io.ErrClosedPipe if Close fires while Write is parked.
func (s *Stream) Write(p []byte) (int, error) {
	// Fast path: reject if already closed before allocating a copy.
	select {
	case <-s.done:
		return 0, io.ErrClosedPipe
	default:
	}
	cp := make([]byte, len(p))
	copy(cp, p)
	select {
	case <-s.done:
		return 0, io.ErrClosedPipe
	case s.ch <- cp:
		s.used.Add(1)
		return len(p), nil
	}
}

// Read drains the buffer. Buffered chunks are always served first (even after
// Close), so no data is lost; once the writer has Closed and the buffer is
// empty, Read returns io.EOF.
func (s *Stream) Read(p []byte) (int, error) {
	if len(s.overflow) > 0 {
		n := copy(p, s.overflow)
		s.overflow = s.overflow[n:]
		return n, nil
	}
	// Prefer a buffered chunk if one is ready, regardless of done state, so
	// Close does not drop already-buffered data.
	select {
	case chunk := <-s.ch:
		return s.deliver(p, chunk), nil
	default:
	}
	// Buffer momentarily empty: block until a chunk arrives or the writer is
	// done. If done fires, drain any final chunk that raced in, else EOF.
	select {
	case chunk := <-s.ch:
		return s.deliver(p, chunk), nil
	case <-s.done:
		select {
		case chunk := <-s.ch:
			return s.deliver(p, chunk), nil
		default:
			return 0, s.endErr()
		}
	}
}

// endErr returns the error supplied to CloseErr, or io.EOF for a clean Close.
// Read by Read only after observing s.done closed, so the closeErr store (which
// happens before close(s.done) in CloseErr) is guaranteed visible.
func (s *Stream) endErr() error {
	if p := s.closeErr.Load(); p != nil {
		return *p
	}
	return io.EOF
}

// deliver copies chunk into p, stashes any remainder as overflow, and
// decrements the buffered-chunk counter.
func (s *Stream) deliver(p, chunk []byte) int {
	s.used.Add(-1)
	n := copy(p, chunk)
	if n < len(chunk) {
		s.overflow = chunk[n:]
	}
	return n
}

// Close marks the writer side done; subsequent Writes return io.ErrClosedPipe
// and readers see EOF after draining buffered chunks. It does NOT close s.ch
// (that would race a concurrent Write), so Close is safe from any goroutine.
// Idempotent.
func (s *Stream) Close() error {
	s.closeOnce.Do(func() {
		close(s.done)
	})
	return nil
}

// CloseErr is like Close but causes Read to return err (instead of io.EOF)
// once buffered chunks are drained — the bounded-buffer analogue of
// io.PipeWriter.CloseWithError. A producer uses it to propagate a failure
// (e.g. a truncated backup) to the consumer as a read error, so the consumer
// doesn't mistake a partial stream for a clean end. A nil err behaves like
// Close. Idempotent; the first Close/CloseErr wins. The error is stored before
// s.done is closed, so any Read that observes done also observes the error.
func (s *Stream) CloseErr(err error) error {
	s.closeOnce.Do(func() {
		if err != nil {
			s.closeErr.Store(&err)
		}
		close(s.done)
	})
	return nil
}

// FillPercent returns 0..100 = buffered chunks / capacity. Observable backpressure.
func (s *Stream) FillPercent() int {
	if s.capChunks == 0 {
		return 0
	}
	pct := int(s.used.Load() * 100 / int64(s.capChunks))
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	return pct
}

// CloseOnCtx closes the stream when ctx is done — convenience for producer
// goroutines so a cancelled job tears the pipe down.
func (s *Stream) CloseOnCtx(ctx context.Context) {
	go func() {
		<-ctx.Done()
		_ = s.Close()
	}()
}
