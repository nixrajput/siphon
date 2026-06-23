package mysqlcommon

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/nixrajput/siphon/internal/canonical"
	"github.com/nixrajput/siphon/internal/driver"
	"github.com/nixrajput/siphon/internal/errs"
)

var _ driver.ChangeStreamer = (*Conn)(nil)

// StreamChanges streams binlog ROW events from `from` as engine-neutral
// CanonicalChanges, decoding the fork's binlog tool's --verbose pseudo-SQL
// (### INSERT/UPDATE/DELETE … ### @N=…). Bounded callers cancel ctx at a target
// position; unbounded callers stream until ctx cancel. ctx cancellation is the
// normal stop signal and is NOT reported as an error.
//
// Column positions (@1,@2,…) are mapped to names via information_schema (cached
// per table); the key is the table's primary-key columns. UPDATE events carry a
// WHERE (old image) block for the key and a SET (new image) block for Values.
//
// NOTE: this parser is structurally complete but UNPROVEN locally (no MySQL
// here). The pseudo-SQL grammar is stable across MySQL/MariaDB --verbose output,
// but value typing (everything decodes as a string) and edge cases (multi-row
// events, NULL rendering, quoting) need validation against a live
// log_bin=ON, binlog_format=ROW server in CI.
func (c *Conn) StreamChanges(ctx context.Context, from canonical.Position, emit func(canonical.CanonicalChange) error) (canonical.Position, error) {
	return c.streamChangesWithStop(ctx, from, nil, emit)
}

// streamChangesWithStop is the shared binlog decode driver behind StreamChanges
// (stopAt==nil, unbounded) and BackupIncremental (stopAt!=nil, bounded). When
// stopAt is non-nil, parsing returns cleanly at the first row event whose binlog
// position reaches or passes stopAt — every change up to that point has been
// emitted, and none past it.
func (c *Conn) streamChangesWithStop(ctx context.Context, from canonical.Position, stopAt *BinlogPosition, emit func(canonical.CanonicalChange) error) (canonical.Position, error) {
	if err := c.ValidateBinlogFormat(ctx); err != nil {
		return canonical.Position{}, err
	}
	if from.BinlogFile == "" {
		pos, err := c.CaptureBinlogPosition(ctx)
		if err != nil {
			return canonical.Position{}, err
		}
		from = canonical.Position{BinlogFile: pos.File, BinlogPos: pos.Position}
	}

	meta := newTableMetaCache(ctx, c)

	since := BinlogPosition{File: from.BinlogFile, Position: from.BinlogPos}
	args := append(binlogArgs(c.p, since, c.binlogBinary),
		"--verbose", "--base64-output=DECODE-ROWS")
	// binlogArgs places the starting binlog file last (positional); the flags
	// above must precede it, so rebuild with the file kept last.
	args = reorderBinlogFileLast(args, since.File)

	cmd := exec.CommandContext(ctx, c.binlogBinary, args...)
	cmd.Env = withMySQLPwd(os.Environ(), c.p.Password)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return canonical.Position{}, &errs.Error{Op: c.binlogBinary + ".stream", Code: errs.CodeSystem, Cause: err}
	}
	if err := cmd.Start(); err != nil {
		return canonical.Position{}, toolErr(c.binlogBinary, c.binlogBinary+".stream", err)
	}

	endPos, parseErr := parseBinlogRows(stdout, meta, emit, since, stopAt)

	// Did we stop on purpose? Either ctx was cancelled (CDC clean stop) or a
	// bounded scan reached its target position. In both cases killing the
	// (possibly still-running) tool is expected and its exec error is noise.
	reachedStop := stopAt != nil && endPos.File == stopAt.File && endPos.Position >= stopAt.Position
	intentionalStop := ctx.Err() != nil || reachedStop
	if intentionalStop {
		_ = cmd.Process.Kill()
	}
	waitErr := cmd.Wait()

	final := canonical.Position{BinlogFile: endPos.File, BinlogPos: endPos.Position}

	// A parse error (other than from a deliberate ctx cancel) is the primary
	// failure and takes precedence.
	if parseErr != nil && ctx.Err() == nil {
		return final, parseErr
	}
	// The tool exited on its own with a non-zero status (bad flags, auth/binlog
	// permission denied, missing file): without this, an early EOF would look
	// like a clean stream with zero changes. Surface it with captured stderr.
	if waitErr != nil && !intentionalStop {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			waitErr = errors.New(waitErr.Error() + ": " + msg)
		}
		return final, toolErr(c.binlogBinary, c.binlogBinary+".stream", waitErr)
	}
	// A bounded scan must actually reach its target; ending before stopAt (e.g.
	// the tool exited at EOF first) means the incremental is incomplete.
	if stopAt != nil && !reachedStop && ctx.Err() == nil {
		return final, &errs.Error{
			Op:    c.binlogBinary + ".stream",
			Code:  errs.CodeSystem,
			Cause: errors.New("binlog stream ended before reaching the requested stop position"),
		}
	}
	return final, nil
}

