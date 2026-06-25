// Package schedule manages a siphon-owned block of entries inside the user's
// crontab. siphon does not run a scheduler daemon itself — it delegates to the
// host's cron, writing lines that invoke `siphon backup <profile>` on a cron
// expression. The OS scheduler (already present and battle-tested) runs the
// jobs; siphon only edits its own managed block.
//
// This file is the pure, I/O-free core: it edits crontab TEXT (add/remove/list
// siphon entries within delimited markers) so it is fully unit-testable. The CLI
// layer reads the current crontab, calls these functions, and writes it back.
package schedule

import (
	"fmt"
	"sort"
	"strings"
)

const (
	beginMarker = "# >>> siphon managed (do not edit this block) >>>"
	endMarker   = "# <<< siphon managed <<<"
	linePrefix  = "# siphon:" // metadata comment preceding each managed job line
)

// Entry is one scheduled backup: a cron expression and the profile to back up.
type Entry struct {
	Profile string
	Cron    string // standard 5-field cron expression
}

// Render produces the crontab line(s) for an entry: a metadata comment (so List
// can recover the profile) followed by the cron line invoking siphon. bin is the
// siphon binary path to invoke.
func (e Entry) render(bin string) string {
	return fmt.Sprintf("%s %s\n%s %s backup %s",
		linePrefix, e.Profile, e.Cron, bin, e.Profile)
}

// List extracts the siphon-managed entries from a crontab's text. Entries
// outside the managed block are ignored. Returns them sorted by profile.
func List(crontab string) []Entry {
	block := extractBlock(crontab)
	var entries []Entry
	lines := strings.Split(block, "\n")
	for i := 0; i < len(lines); i++ {
		meta := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(meta, linePrefix) {
			continue
		}
		profile := strings.TrimSpace(strings.TrimPrefix(meta, linePrefix))
		// The cron line is the next non-empty line; recover its expression (the
		// first 5 fields).
		if i+1 < len(lines) {
			fields := strings.Fields(lines[i+1])
			if len(fields) >= 5 {
				entries = append(entries, Entry{Profile: profile, Cron: strings.Join(fields[:5], " ")})
			}
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Profile < entries[j].Profile })
	return entries
}

// Add returns a new crontab with entry added (or its cron updated if the profile
// already has an entry). Lines outside the managed block are preserved verbatim.
func Add(crontab, bin string, entry Entry) string {
	entries := List(crontab)
	replaced := false
	for i := range entries {
		if entries[i].Profile == entry.Profile {
			entries[i].Cron = entry.Cron
			replaced = true
		}
	}
	if !replaced {
		entries = append(entries, entry)
	}
	return writeBlock(crontab, bin, entries)
}

// Remove returns a new crontab with the named profile's entry removed. Removing
// a profile that has no entry is a no-op.
func Remove(crontab, bin, profile string) string {
	var kept []Entry
	for _, e := range List(crontab) {
		if e.Profile != profile {
			kept = append(kept, e)
		}
	}
	return writeBlock(crontab, bin, kept)
}

// extractBlock returns the text between the markers (exclusive), or "" if the
// managed block is absent.
func extractBlock(crontab string) string {
	b := strings.Index(crontab, beginMarker)
	e := strings.Index(crontab, endMarker)
	if b < 0 || e < 0 || e < b {
		return ""
	}
	return crontab[b+len(beginMarker) : e]
}

// writeBlock returns crontab with its managed block replaced by a freshly
// rendered block for entries (sorted by profile). If entries is empty, the
// managed block is removed entirely. Non-managed lines are preserved.
func writeBlock(crontab, bin string, entries []Entry) string {
	sort.Slice(entries, func(i, j int) bool { return entries[i].Profile < entries[j].Profile })

	// Strip any existing managed block (and the surrounding markers + blank line).
	outside := stripBlock(crontab)

	if len(entries) == 0 {
		return outside
	}

	var b strings.Builder
	b.WriteString(beginMarker)
	b.WriteString("\n")
	for _, e := range entries {
		b.WriteString(e.render(bin))
		b.WriteString("\n")
	}
	b.WriteString(endMarker)
	b.WriteString("\n")

	if strings.TrimSpace(outside) == "" {
		return b.String()
	}
	return strings.TrimRight(outside, "\n") + "\n" + b.String()
}

// stripBlock removes the managed block (markers inclusive) from crontab,
// returning the surrounding user content.
func stripBlock(crontab string) string {
	b := strings.Index(crontab, beginMarker)
	e := strings.Index(crontab, endMarker)
	if b < 0 || e < 0 || e < b {
		return crontab
	}
	before := strings.TrimRight(crontab[:b], "\n")
	after := strings.TrimLeft(crontab[e+len(endMarker):], "\n")
	if before == "" {
		return after
	}
	if after == "" {
		return before + "\n"
	}
	return before + "\n" + after
}
