package mysqlcommon

import (
	"context"
	"errors"
	"testing"

	"github.com/nixrajput/siphon/internal/canonical"
	"github.com/nixrajput/siphon/internal/errs"
)

// TestApplyChange_KeylessUpdateWrapsCodeUser verifies that ApplyChange
// returns *errs.Error with Code==CodeUser and Cause==ErrMissingPrimaryKey
// when called with an UPDATE that has an empty Key map. The guard runs
// before any database access, so a zero *Conn suffices here.
func TestApplyChange_KeylessUpdateWrapsCodeUser(t *testing.T) {
	c := &Conn{engine: "mysql"}
	ch := canonical.CanonicalChange{
		Op:     canonical.OpUpdate,
		Table:  "widgets",
		Key:    nil, // no primary key — should be rejected
		Values: map[string]any{"name": "hammer"},
	}

	err := c.ApplyChange(context.Background(), ch)
	if err == nil {
		t.Fatal("ApplyChange: expected error, got nil")
	}

	var appErr *errs.Error
	if !errors.As(err, &appErr) {
		t.Fatalf("ApplyChange: error is %T, want *errs.Error", err)
	}
	if appErr.Code != errs.CodeUser {
		t.Errorf("ApplyChange: Code = %v, want CodeUser (%v)", appErr.Code, errs.CodeUser)
	}
	if !errors.Is(err, errs.ErrMissingPrimaryKey) {
		t.Errorf("ApplyChange: errors.Is(err, ErrMissingPrimaryKey) = false; err = %v", err)
	}
}

// TestApplyChange_KeylessDeleteWrapsCodeUser mirrors the UPDATE case for DELETE.
func TestApplyChange_KeylessDeleteWrapsCodeUser(t *testing.T) {
	c := &Conn{engine: "mysql"}
	ch := canonical.CanonicalChange{
		Op:    canonical.OpDelete,
		Table: "widgets",
		Key:   map[string]any{}, // empty key — same guard
	}

	err := c.ApplyChange(context.Background(), ch)
	if err == nil {
		t.Fatal("ApplyChange: expected error, got nil")
	}

	var appErr *errs.Error
	if !errors.As(err, &appErr) {
		t.Fatalf("ApplyChange: error is %T, want *errs.Error", err)
	}
	if appErr.Code != errs.CodeUser {
		t.Errorf("ApplyChange: Code = %v, want CodeUser (%v)", appErr.Code, errs.CodeUser)
	}
	if !errors.Is(err, errs.ErrMissingPrimaryKey) {
		t.Errorf("ApplyChange: errors.Is(err, ErrMissingPrimaryKey) = false; err = %v", err)
	}
}
