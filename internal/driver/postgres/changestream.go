package postgres

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pglogrepl"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgproto3"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nixrajput/siphon/internal/canonical"
	"github.com/nixrajput/siphon/internal/driver"
	"github.com/nixrajput/siphon/internal/errs"
)

// Stable, siphon-owned names for the publication and logical slot. Both are
// created on first use with IF-NOT-EXISTS semantics so repeated streaming
// (bounded incremental + unbounded CDC) reuses the same objects.
const (
	siphonPublication = "siphon_pub"
	siphonSlot        = "siphon_logical"
	outputPlugin      = "pgoutput"
)

// standbyInterval bounds how long we block on a single ReceiveMessage and how
// often we ack the server. The server drops a logical replication connection
// that goes silent, so we must send a Standby Status Update at least this
// often even when no rows are flowing.
const standbyInterval = 10 * time.Second

var _ driver.ChangeStreamer = (*Conn)(nil)

// StreamChanges streams logical row changes as engine-neutral CanonicalChanges,
// starting after `from`. It uses pgoutput logical decoding over a dedicated
// replication-mode connection. Bounded callers cancel ctx at a target end
// position; unbounded (CDC) callers stream until ctx cancel. ctx cancellation
// is the normal stop signal and is NOT reported as an error — the final
// Position reached is returned for envelope stamping / CDC state persistence.
func (c *Conn) StreamChanges(ctx context.Context, from canonical.Position, emit func(canonical.CanonicalChange) error) (canonical.Position, error) {
	return c.streamWithStop(ctx, from, 0, emit)
}

// streamWithStop is the shared pgoutput decode driver behind StreamChanges
// (stopLSN==0, unbounded) and BackupIncremental (stopLSN!=0, bounded). It opens
// the replication connection, ensures the slot, resolves the start LSN, starts
// replication, and runs receiveLoop with the given stop bound.
func (c *Conn) streamWithStop(ctx context.Context, from canonical.Position, stopLSN pglogrepl.LSN, emit func(canonical.CanonicalChange) error) (canonical.Position, error) {
	if err := c.requireLogicalWAL(ctx); err != nil {
		return canonical.Position{}, err
	}
	if err := c.ensurePublication(ctx); err != nil {
		return canonical.Position{}, err
	}

	repl, err := pgconn.Connect(ctx, c.replicationDSN())
	if err != nil {
		return canonical.Position{}, &errs.Error{
			Op:    "postgres.stream",
			Code:  errs.CodeSystem,
			Cause: errs.ErrConnectionFailed,
			Hint:  "logical replication connect: " + err.Error(),
		}
	}
	defer func() { _ = repl.Close(context.Background()) }()

	if _, err := ensureLogicalSlot(ctx, repl); err != nil {
		return canonical.Position{}, err
	}

	startLSN, err := resolveStartLSN(ctx, repl, from)
	if err != nil {
		return canonical.Position{}, err
	}

	pluginArgs := []string{
		"proto_version '1'",
		"publication_names '" + siphonPublication + "'",
	}
	if err := pglogrepl.StartReplication(ctx, repl, siphonSlot, startLSN,
		pglogrepl.StartReplicationOptions{PluginArgs: pluginArgs}); err != nil {
		return canonical.Position{}, &errs.Error{Op: "postgres.stream", Code: errs.CodeSystem, Cause: err}
	}

	return receiveLoop(ctx, repl, startLSN, stopLSN, emit)
}

