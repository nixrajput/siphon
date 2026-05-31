package modals

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/nixrajput/siphon/internal/tui/styles"
)

// ErrorModel is a Bubble Tea model that displays an error and a hint in a
// bordered box. It is rendered as an overlay by the dashboard, which owns its
// lifecycle: the dashboard intercepts esc/enter/q to dismiss the overlay, so
// this model never drives dismissal or quitting itself. Update only absorbs a
// window-size hint for layout; it is otherwise inert.
type ErrorModel struct {
	err   error
	hint  string
	width int
}

// NewError constructs an ErrorModel for the given error and hint text.
func NewError(err error, hint string) ErrorModel { return ErrorModel{err: err, hint: hint} }

func (m ErrorModel) Init() tea.Cmd { return nil }

func (m ErrorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if ws, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = ws.Width
	}
	return m, nil
}

func (m ErrorModel) View() string {
	// Guard against a nil error (NewError is exported, so a caller could pass
	// one) and clamp the box width so a zero/small terminal width never yields
	// a negative or absurdly narrow box.
	msg := "unknown error"
	if m.err != nil {
		msg = m.err.Error()
	}
	available := m.width - 4
	if available < 20 {
		available = 20
	}
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#fb7185")).
		Padding(1, 2).
		Width(min(80, available))
	return box.Render(fmt.Sprintf("%s\n\n%s", styles.Err.Render("✗ "+msg), m.hint))
}
