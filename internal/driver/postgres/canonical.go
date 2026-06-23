package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"

	"github.com/nixrajput/siphon/internal/canonical"
	"github.com/nixrajput/siphon/internal/errs"
)

// EmitCanonical writes a table-by-table snapshot of schema as JSONL to w.
func (c *Conn) EmitCanonical(ctx context.Context, schema *canonical.CanonicalSchema, w io.Writer) error {
	if err := canonical.WriteJSONL(w, map[string]*canonical.CanonicalSchema{"schema": schema}); err != nil {
		return err
	}
	for _, t := range schema.Tables {
		if err := pgEmitTable(ctx, c.db, t, w); err != nil {
			return err
		}
	}
	return nil
}

func pgEmitTable(ctx context.Context, db *sql.DB, t canonical.CanonicalTable, w io.Writer) error {
	query, err := canonical.BuildSelectSQL("postgres", t)
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
		for i, col := range t.Columns {
			m[col.Name] = canonical.NormalizeScanned(vals[i])
		}
		if err := canonical.WriteJSONL(w, canonical.CanonicalRow{Table: t.Name, Values: m}); err != nil {
			return err
		}
	}
	return rows.Err()
}

// ConsumeCanonical reads a stream produced by EmitCanonical and replays it into the database.
func (c *Conn) ConsumeCanonical(ctx context.Context, r io.Reader) error {
	dec := json.NewDecoder(r)

	var header struct {
		Schema *canonical.CanonicalSchema `json:"schema"`
	}
	if err := dec.Decode(&header); err != nil {
		if errors.Is(err, io.EOF) {
			return fmt.Errorf("cross-engine: empty stream, schema header missing")
		}
		return fmt.Errorf("cross-engine: decode schema header: %w", err)
	}
	if header.Schema == nil {
		return fmt.Errorf("cross-engine: schema header missing")
	}

	for _, t := range header.Schema.Tables {
		stmt, err := canonical.BuildCreateTableSQL("postgres", t)
		if err != nil {
			return err
		}
		if _, err := c.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("cross-engine: create table %s: %w", t.Name, err)
		}
	}

	for {
		var row canonical.CanonicalRow
		if err := dec.Decode(&row); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return fmt.Errorf("cross-engine: decode row: %w", err)
		}
		if err := pgInsertRow(ctx, c.db, row); err != nil {
			return err
		}
	}
	return nil
}

func pgInsertRow(ctx context.Context, db *sql.DB, row canonical.CanonicalRow) error {
	cols := make([]string, 0, len(row.Values))
	for col := range row.Values {
		cols = append(cols, col)
	}
	sort.Strings(cols)
	sortedVals := make([]any, len(cols))
	for i, col := range cols {
		sortedVals[i] = row.Values[col]
	}
	stmt, err := canonical.BuildInsertSQL("postgres", row.Table, cols)
	if err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, stmt, sortedVals...); err != nil {
		return fmt.Errorf("cross-engine: insert into %s: %w", row.Table, err)
	}
	return nil
}

// ApplyChange applies one CanonicalChange to the database.
func (c *Conn) ApplyChange(ctx context.Context, ch canonical.CanonicalChange) error {
	if err := canonical.ValidateChangeKey(ch); err != nil {
		return &errs.Error{
			Op:    "postgres.apply",
			Code:  errs.CodeUser,
			Cause: errs.ErrMissingPrimaryKey,
			Hint:  err.Error(),
		}
	}

	switch ch.Op {
	case canonical.OpInsert:
		cols := make([]string, 0, len(ch.Values))
		for col := range ch.Values {
			cols = append(cols, col)
		}
		sort.Strings(cols)
		stmt, err := canonical.BuildIdempotentInsertSQL("postgres", ch.Table, cols)
		if err != nil {
			return err
		}
		args := make([]any, len(cols))
		for i, col := range cols {
			args[i] = ch.Values[col]
		}
		_, err = c.db.ExecContext(ctx, stmt, args...)
		return err

	case canonical.OpUpdate:
		setCols, keyCols := canonical.ChangeColumns(ch)
		stmt, err := canonical.BuildUpdateSQL("postgres", ch.Table, setCols, keyCols)
		if err != nil {
			return err
		}
		args := make([]any, 0, len(setCols)+len(keyCols))
		for _, col := range setCols {
			args = append(args, ch.Values[col])
		}
		for _, col := range keyCols {
			args = append(args, ch.Key[col])
		}
		_, err = c.db.ExecContext(ctx, stmt, args...)
		return err

	case canonical.OpDelete:
		_, keyCols := canonical.ChangeColumns(ch)
		stmt, err := canonical.BuildDeleteSQL("postgres", ch.Table, keyCols)
		if err != nil {
			return err
		}
		args := make([]any, len(keyCols))
		for i, col := range keyCols {
			args[i] = ch.Key[col]
		}
		_, err = c.db.ExecContext(ctx, stmt, args...)
		return err

	default:
		return &errs.Error{
			Op:    "postgres.apply",
			Code:  errs.CodeSystem,
			Cause: errs.ErrDumpCorrupt,
			Hint:  "unknown change op " + string(ch.Op),
		}
	}
}
