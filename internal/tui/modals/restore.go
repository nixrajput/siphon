package modals

import (
	"context"

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
//
// The dump-ID step is adaptive: if the catalog contains dumps, a Select field
// is shown; otherwise an Input field is shown (with a title that signals no
// saved dumps exist).
func NewRestore(d app.Deps, defaultProfile, defaultDump string) (*huh.Form, *RestoreResult) {
	res := &RestoreResult{Profile: defaultProfile, DumpID: defaultDump}

	var dumpIDField huh.Field
	metas, err := d.Dumps.List(context.Background())
	if err == nil && len(metas) > 0 {
		opts := make([]huh.Option[string], len(metas))
		for i, m := range metas {
			opts[i] = huh.NewOption(m.ID, m.ID)
		}
		dumpIDField = huh.NewSelect[string]().
			Title("Dump ID (/ to filter)").
			Options(opts...).
			Filtering(true).
			Value(&res.DumpID)
	} else {
		dumpIDField = huh.NewInput().
			Title("Dump ID (type — no saved dumps)").
			Value(&res.DumpID)
	}

	form := huh.NewForm(
		huh.NewGroup(huh.NewSelect[string]().Title("Target profile").Options(profileOptions(d)...).Value(&res.Profile)),
		huh.NewGroup(dumpIDField),
		huh.NewGroup(huh.NewConfirm().Title("Clean target before restore?").Affirmative("Yes").Negative("No").Value(&res.Clean)),
	).WithShowHelp(false)
	return form, res
}
