package canonical

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// CanonicalType is the engine-independent column type vocabulary. A source
// engine's native types are normalized into these; a target engine maps them
// back to its own dialect via MapToNative. This is the pivot that lets data
// move between Postgres, MySQL, and MariaDB.
type CanonicalType string

const (
	CTInt         CanonicalType = "int"
	CTBigInt      CanonicalType = "bigint"
	CTText        CanonicalType = "text"
	CTVarchar     CanonicalType = "varchar"
	CTBoolean     CanonicalType = "boolean"
	CTNumeric     CanonicalType = "numeric"
	CTUUID        CanonicalType = "uuid"
	CTTimestampTZ CanonicalType = "timestamptz"
	CTJSON        CanonicalType = "json"
)

// CanonicalColumn describes one column in canonical terms.
//
// Precision/Scale are only meaningful for CTVarchar (Precision = length) and
// CTNumeric (Precision = total digits, Scale = fractional digits). When both
// are zero the type is emitted bare (e.g. VARCHAR, DECIMAL) — acceptable for
// v1; engines apply their own defaults.
type CanonicalColumn struct {
	Name       string        `json:"name"`
	Type       CanonicalType `json:"type"`
	Nullable   bool          `json:"nullable"`
	Precision  int           `json:"precision,omitempty"`
	Scale      int           `json:"scale,omitempty"`
	PrimaryKey bool          `json:"primary_key,omitempty"`
}

// CanonicalTable is a table name plus its ordered columns.
type CanonicalTable struct {
	Name    string            `json:"name"`
	Columns []CanonicalColumn `json:"columns"`
}

// CanonicalSchema is the full set of tables to transfer.
type CanonicalSchema struct {
	Tables []CanonicalTable `json:"tables"`
}

// CanonicalRow is one row of data, keyed by column name. The compact JSON keys
// keep the JSONL stream small across many rows.
type CanonicalRow struct {
	Table  string         `json:"t"`
	Values map[string]any `json:"v"`
}

// ChangeOp is the kind of row change carried by a CanonicalChange.
type ChangeOp string

const (
	OpInsert ChangeOp = "insert"
	OpUpdate ChangeOp = "update"
	OpDelete ChangeOp = "delete"
)

// CanonicalChange is one engine-neutral row change. Key holds the primary-key
// column values (used to target UPDATE/DELETE; also populated for INSERT).
// Values holds the full post-image row for INSERT/UPDATE; it is empty for DELETE.
type CanonicalChange struct {
	Op     ChangeOp       `json:"op"`
	Table  string         `json:"table"`
	Key    map[string]any `json:"key,omitempty"`
	Values map[string]any `json:"values,omitempty"`
}

// Position is an engine-neutral stream cursor: a Postgres LSN, or a MySQL/MariaDB
// binlog file+offset. Serialized into the dump Envelope and the CDC state file.
type Position struct {
	LSN        string `json:"lsn,omitempty"`
	BinlogFile string `json:"binlog_file,omitempty"`
	BinlogPos  uint64 `json:"binlog_pos,omitempty"`
}

// MapToNative renders a canonical column as a native column type for the given
// target engine. Returns an error for an unknown engine or an unmappable type.
//
// VARCHAR/NUMERIC with Precision>0 carry their precision (and Scale for
// NUMERIC) into the rendered type; otherwise the type is emitted bare.
func MapToNative(engine string, col CanonicalColumn) (string, error) {
	var m map[CanonicalType]string
	switch engine {
	case "postgres":
		m = map[CanonicalType]string{
			CTInt:         "integer",
			CTBigInt:      "bigint",
			CTText:        "text",
			CTVarchar:     "varchar",
			CTBoolean:     "boolean",
			CTNumeric:     "numeric",
			CTUUID:        "uuid",
			CTTimestampTZ: "timestamptz",
			CTJSON:        "jsonb",
		}
	case "mysql", "mariadb":
		m = map[CanonicalType]string{
			CTInt:         "INT",
			CTBigInt:      "BIGINT",
			CTText:        "TEXT",
			CTVarchar:     "VARCHAR",
			CTBoolean:     "TINYINT(1)",
			CTNumeric:     "DECIMAL",
			CTUUID:        "CHAR(36)",
			CTTimestampTZ: "TIMESTAMP",
			CTJSON:        "JSON",
		}
	default:
		return "", fmt.Errorf("cross-engine: unknown engine %q", engine)
	}

	native, ok := m[col.Type]
	if !ok {
		return "", fmt.Errorf("cross-engine: engine %q has no mapping for canonical type %q", engine, col.Type)
	}

	// Decorate variable-length / fixed-precision types when precision is known.
	switch col.Type {
	case CTVarchar:
		if col.Precision > 0 {
			native = fmt.Sprintf("%s(%d)", native, col.Precision)
		}
	case CTNumeric:
		if col.Precision > 0 {
			native = fmt.Sprintf("%s(%d,%d)", native, col.Precision, col.Scale)
		}
	}
	return native, nil
}

// QuoteIdent quotes a SQL identifier for the given engine, escaping any quote
// characters inside it. Identifiers (table/column names) CANNOT be passed as
// bound parameters, so quoting+escaping per engine is the mitigation against a
// name that contains a space, keyword, quote, or `;` — whether malicious or
// merely awkward. Every identifier that reaches generated SQL must pass through
// here.
func QuoteIdent(engine, ident string) (string, error) {
	switch engine {
	case "postgres":
		return `"` + strings.ReplaceAll(ident, `"`, `""`) + `"`, nil
	case "mysql", "mariadb":
		return "`" + strings.ReplaceAll(ident, "`", "``") + "`", nil
	default:
		return "", fmt.Errorf("cross-engine: unknown engine %q", engine)
	}
}

