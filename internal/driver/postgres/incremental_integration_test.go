//go:build integration

package postgres

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/nixrajput/siphon/internal/canonical"
)

// TestBackupIncremental_BoundedCaptureAndReplay exercises the full bounded
// incremental path against a live wal_level=logical server:
//
//	base state → record base-end LSN → insert/update/delete →
//	BackupIncremental (bounded JSONL capture) → drop & recreate the table →
//	replay the captured changes via ApplyChange → assert final row state.
//
// It proves (a) BackupIncremental stops at the captured end position and returns
// it, and (b) the JSONL change body, replayed via ApplyChange, reconstructs the
// post-change state — the same machinery app.Restore uses for incremental links.
func TestBackupIncremental_BoundedCaptureAndReplay(t *testing.T) {
	prof, cleanup := startLogicalPostgres(t)
	defer cleanup()

	ctx := context.Background()
	db, err := sql.Open("pgx", buildDSN(prof))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()

	if _, err := db.ExecContext(ctx,
		`CREATE TABLE widgets(id int primary key, name text)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	// Base state: one row that will be updated, one that will be deleted.
	if _, err := db.ExecContext(ctx, `INSERT INTO widgets VALUES (1,'wrench'),(2,'doomed')`); err != nil {
		t.Fatalf("seed base: %v", err)
	}

	conn := &Conn{db: db, p: prof}

	// Publication + slot must exist before the DML so the slot retains the WAL.
	if err := conn.ensurePublication(ctx); err != nil {
		t.Fatalf("ensure publication: %v", err)
	}

	// Capture the base-end LSN: the incremental resumes from exactly here.
	var baseEnd string
	if err := db.QueryRowContext(ctx, "SELECT pg_current_wal_lsn()::text").Scan(&baseEnd); err != nil {
		t.Fatalf("capture base-end lsn: %v", err)
	}

	// Post-base changes: one insert, one update, one delete.
	if _, err := db.ExecContext(ctx, `INSERT INTO widgets VALUES (3,'fresh')`); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE widgets SET name='spanner' WHERE id=1`); err != nil {
		t.Fatalf("update: %v", err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM widgets WHERE id=2`); err != nil {
		t.Fatalf("delete: %v", err)
	}

	// Bounded incremental capture from the base-end LSN to the current end.
	capCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	var buf bytes.Buffer
	endPos, err := conn.BackupIncremental(capCtx, canonical.Position{LSN: baseEnd}, &buf)
	if err != nil {
		t.Fatalf("BackupIncremental: %v", err)
	}
	if endPos.LSN == "" {
		t.Fatalf("BackupIncremental returned empty end position")
	}

	// Reset the table to its base state, then replay the captured changes; the
	// final state must reflect the insert/update/delete.
	if _, err := db.ExecContext(ctx, `TRUNCATE widgets`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO widgets VALUES (1,'wrench'),(2,'doomed')`); err != nil {
		t.Fatalf("reseed base: %v", err)
	}

	sc := bufio.NewScanner(&buf)
	var applied int
	for sc.Scan() {
		if len(sc.Bytes()) == 0 {
			continue
		}
		var ch canonical.CanonicalChange
		if err := json.Unmarshal(sc.Bytes(), &ch); err != nil {
			t.Fatalf("decode change: %v", err)
		}
		if err := conn.ApplyChange(ctx, ch); err != nil {
			t.Fatalf("ApplyChange %+v: %v", ch, err)
		}
		applied++
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan changes: %v", err)
	}
	if applied < 3 {
		t.Fatalf("expected at least 3 captured changes, applied %d", applied)
	}

	// Assert final state: id=1 updated to spanner, id=2 deleted, id=3 inserted.
	rows := map[int]string{}
	r, err := db.QueryContext(ctx, `SELECT id, name FROM widgets ORDER BY id`)
	if err != nil {
		t.Fatalf("query final: %v", err)
	}
	for r.Next() {
		var id int
		var name string
		if err := r.Scan(&id, &name); err != nil {
			t.Fatalf("scan final: %v", err)
		}
		rows[id] = name
	}
	_ = r.Close()

	if rows[1] != "spanner" {
		t.Errorf("id=1 = %q, want spanner (update not applied)", rows[1])
	}
	if _, ok := rows[2]; ok {
		t.Errorf("id=2 still present (delete not applied): %q", rows[2])
	}
	if rows[3] != "fresh" {
		t.Errorf("id=3 = %q, want fresh (insert not applied)", rows[3])
	}
}
