package postgres

import (
	"bufio"
	"io"
	"strings"

	"github.com/nixrajput/siphon/internal/jobs"
)

// parseProgress reads pg_dump --verbose stderr line-by-line and emits
// progress events. Phase B emits a minimal event-per-table; Phase F adds
// byte/row counters.
func parseProgress(r io.Reader, emit func(jobs.Event)) {
	scan := bufio.NewScanner(r)
	scan.Buffer(make([]byte, 64*1024), 1<<20)
	for scan.Scan() {
		line := scan.Text()
		switch {
		case strings.HasPrefix(line, "pg_dump: dumping contents of table"):
			table := extractAfter(line, "table ")
			emit(jobs.Event{
				Phase:    jobs.PhaseProgress,
				Message:  "dumping " + table,
				Progress: &jobs.Progress{TableActive: table},
			})
		}
	}
	// Scan() returns false on EOF or error; only Err() distinguishes them.
	// A read failure or an over-long line (> the 1<<20 max-token buffer above)
	// would otherwise silently truncate progress. Surface it as a warning —
	// progress parsing is best-effort and must never fail the backup itself.
	if err := scan.Err(); err != nil {
		emit(jobs.Event{
			Phase:   jobs.PhaseWarn,
			Message: "progress parsing stopped early: " + err.Error(),
			Err:     err,
		})
	}
}

func extractAfter(s, key string) string {
	idx := strings.Index(s, key)
	if idx < 0 {
		return ""
	}
	out := strings.TrimSpace(s[idx+len(key):])
	out = strings.Trim(out, `"'`)
	return out
}

// sentinel: parseProgress and extractAfter are defined but not yet wired into
// backup.go (Phase B drains stderr to io.Discard). This prevents the linter
// from flagging them as unused until Phase F wires them in.
var _ = parseProgress
