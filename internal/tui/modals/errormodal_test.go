package modals

import (
	"errors"
	"strings"
	"testing"
)

// TestErrorModel_View_NilError verifies View doesn't panic and shows a fallback
// when constructed with a nil error (NewError is exported, so this is reachable).
func TestErrorModel_View_NilError(t *testing.T) {
	m := NewError(nil, "some hint")
	out := m.View() // must not panic
	if !strings.Contains(out, "unknown error") {
		t.Fatalf("View() with nil err should show fallback; got:\n%s", out)
	}
	if !strings.Contains(out, "some hint") {
		t.Fatalf("View() should still render the hint; got:\n%s", out)
	}
}

// TestErrorModel_View_ZeroWidth verifies View renders without panic at width 0
// (the dashboard does not forward a WindowSizeMsg to the overlay, so width is
// typically 0) — the clamp must keep the box width sane.
func TestErrorModel_View_ZeroWidth(t *testing.T) {
	m := NewError(errors.New("boom"), "")
	out := m.View() // width is 0; clamp must avoid a negative Width
	if !strings.Contains(out, "boom") {
		t.Fatalf("View() should render the error message; got:\n%s", out)
	}
}
