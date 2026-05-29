// Package tui hosts siphon's Bubble Tea application: the multi-panel
// dashboard, its child panels, and the modal forms layered over them.
package tui

import (
	"context"
	"errors"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"

	"github.com/nixrajput/siphon/internal/app"
	"github.com/nixrajput/siphon/internal/errs"
	"github.com/nixrajput/siphon/internal/jobs"
	"github.com/nixrajput/siphon/internal/tui/modals"
	"github.com/nixrajput/siphon/internal/tui/panels"
	"github.com/nixrajput/siphon/internal/tui/styles"
)

// formKind discriminates which app verb an active form will dispatch.
type formKind int

const (
	formNone formKind = iota
	formBackup
	formRestore
)

// Dashboard is the root TUI model. It owns the child panels, focus state,
// key routing, and the active modal form / error overlay.
//
// Panels are held by pointer so that focus mutations made through d.order
// (which holds the same pointers) are visible to the field accessed in
// View(). Copying the Dashboard value copies the pointers, not the panel
// structs, so order and the named fields never diverge (FIX 2, approach a).
type Dashboard struct {
	deps   app.Deps
	keys   KeyMap
	width  int
	height int

	profiles *panels.Profiles
	dumps    *panels.Dumps
	jobs     *panels.Jobs

	order    []Panel
	focusIdx int

	// Active modal form, if any.
	form       *huh.Form
	formKind   formKind
	backupRes  *modals.BackupResult
	restoreRes *modals.RestoreResult

	// Active error overlay, if any.
	errModal *modals.ErrorModel
}

// NewDashboard constructs the root model with the three panels wired up and
// focus on the profiles panel.
func NewDashboard(d app.Deps) Dashboard {
	prof := panels.NewProfiles(d)
	dmp := panels.NewDumps(d)
	jbs := panels.NewJobs()
	pp, dp, jp := &prof, &dmp, &jbs
	pp.SetFocus(true)
	return Dashboard{
		deps:     d,
		keys:     DefaultKeys(),
		profiles: pp,
		dumps:    dp,
		jobs:     jp,
		order:    []Panel{pp, dp, jp},
	}
}

func (d Dashboard) Init() tea.Cmd {
	return tea.Batch(d.profiles.Init(), d.dumps.Init(), d.jobs.Init())
}

func (d Dashboard) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// 1. Error overlay swallows input until dismissed.
	if d.errModal != nil {
		if k, ok := msg.(tea.KeyMsg); ok {
			switch k.String() {
			case "esc", "enter", "q":
				d.errModal = nil
			}
		}
		return d, nil
	}

	// 2. An active form receives all input until it completes or aborts.
	if d.form != nil {
		return d.updateForm(msg)
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		d.width, d.height = msg.Width, msg.Height
		d.layout()
		return d, nil
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, d.keys.Quit):
			return d, tea.Quit
		case key.Matches(msg, d.keys.NextPanel):
			d.advanceFocus(+1)
			return d, nil
		case key.Matches(msg, d.keys.PrevPanel):
			d.advanceFocus(-1)
			return d, nil
		case key.Matches(msg, d.keys.Backup):
			return d.openBackup()
		case key.Matches(msg, d.keys.Restore):
			return d.openRestore()
		}
	case JobChannelMsg:
		return d, panels.SubscribeCmd(msg.Channel)
	case panels.JobEventInternal:
		updated, cmd := d.jobs.Update(msg)
		*d.jobs = updated.(panels.Jobs)
		// React to the runner's terminal event (sent as a normal event before
		// the channel closes): surface failures and refresh the catalog.
		switch msg.Event.Phase {
		case jobs.PhaseError, jobs.PhaseCancelled:
			err := msg.Event.Err
			if err == nil {
				if msg.Event.Message != "" {
					err = errors.New(msg.Event.Message)
				} else {
					err = errors.New("job failed")
				}
			}
			hint := ""
			var se *errs.Error
			if errors.As(err, &se) {
				hint = se.Hint
			}
			em := modals.NewError(err, hint)
			d.errModal = &em
		case jobs.PhaseDone:
			d.dumps.Reload()
		}
		return d, cmd
	case ErrorMsg:
		em := modals.NewError(msg.Err, msg.Hint)
		d.errModal = &em
		return d, nil
	}

	// Route remaining messages to the focused panel.
	if d.focusIdx >= 0 && d.focusIdx < len(d.order) {
		updated, cmd := d.order[d.focusIdx].Update(msg)
		d.applyPanelUpdate(updated)
		return d, cmd
	}
	return d, nil
}

