package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// EmitCanonical writes a table-by-table snapshot of schema as JSONL to w.
//
// Line 1 is the schema header ({"schema": {...}}); each subsequent line is one
// CanonicalRow ({"t": table, "v": {col: val}}). The engine param names the
// SOURCE engine and is used only to quote identifiers in the SELECT — values
// are never interpolated.
func EmitCanonical(ctx context.Context, db *sql.DB, engine string, schema *CanonicalSchema, w io.Writer) error {
	if err := writeJSONL(w, map[string]*CanonicalSchema{"schema": schema}); err != nil {
		return err
	}
	for _, t := range schema.Tables {
		if err := emitTable(ctx, db, engine, t, w); err != nil {
			return err
		}
	}
	return nil
}

// emitTable streams one table's rows as CanonicalRow JSONL lines.
func emitTable(ctx context.Context, db *sql.DB, engine string, t CanonicalTable, w io.Writer) error {
	query, err := buildSelectSQL(engine, t)
	if err != nil {
		return err
	}

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("cross-engine: select %s: %w", t.Name, err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		vals := make([]any, len(t.Columns))
		ptrs := make([]any, len(t.Columns))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return fmt.Errorf("cross-engine: scan %s: %w", t.Name, err)
		}
		m := make(map[string]any, len(t.Columns))
		for i, c := range t.Columns {
			m[c.Name] = normalizeScanned(vals[i])
		}
		if err := writeJSONL(w, CanonicalRow{Table: t.Name, Values: m}); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("cross-engine: iterate %s: %w", t.Name, err)
	}
	return nil
}

// buildSelectSQL builds `SELECT <quoted cols> FROM <quoted table>` with every
// identifier quoted for the engine. Pure function — unit-testable without a DB.
func buildSelectSQL(engine string, t CanonicalTable) (string, error) {
	qt, err := quoteIdent(engine, t.Name)
	if err != nil {
		return "", err
	}
	cols := make([]string, len(t.Columns))
	for i, c := range t.Columns {
		qc, err := quoteIdent(engine, c.Name)
		if err != nil {
			return "", err
		}
		cols[i] = qc
	}
	return fmt.Sprintf("SELECT %s FROM %s", strings.Join(cols, ", "), qt), nil
}

// normalizeScanned converts a value scanned from database/sql into a form that
// round-trips correctly through JSON. database/sql commonly yields []byte for
// text/numeric/json columns; marshaling []byte to JSON produces base64, which
// corrupts the value when ConsumeCanonical re-inserts it. Converting to string
// makes it marshal as a JSON string instead, preserving the original bytes.
func normalizeScanned(v any) any {
	if b, ok := v.([]byte); ok {
		return string(b)
	}
	return v
}

// writeJSONL marshals v and writes it followed by a newline.
func writeJSONL(w io.Writer, v any) error {
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
