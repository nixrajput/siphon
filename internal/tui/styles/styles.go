// Package styles holds Lipgloss styles shared across the TUI. Keep this
// small and deliberate — every visual decision lives here so the look is
// consistent across panels and modals.
package styles

import "github.com/charmbracelet/lipgloss"

var (
	Title       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7dd3fc"))
	Subtitle    = lipgloss.NewStyle().Faint(true)
	Dim         = lipgloss.NewStyle().Faint(true)
	Ok          = lipgloss.NewStyle().Foreground(lipgloss.Color("#4ade80"))
	Warn        = lipgloss.NewStyle().Foreground(lipgloss.Color("#fbbf24"))
	Err         = lipgloss.NewStyle().Foreground(lipgloss.Color("#fb7185"))
	Label       = lipgloss.NewStyle().Foreground(lipgloss.Color("#a3e635"))
	Selection   = lipgloss.NewStyle().Background(lipgloss.Color("#1e3a8a")).Foreground(lipgloss.Color("#f0f9ff"))
	PanelTitle  = lipgloss.NewStyle().Bold(true).Padding(0, 1).Foreground(lipgloss.Color("#c084fc"))
	Border      = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#1f2733")).Padding(0, 1)
	BorderFocus = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#7dd3fc")).Padding(0, 1)
	StatusBar   = lipgloss.NewStyle().Background(lipgloss.Color("#0c0f14")).Foreground(lipgloss.Color("#6b7280")).Padding(0, 1)
)
