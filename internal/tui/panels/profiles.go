// Package panels contains the dashboard's child panels. Each panel is an
// independent tea.Model that conforms to siphon/internal/tui.Panel.
package panels

import (
	"fmt"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/nixrajput/siphon/internal/app"
	"github.com/nixrajput/siphon/internal/tui/styles"
)

type profileItem struct {
	name   string
	driver string
	host   string
}

func (p profileItem) Title() string       { return p.name }
func (p profileItem) Description() string { return fmt.Sprintf("%s · %s", p.driver, p.host) }
func (p profileItem) FilterValue() string { return p.name }

type Profiles struct {
	deps    app.Deps
	list    list.Model
	focused bool
}

func NewProfiles(d app.Deps) Profiles {
	l := list.New(loadItems(d), list.NewDefaultDelegate(), 0, 0)
	l.Title = "Profiles"
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.DisableQuitKeybindings()
	return Profiles{deps: d, list: l}
}

func loadItems(d app.Deps) []list.Item {
	names := d.Profiles.List()
	items := make([]list.Item, 0, len(names))
	for _, n := range names {
		p, _ := d.Profiles.Get(n)
		items = append(items, profileItem{name: n, driver: p.Driver, host: p.Host})
	}
	return items
}

func (p Profiles) Init() tea.Cmd { return nil }

func (p Profiles) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if !p.focused {
		return p, nil
	}
	var cmd tea.Cmd
	p.list, cmd = p.list.Update(msg)
	return p, cmd
}

func (p Profiles) View() string {
	border := styles.Border
	if p.focused {
		border = styles.BorderFocus
	}
	return border.Render(p.list.View())
}

func (p Profiles) Title() string { return "Profiles" }
func (p *Profiles) SetSize(w, h int) {
	p.list.SetSize(w-2, h-2)
}
func (p *Profiles) SetFocus(b bool) { p.focused = b }

// SelectedName returns the currently highlighted profile, or "" if none.
func (p Profiles) SelectedName() string {
	if i, ok := p.list.SelectedItem().(profileItem); ok {
		return i.name
	}
	return ""
}

// Reload re-reads the profile list. Call after add/remove.
func (p *Profiles) Reload() { p.list.SetItems(loadItems(p.deps)) }

// Compile-time check.
var _ tea.Model = Profiles{}
