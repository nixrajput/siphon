// Package mariadb implements siphon's MariaDB driver. Same shape as the MySQL
// driver — a thin wrapper over the shared mysqlcommon implementation — but
// shells out to the fork's renamed binaries (mariadb-dump / mariadb).
package mariadb

import (
	"context"

	"github.com/nixrajput/siphon/internal/driver"
	mysqlcommon "github.com/nixrajput/siphon/internal/driver/_mysqlcommon"
)

func init() { driver.Register(&Driver{}) }

type Driver struct{}

func (Driver) Name() string { return "mariadb" }

func (Driver) Capabilities() driver.Capabilities {
	return driver.Capabilities{
		Incremental:        true, // binlog (mysqlbinlog/mariadb-binlog)
		NativeStream:       true,
		PerTable:           true,
		SchemaOnly:         true,
		DataOnly:           true,
		Parallel:           false, // mariadb-dump is single-threaded
		Compression:        true,
		BinaryFormat:       false, // SQL text dump, not a binary archive
		CrossEngineSource:  true,
		CrossEngineTarget:  true,
		CDC:                true,
		NativeBackpressure: true,
		// CrossVersionIncremental defaults false
	}
}

func (Driver) Connect(ctx context.Context, p driver.Profile) (driver.Conn, error) {
	return mysqlcommon.NewConn(ctx, p, "mariadb-dump", "mariadb", "mariadb-binlog", "mariadb.connect", "mariadb")
}

// Compile-time check.
var _ driver.Driver = Driver{}
