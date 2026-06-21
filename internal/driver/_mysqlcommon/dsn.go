// Package mysqlcommon holds the shared implementation between the MySQL and
// MariaDB drivers (forks with near-identical tooling). The underscore-prefixed
// directory keeps it out of "go build ./..." while remaining importable by the
// sibling driver packages.
package mysqlcommon

import (
	"fmt"

	"github.com/nixrajput/siphon/internal/driver"
)

// DSN builds a go-sql-driver/mysql connection string:
// user:pass@tcp(host:port)/db?parseTime=true&tls=<mode>
//
// This trusts p.User to be free of ':' — the go-sql-driver DSN format splits
// user:pass on the first ':', so passwords need no escaping (the library
// tolerates them), but a ':' in the username would corrupt parsing. MySQL
// usernames don't contain ':' in practice.
func DSN(p driver.Profile) string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true&tls=%s",
		p.User, p.Password, p.Host, p.Port, p.Database, tlsParam(p.SSLMode))
}

func tlsParam(mode string) string {
	switch mode {
	case "require", "verify-ca", "verify-full":
		return "true"
	case "disable":
		return "false"
	default:
		return "preferred"
	}
}
