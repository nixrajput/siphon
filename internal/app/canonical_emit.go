package app

import (
	"context"
	"database/sql"
	"fmt"
	"io"

	"github.com/nixrajput/siphon/internal/canonical"
)

// EmitCanonical writes a table-by-table snapshot of schema as JSONL to w.
//
// Line 1 is the schema header ({"schema": {...}}); each subsequent line is one
// CanonicalRow ({"t": table, "v": {col: val}}). The engine param names the
// SOURCE engine and is used only to quote identifiers in the SELECT — values
// are never interpolated.
func EmitCanonical(ctx context.Context, db *sql.DB, engine string, schema *canonical.CanonicalSchema, w io.Writer) error {
	if err := canonical.WriteJSONL(w, map[string]*canonical.CanonicalSchema{"schema": schema}); err != nil {
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
func emitTable(ctx context.Context, db *sql.DB, engine string, t canonical.CanonicalTable, w io.Writer) error {
	query, err := canonical.BuildSelectSQL(engine, t)
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
			m[c.Name] = canonical.NormalizeScanned(vals[i])
		}
		if err := canonical.WriteJSONL(w, canonical.CanonicalRow{Table: t.Name, Values: m}); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("cross-engine: iterate %s: %w", t.Name, err)
	}
	return nil
}
