package app

import (
	"context"
	"time"

	"github.com/nixrajput/siphon/internal/dumps"
)

// PruneOpts configures a prune run. Policy is resolved by the caller (the CLI
// maps config + flags into it), so this layer stays config-agnostic. Profile
// scopes the catalog to one profile's dumps ("" = all profiles). Apply performs
// deletions; otherwise the run is a dry-run that only computes the plan.
type PruneOpts struct {
	Profile string
	Policy  dumps.RetentionPolicy
	Apply   bool
}

// ChainOutcome is one chain's place in the plan, flattened for reporting.
type ChainOutcome struct {
	Root      string
	DumpIDs   []string
	SizeBytes int64
	Pruned    bool     // true = scheduled for (or performed) deletion
	Deleted   []string // dump IDs actually deleted (Apply only)
	Errors    []string // per-dump deletion failures (Apply only)
}

// PruneResult is the structured outcome the CLI renders.
type PruneResult struct {
	Profile   string
	Apply     bool
	Outcomes  []ChainOutcome
	Reclaimed int64 // bytes freed (sum of successfully deleted dumps; Apply only)
	Failed    int   // count of dumps that failed to delete
}

// Prune groups the catalog into chains, plans retention over them, and (when
// Apply) deletes the pruned chains. It is synchronous (like Verify): prune is a
// list + a few deletes, not a long stream, so it returns a structured result
// directly rather than over the job channel.
//
// Deletion is chain-aware and leaf-inward: within a pruned chain, incrementals
// are removed before the base, so an interrupted prune leaves at worst a
// complete shorter chain — never a base missing under a surviving incremental.
func Prune(ctx context.Context, d Deps, opt PruneOpts) (*PruneResult, error) {
	all, err := d.Dumps.List(ctx)
	if err != nil {
		return nil, err
	}
	// Scope to the requested profile before grouping, so chains and the plan
	// reflect only this profile's backups.
	if opt.Profile != "" {
		filtered := make([]dumps.Meta, 0, len(all))
		for _, m := range all {
			if m.Profile == opt.Profile {
				filtered = append(filtered, m)
			}
		}
		all = filtered
	}

	plan := dumps.Plan(dumps.GroupChains(all), opt.Policy, time.Now())

	result := &PruneResult{Profile: opt.Profile, Apply: opt.Apply}
	for _, ch := range plan.Keep {
		result.Outcomes = append(result.Outcomes, chainOutcome(ch, false))
	}
	for _, ch := range plan.Prune {
		oc := chainOutcome(ch, true)
		if opt.Apply {
			applyChainDeletion(ctx, d, ch, &oc, result)
		}
		result.Outcomes = append(result.Outcomes, oc)
	}
	return result, nil
}

// applyChainDeletion deletes a pruned chain's members leaf-inward (incrementals
// before the base) and STOPS at the first failure within the chain. Stopping is
// the invariant: if a leaf delete fails (its meta may be gone but the dump still
// catalogued), continuing to delete its ancestors would orphan that leaf —
// exactly what the chain-aware design prevents. Aborting the chain leaves a
// complete, still-restorable prefix. Other chains are unaffected; the failure is
// recorded and the run continues with them.
func applyChainDeletion(ctx context.Context, d Deps, ch dumps.Chain, oc *ChainOutcome, result *PruneResult) {
	for i := len(ch.Members) - 1; i >= 0; i-- {
		m := ch.Members[i]
		if err := d.Dumps.Delete(ctx, m.ID); err != nil {
			oc.Errors = append(oc.Errors, m.ID+": "+err.Error())
			result.Failed++
			return // do not delete this member's ancestors — would orphan it
		}
		oc.Deleted = append(oc.Deleted, m.ID)
		result.Reclaimed += m.SizeBytes
	}
}

func chainOutcome(ch dumps.Chain, pruned bool) ChainOutcome {
	oc := ChainOutcome{Root: ch.Root, Pruned: pruned}
	for _, m := range ch.Members {
		oc.DumpIDs = append(oc.DumpIDs, m.ID)
		oc.SizeBytes += m.SizeBytes
	}
	return oc
}
