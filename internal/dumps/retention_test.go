package dumps

import (
	"sort"
	"testing"
	"time"
)

// fixed reference "now" so age/GFS math is deterministic.
var refNow = time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)

// dump builds a Meta with an ID, profile, creation time, and optional chain links.
func dump(id, profile string, created time.Time, baseID, parentID string) Meta {
	return Meta{ID: id, Profile: profile, Created: created, BaseID: baseID, ParentID: parentID, SizeBytes: 100}
}

func daysAgo(n int) time.Time { return refNow.AddDate(0, 0, -n) }
func roots(chains []Chain) []string {
	out := make([]string, len(chains))
	for i, c := range chains {
		out[i] = c.Root
	}
	sort.Strings(out)
	return out
}

func TestGroupChains(t *testing.T) {
	t.Run("singletons and an incremental chain", func(t *testing.T) {
		ds := []Meta{
			dump("full1", "p", daysAgo(10), "", ""),          // legacy full (empty BaseID)
			dump("base2", "p", daysAgo(5), "base2", ""),      // self-rooted base
			dump("inc2a", "p", daysAgo(4), "base2", "base2"), // incremental of base2
			dump("inc2b", "p", daysAgo(3), "base2", "inc2a"), // incremental of inc2a
		}
		chains := GroupChains(ds)
		if len(chains) != 2 {
			t.Fatalf("got %d chains, want 2: %+v", len(chains), chains)
		}
		got := roots(chains)
		if got[0] != "base2" || got[1] != "full1" {
			t.Errorf("roots = %v, want [base2 full1]", got)
		}
		// base2's chain has 3 members, base first.
		for _, c := range chains {
			if c.Root == "base2" {
				if len(c.Members) != 3 || c.Members[0].ID != "base2" {
					t.Errorf("base2 chain = %+v, want 3 members base-first", c.Members)
				}
			}
		}
	})

	t.Run("orphaned incremental anchors its own chain (no crash, no drop)", func(t *testing.T) {
		// inc points at a base that isn't in the set.
		ds := []Meta{dump("inc", "p", daysAgo(1), "missingbase", "missingbase")}
		chains := GroupChains(ds)
		if len(chains) != 1 || chains[0].Root != "missingbase" {
			t.Fatalf("orphan grouping = %+v, want one chain rooted at missingbase", chains)
		}
	})

	t.Run("members ordered by topology, not Created, on tied timestamps", func(t *testing.T) {
		// Base and incremental share the SAME Created time (clock skew / coarse
		// granularity). Topological order MUST still put the base first, because
		// the leaf-inward delete path deletes last-to-first and would otherwise
		// remove the base before its surviving child.
		same := daysAgo(2)
		ds := []Meta{
			dump("inc", "p", same, "base", "base"), // listed first, same timestamp
			dump("base", "p", same, "base", ""),
		}
		chains := GroupChains(ds)
		if len(chains) != 1 {
			t.Fatalf("got %d chains, want 1", len(chains))
		}
		got := chains[0].Members
		if len(got) != 2 || got[0].ID != "base" || got[1].ID != "inc" {
			t.Errorf("member order = [%s %s], want [base inc] (base first despite tie)", got[0].ID, got[1].ID)
		}
	})

	t.Run("members topologically ordered when a child predates its parent", func(t *testing.T) {
		// Pathological: the incremental's Created is BEFORE the base's (skew).
		// Topology must still win — base first.
		ds := []Meta{
			dump("base", "p", daysAgo(1), "base", ""),
			dump("inc", "p", daysAgo(5), "base", "base"), // older than its base
		}
		chains := GroupChains(ds)
		got := chains[0].Members
		if got[0].ID != "base" {
			t.Errorf("member order = [%s ...], want base first despite older child", got[0].ID)
		}
	})
}

