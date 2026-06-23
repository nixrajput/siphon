package mysqlcommon

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/nixrajput/siphon/internal/driver"
	"github.com/nixrajput/siphon/internal/errs"
	"github.com/nixrajput/siphon/internal/jobs"

	_ "github.com/go-sql-driver/mysql" // register "mysql" as a database/sql driver
)

// Open returns a database/sql handle for the profile. It does not probe the
// connection; callers ping as needed.
func Open(p driver.Profile) (*sql.DB, error) {
	return sql.Open("mysql", DSN(p))
}

// Conn is the shared MySQL/MariaDB connection. The only per-fork difference is
// the dump/client binary names, injected at construction, so both drivers reuse
// this single implementation of the driver.Conn contract.
type Conn struct {
	db           *sql.DB
	p            driver.Profile
	dumpBinary   string // "mysqldump" or "mariadb-dump"
	clientBinary string // "mysql" or "mariadb"
	binlogBinary string // "mysqlbinlog" or "mariadb-binlog"
	engine       string // "mysql" or "mariadb"
}

var _ driver.Conn = (*Conn)(nil)
var _ driver.SchemaInspector = (*Conn)(nil)
var _ driver.CanonicalTransfer = (*Conn)(nil)

// NewConn opens + pings the database and returns a ready Conn. The ping is
// wrapped in a bounded retry (jobs.Retry, 3 attempts) — same policy as the
// Postgres driver (spec §4.3). connOp is the error-wrapping op label, e.g.
// "mysql.connect" / "mariadb.connect", so errors name the right driver. The
// three binary names (dump/client/binlog) are the only per-fork difference.
func NewConn(ctx context.Context, p driver.Profile, dumpBinary, clientBinary, binlogBinary, connOp, engine string) (*Conn, error) {
	db, err := Open(p)
	if err != nil {
		return nil, connErr(connOp, err)
	}
	if err := jobs.Retry(ctx, 3, func() error { return db.PingContext(ctx) }); err != nil {
		_ = db.Close()
		return nil, connErr(connOp, err)
	}
	return &Conn{db: db, p: p, dumpBinary: dumpBinary, clientBinary: clientBinary, binlogBinary: binlogBinary, engine: engine}, nil
}

// connErr wraps a connection failure in the shape the harness asserts on
// (errors.Is(err, errs.ErrConnectionFailed)), matching the postgres driver.
func connErr(op string, err error) error {
	return &errs.Error{
		Op:    op,
		Code:  errs.CodeSystem,
		Cause: errs.ErrConnectionFailed,
		Hint:  err.Error(),
	}
}

// Close releases the underlying connection pool.
func (c *Conn) Close() error { return c.db.Close() }

// inspectQuery lists tables in the target schema with row estimates and
// on-disk size (data + index). TABLE_ROWS is an estimate for InnoDB and NULL
// for some engines/views; IFNULL clamps the nullable columns to 0.
const inspectQuery = `
SELECT TABLE_NAME,
       IFNULL(TABLE_ROWS, 0),
       IFNULL(DATA_LENGTH + INDEX_LENGTH, 0)
FROM information_schema.tables
WHERE TABLE_SCHEMA = ?
ORDER BY (DATA_LENGTH + INDEX_LENGTH) DESC
`

func (c *Conn) Inspect(ctx context.Context) (*driver.Schema, error) {
	rows, err := c.db.QueryContext(ctx, inspectQuery, c.p.Database)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := &driver.Schema{}
	for rows.Next() {
		var t driver.TableMeta
		if err := rows.Scan(&t.Name, &t.Rows, &t.SizeBytes); err != nil {
			return nil, err
		}
		out.Tables = append(out.Tables, t)
	}
	return out, rows.Err()
}

// Backup streams a dump of the database to w via the fork's dump binary.
func (c *Conn) Backup(ctx context.Context, opt driver.BackupOpts, w io.Writer) error {
	return BackupWith(ctx, c.dumpBinary, c.p, opt, w)
}

// Restore pipes r into the fork's client binary.
func (c *Conn) Restore(ctx context.Context, opt driver.RestoreOpts, r io.Reader) error {
	return RestoreWith(ctx, c.clientBinary, c.p, opt, r)
}

// Verify performs a checksum-only check on the dump stream. Header-format
// checks land in Phase F when the siphon envelope exists.
func (c *Conn) Verify(_ context.Context, r io.Reader) (*driver.VerifyReport, error) {
	started := time.Now()
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return nil, err
	}
	return &driver.VerifyReport{
		Checksum: "sha256:" + hex.EncodeToString(h.Sum(nil)),
		OK:       true,
		Started:  started,
		Finished: time.Now(),
	}, nil
}

// BackupWith spawns the given dump binary (mysqldump or mariadb-dump) and
// streams its stdout to w. ctx cancellation propagates via exec.CommandContext.
func BackupWith(ctx context.Context, binary string, p driver.Profile, opt driver.BackupOpts, w io.Writer) error {
	cmd := exec.CommandContext(ctx, binary, BuildDumpArgs(p, opt)...)
	cmd.Env = withMySQLPwd(os.Environ(), p.Password)
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
	cmd.Env = withMySQLPwd(os.Environ(), p.Password)
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

// withMySQLPwd returns base with any pre-existing MYSQL_PWD entry removed and a
// single MYSQL_PWD=pw appended. Without the filter, a MYSQL_PWD already in the
// parent environment would remain alongside ours; the tool's behavior on a
// duplicated key is unspecified, so we guarantee exactly one.
func withMySQLPwd(base []string, pw string) []string {
	out := make([]string, 0, len(base)+1)
	for _, kv := range base {
		if strings.HasPrefix(kv, "MYSQL_PWD=") {
			continue
		}
		out = append(out, kv)
	}
	return append(out, "MYSQL_PWD="+pw)
}
