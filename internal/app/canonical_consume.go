package app

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// ConsumeCanonical reads a JSONL stream produced by EmitCanonical and replays
// it into db using engine's dialect: line 1 is the schema header (used to
// CREATE TABLE IF NOT EXISTS for each table), every subsequent line is a
// CanonicalRow that is INSERTed. Values are always bound as parameters; only
// identifiers are interpolated (quoted via quoteIdent).
func ConsumeCanonical(ctx context.Context, db *sql.DB, engine string, r io.Reader) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 1<<20), 16<<20)

	if !sc.Scan() {
		if err := sc.Err(); err != nil {
			return fmt.Errorf("cross-engine: read schema header: %w", err)
		}
		return fmt.Errorf("cross-engine: empty stream, schema header missing")
	}

	var header struct {
		Schema *CanonicalSchema `json:"schema"`
	}
	if err := json.Unmarshal(sc.Bytes(), &header); err != nil {
		return fmt.Errorf("cross-engine: decode schema header: %w", err)
	}
	if header.Schema == nil {
		return fmt.Errorf("cross-engine: schema header missing")
	}

	if err := synthCreateTables(ctx, db, engine, header.Schema); err != nil {
		return err
	}

	for sc.Scan() {
		line := sc.Bytes()
		if len(strings.TrimSpace(string(line))) == 0 {
			continue
		}
		var row CanonicalRow
		if err := json.Unmarshal(line, &row); err != nil {
			return fmt.Errorf("cross-engine: decode row: %w", err)
		}
		if err := insertRow(ctx, db, engine, row); err != nil {
			return err
		}
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("cross-engine: read rows: %w", err)
	}
	return nil
}

// synthCreateTables issues a CREATE TABLE IF NOT EXISTS per table.
func synthCreateTables(ctx context.Context, db *sql.DB, engine string, schema *CanonicalSchema) error {
	for _, t := range schema.Tables {
		stmt, err := buildCreateTableSQL(engine, t)
		if err != nil {
			return err
		}
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("cross-engine: create table %s: %w", t.Name, err)
		}
	}
	return nil
}

// insertRow inserts a single CanonicalRow, binding values as parameters.
func insertRow(ctx context.Context, db *sql.DB, engine string, row CanonicalRow) error {
	// Build column list and value list in the SAME pass so col[i] and val[i]
	// stay aligned even though map iteration order is unspecified.
	cols := make([]string, 0, len(row.Values))
	vals := make([]any, 0, len(row.Values))
	for c, v := range row.Values {
		cols = append(cols, c)
		vals = append(vals, v)
	}

	stmt, err := buildInsertSQL(engine, row.Table, cols)
	if err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, stmt, vals...); err != nil {
		return fmt.Errorf("cross-engine: insert into %s: %w", row.Table, err)
	}
	return nil
}

// buildCreateTableSQL renders a CREATE TABLE IF NOT EXISTS statement with every
// identifier quoted for the engine and each column mapped via MapToNative. Pure
// function — unit-testable without a DB.
func buildCreateTableSQL(engine string, t CanonicalTable) (string, error) {
	qt, err := quoteIdent(engine, t.Name)
	if err != nil {
		return "", err
	}
	defs := make([]string, len(t.Columns))
	for i, c := range t.Columns {
		qc, err := quoteIdent(engine, c.Name)
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
	}
	return fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (%s)", qt, strings.Join(defs, ", ")), nil
}

// buildInsertSQL renders an INSERT with quoted identifiers and per-engine
// placeholders. Pure function — unit-testable without a DB. Returns an error
// for an empty column set (an INSERT with no columns is malformed).
func buildInsertSQL(engine, table string, cols []string) (string, error) {
	if len(cols) == 0 {
		return "", fmt.Errorf("cross-engine: insert into %s: no columns", table)
	}
	qt, err := quoteIdent(engine, table)
	if err != nil {
		return "", err
	}
	qcols := make([]string, len(cols))
	phs := make([]string, len(cols))
	for i, c := range cols {
		qc, err := quoteIdent(engine, c)
		if err != nil {
			return "", err
		}
		qcols[i] = qc
		phs[i] = placeholder(engine, i+1)
	}
	return fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		qt, strings.Join(qcols, ","), strings.Join(phs, ",")), nil
}
