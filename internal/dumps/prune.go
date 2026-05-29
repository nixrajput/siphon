package dumps

import "time"

// RetentionPolicy is filled in by Phase G. Phase B ships only a dry-run prune
// that lists what *would* be deleted given a max-age policy.
type RetentionPolicy struct {
	MaxAge time.Duration
}

// PruneReport contains the outcome of a dry-run prune operation.
type PruneReport struct {
	Would []Meta
	Kept  []Meta
}

// PruneDryRun returns which dumps would be deleted under policy p without
// actually deleting anything.
func (c *Catalog) PruneDryRun(p RetentionPolicy) (PruneReport, error) {
	all, err := c.List()
	if err != nil {
		return PruneReport{}, err
	}
	now := time.Now()
	var report PruneReport
	for _, m := range all {
		if p.MaxAge > 0 && now.Sub(m.Created) > p.MaxAge {
			report.Would = append(report.Would, m)
		} else {
			report.Kept = append(report.Kept, m)
		}
	}
	return report, nil
}
