package secrets

// Passthrough returns the ref as-is. Used for literal cleartext values
// (or for values that have already been env-interpolated at config-load time).
type Passthrough struct{}

func (Passthrough) Scheme() string                     { return "" }
func (Passthrough) Resolve(ref string) (string, error) { return ref, nil }
