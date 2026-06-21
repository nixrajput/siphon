package mysqlcommon

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"os"
	"os/exec"

	"github.com/nixrajput/siphon/internal/driver"
	"github.com/nixrajput/siphon/internal/errs"

	_ "github.com/go-sql-driver/mysql" // register "mysql" as a database/sql driver
)

// Open returns a database/sql handle for the profile. It does not probe the
// connection; callers ping as needed.
func Open(p driver.Profile) (*sql.DB, error) {
	return sql.Open("mysql", DSN(p))
}

// BackupWith spawns the given dump binary (mysqldump or mariadb-dump) and
// streams its stdout to w. ctx cancellation propagates via exec.CommandContext.
func BackupWith(ctx context.Context, binary string, p driver.Profile, opt driver.BackupOpts, w io.Writer) error {
	cmd := exec.CommandContext(ctx, binary, BuildDumpArgs(p, opt)...)
	cmd.Env = append(os.Environ(), "MYSQL_PWD="+p.Password)
	cmd.Stdout = w
	// Discard stderr directly. StderrPipe + a drain goroutine would race with
	// cmd.Wait() (Wait closes the pipe once the process exits). Phase F will
	// wire stderr through progress parsing for all drivers; until then,
	// discarding is safe.
	cmd.Stderr = io.Discard

	if err := cmd.Run(); err != nil {
		return toolErr(binary, binary+".backup", err)
	}
	return nil
}

// RestoreWith spawns the given client binary (mysql or mariadb) and pipes r
// into its stdin. ctx cancellation propagates via exec.CommandContext.
func RestoreWith(ctx context.Context, binary string, p driver.Profile, opt driver.RestoreOpts, r io.Reader) error {
	cmd := exec.CommandContext(ctx, binary, BuildRestoreArgs(p, opt)...)
	cmd.Env = append(os.Environ(), "MYSQL_PWD="+p.Password)
	cmd.Stdin = r
	cmd.Stderr = io.Discard // don't leak client diagnostics to the user's terminal; Phase F captures these

	if err := cmd.Run(); err != nil {
		return toolErr(binary, binary+".restore", err)
	}
	return nil
}

// toolErr maps a missing binary to errs.ErrToolMissing and any other failure
// to a plain system error.
func toolErr(binary, op string, err error) *errs.Error {
	var execErr *exec.Error
	if errors.As(err, &execErr) && errors.Is(execErr.Err, exec.ErrNotFound) {
		return &errs.Error{
			Op:    op,
			Code:  errs.CodeSystem,
			Cause: errs.ErrToolMissing,
			Hint:  "install the " + binary + " client",
		}
	}
	return &errs.Error{Op: op, Code: errs.CodeSystem, Cause: err}
}
