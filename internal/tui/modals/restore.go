package modals

import (
	"github.com/charmbracelet/huh"

	"github.com/nixrajput/siphon/internal/app"
)

// RestoreResult holds the values bound by the restore form. The dashboard
// reads it after the form completes.
type RestoreResult struct {
	Profile string
	DumpID  string
	Clean   bool
}

// NewRestore builds a Huh form for selecting a restore target. The returned
// form binds its inputs into the returned *RestoreResult, which the dashboard
// reads once the form reaches huh.StateCompleted.
func NewRestore(d app.Deps, defaultProfile, defaultDump string) (*huh.Form, *RestoreResult) {
	res := &RestoreResult{Profile: defaultProfile, DumpID: defaultDump}
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().Title("Target profile").Options(profileOptions(d)...).Value(&res.Profile),
			huh.NewInput().Title("Dump ID").Value(&res.DumpID),
			huh.NewConfirm().Title("Clean target before restore?").Affirmative("Yes").Negative("No").Value(&res.Clean),
		),
	)
	return form, res
}
