package modals

import (
	"github.com/charmbracelet/huh"

	"github.com/nixrajput/siphon/internal/app"
)

// RestoreResult captures the values collected by the restore form.
type RestoreResult struct {
	Profile string
	DumpID  string
	Clean   bool
	Cancel  bool
}

// NewRestore builds a Huh form for selecting a restore target.
func NewRestore(d app.Deps, defaultProfile, defaultDump string) *huh.Form {
	profile := defaultProfile
	dump := defaultDump
	clean := false
	return huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().Title("Target profile").Options(profileOptions(d)...).Value(&profile),
			huh.NewInput().Title("Dump ID").Value(&dump),
			huh.NewConfirm().Title("Clean target before restore?").Affirmative("Yes").Negative("No").Value(&clean),
		),
	)
}
