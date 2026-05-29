// Package tui hosts siphon's Bubble Tea application. Phase A ships a
// minimal placeholder; the real multi-panel dashboard lands in Phase C.
package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type Model struct {
	quitting bool
}

// New returns a fresh, ready-to-run TUI model.
func New() Model { return Model{} }

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "q", "esc", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		}
	}
	return m, nil
}

var (
	title    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7dd3fc"))
	subtitle = lipgloss.NewStyle().Faint(true)
)

func (m Model) View() string {
	if m.quitting {
		return ""
	}
	return title.Render("siphon") + "\n" +
		subtitle.Render("sync any database, anywhere") + "\n\n" +
		"press q to quit.\n"
}

// Run starts the TUI program and blocks until it exits.
func Run() error {
	_, err := tea.NewProgram(New()).Run()
	return err
}