func TestGroupChains_DeterministicOnTiedChainTimestamps(t *testing.T) {
	// Two independent chains with identical newest() timestamps must sort in a
	// stable order (by Root) across runs — a destructive planner cannot depend on
	// randomized map iteration. Run the grouping repeatedly and assert identical
	// output.
	same := daysAgo(3)
	ds := []Meta{
		dump("zzz", "p", same, "", ""),
		dump("aaa", "p", same, "", ""),
		dump("mmm", "p", same, "", ""),
	}
	first := roots(GroupChains(ds))
	for i := 0; i < 20; i++ {
		if got := roots(GroupChains(ds)); !equalStrings(got, first) {
			t.Fatalf("non-deterministic chain order: run %d = %v, first = %v", i, got, first)
		}
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestPlan_EmptyPolicyKeepsEverything(t *testing.T) {
	ds := []Meta{
		dump("a", "p", daysAgo(100), "", ""),
		dump("b", "p", daysAgo(200), "", ""),
	}
	plan := Plan(GroupChains(ds), RetentionPolicy{}, refNow)
	if len(plan.Prune) != 0 {
		t.Errorf("empty policy pruned %d chains, want 0 (keep everything)", len(plan.Prune))
	}
	if len(plan.Keep) != 2 {
		t.Errorf("empty policy kept %d chains, want 2", len(plan.Keep))
	}
}

func TestPlan_KeepLast(t *testing.T) {
	var ds []Meta
	for i := 0; i < 5; i++ {
		ds = append(ds, dump("c"+string(rune('0'+i)), "p", daysAgo(i), "", ""))
	}
	plan := Plan(GroupChains(ds), RetentionPolicy{KeepLast: 2}, refNow)
	if len(plan.Keep) != 2 || len(plan.Prune) != 3 {
		t.Fatalf("keep-last-2 = keep %d / prune %d, want 2/3", len(plan.Keep), len(plan.Prune))
	}
	// The two kept are the newest (c0 = today, c1 = yesterday).
	got := roots(plan.Keep)
	if got[0] != "c0" || got[1] != "c1" {
		t.Errorf("kept roots = %v, want [c0 c1]", got)
	}
}

func TestPlan_MaxAge_UsesNewestMember(t *testing.T) {
	// An OLD base with a FRESH incremental: the chain must be treated as young
	// (newest member governs age) and kept under a 7-day max-age.
	ds := []Meta{
		dump("oldbase", "p", daysAgo(30), "oldbase", ""),
		dump("freshinc", "p", daysAgo(1), "oldbase", "oldbase"),
	}
	plan := Plan(GroupChains(ds), RetentionPolicy{MaxAge: 7 * 24 * time.Hour}, refNow)
	if len(plan.Prune) != 0 {
		t.Errorf("chain with fresh incremental was pruned by max-age: %+v", plan.Prune)
	}
}

func TestPlan_Union_RuleOnlyProtects(t *testing.T) {
	// keep-last-1 alone would prune the 5-day-old chain; adding max-age 7d must
	// protect it (union, not intersection).
	ds := []Meta{
		dump("today", "p", daysAgo(0), "", ""),
		dump("fivedays", "p", daysAgo(5), "", ""),
		dump("tendays", "p", daysAgo(10), "", ""),
	}
	keepLastOnly := Plan(GroupChains(ds), RetentionPolicy{KeepLast: 1}, refNow)
	if len(keepLastOnly.Keep) != 1 {
		t.Fatalf("keep-last-1 kept %d, want 1", len(keepLastOnly.Keep))
	}
	union := Plan(GroupChains(ds), RetentionPolicy{KeepLast: 1, MaxAge: 7 * 24 * time.Hour}, refNow)
	got := roots(union.Keep)
	// today (both rules) + fivedays (max-age) kept; tendays pruned.
	if len(got) != 2 || got[0] != "fivedays" || got[1] != "today" {
		t.Errorf("union kept = %v, want [fivedays today]", got)
	}
}

func TestPlan_GFS_Daily(t *testing.T) {
	// Two chains on the same day + one on a prior day. gfs-daily:2 keeps the
	// newest chain of each of the 2 most-recent days = 2 chains; the older
	// same-day chain is pruned.
	ds := []Meta{
		dump("day0_late", "p", refNow.Add(-1*time.Hour), "", ""),
		dump("day0_early", "p", refNow.Add(-5*time.Hour), "", ""),
		dump("day1", "p", daysAgo(1), "", ""),
		dump("day2", "p", daysAgo(2), "", ""),
	}
	plan := Plan(GroupChains(ds), RetentionPolicy{GFS: GFSPolicy{Daily: 2}}, refNow)
	got := roots(plan.Keep)
	// kept: day0_late (today's representative) + day1 (yesterday). day0_early and
	// day2 pruned.
	if len(got) != 2 || got[0] != "day0_late" || got[1] != "day1" {
		t.Errorf("gfs-daily:2 kept = %v, want [day0_late day1]", got)
	}
}

func TestPlan_GFS_MultiTierKeptOnce(t *testing.T) {
	// A single recent chain is the representative of today's day AND this week's
	// week AND this month's month. It must be kept exactly once, not duplicated.
	ds := []Meta{dump("recent", "p", refNow.Add(-1*time.Hour), "", "")}
	plan := Plan(GroupChains(ds), RetentionPolicy{GFS: GFSPolicy{Daily: 1, Weekly: 1, Monthly: 1}}, refNow)
	if len(plan.Keep) != 1 || len(plan.Prune) != 0 {
		t.Errorf("multi-tier single chain = keep %d / prune %d, want 1/0", len(plan.Keep), len(plan.Prune))
	}
}

func TestPlan_GFS_FewerChainsThanBuckets(t *testing.T) {
	// gfs-daily:30 with only 2 chains: keep both, prune none (no crash, no
	// phantom buckets).
	ds := []Meta{
		dump("a", "p", daysAgo(0), "", ""),
		dump("b", "p", daysAgo(1), "", ""),
	}
	plan := Plan(GroupChains(ds), RetentionPolicy{GFS: GFSPolicy{Daily: 30}}, refNow)
	if len(plan.Keep) != 2 || len(plan.Prune) != 0 {
		t.Errorf("gfs over-provisioned = keep %d / prune %d, want 2/0", len(plan.Keep), len(plan.Prune))
	}
}

func TestPlan_PrunedChainKeepsMembersTogether(t *testing.T) {
	// A pruned multi-dump chain must surface ALL its members for deletion.
	ds := []Meta{
		dump("keep", "p", daysAgo(0), "", ""),
		dump("oldbase", "p", daysAgo(40), "oldbase", ""),
		dump("oldinc", "p", daysAgo(39), "oldbase", "oldbase"),
	}
	plan := Plan(GroupChains(ds), RetentionPolicy{KeepLast: 1}, refNow)
	if len(plan.Prune) != 1 {
		t.Fatalf("pruned %d chains, want 1", len(plan.Prune))
	}
	if len(plan.Prune[0].Members) != 2 {
		t.Errorf("pruned chain has %d members, want 2 (base+inc together)", len(plan.Prune[0].Members))
	}
}
