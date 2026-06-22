package app

import (
	"context"
	"errors"
	"testing"

	"github.com/nixrajput/siphon/internal/canonical"
	"github.com/nixrajput/siphon/internal/errs"
)

// Tests for EmitCanonical and ConsumeCanonical (db-touching orchestration) will
// live here. Pure-function tests (types, builders, quote/placeholder) are in
// internal/canonical/canonical_test.go.

// TestApplyChange_BuildsCorrectSQLPerOp asserts the deterministic column
// ordering that changeColumns produces for UPDATE dispatch.
func TestApplyChange_BuildsCorrectSQLPerOp(t *testing.T) {
	ch := canonical.CanonicalChange{
		Op:     canonical.OpUpdate,
		Table:  "t",
		Key:    map[string]any{"id": 1},
		Values: map[string]any{"id": 1, "name": "x"},
	}
	set, key := changeColumns(ch)
	// Key column must not appear in SET.
	if len(key) != 1 || key[0] != "id" {
		t.Fatalf("key cols = %v", key)
	}
	if len(set) != 1 || set[0] != "name" {
		t.Fatalf("set cols = %v want [name]", set)
	}
}

// TestApplyChange_KeylessUpdateDelete_Rejected confirms that UPDATE and DELETE
// with an empty Key return a CodeUser *errs.Error before touching the db.
// A nil *sql.DB is intentionally passed to prove the guard fires first.
func TestApplyChange_KeylessUpdateDelete_Rejected(t *testing.T) {
	for _, op := range []canonical.ChangeOp{canonical.OpUpdate, canonical.OpDelete} {
		ch := canonical.CanonicalChange{Op: op, Table: "t", Values: map[string]any{"v": 1}} // no Key
		err := ApplyChange(context.Background(), nil, "postgres", ch)
		if err == nil {
			t.Fatalf("op %s: expected error for empty Key, got nil", op)
		}
		var e *errs.Error
		if !errors.As(err, &e) || e.Code != errs.CodeUser {
			t.Fatalf("op %s: want *errs.Error CodeUser, got %v", op, err)
		}
	}
}

// TestChangeColumns_MultipleKeyAndSet confirms sorted, disjoint outputs.
func TestChangeColumns_MultipleKeyAndSet(t *testing.T) {
	ch := canonical.CanonicalChange{
		Op:     canonical.OpUpdate,
		Table:  "t",
		Key:    map[string]any{"b": 2, "a": 1},
		Values: map[string]any{"a": 1, "b": 2, "z": "v", "m": "w"},
	}
	set, key := changeColumns(ch)
	// Keys must be sorted.
	if len(key) != 2 || key[0] != "a" || key[1] != "b" {
		t.Fatalf("key cols = %v want [a b]", key)
	}
	// SET cols must be sorted, disjoint from key.
	if len(set) != 2 || set[0] != "m" || set[1] != "z" {
		t.Fatalf("set cols = %v want [m z]", set)
	}
}
