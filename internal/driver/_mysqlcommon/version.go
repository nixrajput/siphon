package mysqlcommon

import (
	"context"
	"database/sql"
	"strings"
)

// Fork identifies which MySQL-family engine a connection is talking to.
type Fork int

const (
	ForkUnknown Fork = iota
	ForkMySQL
	ForkMariaDB
)

// DetectFork queries SELECT VERSION() and classifies the server as MySQL or
// MariaDB. MariaDB embeds "mariadb" in its version string; everything else
// that responds is treated as MySQL. Exported for cross-driver use even where
// callers don't yet branch on the result.
func DetectFork(ctx context.Context, db *sql.DB) (Fork, string, error) {
	var version string
	if err := db.QueryRowContext(ctx, "SELECT VERSION()").Scan(&version); err != nil {
		return ForkUnknown, "", err
	}
	if strings.Contains(strings.ToLower(version), "mariadb") {
		return ForkMariaDB, version, nil
	}
	return ForkMySQL, version, nil
}