// receiveLoop runs the pgoutput receive/decode/ack cycle until ctx is cancelled
// or emit returns an error. It returns the final committed LSN as a Position.
//
// stopLSN bounds a BOUNDED (incremental) capture: when non-zero, the loop returns
// cleanly once the client position reaches or passes it — but only at a message
// boundary AFTER all changes up to that point have been decoded and emitted, so
// the bound never truncates a change that committed at or before stopLSN. A zero
// stopLSN means unbounded (CDC): stream until ctx cancel.
func receiveLoop(ctx context.Context, repl *pgconn.PgConn, startLSN, stopLSN pglogrepl.LSN, emit func(canonical.CanonicalChange) error) (canonical.Position, error) {
	relations := map[uint32]*pglogrepl.RelationMessage{}
	typeMap := pgtype.NewMap()
	clientXLogPos := startLSN
	nextDeadline := time.Now().Add(standbyInterval)

	finalPos := func() canonical.Position { return canonical.Position{LSN: clientXLogPos.String()} }

	for {
		// ctx cancel is the normal bounded/unbounded stop signal: ack the last
		// LSN best-effort and return cleanly.
		if ctx.Err() != nil {
			_ = pglogrepl.SendStandbyStatusUpdate(context.Background(), repl,
				pglogrepl.StandbyStatusUpdate{WALWritePosition: clientXLogPos})
			return finalPos(), nil
		}

		// Bounded (incremental) stop: once the client has caught up to the target
		// end LSN, every change committed at or before it has already been decoded
		// and emitted (XLogData advances clientXLogPos only after decodeWALData),
		// so it is safe to ack and return the end position.
		if stopLSN != 0 && clientXLogPos >= stopLSN {
			_ = pglogrepl.SendStandbyStatusUpdate(context.Background(), repl,
				pglogrepl.StandbyStatusUpdate{WALWritePosition: clientXLogPos})
			return finalPos(), nil
		}

		if time.Now().After(nextDeadline) {
			if err := pglogrepl.SendStandbyStatusUpdate(ctx, repl,
				pglogrepl.StandbyStatusUpdate{WALWritePosition: clientXLogPos}); err != nil {
				return finalPos(), &errs.Error{Op: "postgres.stream", Code: errs.CodeSystem, Cause: err}
			}
			nextDeadline = time.Now().Add(standbyInterval)
		}

		recvCtx, cancel := context.WithDeadline(ctx, nextDeadline)
		rawMsg, err := repl.ReceiveMessage(recvCtx)
		cancel()
		if err != nil {
			if pgconn.Timeout(err) {
				continue // deadline hit with no data: loop to send a standby ack
			}
			if ctx.Err() != nil {
				return finalPos(), nil // cancelled mid-receive: clean stop
			}
			return finalPos(), &errs.Error{Op: "postgres.stream", Code: errs.CodeSystem, Cause: err}
		}

		if errMsg, ok := rawMsg.(*pgproto3.ErrorResponse); ok {
			return finalPos(), &errs.Error{
				Op:   "postgres.stream",
				Code: errs.CodeSystem,
				Cause: errors.New("replication stream error: " +
					errMsg.Severity + " " + errMsg.Message),
			}
		}

		cd, ok := rawMsg.(*pgproto3.CopyData)
		if !ok {
			continue
		}

		switch cd.Data[0] {
		case pglogrepl.PrimaryKeepaliveMessageByteID:
			pkm, err := pglogrepl.ParsePrimaryKeepaliveMessage(cd.Data[1:])
			if err != nil {
				return finalPos(), &errs.Error{Op: "postgres.stream", Code: errs.CodeSystem, Cause: err}
			}
			if pkm.ServerWALEnd > clientXLogPos {
				clientXLogPos = pkm.ServerWALEnd
			}
			if pkm.ReplyRequested {
				nextDeadline = time.Time{} // force an immediate ack next iteration
			}

		case pglogrepl.XLogDataByteID:
			xld, err := pglogrepl.ParseXLogData(cd.Data[1:])
			if err != nil {
				return finalPos(), &errs.Error{Op: "postgres.stream", Code: errs.CodeSystem, Cause: err}
			}
			if err := decodeWALData(xld.WALData, relations, typeMap, emit); err != nil {
				return finalPos(), err
			}
			end := xld.WALStart + pglogrepl.LSN(len(xld.WALData))
			if end > clientXLogPos {
				clientXLogPos = end
			}
		}
	}
}

// decodeWALData parses one pgoutput message and, for Insert/Update/Delete,
// builds a CanonicalChange and passes it to emit. Relation messages are cached
// so later DML can resolve column names and key flags. An emit error stops the
// stream and is propagated verbatim.
func decodeWALData(walData []byte, relations map[uint32]*pglogrepl.RelationMessage, typeMap *pgtype.Map, emit func(canonical.CanonicalChange) error) error {
	msg, err := pglogrepl.Parse(walData)
	if err != nil {
		return &errs.Error{Op: "postgres.stream", Code: errs.CodeSystem, Cause: err}
	}

	switch m := msg.(type) {
	case *pglogrepl.RelationMessage:
		relations[m.RelationID] = m

	case *pglogrepl.InsertMessage:
		rel, ok := relations[m.RelationID]
		if !ok {
			return relErr(m.RelationID)
		}
		vals := tupleValues(rel, m.Tuple, typeMap)
		return emit(canonical.CanonicalChange{
			Op:     canonical.OpInsert,
			Table:  rel.RelationName,
			Key:    keyFromValues(rel, vals),
			Values: vals,
		})

	case *pglogrepl.UpdateMessage:
		rel, ok := relations[m.RelationID]
		if !ok {
			return relErr(m.RelationID)
		}
		vals := tupleValues(rel, m.NewTuple, typeMap)
		// Prefer the old-tuple image for the key (REPLICA IDENTITY); fall back to
		// the new image when the server sent no old tuple (key unchanged).
		key := keyFromValues(rel, vals)
		if m.OldTuple != nil {
			if k := keyFromValues(rel, tupleValues(rel, m.OldTuple, typeMap)); len(k) > 0 {
				key = k
			}
		}
		return emit(canonical.CanonicalChange{
			Op:     canonical.OpUpdate,
			Table:  rel.RelationName,
			Key:    key,
			Values: vals,
		})

	case *pglogrepl.DeleteMessage:
		rel, ok := relations[m.RelationID]
		if !ok {
			return relErr(m.RelationID)
		}
		old := tupleValues(rel, m.OldTuple, typeMap)
		return emit(canonical.CanonicalChange{
			Op:    canonical.OpDelete,
			Table: rel.RelationName,
			Key:   keyFromValues(rel, old),
		})
	}
	// Begin/Commit/Type/Truncate/Origin/streaming markers carry no row change.
	return nil
}

