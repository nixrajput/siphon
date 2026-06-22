package app

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

// ConsumeCanonical reads a stream produced by EmitCanonical and replays it into
// db using engine's dialect: the first JSON value is the schema header (used to
// CREATE TABLE IF NOT EXISTS for each table), every subsequent JSON value is a
// CanonicalRow that is INSERTed. Values are always bound as parameters; only
// identifiers are interpolated (quoted via canonical.QuoteIdent).
//
// A json.Decoder reads successive JSON values directly, so there is no
// token-size cap: a single large row can no longer abort the stream mid-replay
// (which a bufio.Scanner's "token too long" would have done).
func ConsumeCanonical(ctx context.Context, db *sql.DB, engine string, r io.Reader) error {
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

	if err := synthCreateTables(ctx, db, engine, header.Schema); err != nil {
		return err
	}

	for {
		var row canonical.CanonicalRow
		if err := dec.Decode(&row); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return fmt.Errorf("cross-engine: decode row: %w", err)
		}
		if err := insertRow(ctx, db, engine, row); err != nil {
			return err
		}
	}
	return nil
}

// synthCreateTables issues a CREATE TABLE IF NOT EXISTS per table.
func synthCreateTables(ctx context.Context, db *sql.DB, engine string, schema *canonical.CanonicalSchema) error {
	for _, t := range schema.Tables {
		stmt, err := canonical.BuildCreateTableSQL(engine, t)
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
func insertRow(ctx context.Context, db *sql.DB, engine string, row canonical.CanonicalRow) error {
	// Build column list and value list in the SAME pass so col[i] and val[i]
	// stay aligned even though map iteration order is unspecified.
	cols := make([]string, 0, len(row.Values))
	vals := make([]any, 0, len(row.Values))
	for c, v := range row.Values {
		cols = append(cols, c)
		vals = append(vals, v)
	}

	stmt, err := canonical.BuildInsertSQL(engine, row.Table, cols)
	if err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, stmt, vals...); err != nil {
		return fmt.Errorf("cross-engine: insert into %s: %w", row.Table, err)
	}
	return nil
}

// changeColumns returns the SET columns (Values minus Key, sorted) and the Key
// columns (sorted), giving deterministic statement shape and argument order.
func changeColumns(ch canonical.CanonicalChange) (setCols, keyCols []string) {
	for k := range ch.Key {
		keyCols = append(keyCols, k)
	}
	sort.Strings(keyCols)
	keySet := make(map[string]bool, len(keyCols))
	for _, k := range keyCols {
		keySet[k] = true
	}
	for c := range ch.Values {
		if !keySet[c] {
			setCols = append(setCols, c)
		}
	}
	sort.Strings(setCols)
	return setCols, keyCols
}

// ApplyChange applies one CanonicalChange to db using engine's SQL dialect.
// UPDATE and DELETE require a non-empty Key (the row's primary key); passing
// an empty Key for those ops is a user error.
func ApplyChange(ctx context.Context, db *sql.DB, engine string, ch canonical.CanonicalChange) error {
	switch ch.Op {
	case canonical.OpInsert:
		cols := make([]string, 0, len(ch.Values))
		for c := range ch.Values {
			cols = append(cols, c)
		}
		sort.Strings(cols)
		stmt, err := canonical.BuildInsertSQL(engine, ch.Table, cols)
		if err != nil {
			return err
		}
		args := make([]any, len(cols))
		for i, c := range cols {
			args[i] = ch.Values[c]
		}
		_, err = db.ExecContext(ctx, stmt, args...)
		return err

	case canonical.OpUpdate:
		setCols, keyCols := changeColumns(ch)
		if len(keyCols) == 0 {
			return &errs.Error{
				Op:    "canonical.apply",
				Code:  errs.CodeUser,
				Cause: errs.ErrMissingPrimaryKey,
				Hint:  "UPDATE on table " + ch.Table + " has no primary key",
			}
		}
		stmt, err := canonical.BuildUpdateSQL(engine, ch.Table, setCols, keyCols)
		if err != nil {
			return err
		}
		args := make([]any, 0, len(setCols)+len(keyCols))
		for _, c := range setCols {
			args = append(args, ch.Values[c])
		}
		for _, c := range keyCols {
			args = append(args, ch.Key[c])
		}
		_, err = db.ExecContext(ctx, stmt, args...)
		return err

	case canonical.OpDelete:
		keyCols := make([]string, 0, len(ch.Key))
		for k := range ch.Key {
			keyCols = append(keyCols, k)
		}
		sort.Strings(keyCols)
		if len(keyCols) == 0 {
			return &errs.Error{
				Op:    "canonical.apply",
				Code:  errs.CodeUser,
				Cause: errs.ErrMissingPrimaryKey,
				Hint:  "DELETE on table " + ch.Table + " has no primary key",
			}
		}
		stmt, err := canonical.BuildDeleteSQL(engine, ch.Table, keyCols)
		if err != nil {
			return err
		}
		args := make([]any, len(keyCols))
		for i, c := range keyCols {
			args[i] = ch.Key[c]
		}
		_, err = db.ExecContext(ctx, stmt, args...)
		return err

	default:
		return &errs.Error{
			Op:    "canonical.apply",
			Code:  errs.CodeSystem,
			Cause: errs.ErrDumpCorrupt,
			Hint:  "unknown change op " + string(ch.Op),
		}
	}
}