func (d Dashboard) View() string {
	if d.errModal != nil {
		return d.errModal.View()
	}
	if d.form != nil {
		hint := styles.StatusBar.Width(d.width).Render(d.formHint())
		return d.form.View() + "\n" + hint
	}
	body := lipgloss.JoinHorizontal(lipgloss.Top,
		d.profiles.View(), d.dumps.View(), d.jobs.View())
	status := styles.StatusBar.Width(d.width).
		Render("b backup · r restore · tab focus · q quit")
	return body + "\n" + status
}

// formHint returns the command-row hint text for the currently active form.
func (d Dashboard) formHint() string {
	switch d.formKind {
	case formBackup:
		return "↑/↓ select · enter confirm · esc cancel"
	case formRestore:
		return "↑/↓ select · type dump id · enter next · esc cancel"
	default:
		return "enter next · esc cancel"
	}
}

// updateForm routes a message to the active form and, on completion, fires
// the corresponding app verb. On abort it tears the form down.
func (d Dashboard) updateForm(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Esc cancels the form without dispatching.
	if k, ok := msg.(tea.KeyMsg); ok && k.Type == tea.KeyEsc {
		d.clearForm()
		return d, nil
	}
	model, cmd := d.form.Update(msg)
	if f, ok := model.(*huh.Form); ok {
		d.form = f
	}
	switch d.form.State {
	case huh.StateCompleted:
		dispatch := d.dispatchForm()
		d.clearForm()
		return d, tea.Batch(cmd, dispatch)
	case huh.StateAborted:
		d.clearForm()
		return d, cmd
	}
	return d, cmd
}

// dispatchForm builds the command that fires the verb for the just-completed
// form. Returns nil when there is nothing to dispatch.
func (d Dashboard) dispatchForm() tea.Cmd {
	switch d.formKind {
	case formBackup:
		if d.backupRes == nil || d.backupRes.Profile == "" {
			return nil
		}
		opt := app.BackupOpts{Profile: d.backupRes.Profile}
		return func() tea.Msg {
			ch, id, err := app.Backup(context.Background(), d.deps, opt)
			if err != nil {
				return ErrorMsg{Err: err, Hint: "check that the profile + secrets resolve"}
			}
			return JobChannelMsg{Channel: ch, JobID: id}
		}
	case formRestore:
		if d.restoreRes == nil || d.restoreRes.Profile == "" || d.restoreRes.DumpID == "" {
			return nil
		}
		opt := app.RestoreOpts{
			Profile: d.restoreRes.Profile,
			DumpID:  d.restoreRes.DumpID,
			Clean:   d.restoreRes.Clean,
		}
		return func() tea.Msg {
			ch, id, err := app.Restore(context.Background(), d.deps, opt)
			if err != nil {
				return ErrorMsg{Err: err, Hint: "check that the dump + target profile resolve"}
			}
			return JobChannelMsg{Channel: ch, JobID: id}
		}
	}
	return nil
}

func (d *Dashboard) clearForm() {
	d.form = nil
	d.formKind = formNone
	d.backupRes = nil
	d.restoreRes = nil
}

// openBackup builds and activates the backup form.
func (d Dashboard) openBackup() (tea.Model, tea.Cmd) {
	form, res := modals.NewBackup(d.deps, d.profiles.SelectedName())
	d.form = form
	d.formKind = formBackup
	d.backupRes = res
	return d, d.form.Init()
}

// openRestore builds and activates the restore form, defaulting the dump ID
// to the dumps panel's current selection.
func (d Dashboard) openRestore() (tea.Model, tea.Cmd) {
	form, res := modals.NewRestore(d.deps, d.profiles.SelectedName(), d.dumps.SelectedID())
	d.form = form
	d.formKind = formRestore
	d.restoreRes = res
	return d, d.form.Init()
}

func (d *Dashboard) layout() {
	if d.width == 0 || d.height == 0 {
		return
	}
	colW := (d.width - 6) / 3
	rowH := d.height - 4
	d.profiles.SetSize(colW, rowH)
	d.dumps.SetSize(colW, rowH)
	d.jobs.SetSize(colW, rowH)
}

func (d *Dashboard) advanceFocus(delta int) {
	d.order[d.focusIdx].SetFocus(false)
	d.focusIdx = (d.focusIdx + delta + len(d.order)) % len(d.order)
	d.order[d.focusIdx].SetFocus(true)
}

// applyPanelUpdate writes a panel's post-Update value back through the stored
// pointer so the change survives the Dashboard value copy.
func (d *Dashboard) applyPanelUpdate(updated tea.Model) {
	switch p := updated.(type) {
	case panels.Profiles:
		*d.profiles = p
	case panels.Dumps:
		*d.dumps = p
	case panels.Jobs:
		*d.jobs = p
	}
}
