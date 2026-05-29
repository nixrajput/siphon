package driver

import (
	"sync"

	"github.com/nixrajput/siphon/internal/errs"
)

var (
	mu       sync.RWMutex
	registry = map[string]Driver{}
)

// Register adds d to the process-wide registry. Drivers call Register
// from package init() so they are available before any verb runs.
//
// Register panics if called twice for the same driver name. This mirrors
// the convention used by database/sql.Register: registration is an init()
// concern, and a duplicate almost always indicates a copy-paste bug we
// want to surface loudly at startup rather than have silently overwrite.
func Register(d Driver) {
	mu.Lock()
	defer mu.Unlock()
	if _, exists := registry[d.Name()]; exists {
		panic("driver: Register called twice for driver " + d.Name())
	}
	registry[d.Name()] = d
}

// Get returns the registered driver with the given name. If no driver
// matches, it returns errs.ErrDriverUnsupported so callers can match via
// errors.Is and surface a consistent exit code.
func Get(name string) (Driver, error) {
	mu.RLock()
	defer mu.RUnlock()
	d, ok := registry[name]
	if !ok {
		return nil, errs.ErrDriverUnsupported
	}
	return d, nil
}

// List returns the names of all registered drivers. Order is unspecified.
func List() []string {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]string, 0, len(registry))
	for n := range registry {
		out = append(out, n)
	}
	return out
}
