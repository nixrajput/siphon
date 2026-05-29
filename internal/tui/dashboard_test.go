package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/nixrajput/siphon/internal/app"
	"github.com/nixrajput/siphon/internal/config"
	"github.com/nixrajput/siphon/internal/dumps"
	"github.com/nixrajput/siphon/internal/profile"
	"github.com/nixrajput/siphon/internal/secrets"
)

func testDeps(t *testing.T) app.Deps {
	t.Helper()
	cfg := &config.Config{}
	res := secrets.NewResolver(secrets.Passthrough{})
	ps := profile.New(cfg, res, func(*config.Config) error { return nil })
	cat, err := dumps.NewCatalog(t.TempDir())
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}
	return app.Deps{Profiles: ps, Dumps: cat, Drivers: app.DefaultDrivers()}
}

func TestDashboard_View_ContainsPanelsAndQuitHint(t *testing.T) {
	d := NewDashboard(testDeps(t))
	out, _ := d.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	view := out.(Dashboard).View()
	for _, want := range []string{"Profiles", "Dumps", "Jobs", "quit"} {
		if !strings.Contains(view, want) {
			t.Fatalf("View() missing %q; got:\n%s", want, view)
		}
	}
}

func TestDashboard_Update_QQuits(t *testing.T) {
	d := NewDashboard(testDeps(t))
	_, cmd := d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil {
		t.Fatal("Update on 'q' returned no command; expected tea.Quit")
	}
}

// TestDashboard_Tab_MovesFocus verifies FIX 2: pressing tab advances focus
// through the panels and the change is visible in the rendered frame.
func TestDashboard_Tab_MovesFocus(t *testing.T) {
	// Force color rendering so the focus-border color difference is visible
	// in the rendered frame even off a TTY.
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })

	d := NewDashboard(testDeps(t))
	before := d.View()

	out, _ := d.Update(tea.KeyMsg{Type: tea.KeyTab})
	moved := out.(Dashboard)
	if moved.focusIdx != 1 {
		t.Fatalf("after tab, focusIdx = %d; want 1", moved.focusIdx)
	}
	if moved.View() == before {
		t.Fatal("View did not change after tab; focus border did not move")
	}

	// Wrap-around: from last panel, tab returns to the first.
	out, _ = moved.Update(tea.KeyMsg{Type: tea.KeyTab})
	out, _ = out.(Dashboard).Update(tea.KeyMsg{Type: tea.KeyTab})
	if got := out.(Dashboard).focusIdx; got != 0 {
		t.Fatalf("after wrap, focusIdx = %d; want 0", got)
	}
}
