package panels

import (
	"context"
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/nixrajput/siphon/internal/app"
	"github.com/nixrajput/siphon/internal/tui/styles"
)

// Dumps is a panel that displays the dump catalog in a scrollable table.
type Dumps struct {
	deps    app.Deps
	tbl     table.Model
	focused bool
}

// NewDumps creates a Dumps panel and performs an initial catalog load.
func NewDumps(d app.Deps) Dumps {
	cols := []table.Column{
		{Title: "ID", Width: 26},
		{Title: "Profile", Width: 12},
		{Title: "Created", Width: 20},
		{Title: "Size", Width: 10},
	}
	t := table.New(table.WithColumns(cols), table.WithFocused(false))
	dp := Dumps{deps: d, tbl: t}
	dp.Reload()
	return dp
}

func (p Dumps) Init() tea.Cmd { return nil }

func (p Dumps) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if !p.focused {
		return p, nil
	}
	var cmd tea.Cmd
	p.tbl, cmd = p.tbl.Update(msg)
	return p, cmd
}

func (p Dumps) View() string {
	border := styles.Border
	if p.focused {
		border = styles.BorderFocus
	}
	header := styles.PanelTitle.Render("Dumps")
	return border.Render(header + "\n" + p.tbl.View())
}

// Title returns the panel's display name.
func (p Dumps) Title() string { return "Dumps" }

// SetSize resizes the inner table to fit within the panel borders.
func (p *Dumps) SetSize(w, h int) {
	p.tbl.SetWidth(w - 2)
	p.tbl.SetHeight(h - 4)
}

// SetFocus toggles keyboard focus on the table.
func (p *Dumps) SetFocus(b bool) {
	p.focused = b
	p.tbl.Focus()
	if !b {
		p.tbl.Blur()
	}
}

// SelectedID returns the currently highlighted dump ID, or "" if empty.
func (p Dumps) SelectedID() string {
	r := p.tbl.SelectedRow()
	if len(r) == 0 {
		return ""
	}
	return r[0]
}

// Reload rebuilds the rows from the catalog.
func (p *Dumps) Reload() {
	all, err := p.deps.Dumps.List(context.Background())
	if err != nil {
		return
	}
	rows := make([]table.Row, 0, len(all))
	for _, m := range all {
		rows = append(rows, table.Row{
			m.ID,
			m.Profile,
			m.Created.Format(time.RFC3339),
			humanSize(m.SizeBytes),
		})
	}
	p.tbl.SetRows(rows)
}

func humanSize(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1fGB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1fMB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1fKB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%dB", n)
}

// Compile-time check.
var _ tea.Model = Dumps{}
