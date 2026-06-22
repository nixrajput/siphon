package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/nixrajput/siphon/internal/canonical"
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