// reorderBinlogFileLast removes any occurrence of the positional binlog file
// from args and re-appends it at the end, so flags appended after binlogArgs
// (which already placed the file last) stay before the positional argument.
func reorderBinlogFileLast(args []string, file string) []string {
	out := make([]string, 0, len(args))
	for _, a := range args {
		if a == file {
			continue
		}
		out = append(out, a)
	}
	return append(out, file)
}

// colVal is a parsed column assignment value. isNull distinguishes a SQL NULL
// (mysqlbinlog renders it as the bare token NULL) from the string literal
// "NULL" (rendered quoted as 'NULL') — conflating them, as a plain string would,
// turns a legitimate 'NULL' string into a nil value and corrupts data.
type colVal struct {
	str    string
	isNull bool
}

// rowEvent is the in-progress decode of a single ### INSERT/UPDATE/DELETE block.
type rowEvent struct {
	op      canonical.ChangeOp
	table   string // unqualified table name
	whereM  map[int]colVal
	setM    map[int]colVal
	section string // "where" or "set", controls which map @N= lines fill
}

// parseBinlogRows scans the binlog tool's --verbose output line by line,
// assembling ### blocks into CanonicalChanges and calling emit per change. It
// returns the final binlog position seen (from `# at <pos>` comments) so the
// caller can stamp the envelope / CDC state.
func parseBinlogRows(r io.Reader, meta *tableMetaCache, emit func(canonical.CanonicalChange) error, start BinlogPosition, stopAt *BinlogPosition) (BinlogPosition, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	pos := start
	var ev *rowEvent

	flush := func() error {
		if ev == nil {
			return nil
		}
		ch, err := meta.toChange(ev)
		ev = nil
		if err != nil {
			return err
		}
		if ch == nil {
			return nil
		}
		return emit(*ch)
	}

	for sc.Scan() {
		line := sc.Text()

		// Track the current binlog offset from "# at <pos>" markers.
		if p, ok := parseAtMarker(line); ok {
			pos.Position = p
			// Bounded (incremental) stop: an "# at" marker precedes each event, so
			// once the marker reaches/passes the captured end position in the end
			// file, every earlier event has been parsed. Flush the pending event
			// and return at this clean boundary.
			if stopAt != nil && pos.File == stopAt.File && pos.Position >= stopAt.Position {
				if err := flush(); err != nil {
					return pos, err
				}
				return pos, nil
			}
			continue
		}
		// Track the active binlog file from rotate events.
		if f, ok := parseRotateFile(line); ok {
			pos.File = f
			continue
		}

		t := strings.TrimSpace(line)
		if !strings.HasPrefix(t, "###") {
			continue
		}
		body := strings.TrimSpace(strings.TrimPrefix(t, "###"))

		switch {
		case strings.HasPrefix(body, "INSERT INTO "):
			if err := flush(); err != nil {
				return pos, err
			}
			ev = &rowEvent{op: canonical.OpInsert, table: tableFromRef(body[len("INSERT INTO "):]), setM: map[int]colVal{}, section: "set"}
		case strings.HasPrefix(body, "DELETE FROM "):
			if err := flush(); err != nil {
				return pos, err
			}
			ev = &rowEvent{op: canonical.OpDelete, table: tableFromRef(body[len("DELETE FROM "):]), whereM: map[int]colVal{}, section: "where"}
		case strings.HasPrefix(body, "UPDATE "):
			if err := flush(); err != nil {
				return pos, err
			}
			ev = &rowEvent{op: canonical.OpUpdate, table: tableFromRef(body[len("UPDATE "):]), whereM: map[int]colVal{}, setM: map[int]colVal{}}
		case body == "WHERE":
			if ev != nil {
				ev.section = "where"
				if ev.whereM == nil {
					ev.whereM = map[int]colVal{}
				}
			}
		case body == "SET":
			if ev != nil {
				ev.section = "set"
				if ev.setM == nil {
					ev.setM = map[int]colVal{}
				}
			}
		case strings.HasPrefix(body, "@"):
			if ev != nil {
				if n, val, ok := parseColAssign(body); ok {
					if ev.section == "where" {
						ev.whereM[n] = val
					} else {
						ev.setM[n] = val
					}
				}
			}
		}
	}
	if err := flush(); err != nil {
		return pos, err
	}
	if err := sc.Err(); err != nil {
		return pos, &errs.Error{Op: "mysql.stream", Code: errs.CodeSystem, Cause: err}
	}
	return pos, nil
}

