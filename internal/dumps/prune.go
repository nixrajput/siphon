package dumps

import (
	"context"
	"time"
)

// PruneReport contains the outcome of a dry-run prune operation, flattened to
// individual dumps (chain members of pruned chains are listed in Would, members
// of kept chains in Kept).
type PruneReport struct {
	Would []Meta
	Kept  []Meta
}

// PruneDryRun returns which dumps would be deleted under policy p without
// deleting anything. It groups the catalog into chains, runs the chain-aware
// retention engine, and flattens the plan back to dumps. Chain-aware: a base is
// only in Would when its whole chain is pruned, so an incremental is never
// orphaned.
func (c *Catalog) PruneDryRun(ctx context.Context, p RetentionPolicy) (PruneReport, error) {
	all, err := c.List(ctx)
	if err != nil {
		return PruneReport{}, err
	}
	plan := Plan(GroupChains(all), p, time.Now())
	var report PruneReport
	for _, ch := range plan.Prune {
		report.Would = append(report.Would, ch.Members...)
	}
	for _, ch := range plan.Keep {
		report.Kept = append(report.Kept, ch.Members...)
	}
	return report, nil
}
