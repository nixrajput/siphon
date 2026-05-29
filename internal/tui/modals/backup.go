// Package modals holds short-lived Bubble Tea models presented over the
// dashboard. Each modal exits by sending a result message and quitting
// its own program loop; the dashboard listens for the result and resumes.
package modals

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"

	"github.com/nixrajput/siphon/internal/app"
)

// BackupResult is sent by the backup modal when the user submits or cancels.
type BackupResult struct {
	Profile string
	Cancel  bool
}

// NewBackup builds a Huh form that lets the user pick a backup profile.
// onDone is a placeholder hook consumed by Task 8 (dashboard wiring).
// The returned cmd is a scaffold stub; Task 8 replaces it with real dispatch.
func NewBackup(d app.Deps, defaultProfile string, onDone func(BackupResult) tea.Cmd) (*huh.Form, tea.Cmd) {
	var profile = defaultProfile
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Profile").
				Options(profileOptions(d)...).
				Value(&profile),
		),
	)
	cmd := func() tea.Msg {
		// Scaffold stub: Task 8 replaces this with app.Backup dispatch.
		// onDone and profile are captured here so they are wired at that point.
		_ = onDone(BackupResult{Profile: profile})
		return nil
	}
	return form, cmd
}

func profileOptions(d app.Deps) []huh.Option[string] {
	names := d.Profiles.List()
	out := make([]huh.Option[string], 0, len(names))
	for _, n := range names {
		out = append(out, huh.NewOption(n, n))
	}
	return out
}
