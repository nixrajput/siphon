package mysqlcommon

import (
	"strconv"

	"github.com/nixrajput/siphon/internal/driver"
)

// BuildDumpArgs assembles the mysqldump argument vector for a backup. The
// binary name itself is supplied by the caller (mysqldump vs mariadb-dump).
func BuildDumpArgs(p driver.Profile, opt driver.BackupOpts) []string {
	args := []string{
		"-h", p.Host,
		"-P", strconv.Itoa(p.Port),
		"-u", p.User,
		"--ssl-mode=" + cliSSLMode(p.SSLMode),
		"--single-transaction",
		"--routines",
		"--triggers",
		"--events",
		"--no-tablespaces",
		"--skip-comments",
		p.Database,
	}
	if opt.SchemaOnly {
		args = append(args, "--no-data")
	}
	if opt.DataOnly {
		args = append(args, "--no-create-info")
	}
	args = append(args, opt.IncludeTables...)
	for _, t := range opt.ExcludeTables {
		args = append(args, "--ignore-table="+p.Database+"."+t)
	}
	return args
}

// BuildRestoreArgs assembles the mysql client argument vector for a restore.
// The dump file is authoritative for shape; the restore client just pipes it
// in. Clean is a no-op here because mysqldump output already emits
// DROP TABLE IF EXISTS / CREATE TABLE, making the restore idempotent.
func BuildRestoreArgs(p driver.Profile, _ driver.RestoreOpts) []string {
	return []string{
		"-h", p.Host,
		"-P", strconv.Itoa(p.Port),
		"-u", p.User,
		"--ssl-mode=" + cliSSLMode(p.SSLMode),
		"--default-character-set=utf8mb4",
		p.Database,
	}
}

// cliSSLMode maps a profile SSLMode to the mysqldump/mysql client's
// --ssl-mode value, so backup/restore honor the same TLS policy the DSN does
// (otherwise the tools fall back to the client default, PREFERRED). The inputs
// mirror tlsParam in dsn.go.
func cliSSLMode(mode string) string {
	switch mode {
	case "disable":
		return "DISABLED"
	case "require":
		return "REQUIRED"
	case "verify-ca":
		return "VERIFY_CA"
	case "verify-full":
		return "VERIFY_IDENTITY"
	default:
		return "PREFERRED"
	}
}
