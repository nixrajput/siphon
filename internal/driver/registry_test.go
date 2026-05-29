package driver

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/nixrajput/siphon/internal/errs"
)

type fakeDriver struct {
	name string
	caps Capabilities
}

func (f *fakeDriver) Name() string                                   { return f.name }
func (f *fakeDriver) Capabilities() Capabilities                     { return f.caps }
func (f *fakeDriver) Connect(context.Context, Profile) (Conn, error) { return nil, nil }

func TestRegister_And_Get_Roundtrip(t *testing.T) {
	t.Cleanup(reset)
	Register(&fakeDriver{name: "test-driver"})

	got, err := Get("test-driver")
	if err != nil {
		t.Fatalf("Get returned unexpected error: %v", err)
	}
	if got.Name() != "test-driver" {
		t.Fatalf("Get returned driver with Name %q; want %q", got.Name(), "test-driver")
	}
}

func TestGet_Unknown_ReturnsErrDriverUnsupported(t *testing.T) {
	t.Cleanup(reset)
	_, err := Get("does-not-exist")
	if !errors.Is(err, errs.ErrDriverUnsupported) {
		t.Fatalf("Get returned %v; want errs.ErrDriverUnsupported", err)
	}
}

func TestList_ReturnsAllRegistered(t *testing.T) {
	t.Cleanup(reset)
	Register(&fakeDriver{name: "a"})
	Register(&fakeDriver{name: "b"})

	names := List()
	if len(names) != 2 {
		t.Fatalf("List() returned %d names; want 2", len(names))
	}

	have := map[string]bool{names[0]: true, names[1]: true}
	if !have["a"] || !have["b"] {
		t.Fatalf("List() = %v; want [a b]", names)
	}
}

func TestRegister_Duplicate_Panics(t *testing.T) {
	t.Cleanup(reset)
	Register(&fakeDriver{name: "dup"})

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("Register did not panic on duplicate name")
		}
	}()
	Register(&fakeDriver{name: "dup"})
}

// reset clears the registry between tests since it is process-wide.
func reset() {
	mu.Lock()
	defer mu.Unlock()
	registry = map[string]Driver{}
}

// Compile-time check that fakeDriver implements Driver.
var _ Driver = (*fakeDriver)(nil)

// Silence the unused-import warning for io while tests don't exercise Conn yet.
var _ io.Writer = io.Discard
