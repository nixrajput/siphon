package panels

import (
	"strings"
	"testing"

	"github.com/nixrajput/siphon/internal/jobs"
)

// feed pushes a JobEventInternal through Update and returns the updated panel,
// mirroring how the dashboard forwards events and writes the panel back.
func feed(p Jobs, e jobs.Event) Jobs {
	m, _ := p.Update(JobEventInternal{Event: e})
	return m.(Jobs)
}

// TestJobs_View_DeterministicOrder verifies rows render in first-seen order
// (not the map's random iteration order). Jobs are fed B, A, C and must render
// in exactly that order, stably.
func TestJobs_View_DeterministicOrder(t *testing.T) {
	p := NewJobs()
	// Distinct stages so we can locate each job's row in the output.
	p = feed(p, jobs.Event{JobID: "id-b", Stage: "bbk", Phase: jobs.PhaseProgress})
	p = feed(p, jobs.Event{JobID: "id-a", Stage: "ars", Phase: jobs.PhaseProgress})
	p = feed(p, jobs.Event{JobID: "id-c", Stage: "csy", Phase: jobs.PhaseProgress})

	out := p.View()
	ib := strings.Index(out, "bbk")
	ia := strings.Index(out, "ars")
	ic := strings.Index(out, "csy")
	if ib < 0 || ia < 0 || ic < 0 {
		t.Fatalf("View missing a job row: bbk=%d ars=%d csy=%d\n%s", ib, ia, ic, out)
	}
	// First-seen order is B, A, C — assert that exact positional order.
	if ib >= ia || ia >= ic {
		t.Fatalf("rows not in first-seen order (want bbk<ars<csy); got bbk=%d ars=%d csy=%d", ib, ia, ic)
	}

	// A later update to an existing job must not change ordering.
	p = feed(p, jobs.Event{JobID: "id-a", Stage: "ars", Phase: jobs.PhaseDone})
	out2 := p.View()
	jb, ja, jc := strings.Index(out2, "bbk"), strings.Index(out2, "ars"), strings.Index(out2, "csy")
	if jb >= ja || ja >= jc {
		t.Fatalf("order changed after updating an existing job:\n%s", out2)
	}
}
