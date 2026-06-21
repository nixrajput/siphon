// Package mysqlcommon holds the shared implementation between the MySQL and
// MariaDB drivers (forks with near-identical tooling). The underscore-prefixed
// directory keeps it out of "go build ./..." while remaining importable by the
// sibling driver packages.
package mysqlcommon

import (
	"fmt"

	gomysql "github.com/go-sql-driver/mysql"

	"github.com/nixrajput/siphon/internal/driver"
)

// DSN builds a go-sql-driver/mysql connection string via mysql.Config.FormatDSN,
// the driver's own canonical builder, rather than hand-formatting. FormatDSN
// path-escapes the DBName and round-trips with the driver's ParseDSN, so we
// stay aligned with whatever the library considers valid. (Note: it does not
// percent-encode the user/password — those still rely on positional parsing —
// so a ':' in the username remains unsupported; MySQL usernames don't contain
// one in practice.)
func DSN(p driver.Profile) string {
	cfg := gomysql.NewConfig()
	cfg.User = p.User
	cfg.Passwd = p.Password
	cfg.Net = "tcp"
	cfg.Addr = fmt.Sprintf("%s:%d", p.Host, p.Port)
	cfg.DBName = p.Database
	cfg.ParseTime = true
	cfg.TLSConfig = tlsParam(p.SSLMode)
	return cfg.FormatDSN()
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
