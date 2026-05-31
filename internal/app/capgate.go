package app

import (
	"github.com/nixrajput/siphon/internal/driver"
	"github.com/nixrajput/siphon/internal/errs"
)

// Capability identifies a specific feature flag a verb requires. The string
// values mirror the driver.Capabilities field semantics and appear in the
// user-facing hint when a driver lacks the capability.
type Capability string

const (
	CapIncremental             Capability = "incremental"
	CapNativeStream            Capability = "native-stream"
	CapPerTable                Capability = "per-table"
	CapSchemaOnly              Capability = "schema-only"
	CapDataOnly                Capability = "data-only"
	CapParallel                Capability = "parallel"
	CapCompression             Capability = "compression"
	CapBinaryFormat            Capability = "binary-format"
	CapCrossEngineSource       Capability = "cross-engine-source"
	CapCrossEngineTarget       Capability = "cross-engine-target"
	CapCDC                     Capability = "cdc"
	CapNativeBackpressure      Capability = "native-backpressure"
	CapCrossVersionIncremental Capability = "cross-version-incremental"
)

// RequireCapability resolves profileName to its driver and checks that the
// driver supports requiredCap. It returns nil when supported, or a CodeUser
// errs.Error wrapping ErrDriverUnsupported (with an actionable hint) when not.
//
// Callers in the presentation layer (CLI/TUI) use this for verb pre-flight so
// an unsupported affordance fails fast with a clear message rather than
// crashing partway through. It takes a profile name (not a driver.Driver) so
// the CLI never has to import the driver package (depguard boundary).
func RequireCapability(d Deps, profileName string, requiredCap Capability) error {
	resolved, err := d.Profiles.Resolve(profileName)
	if err != nil {
		return err
	}
	drv, err := d.Drivers.Get(resolved.Driver)
	if err != nil {
		return err
	}
	if driverSupports(drv, requiredCap) {
		return nil
	}
	return &errs.Error{
		Op:    "capgate." + string(requiredCap),
		Code:  errs.CodeUser,
		Cause: errs.ErrDriverUnsupported,
		Hint:  drv.Name() + " driver does not support " + string(requiredCap),
	}
}

// driverSupports maps a Capability to the corresponding driver.Capabilities flag.
//
// Keep in sync with driver.Capabilities: a new field there needs a matching
// Cap* constant above and a case here. A missing case falls through to false
// (fail-safe: the gate blocks rather than silently permitting an unsupported op).
func driverSupports(drv driver.Driver, requiredCap Capability) bool {
	c := drv.Capabilities()
	switch requiredCap {
	case CapIncremental:
		return c.Incremental
	case CapNativeStream:
		return c.NativeStream
	case CapPerTable:
		return c.PerTable
	case CapSchemaOnly:
		return c.SchemaOnly
	case CapDataOnly:
		return c.DataOnly
	case CapParallel:
		return c.Parallel
	case CapCompression:
		return c.Compression
	case CapBinaryFormat:
		return c.BinaryFormat
	case CapCrossEngineSource:
		return c.CrossEngineSource
	case CapCrossEngineTarget:
		return c.CrossEngineTarget
	case CapCDC:
		return c.CDC
	case CapNativeBackpressure:
		return c.NativeBackpressure
	case CapCrossVersionIncremental:
		return c.CrossVersionIncremental
	}
	return false
}
