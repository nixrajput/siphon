package postgres

import (
	"strings"
	"testing"

	"github.com/nixrajput/siphon/internal/jobs"
)

func TestParseProgress_EmitsPerTable(t *testing.T) {
	stderr := strings.Join([]string{
		"pg_dump: last built-in OID is 16383",
		`pg_dump: dumping contents of table "public.widgets"`,
		`pg_dump: dumping contents of table "public.orders"`,
		"pg_dump: saving database definition",
	}, "\n")

	var events []jobs.Event
	parseProgress(strings.NewReader(stderr), func(e jobs.Event) { events = append(events, e) })

	if len(events) != 2 {
		t.Fatalf("emitted %d events; want 2 (one per dumped table)", len(events))
	}
	for i, want := range []string{"public.widgets", "public.orders"} {
		if events[i].Phase != jobs.PhaseProgress {
			t.Errorf("event[%d].Phase = %v; want PhaseProgress", i, events[i].Phase)
		}
		if events[i].Progress == nil || events[i].Progress.TableActive != want {
			t.Errorf("event[%d] TableActive = %v; want %q", i, events[i].Progress, want)
		}
	}
}

// TestParseProgress_SurfacesScanError verifies the scan.Err() check: a line
// longer than the 1<<20 max-token buffer makes Scan() stop early, and that
// must be surfaced as a PhaseWarn event rather than silently swallowed.
func TestParseProgress_SurfacesScanError(t *testing.T) {
	// One token larger than the 1 MiB max buffer, with no newline → ErrTooLong.
	huge := strings.Repeat("x", (1<<20)+1)

	var events []jobs.Event
	parseProgress(strings.NewReader(huge), func(e jobs.Event) { events = append(events, e) })

	if len(events) == 0 {
		t.Fatal("expected a warning event for the over-long line; got none (error was swallowed)")
	}
	last := events[len(events)-1]
	if last.Phase != jobs.PhaseWarn {
		t.Fatalf("last event Phase = %v; want PhaseWarn", last.Phase)
	}
	if last.Err == nil {
		t.Fatal("warn event should carry the scan error in Err")
	}
}