func relErr(id uint32) error {
	return &errs.Error{
		Op:    "postgres.stream",
		Code:  errs.CodeSystem,
		Cause: errors.New("pgoutput: unknown relation ID " + strconv.FormatUint(uint64(id), 10)),
		Hint:  "a DML message arrived before its relation definition",
	}
}

// tupleValues decodes a pgoutput tuple into a column-name → Go-value map. NULL
// columns map to nil; unchanged-TOAST columns ('u') are omitted (their value
// was not transmitted). Text-format columns are decoded via the pgtype map for
// the column's OID, falling back to the raw string.
func tupleValues(rel *pglogrepl.RelationMessage, tuple *pglogrepl.TupleData, typeMap *pgtype.Map) map[string]any {
	if tuple == nil {
		return nil
	}
	out := make(map[string]any, len(tuple.Columns))
	for i, col := range tuple.Columns {
		if i >= len(rel.Columns) {
			break
		}
		name := rel.Columns[i].Name
		switch col.DataType {
		case 'n': // null
			out[name] = nil
		case 'u': // unchanged TOAST: value not sent, leave it out
			continue
		case 't': // text
			out[name] = decodeText(typeMap, col.Data, rel.Columns[i].DataType)
		default: // binary ('b') or unexpected: keep the raw bytes as a string
			out[name] = string(col.Data)
		}
	}
	return out
}

// decodeText decodes a text-format column using the pgtype codec for its OID,
// returning the original string when the OID is unknown or decoding fails.
func decodeText(typeMap *pgtype.Map, data []byte, oid uint32) any {
	if dt, ok := typeMap.TypeForOID(oid); ok {
		if v, err := dt.Codec.DecodeValue(typeMap, oid, pgtype.TextFormatCode, data); err == nil {
			return v
		}
	}
	return string(data)
}

// keyFromValues extracts the primary-key columns (RelationMessageColumn.Flags
// bit 1 = part of the key, set under REPLICA IDENTITY DEFAULT/USING INDEX) from
// a decoded value map. When the relation declares no key columns (e.g. REPLICA
// IDENTITY FULL with no PK), it falls back to the full value set so UPDATE and
// DELETE still have something to target.
func keyFromValues(rel *pglogrepl.RelationMessage, vals map[string]any) map[string]any {
	key := map[string]any{}
	hasKeyCol := false
	for _, col := range rel.Columns {
		if col.Flags&1 == 1 {
			hasKeyCol = true
			if v, ok := vals[col.Name]; ok {
				key[col.Name] = v
			}
		}
	}
	if !hasKeyCol {
		// REPLICA IDENTITY FULL / no declared key: use every column as the key.
		full := make(map[string]any, len(vals))
		for k, v := range vals {
			full[k] = v
		}
		return full
	}
	return key
}

// requireLogicalWAL fails fast with an actionable hint when the server is not
// configured for logical decoding (wal_level must be 'logical').
func (c *Conn) requireLogicalWAL(ctx context.Context) error {
	var level string
	if err := c.db.QueryRowContext(ctx, "SHOW wal_level").Scan(&level); err != nil {
		return &errs.Error{Op: "postgres.stream", Code: errs.CodeSystem, Cause: err}
	}
	if !strings.EqualFold(level, "logical") {
		return &errs.Error{
			Op:    "postgres.stream",
			Code:  errs.CodeUser,
			Cause: errors.New("wal_level is " + level),
			Hint:  "set wal_level = logical in postgresql.conf and restart the server",
		}
	}
	return nil
}

