// Package modals holds short-lived Bubble Tea forms presented over the
// dashboard. Each form binds its inputs into a result struct that the
// dashboard reads once the form reaches huh.StateCompleted.
package modals

import (
	"github.com/charmbracelet/huh"

	"github.com/nixrajput/siphon/internal/app"
)

// BackupResult holds the values bound by the backup form. The dashboard
// reads it after the form completes.
type BackupResult struct {
	Profile string
}

// NewBackup builds a Huh form that lets the user pick a backup profile. The
// returned form binds its selection into the returned *BackupResult, which
// the dashboard reads once the form reaches huh.StateCompleted.
func NewBackup(d app.Deps, defaultProfile string) (*huh.Form, *BackupResult) {
	res := &BackupResult{Profile: defaultProfile}
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Profile").
				Options(profileOptions(d)...).
				Value(&res.Profile),
		),
	)
	return form, res
}

func profileOptions(d app.Deps) []huh.Option[string] {
	names := d.Profiles.List()
	out := make([]huh.Option[string], 0, len(names))
	for _, n := range names {
		out = append(out, huh.NewOption(n, n))
	}
	return out
}
