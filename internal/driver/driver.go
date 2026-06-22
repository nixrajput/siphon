// Package driver defines the contract every database adapter implements,
// and a process-wide registry for looking them up by name.
package driver

import (
	"context"
	"io"
	"time"

	"github.com/nixrajput/siphon/internal/canonical"
)

// Driver is the protocol-level abstraction for a database engine.
// Every adapter (postgres, mysql, mariadb, ...) implements Driver.
//
// Drivers are stateless. Connection state lives in Conn, scoped per-verb.
type Driver interface {
	Name() string
	Capabilities() Capabilities
	Connect(ctx context.Context, p Profile) (Conn, error)
}

// Conn is an open connection plus the verbs that operate on it.
// Conn is the unit of cancellation: ctx propagates to spawned subprocesses.
type Conn interface {
	Inspect(ctx context.Context) (*Schema, error)
	Backup(ctx context.Context, opt BackupOpts, w io.Writer) error
	Restore(ctx context.Context, opt RestoreOpts, r io.Reader) error
	Verify(ctx context.Context, r io.Reader) (*VerifyReport, error)
	Close() error
}

// SchemaInspector is an optional interface a Conn may implement to expose typed
// schema information for cross-engine transfers.
type SchemaInspector interface {
	InspectSchema(ctx context.Context) (*canonical.CanonicalSchema, error)
}

// CanonicalTransfer is an optional interface a Conn may implement to support
// cross-engine data transfer via the canonical JSONL format.
type CanonicalTransfer interface {
	EmitCanonical(ctx context.Context, schema *canonical.CanonicalSchema, w io.Writer) error
	ConsumeCanonical(ctx context.Context, r io.Reader) error
	ApplyChange(ctx context.Context, ch canonical.CanonicalChange) error
}

// ChangeStreamer is an optional Conn capability: stream logical row changes as
// engine-neutral CanonicalChanges, starting after `from`. Bounded callers cancel
// ctx at a target end position; unbounded (CDC) callers stream until ctx cancel.
// Returns the final Position reached (for envelope stamping / CDC state persistence).
type ChangeStreamer interface {
	StreamChanges(ctx context.Context, from canonical.Position, emit func(canonical.CanonicalChange) error) (canonical.Position, error)
}

// Capabilities describes what an engine supports. Each flag gates a UI
// affordance or feature path. Drivers must declare honestly.
type Capabilities struct {
	Incremental             bool
	NativeStream            bool
	PerTable                bool
	SchemaOnly              bool
	DataOnly                bool
	Parallel                bool
	Compression             bool
	BinaryFormat            bool
	CrossEngineSource       bool
	CrossEngineTarget       bool
	CDC                     bool
	NativeBackpressure      bool
	CrossVersionIncremental bool
}

// Profile is the connection descriptor passed to Connect. Secrets are
// resolved before this struct reaches Connect; never wire a SecretRef here.
type Profile struct {
	Name     string
	Driver   string
	Host     string
	Port     int
	User     string
	Password string
	Database string
	SSLMode  string
}

// Schema is the result of Conn.Inspect.
type Schema struct {
	Tables []TableMeta
}

type TableMeta struct {
	Name      string
	Rows      int64
	SizeBytes int64
}

// BackupOpts configures Conn.Backup.
type BackupOpts struct {
	IncludeTables    []string
	ExcludeTables    []string
	ExcludeDataFrom  []string
	SchemaOnly       bool
	DataOnly         bool
	CompressionLevel int
	Parallel         int
}

// RestoreOpts configures Conn.Restore.
type RestoreOpts struct {
	TargetTables []string
	DataOnly     bool
	SchemaOnly   bool
	Clean        bool
}

// VerifyReport is the result of Conn.Verify.
type VerifyReport struct {
	Checksum string
	OK       bool
	Started  time.Time
	Finished time.Time
}
