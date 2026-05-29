package tui

import (
	"io"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
)

// TestDashboard_Initial_Snapshot captures the initial rendered dashboard.
func TestDashboard_Initial_Snapshot(t *testing.T) {
	d := NewDashboard(testDeps(t))
	tm := teatest.NewTestModel(t, d, teatest.WithInitialTermSize(120, 30))
	tm.Send(tea.WindowSizeMsg{Width: 120, Height: 30})

	// Wait until output is non-empty.
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return len(b) > 0
	}, teatest.WithCheckInterval(20*time.Millisecond), teatest.WithDuration(2*time.Second))

	// Quit so the program finishes and FinalOutput is readable.
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))

	out, err := io.ReadAll(tm.FinalOutput(t, teatest.WithFinalTimeout(2*time.Second)))
	if err != nil {
		t.Fatal(err)
	}
	teatest.RequireEqualOutput(t, out)
}

// TestDashboard_FocusDumps_Snapshot captures the dashboard after one Tab press
// (focus moved to the Dumps panel).
func TestDashboard_FocusDumps_Snapshot(t *testing.T) {
	d := NewDashboard(testDeps(t))
	tm := teatest.NewTestModel(t, d, teatest.WithInitialTermSize(120, 30))
	tm.Send(tea.WindowSizeMsg{Width: 120, Height: 30})
	tm.Send(tea.KeyMsg{Type: tea.KeyTab})

	// Wait until output is non-empty.
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return len(b) > 0
	}, teatest.WithCheckInterval(20*time.Millisecond), teatest.WithDuration(2*time.Second))

	// Quit so the program finishes and FinalOutput is readable.
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))

	out, err := io.ReadAll(tm.FinalOutput(t, teatest.WithFinalTimeout(2*time.Second)))
	if err != nil {
		t.Fatal(err)
	}
	teatest.RequireEqualOutput(t, out)
}
