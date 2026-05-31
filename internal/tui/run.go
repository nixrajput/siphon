package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/nixrajput/siphon/internal/app"
)

// Run launches the dashboard with the given app deps and blocks until the
// user quits. It replaces the Phase A no-arg Run().
func Run(deps app.Deps) error {
	_, err := tea.NewProgram(NewDashboard(deps), tea.WithAltScreen()).Run()
	return err
}
