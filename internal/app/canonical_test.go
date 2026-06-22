package app

import (
	"testing"

	"github.com/nixrajput/siphon/internal/canonical"
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
