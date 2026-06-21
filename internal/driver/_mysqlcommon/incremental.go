package mysqlcommon

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/nixrajput/siphon/internal/driver"
	"github.com/nixrajput/siphon/internal/errs"
)

// BinlogPosition identifies the binlog file + offset where incremental events
// begin. Captured at base-backup time and stored in the dump Envelope.
type BinlogPosition struct {
	File     string // e.g. "mysql-bin.000123"
	Position uint64
}

// CaptureBinlogPosition records the current binlog coordinates. Tries the
// MySQL 8.4+ statement first, then the pre-8.4 form, so it works across
// versions and both forks.
func (c *Conn) CaptureBinlogPosition(ctx context.Context) (BinlogPosition, error) {
	// SHOW BINARY LOG STATUS (MySQL 8.4+) and SHOW MASTER STATUS (older MySQL /
	// MariaDB) return the same first two columns (File, Position) but differ in
	// trailing columns, so scan into a flexible set. We read File+Position via a
	// columns-agnostic approach: fetch all columns, take the first two.
	for _, q := range []string{"SHOW BINARY LOG STATUS", "SHOW MASTER STATUS"} {
		pos, ok := tryBinlogStatus(ctx, c.db, q)
		if ok {
			return pos, nil
		}
	}
	return BinlogPosition{}, &errs.Error{
		Op:    "mysql.binlog_position",
		Code:  errs.CodeSystem,
		Cause: errors.New("could not read binlog position"),
		Hint:  "ensure log_bin = ON and binlog_format = ROW on the source",
	}
}

// tryBinlogStatus runs a SHOW ... STATUS query and extracts File+Position from
// the first two columns, tolerating the differing trailing-column counts across
// MySQL versions and MariaDB by scanning into []sql.RawBytes sized to the
// actual column count.
func tryBinlogStatus(ctx context.Context, db *sql.DB, query string) (BinlogPosition, bool) {
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return BinlogPosition{}, false
	}
	defer func() { _ = rows.Close() }()
	cols, err := rows.Columns()
	if err != nil || len(cols) < 2 {
		return BinlogPosition{}, false
	}
	if !rows.Next() {
		return BinlogPosition{}, false
	}
	raw := make([]sql.RawBytes, len(cols))
	scanArgs := make([]any, len(cols))
	for i := range raw {
		scanArgs[i] = &raw[i]
	}
	if err := rows.Scan(scanArgs...); err != nil {
		return BinlogPosition{}, false
	}
	file := string(raw[0])
	pos, err := strconv.ParseUint(string(raw[1]), 10, 64)
	if err != nil || file == "" {
		return BinlogPosition{}, false
	}
	return BinlogPosition{File: file, Position: pos}, true
}

// ValidateBinlogFormat returns a CodeUser error if binlog_format != ROW.
func (c *Conn) ValidateBinlogFormat(ctx context.Context) error {
	var format string
	if err := c.db.QueryRowContext(ctx, "SELECT @@binlog_format").Scan(&format); err != nil {
		return &errs.Error{Op: "mysql.incremental", Code: errs.CodeSystem, Cause: err}
	}
	if !strings.EqualFold(format, "ROW") {
		return &errs.Error{
			Op:    "mysql.incremental",
			Code:  errs.CodeUser,
			Cause: errors.New("binlog_format is " + format),
			Hint:  "set binlog_format = ROW on the source and restart the server",
		}
	}
	return nil
}

// binlogArgs builds the argument vector for the fork's binlog tool
// (mysqlbinlog / mariadb-binlog). The password is passed via MYSQL_PWD in the
// environment (see BackupIncremental), never on the command line. The starting
// binlog file is the final positional arg; --to-last-log continues through all
// subsequent rotated files to the current end of the binlog.
func binlogArgs(p driver.Profile, since BinlogPosition) []string {
	return []string{
		"-h", p.Host,
		"-P", strconv.Itoa(p.Port),
		"-u", p.User,
		"--read-from-remote-server",
		"--to-last-log",
		"--start-position=" + strconv.FormatUint(since.Position, 10),
		since.File,
	}
}

// BackupIncremental streams binlog events from `since` to current end-of-binlog
// into w, using the fork's binlog tool (mysqlbinlog / mariadb-binlog).
//
// NOTE: the --read-from-remote-server + --start-position invocation is
// structurally complete but UNPROVEN locally (no Docker/MySQL here). The exact
// remote-auth flags and whether a single starting binlog file suffices vs.
// needing --to-last-log need validation against a live log_bin=ON,
// binlog_format=ROW server (see CI / the incremental wiring task).
func (c *Conn) BackupIncremental(ctx context.Context, since BinlogPosition, w io.Writer) error {
	cmd := exec.CommandContext(ctx, c.binlogBinary, binlogArgs(c.p, since)...)
	cmd.Env = withMySQLPwd(os.Environ(), c.p.Password)
	cmd.Stdout = w
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return &errs.Error{Op: c.binlogBinary + ".backup_incremental", Code: errs.CodeSystem, Cause: err}
	}
	return nil
}
