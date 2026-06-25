package dumps

import (
	"fmt"
	"sort"
	"time"
)

// GFSPolicy is a grandfather-father-son retention rule: keep the newest chain
// in each of the most-recent Daily calendar days, Weekly ISO weeks, and Monthly
// calendar months. A zero field disables that tier; an all-zero GFSPolicy is off.
type GFSPolicy struct {
	Daily   int
	Weekly  int
	Monthly int
}

func (g GFSPolicy) active() bool { return g.Daily > 0 || g.Weekly > 0 || g.Monthly > 0 }

// RetentionPolicy decides which dump chains to keep. A chain is kept if it
// satisfies ANY active rule (union semantics): adding a rule can only ever
// protect more chains, never fewer, so a misconfiguration cannot silently
// delete data a rule meant to keep. An all-zero policy keeps everything (prune
// is a no-op) — the dangerous "delete everything" direction requires explicit
// configuration, never silence.
type RetentionPolicy struct {
	KeepLast int           // keep the N newest chains (0 = rule off)
	MaxAge   time.Duration // keep chains younger than this (0 = rule off)
	GFS      GFSPolicy     // keep-by-calendar-bucket (all-zero = off)
}

// IsEmpty reports whether no rule is active, i.e. the policy keeps everything.
func (p RetentionPolicy) IsEmpty() bool {
	return p.KeepLast <= 0 && p.MaxAge <= 0 && !p.GFS.active()
}

// Chain is a restorable unit: a base dump plus its ordered incrementals. The
// chain is the unit of retention — it is kept or pruned as a whole, so an
// incremental can never be orphaned from its base.
type Chain struct {
	Root    string // root dump ID (the base), the chain's stable key
	Members []Meta // base first, then incrementals in apply order
}

// Age timestamp: a chain is "as young as" its NEWEST member, not its base. An
// actively-appended chain (old base, fresh incremental) is therefore treated as
// young and is never pruned mid-life by a max-age or GFS rule.
func (c Chain) newest() time.Time {
	t := c.Members[0].Created
	for _, m := range c.Members[1:] {
		if m.Created.After(t) {
			t = m.Created
		}
	}
	return t
}

// RetentionPlan is the engine's decision: which chains to keep and which to
// prune, with no side effects.
type RetentionPlan struct {
	Keep  []Chain
	Prune []Chain
}

// GroupChains folds a flat dump list into chains keyed by root BaseID. A full
// backup (BaseID == ID, or legacy empty BaseID) is a singleton chain; its
// incrementals attach to it via their BaseID. A dump whose root is missing from
// the set anchors its own chain rather than being dropped, so a partially
// present catalog never loses entries silently. Within each chain, members are
// ordered by Created (base first).
func GroupChains(dumps []Meta) []Chain {
	byRoot := map[string][]Meta{}
	for _, m := range dumps {
		root := m.BaseID
		if root == "" {
			root = m.ID // legacy full backup: its own root
		}
		byRoot[root] = append(byRoot[root], m)
	}
	chains := make([]Chain, 0, len(byRoot))
	for root, members := range byRoot {
		sort.SliceStable(members, func(i, j int) bool {
			return members[i].Created.Before(members[j].Created)
		})
		chains = append(chains, Chain{Root: root, Members: members})
	}
	// Deterministic order: newest chain (by newest member) first.
	sort.SliceStable(chains, func(i, j int) bool {
		return chains[i].newest().After(chains[j].newest())
	})
	return chains
}

// Plan decides which chains to keep vs prune under p, as of now. It is pure: no
// I/O, no clock — now is injected — so every rule and edge case is unit-testable
// with synthetic fixtures. An empty policy keeps everything.
func Plan(chains []Chain, p RetentionPolicy, now time.Time) RetentionPlan {
	if p.IsEmpty() {
		return RetentionPlan{Keep: append([]Chain(nil), chains...)}
	}

	// chains is already sorted newest-first by GroupChains; copy defensively.
	ordered := append([]Chain(nil), chains...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].newest().After(ordered[j].newest())
	})

	keep := map[string]bool{} // union of every active rule's keep set

	if p.KeepLast > 0 {
		for i := 0; i < len(ordered) && i < p.KeepLast; i++ {
			keep[ordered[i].Root] = true
		}
	}
	if p.MaxAge > 0 {
		for _, c := range ordered {
			if now.Sub(c.newest()) < p.MaxAge {
				keep[c.Root] = true
			}
		}
	}
	if p.GFS.active() {
		for _, root := range gfsKeep(ordered, p.GFS) {
			keep[root] = true
		}
	}

	var plan RetentionPlan
	for _, c := range ordered {
		if keep[c.Root] {
			plan.Keep = append(plan.Keep, c)
		} else {
			plan.Prune = append(plan.Prune, c)
		}
	}
	return plan
}

// gfsKeep returns the roots of chains retained by the GFS rule: the newest chain
// in each of the most-recent Daily days, Weekly ISO weeks, and Monthly months.
// `ordered` must be newest-first. A chain that is the newest in more than one
// tier (e.g. today's daily AND this week's weekly) is simply listed once by the
// caller's set.
func gfsKeep(ordered []Chain, g GFSPolicy) []string {
	var roots []string
	// For each tier, walk newest→oldest assigning chains to calendar buckets;
	// the FIRST chain seen for a bucket (newest, since input is newest-first) is
	// that bucket's representative. Keep representatives of the most-recent N
	// distinct buckets.
	pick := func(limit int, bucketKey func(time.Time) string) {
		if limit <= 0 {
			return
		}
		seen := map[string]bool{}
		kept := 0
		for _, c := range ordered {
			k := bucketKey(c.newest())
			if seen[k] {
				continue // older chain in an already-represented bucket
			}
			seen[k] = true
			roots = append(roots, c.Root)
			kept++
			if kept >= limit {
				return
			}
		}
	}

	pick(g.Daily, func(t time.Time) string {
		y, m, d := t.Date()
		return fmt.Sprintf("%04d-%02d-%02d", y, int(m), d)
	})
	pick(g.Weekly, func(t time.Time) string {
		y, w := t.ISOWeek()
		return fmt.Sprintf("%04d-W%02d", y, w)
	})
	pick(g.Monthly, func(t time.Time) string {
		y, m, _ := t.Date()
		return fmt.Sprintf("%04d-%02d", y, int(m))
	})
	return roots
}