// ensurePublication creates the siphon publication FOR ALL TABLES if it does
// not already exist. CREATE PUBLICATION has no IF NOT EXISTS clause (pre-PG13),
// so we query pg_publication first and skip creation when present.
func (c *Conn) ensurePublication(ctx context.Context) error {
	var exists bool
	if err := c.db.QueryRowContext(ctx,
		"SELECT EXISTS (SELECT 1 FROM pg_publication WHERE pubname = $1)", siphonPublication).Scan(&exists); err != nil {
		return &errs.Error{Op: "postgres.stream", Code: errs.CodeSystem, Cause: err}
	}
	if exists {
		return nil
	}
	// The publication name is a fixed constant, so direct interpolation is safe.
	if _, err := c.db.ExecContext(ctx, "CREATE PUBLICATION "+siphonPublication+" FOR ALL TABLES"); err != nil {
		// Tolerate a concurrent creator (duplicate_object, SQLSTATE 42710).
		if strings.Contains(err.Error(), "already exists") {
			return nil
		}
		return &errs.Error{
			Op:    "postgres.stream",
			Code:  errs.CodeUser,
			Cause: err,
			Hint:  "creating publication requires a superuser or a role with CREATE on the database",
		}
	}
	return nil
}

// ensureLogicalSlot creates the pgoutput logical slot on the replication
// connection if absent. CreateReplicationSlot errors with "already exists" when
// the slot is present; that is tolerated so repeated streams reuse the slot.
//
// It returns the slot's consistent point — the LSN from which the freshly
// created slot guarantees every subsequent change is retained and decodable —
// when it creates the slot, and "" when the slot already existed (the existing
// slot is already retaining WAL, so the caller has no new anchor to record).
func ensureLogicalSlot(ctx context.Context, repl *pgconn.PgConn) (string, error) {
	res, err := pglogrepl.CreateReplicationSlot(ctx, repl, siphonSlot, outputPlugin,
		pglogrepl.CreateReplicationSlotOptions{Temporary: false})
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			return "", nil
		}
		return "", &errs.Error{Op: "postgres.stream", Code: errs.CodeSystem, Cause: err}
	}
	return res.ConsistentPoint, nil
}

// establishLogicalSlot opens a short-lived replication connection, ensures the
// publication and logical slot exist, and returns the slot's consistent point
// (empty when the slot already existed).
//
// This is the slot-establishment primitive used to anchor WAL retention BEFORE
// a workload runs: a logical slot only retains and decodes WAL produced after
// its own creation, so the slot must exist before the changes a later
// incremental or CDC run wants to capture. CurrentPosition calls this so a base
// backup leaves the slot in place, and the changes that follow are retained.
func (c *Conn) establishLogicalSlot(ctx context.Context) (string, error) {
	if err := c.requireLogicalWAL(ctx); err != nil {
		return "", err
	}
	if err := c.ensurePublication(ctx); err != nil {
		return "", err
	}
	repl, err := pgconn.Connect(ctx, c.replicationDSN())
	if err != nil {
		return "", &errs.Error{
			Op:    "postgres.stream",
			Code:  errs.CodeSystem,
			Cause: errs.ErrConnectionFailed,
			Hint:  "logical replication connect: " + err.Error(),
		}
	}
	defer func() { _ = repl.Close(context.Background()) }()
	return ensureLogicalSlot(ctx, repl)
}

// resolveStartLSN returns the LSN to start replication from. A non-empty
// from.LSN is parsed and used verbatim; otherwise we start from the server's
// current WAL position via IdentifySystem.
func resolveStartLSN(ctx context.Context, repl *pgconn.PgConn, from canonical.Position) (pglogrepl.LSN, error) {
	if from.LSN != "" {
		lsn, err := pglogrepl.ParseLSN(from.LSN)
		if err != nil {
			return 0, &errs.Error{
				Op:    "postgres.stream",
				Code:  errs.CodeUser,
				Cause: err,
				Hint:  "invalid start LSN " + from.LSN,
			}
		}
		return lsn, nil
	}
	sys, err := pglogrepl.IdentifySystem(ctx, repl)
	if err != nil {
		return 0, &errs.Error{Op: "postgres.stream", Code: errs.CodeSystem, Cause: err}
	}
	return sys.XLogPos, nil
}

// replicationDSN builds a libpq keyword DSN like buildDSN but adds
// replication=database, which pgconn requires to open a logical replication
// connection (a plain connection rejects START_REPLICATION).
func (c *Conn) replicationDSN() string {
	return buildDSN(c.p) + " replication=database"
}
