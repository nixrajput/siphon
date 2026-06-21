package app

import (
	"fmt"
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
	Name      string        `json:"name"`
	Type      CanonicalType `json:"type"`
	Nullable  bool          `json:"nullable"`
	Precision int           `json:"precision,omitempty"`
	Scale     int           `json:"scale,omitempty"`
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

// quoteIdent quotes a SQL identifier for the given engine, escaping any quote
// characters inside it. Identifiers (table/column names) CANNOT be passed as
// bound parameters, so quoting+escaping per engine is the mitigation against a
// name that contains a space, keyword, quote, or `;` — whether malicious or
// merely awkward. Every identifier that reaches generated SQL must pass through
// here.
func quoteIdent(engine, ident string) (string, error) {
	switch engine {
	case "postgres":
		return `"` + strings.ReplaceAll(ident, `"`, `""`) + `"`, nil
	case "mysql", "mariadb":
		return "`" + strings.ReplaceAll(ident, "`", "``") + "`", nil
	default:
		return "", fmt.Errorf("cross-engine: unknown engine %q", engine)
	}
}

// placeholder returns the bind-parameter placeholder for position n (1-based)
// in the given engine's dialect: $n for Postgres, ? for MySQL/MariaDB.
func placeholder(engine string, n int) string {
	switch engine {
	case "postgres":
		return "$" + strconv.Itoa(n)
	default:
		return "?"
	}
}
