package schedule

import (
	"strings"
	"testing"
)

const bin = "/usr/local/bin/siphon"

func TestAddListRoundTrip(t *testing.T) {
	tab := Add("", bin, Entry{Profile: "prod", Cron: "0 2 * * *"})
	tab = Add(tab, bin, Entry{Profile: "staging", Cron: "30 3 * * *"})

	entries := List(tab)
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2: %+v", len(entries), entries)
	}
	// Sorted by profile.
	if entries[0].Profile != "prod" || entries[0].Cron != "0 2 * * *" {
		t.Errorf("entries[0] = %+v, want prod / 0 2 * * *", entries[0])
	}
	if entries[1].Profile != "staging" || entries[1].Cron != "30 3 * * *" {
		t.Errorf("entries[1] = %+v, want staging / 30 3 * * *", entries[1])
	}
	// The rendered crontab actually invokes siphon backup (args are shell-quoted).
	if !strings.Contains(tab, "'"+bin+"' backup 'prod'") {
		t.Errorf("crontab missing backup invocation:\n%s", tab)
	}
}

func TestRenderShellQuotesArgs(t *testing.T) {
	// A profile name (or bin path) containing a space or quote must be quoted so
	// it stays one argv element and cannot inject into the cron command.
	tab := Add("", "/opt/my tools/siphon", Entry{Profile: "weird'; rm -rf /", Cron: "0 0 * * *"})
	// The dangerous content must appear only inside single quotes, with the inner
	// quote escaped — never as a bare token that sh would re-parse.
	if !strings.Contains(tab, `'/opt/my tools/siphon' backup 'weird'\''; rm -rf /'`) {
		t.Errorf("args not safely shell-quoted:\n%s", tab)
	}
	// And it still round-trips through List (the metadata comment carries the raw
	// profile, so List recovers it regardless of quoting).
	if got := List(tab); len(got) != 1 || got[0].Profile != "weird'; rm -rf /" {
		t.Errorf("List did not recover the raw profile: %+v", got)
	}
}

func TestAddReschedulesExistingProfile(t *testing.T) {
	tab := Add("", bin, Entry{Profile: "prod", Cron: "0 2 * * *"})
	tab = Add(tab, bin, Entry{Profile: "prod", Cron: "0 5 * * *"}) // same profile, new time

	entries := List(tab)
	if len(entries) != 1 {
		t.Fatalf("reschedule created a duplicate: %+v", entries)
	}
	if entries[0].Cron != "0 5 * * *" {
		t.Errorf("cron = %q, want updated 0 5 * * *", entries[0].Cron)
	}
}

func TestPreservesNonManagedLines(t *testing.T) {
	user := "# my own job\n0 0 * * * /bin/echo hi\n"
	tab := Add(user, bin, Entry{Profile: "prod", Cron: "0 2 * * *"})

	if !strings.Contains(tab, "/bin/echo hi") {
		t.Errorf("user's own cron line was lost:\n%s", tab)
	}
	if len(List(tab)) != 1 {
		t.Errorf("siphon entry not added alongside user line")
	}

	// Removing the siphon entry must leave the user's line intact and drop the
	// managed block entirely.
	tab = Remove(tab, bin, "prod")
	if !strings.Contains(tab, "/bin/echo hi") {
		t.Errorf("user line lost on remove:\n%s", tab)
	}
	if strings.Contains(tab, beginMarker) {
		t.Errorf("empty managed block was left behind:\n%s", tab)
	}
}

func TestRemoveNonexistentIsNoOp(t *testing.T) {
	tab := Add("", bin, Entry{Profile: "prod", Cron: "0 2 * * *"})
	out := Remove(tab, bin, "nope")
	if len(List(out)) != 1 {
		t.Errorf("removing a nonexistent profile changed the entries: %+v", List(out))
	}
}

func TestListEmpty(t *testing.T) {
	if got := List(""); len(got) != 0 {
		t.Errorf("List(empty) = %+v, want none", got)
	}
	if got := List("# just a user comment\n"); len(got) != 0 {
		t.Errorf("List(no managed block) = %+v, want none", got)
	}
}
