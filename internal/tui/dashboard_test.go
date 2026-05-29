package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestModel_View_ContainsTitleAndQuitHint(t *testing.T) {
	m := New()
	view := m.View()
	if !strings.Contains(view, "siphon") {
		t.Fatalf("View() = %q; want it to contain 'siphon'", view)
	}
	if !strings.Contains(view, "quit") {
		t.Fatalf("View() = %q; want it to contain quit hint", view)
	}
}

func TestModel_Update_QQuits(t *testing.T) {
	m := New()
	out, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil {
		t.Fatal("Update on 'q' returned no command; expected tea.Quit")
	}
	if updated, ok := out.(Model); !ok || !updated.quitting {
		t.Fatalf("Update did not flag quitting; got model=%+v ok=%v", out, ok)
	}
}

func TestModel_Update_OtherKey_Noop(t *testing.T) {
	m := New()
	out, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	if cmd != nil {
		t.Fatalf("Update on 'x' returned a command; expected nil")
	}
	if updated, ok := out.(Model); !ok || updated.quitting {
		t.Fatalf("Update flagged quitting on 'x'; should not")
	}
}
