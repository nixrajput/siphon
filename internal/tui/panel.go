package tui

import tea "github.com/charmbracelet/bubbletea"

// Panel is the small contract every child component implements so the
// Dashboard can route key events, lifecycle messages, and resize hints
// to the focused panel uniformly.
type Panel interface {
	tea.Model
	Title() string
	SetSize(w, h int)
	SetFocus(bool)
}
