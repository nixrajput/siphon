package modals

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/nixrajput/siphon/internal/tui/styles"
)

// ErrorModel is a Bubble Tea model that displays an error and a hint in a
// bordered box. Pressing esc, enter, or q quits the program.
type ErrorModel struct {
	err   error
	hint  string
	width int
}

// NewError constructs an ErrorModel for the given error and hint text.
func NewError(err error, hint string) ErrorModel { return ErrorModel{err: err, hint: hint} }

func (m ErrorModel) Init() tea.Cmd { return nil }

func (m ErrorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
	case tea.KeyMsg:
		if k := msg.String(); k == "esc" || k == "enter" || k == "q" {
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m ErrorModel) View() string {
	body := fmt.Sprintf("%s\n\n%s", styles.Err.Render("✗ "+m.err.Error()), m.hint)
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#fb7185")).
		Padding(1, 2).
		Width(min(80, m.width-4))
	return box.Render(body)
}