// Placeholder returns the bind-parameter placeholder for position n (1-based)
// in the given engine's dialect: $n for Postgres, ? for MySQL/MariaDB.
func Placeholder(engine string, n int) string {
	switch engine {
	case "postgres":
		return "$" + strconv.Itoa(n)
	default:
		return "?"
	}
}

// BuildSelectSQL builds `SELECT <quoted cols> FROM <quoted table>` with every
// identifier quoted for the engine. Pure function — unit-testable without a DB.
func BuildSelectSQL(engine string, t CanonicalTable) (string, error) {
	qt, err := QuoteIdent(engine, t.Name)
	if err != nil {
		return "", err
	}
	cols := make([]string, len(t.Columns))
	for i, c := range t.Columns {
		qc, err := QuoteIdent(engine, c.Name)
		if err != nil {
			return "", err
		}
		cols[i] = qc
	}
	return fmt.Sprintf("SELECT %s FROM %s", strings.Join(cols, ", "), qt), nil
}

// BuildCreateTableSQL renders a CREATE TABLE IF NOT EXISTS statement with every
// identifier quoted for the engine and each column mapped via MapToNative. Pure
// function — unit-testable without a DB.
func BuildCreateTableSQL(engine string, t CanonicalTable) (string, error) {
	qt, err := QuoteIdent(engine, t.Name)
	if err != nil {
		return "", err
	}
	defs := make([]string, len(t.Columns))
	var pkCols []string
	for i, c := range t.Columns {
		qc, err := QuoteIdent(engine, c.Name)
		if err != nil {
			return "", err
		}
		native, err := MapToNative(engine, c)
		if err != nil {
			return "", err
		}
		def := qc + " " + native
		if !c.Nullable {
			def += " NOT NULL"
		}
		defs[i] = def
		if c.PrimaryKey {
			pkCols = append(pkCols, qc)
		}
	}
	if len(pkCols) > 0 {
		defs = append(defs, "PRIMARY KEY ("+strings.Join(pkCols, ", ")+")")
	}
	return fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (%s)", qt, strings.Join(defs, ", ")), nil
}

// BuildUpdateSQL builds an UPDATE statement for the given engine: SET columns
// come first (1-based placeholders), then key columns. Identifiers are always
// quoted via QuoteIdent; values are bound, not interpolated.
func BuildUpdateSQL(engine, table string, setCols, keyCols []string) (string, error) {
	qt, err := QuoteIdent(engine, table)
	if err != nil {
		return "", err
	}
	n := 1
	sets := make([]string, len(setCols))
	for i, c := range setCols {
		qc, err := QuoteIdent(engine, c)
		if err != nil {
			return "", err
		}
		sets[i] = qc + " = " + Placeholder(engine, n)
		n++
	}
	wheres := make([]string, len(keyCols))
	for i, c := range keyCols {
		qc, err := QuoteIdent(engine, c)
		if err != nil {
			return "", err
		}
		wheres[i] = qc + " = " + Placeholder(engine, n)
		n++
	}
	return "UPDATE " + qt + " SET " + strings.Join(sets, ", ") + " WHERE " + strings.Join(wheres, " AND "), nil
}

// BuildDeleteSQL builds a DELETE FROM statement for the given engine with all
// key columns in the WHERE clause. Identifiers are always quoted; values are
// bound (1-based for Postgres, ? for MySQL/MariaDB).
func BuildDeleteSQL(engine, table string, keyCols []string) (string, error) {
	qt, err := QuoteIdent(engine, table)
	if err != nil {
		return "", err
	}
	wheres := make([]string, len(keyCols))
	for i, c := range keyCols {
		qc, err := QuoteIdent(engine, c)
		if err != nil {
			return "", err
		}
		wheres[i] = qc + " = " + Placeholder(engine, i+1)
	}
	return "DELETE FROM " + qt + " WHERE " + strings.Join(wheres, " AND "), nil
}

// BuildInsertSQL renders an INSERT with quoted identifiers and per-engine
// placeholders. Pure function — unit-testable without a DB. Returns an error
// for an empty column set (an INSERT with no columns is malformed).
func BuildInsertSQL(engine, table string, cols []string) (string, error) {
	if len(cols) == 0 {
		return "", fmt.Errorf("cross-engine: insert into %s: no columns", table)
	}
	qt, err := QuoteIdent(engine, table)
	if err != nil {
		return "", err
	}
	qcols := make([]string, len(cols))
	phs := make([]string, len(cols))
	for i, c := range cols {
		qc, err := QuoteIdent(engine, c)
		if err != nil {
			return "", err
		}
		qcols[i] = qc
		phs[i] = Placeholder(engine, i+1)
	}
	return fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		qt, strings.Join(qcols, ","), strings.Join(phs, ",")), nil
}

// WriteJSONL marshals v and writes it followed by a newline.
func WriteJSONL(w io.Writer, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if _, err := w.Write(b); err != nil {
		return err
	}
	_, err = w.Write([]byte("\n"))
	return err
}

// NormalizeScanned converts a value scanned from database/sql into a form that
// round-trips correctly through JSON. database/sql commonly yields []byte for
// text/numeric/json columns; marshaling []byte to JSON produces base64, which
// corrupts the value when ConsumeCanonical re-inserts it. Converting to string
// makes it marshal as a JSON string instead, preserving the original bytes.
func NormalizeScanned(v any) any {
	if b, ok := v.([]byte); ok {
		return string(b)
	}
	return v
}
