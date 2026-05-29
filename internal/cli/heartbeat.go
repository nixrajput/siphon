package cli

import (
	"fmt"
	"io"
	"time"

	"github.com/nixrajput/siphon/internal/errs"
	"github.com/nixrajput/siphon/internal/jobs"
)

// Heartbeat consumes a job Event channel and prints CLI-friendly lines
// to out. It returns the job's terminal error (nil on success) so callers
// can propagate failures to the process exit code.
func Heartbeat(out io.Writer, ch <-chan jobs.Event) error {
	start := time.Now()
	var resultErr error
	for e := range ch {
		switch e.Phase {
		case jobs.PhaseStarted:
			_, _ = fmt.Fprintf(out, "==> %s started\n", e.Stage)
		case jobs.PhaseProgress:
			if e.Message != "" {
				_, _ = fmt.Fprintf(out, "  • %s\n", e.Message)
			}
		case jobs.PhaseWarn:
			_, _ = fmt.Fprintf(out, "  ! %s\n", e.Message)
		case jobs.PhaseDone:
			_, _ = fmt.Fprintf(out, "  ✓ %s done in %s\n", e.Stage, time.Since(start).Round(time.Millisecond))
		case jobs.PhaseError:
			_, _ = fmt.Fprintf(out, "  ✗ %s failed: %v\n", e.Stage, e.Err)
			resultErr = e.Err
		case jobs.PhaseCancelled:
			_, _ = fmt.Fprintf(out, "  ✗ %s cancelled\n", e.Stage)
			resultErr = &errs.Error{Op: e.Stage, Code: errs.CodeCancelled, Cause: errs.ErrCancelled}
		}
	}
	return resultErr
}
