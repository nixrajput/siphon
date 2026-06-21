package dumps

import (
	"errors"
	"fmt"
)

// ResolveChain returns the ordered list of Metas that, applied in order,
// reconstruct targetID. Element 0 is the base; the last element is the target.
//
// It walks ParentID backwards until it reaches a base (BaseID == ID, or a
// legacy empty BaseID). Cycles and broken chains (a missing parent) are
// reported as errors rather than looping forever or silently truncating.
func (c *Catalog) ResolveChain(targetID string) ([]Meta, error) {
	visited := map[string]bool{}
	var chain []Meta

	cur := targetID
	for {
		if visited[cur] {
			return nil, fmt.Errorf("chain cycle detected at %s", cur)
		}
		visited[cur] = true

		m, err := c.ReadMeta(cur)
		if err != nil {
			return nil, fmt.Errorf("chain broken at %s: %w", cur, err)
		}
		chain = append([]Meta{*m}, chain...) // prepend

		// Reached the base: self-referential BaseID, or legacy empty BaseID.
		if m.BaseID == "" || m.BaseID == m.ID {
			return chain, nil
		}

		// Mid-chain incremental must point at a parent.
		if m.ParentID == "" {
			return nil, errors.New("chain broken: incremental " + m.ID + " has no parent_id")
		}
		cur = m.ParentID
	}
}