// parseAtMarker extracts the offset from a "# at 12345" binlog comment.
func parseAtMarker(line string) (uint64, bool) {
	t := strings.TrimSpace(line)
	const prefix = "# at "
	if !strings.HasPrefix(t, prefix) {
		return 0, false
	}
	n, err := strconv.ParseUint(strings.TrimSpace(t[len(prefix):]), 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

// parseRotateFile extracts the next binlog file name from a Rotate-event
// comment, e.g. "#... Rotate to mysql-bin.000124  pos: 4".
func parseRotateFile(line string) (string, bool) {
	i := strings.Index(line, "Rotate to ")
	if i < 0 {
		return "", false
	}
	rest := strings.TrimSpace(line[i+len("Rotate to "):])
	if j := strings.Index(rest, " "); j >= 0 {
		rest = rest[:j]
	}
	rest = strings.TrimSuffix(strings.TrimSpace(rest), ";")
	if rest == "" {
		return "", false
	}
	return rest, true
}

// tableFromRef extracts the unqualified table name from a "`db`.`tbl`" or
// "db.tbl" reference, stripping backticks and any trailing tokens.
func tableFromRef(ref string) string {
	ref = strings.TrimSpace(ref)
	if i := strings.IndexAny(ref, " \t"); i >= 0 {
		ref = ref[:i]
	}
	if i := strings.LastIndex(ref, "."); i >= 0 {
		ref = ref[i+1:]
	}
	return strings.Trim(ref, "`")
}

// parseColAssign parses a "@3=value" assignment into its 1-based column index
// and typed value. A bare NULL token is reported as a SQL NULL (isNull=true);
// a quoted value (including 'NULL') is unquoted and reported as a string. The
// mysqlbinlog "/* ... */" type comment is stripped only when it appears OUTSIDE
// a quoted string, so a value like 'a /* b' keeps its literal content.
func parseColAssign(body string) (int, colVal, bool) {
	eq := strings.IndexByte(body, '=')
	if eq < 0 || !strings.HasPrefix(body, "@") {
		return 0, colVal{}, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(body[1:eq]))
	if err != nil {
		return 0, colVal{}, false
	}
	val := strings.TrimSpace(body[eq+1:])
	val = stripTrailingTypeComment(val)
	if val == "NULL" { // bare token, not quoted: a real SQL NULL
		return n, colVal{isNull: true}, true
	}
	return n, colVal{str: unquoteBinlogValue(val)}, true
}

// stripTrailingTypeComment removes a trailing mysqlbinlog "/* ... */" type
// annotation, but only when the "/*" falls outside any single-quoted string.
// Scanning quote-aware prevents truncating a quoted value that itself contains
// the characters "/*" (e.g. 'a /* b').
func stripTrailingTypeComment(v string) string {
	inQuote := false
	for i := 0; i+1 < len(v); i++ {
		switch v[i] {
		case '\\': // skip the escaped next byte inside a quoted string
			if inQuote {
				i++
			}
		case '\'':
			inQuote = !inQuote
		case '/':
			if !inQuote && v[i+1] == '*' {
				return strings.TrimSpace(v[:i])
			}
		}
	}
	return v
}

// unquoteBinlogValue strips the surrounding single quotes mysqlbinlog adds to
// string values and unescapes the doubled/backslash escapes it emits.
func unquoteBinlogValue(v string) string {
	if len(v) >= 2 && v[0] == '\'' && v[len(v)-1] == '\'' {
		inner := v[1 : len(v)-1]
		inner = strings.ReplaceAll(inner, `\'`, `'`)
		inner = strings.ReplaceAll(inner, `''`, `'`)
		inner = strings.ReplaceAll(inner, `\\`, `\`)
		return inner
	}
	return v
}

// tableMetaCache resolves @N column positions to names and identifies primary
// keys, caching the per-table layout (the binlog stream repeats events for the
// same tables, so one information_schema lookup per table suffices).
type tableMetaCache struct {
	ctx   context.Context
	conn  *Conn
	cache map[string]*tableLayout
}

type tableLayout struct {
	cols []string        // ordered column names; index 0 == @1
	pk   map[string]bool // primary-key column names
}

func newTableMetaCache(ctx context.Context, c *Conn) *tableMetaCache {
	return &tableMetaCache{ctx: ctx, conn: c, cache: map[string]*tableLayout{}}
}

// layout returns the cached (or freshly queried) column order + PK set for a
// table in the connection's database.
func (m *tableMetaCache) layout(table string) (*tableLayout, error) {
	if l, ok := m.cache[table]; ok {
		return l, nil
	}
	const colQ = `SELECT COLUMN_NAME FROM information_schema.COLUMNS
WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ? ORDER BY ORDINAL_POSITION`
	rows, err := m.conn.db.QueryContext(m.ctx, colQ, m.conn.p.Database, table)
	if err != nil {
		return nil, &errs.Error{Op: "mysql.stream", Code: errs.CodeSystem, Cause: err}
	}
	var cols []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			_ = rows.Close()
			return nil, &errs.Error{Op: "mysql.stream", Code: errs.CodeSystem, Cause: err}
		}
		cols = append(cols, name)
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return nil, &errs.Error{Op: "mysql.stream", Code: errs.CodeSystem, Cause: err}
	}

	const pkQ = `SELECT COLUMN_NAME FROM information_schema.KEY_COLUMN_USAGE
WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ? AND CONSTRAINT_NAME = 'PRIMARY'`
	pkRows, err := m.conn.db.QueryContext(m.ctx, pkQ, m.conn.p.Database, table)
	if err != nil {
		return nil, &errs.Error{Op: "mysql.stream", Code: errs.CodeSystem, Cause: err}
	}
	pk := map[string]bool{}
	for pkRows.Next() {
		var name string
		if err := pkRows.Scan(&name); err != nil {
			_ = pkRows.Close()
			return nil, &errs.Error{Op: "mysql.stream", Code: errs.CodeSystem, Cause: err}
		}
		pk[name] = true
	}
	_ = pkRows.Close()
	if err := pkRows.Err(); err != nil {
		return nil, &errs.Error{Op: "mysql.stream", Code: errs.CodeSystem, Cause: err}
	}

	l := &tableLayout{cols: cols, pk: pk}
	m.cache[table] = l
	return l, nil
}

// toChange converts a fully-parsed rowEvent into a CanonicalChange, mapping
// @N positions to column names and selecting key columns from the PK. INSERT
// uses the SET image for both Values and Key; UPDATE uses WHERE (old) for the
// key and SET (new) for Values; DELETE uses WHERE (old) for the key.
func (m *tableMetaCache) toChange(ev *rowEvent) (*canonical.CanonicalChange, error) {
	l, err := m.layout(ev.table)
	if err != nil {
		return nil, err
	}
	nameOf := func(idx1 int) (string, bool) { // 1-based @N → name
		if idx1 < 1 || idx1 > len(l.cols) {
			return "", false
		}
		return l.cols[idx1-1], true
	}
	toMap := func(byPos map[int]colVal) map[string]any {
		if byPos == nil {
			return nil
		}
		out := make(map[string]any, len(byPos))
		for n, v := range byPos {
			if name, ok := nameOf(n); ok {
				if v.isNull {
					out[name] = nil
				} else {
					out[name] = v.str
				}
			}
		}
		return out
	}
	keyFrom := func(vals map[string]any) map[string]any {
		key := map[string]any{}
		for name := range vals {
			if l.pk[name] {
				key[name] = vals[name]
			}
		}
		if len(key) == 0 { // no PK: fall back to the full row as the key
			for k, v := range vals {
				key[k] = v
			}
		}
		return key
	}

	switch ev.op {
	case canonical.OpInsert:
		vals := toMap(ev.setM)
		return &canonical.CanonicalChange{Op: canonical.OpInsert, Table: ev.table, Key: keyFrom(vals), Values: vals}, nil
	case canonical.OpUpdate:
		vals := toMap(ev.setM)
		old := toMap(ev.whereM)
		return &canonical.CanonicalChange{Op: canonical.OpUpdate, Table: ev.table, Key: keyFrom(old), Values: vals}, nil
	case canonical.OpDelete:
		old := toMap(ev.whereM)
		return &canonical.CanonicalChange{Op: canonical.OpDelete, Table: ev.table, Key: keyFrom(old)}, nil
	default:
		return nil, &errs.Error{Op: "mysql.stream", Code: errs.CodeSystem, Cause: errors.New("unknown row op")}
	}
}

