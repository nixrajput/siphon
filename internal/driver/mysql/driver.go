// Package mysql implements siphon's MySQL driver. It is a thin wrapper over the
// shared mysqlcommon implementation; only the binary names and the honestly-
// declared Capabilities differ from the MariaDB driver.
package mysql

import (
	"context"

	"github.com/nixrajput/siphon/internal/driver"
	mysqlcommon "github.com/nixrajput/siphon/internal/driver/_mysqlcommon"
)

func init() { driver.Register(&Driver{}) }

type Driver struct{}

func (Driver) Name() string { return "mysql" }

func (Driver) Capabilities() driver.Capabilities {
	return driver.Capabilities{
		Incremental:        true, // binlog (mysqlbinlog/mariadb-binlog)
		NativeStream:       true,
		PerTable:           true,
		SchemaOnly:         true,
		DataOnly:           true,
		Parallel:           false, // mysqldump is single-threaded
		Compression:        true,
		BinaryFormat:       false, // SQL text dump, not a binary archive
		CrossEngineSource:  false, // Phase F
		CrossEngineTarget:  false, // Phase F
		CDC:                false, // Phase F
		NativeBackpressure: true,
		// CrossVersionIncremental defaults false
	}
}

func (Driver) Connect(ctx context.Context, p driver.Profile) (driver.Conn, error) {
	return mysqlcommon.NewConn(ctx, p, "mysqldump", "mysql", "mysqlbinlog", "mysql.connect")
}

// Compile-time check.
var _ driver.Driver = Driver{}
