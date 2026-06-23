package mysqlcommon

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"strconv"
	"strings"

	"github.com/nixrajput/siphon/internal/canonical"
	"github.com/nixrajput/siphon/internal/driver"
	"github.com/nixrajput/siphon/internal/errs"
)

// BinlogPosition identifies the binlog file + offset where incremental events
// begin. Captured at base-backup time and stored in the dump Envelope.
type BinlogPosition struct {
	File     string // e.g. "mysql-bin.000123"
	Position uint64
}

var _ driver.BasePositioner = (*Conn)(nil)

// CurrentPosition returns the server's current binlog coordinates as a canonical
// Position. app.Backup calls this right after a full backup so the base dump's
// Envelope records where the first incremental should resume from.
func (c *Conn) CurrentPosition(ctx context.Context) (canonical.Position, error) {
	pos, err := c.CaptureBinlogPosition(ctx)
	if err != nil {
		return canonical.Position{}, err
	}
	return canonical.Position{BinlogFile: pos.File, BinlogPos: pos.Position}, nil
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
//
// SSL flags are fork-specific, keyed on binlogBinary: mysqlbinlog takes
// --ssl-mode=<DISABLED|REQUIRED|VERIFY_CA|VERIFY_IDENTITY>, while
// mariadb-binlog takes --ssl / --skip-ssl. The starting binlog file MUST stay
// last (positional), so SSL flags are inserted before it.
func binlogArgs(p driver.Profile, since BinlogPosition, binlogBinary string) []string {
	args := []string{
		"-h", p.Host,
		"-P", strconv.Itoa(p.Port),
		"-u", p.User,
		"--read-from-remote-server",
		"--to-last-log",
		"--start-position=" + strconv.FormatUint(since.Position, 10),
	}
	args = append(args, binlogSSLArgs(p.SSLMode, binlogBinary)...)
	return append(args, since.File)
}

// binlogSSLArgs maps Profile.SSLMode to the fork-specific binlog TLS flags.
// mysqlbinlog uses --ssl-mode=<level>; mariadb-binlog uses --ssl / --skip-ssl.
// An empty/PREFERRED policy omits the flag entirely (the tool's default).
func binlogSSLArgs(sslMode, binlogBinary string) []string {
	switch binlogBinary {
	case "mysqlbinlog":
		var level string
		switch sslMode {
		case "disable":
			level = "DISABLED"
		case "require":
			level = "REQUIRED"
		case "verify-ca":
			level = "VERIFY_CA"
		case "verify-full":
			level = "VERIFY_IDENTITY"
		default:
			level = "PREFERRED"
		}
		return []string{"--ssl-mode=" + level}
	case "mariadb-binlog":
		switch sslMode {
		case "require", "verify-ca", "verify-full":
			return []string{"--ssl"}
		case "disable":
			return []string{"--skip-ssl"}
		default:
			return nil // PREFERRED/empty: omit, use the tool's default
		}
	default:
		return nil
	}
}

var _ driver.IncrementalBackuper = (*Conn)(nil)

// BackupIncremental captures the BOUNDED change set from `since` to the current
// end-of-binlog, serializing each CanonicalChange to w as JSONL, and returns the
// end Position reached.
//
// Bounding mechanism: the end binlog coordinates are captured up front via
// CaptureBinlogPosition and passed to the shared binlog decode loop as a stop
// target. Parsing returns cleanly at the first "# at" marker that reaches or
// passes the captured end offset in the end file, so every event up to it is
// emitted and none past it. This reuses StreamChanges' decode machinery so the
// incremental body is engine-neutral JSONL that the restore path replays via
// ApplyChange (rather than raw binlog bytes).
//
// This path is exercised against a live log_bin=ON, binlog_format=ROW server only
// in CI; it is not validated locally (no MySQL/MariaDB here).
func (c *Conn) BackupIncremental(ctx context.Context, since canonical.Position, w io.Writer) (canonical.Position, error) {
	end, err := c.CaptureBinlogPosition(ctx)
	if err != nil {
		return canonical.Position{}, err
	}
	stopAt := &BinlogPosition{File: end.File, Position: end.Position}

	emit := func(ch canonical.CanonicalChange) error {
		return canonical.WriteJSONL(w, ch)
	}
	return c.streamChangesWithStop(ctx, since, stopAt, emit)
}
