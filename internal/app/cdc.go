package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/nixrajput/siphon/internal/audit"
	"github.com/nixrajput/siphon/internal/canonical"
	"github.com/nixrajput/siphon/internal/driver"
	"github.com/nixrajput/siphon/internal/errs"
	"github.com/nixrajput/siphon/internal/jobs"
)

// CDCState is persisted between runs so a long-running continuous sync can
// resume from the last applied position after a restart.
type CDCState struct {
	JobID          string    `json:"job_id"`
	Source         string    `json:"source_profile"`
	Target         string    `json:"target_profile"`
	LastAppliedLSN string    `json:"last_applied_lsn,omitempty"`
	LastBinlogFile string    `json:"last_binlog_file,omitempty"`
	LastBinlogPos  uint64    `json:"last_binlog_pos,omitempty"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// position renders the resume cursor stored in this state as a canonical.Position.
func (s *CDCState) position() canonical.Position {
	return canonical.Position{
		LSN:        s.LastAppliedLSN,
		BinlogFile: s.LastBinlogFile,
		BinlogPos:  s.LastBinlogPos,
	}
}

// setPosition records p as the new resume cursor.
func (s *CDCState) setPosition(p canonical.Position) {
	s.LastAppliedLSN = p.LSN
	s.LastBinlogFile = p.BinlogFile
	s.LastBinlogPos = p.BinlogPos
}

// hasPosition reports whether any resume cursor has been recorded.
func (s *CDCState) hasPosition() bool {
	return s.LastAppliedLSN != "" || s.LastBinlogFile != ""
}

// CDCStateDir returns the per-user directory holding CDC resume state. It
// honors SIPHON_STATE_HOME, then XDG_STATE_HOME, then $HOME/.local/state —
// mirroring how internal/config resolves its config path, so tests can redirect
// it without writing to the real home.
func CDCStateDir() string {
	if v := os.Getenv("SIPHON_STATE_HOME"); v != "" {
		return filepath.Join(v, "cdc")
	}
	if v := os.Getenv("XDG_STATE_HOME"); v != "" {
		return filepath.Join(v, "siphon", "cdc")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "state", "siphon", "cdc")
}

func saveCDCState(s *CDCState) error {
	if err := os.MkdirAll(CDCStateDir(), 0o700); err != nil {
		return err
	}
	s.UpdatedAt = time.Now().UTC()
	body, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(CDCStateDir(), s.JobID+".state"), body, 0o600)
}

func loadCDCState(jobID string) (*CDCState, error) {
	body, err := os.ReadFile(filepath.Join(CDCStateDir(), jobID+".state"))
	if err != nil {
		return nil, err
	}
	s := &CDCState{}
	return s, json.Unmarshal(body, s)
}

// cdcJobID derives a stable per-(source,target) identifier so a restart of the
// same continuous sync resumes from the same state file. A short hash keeps the
// filename safe regardless of profile-name characters.
func cdcJobID(from, to string) string {
	sum := sha256.Sum256([]byte(from + "\x00" + to))
	return "cdc-" + hex.EncodeToString(sum[:8])
}

// RunCDC starts (or resumes) a continuous, unbounded sync: it tails the source's
// logical change stream and applies each engine-neutral CanonicalChange to the
// target. It works same-engine and cross-engine alike because CanonicalChange is
// engine-neutral and ApplyChange replays it natively on the target.
//
// Both drivers must advertise CapCDC. On a first run (no prior state) it captures
// a consistent start position, takes an initial schema+data snapshot, then streams
// changes committed after that position. On a restart it resumes from the saved
// position with no snapshot.
//
// Resume granularity is "since the last clean exit": RunCDC persists the
// streamer's returned final position when the stream stops (the position is tied
// to what was actually delivered, never ahead of it). Re-applying the tail after
// a crash is safe because ApplyChange is idempotent (INSERT upserts; UPDATE and
// DELETE target by primary key).
//
// ctx cancellation is the normal stop signal (matching the bounded-stream
// convention): on cancel RunCDC persists final state and returns nil; only a
// non-cancel StreamChanges error is surfaced.
func RunCDC(parent context.Context, d Deps, opt SyncOpts) (<-chan jobs.Event, string, error) {
	src, err := d.Profiles.Resolve(opt.From)
	if err != nil {
		return nil, "", err
	}
	dst, err := d.Profiles.Resolve(opt.To)
	if err != nil {
		return nil, "", err
	}
	if err := RequireCapability(d, opt.From, CapCDC); err != nil {
		return nil, "", err
	}
	if err := RequireCapability(d, opt.To, CapCDC); err != nil {
		return nil, "", err
	}
	srcDrv, err := d.Drivers.Get(src.Driver)
	if err != nil {
		return nil, "", err
	}
	dstDrv, err := d.Drivers.Get(dst.Driver)
	if err != nil {
		return nil, "", err
	}

	// CDC is destructive (it continuously applies changes to the target), so it
	// runs through the same gate/audit seam as the other verbs.
	done, err := guardedOp(parent, d, audit.OpSync, opt.From, opt.To)
	if err != nil {
		return nil, "", err
	}

	jobID := cdcJobID(opt.From, opt.To)

	return launchGuarded(d.Runner, parent, done, jobs.Job{
		Stage: "cdc",
		Func: func(ctx context.Context, emit func(jobs.Event)) (retErr error) {
			defer func() { done(retErr) }()
			emit(jobs.Event{Phase: jobs.PhaseProgress, Message: "CDC mode started"})

			srcConn, err := srcDrv.Connect(ctx, src)
			if err != nil {
				return err
			}
			defer func() { _ = srcConn.Close() }()

			dstConn, err := dstDrv.Connect(ctx, dst)
			if err != nil {
				return err
			}
			defer func() { _ = dstConn.Close() }()

			srcStreamer, ok := srcConn.(driver.ChangeStreamer)
			if !ok {
				return cdcUnsupported(src.Driver, "ChangeStreamer")
			}
			srcPositioner, ok := srcConn.(driver.BasePositioner)
			if !ok {
				return cdcUnsupported(src.Driver, "BasePositioner")
			}
			dstXfer, ok := dstConn.(driver.CanonicalTransfer)
			if !ok {
				return cdcUnsupported(dst.Driver, "CanonicalTransfer")
			}

			// Resume from prior state when present; otherwise capture a consistent
			// start position and take an initial snapshot before streaming.
			state := &CDCState{JobID: jobID, Source: opt.From, Target: opt.To}
			var from canonical.Position
			if prev, loadErr := loadCDCState(jobID); loadErr == nil && prev.hasPosition() {
				state = prev
				from = prev.position()
				emit(jobs.Event{Phase: jobs.PhaseProgress, Message: "CDC resuming from saved position"})
			} else {
				from, err = srcPositioner.CurrentPosition(ctx)
				if err != nil {
					return err
				}
				if snapErr := cdcSnapshot(ctx, srcConn, dstXfer, src.Driver, opt.Tables); snapErr != nil {
					return snapErr
				}
				state.setPosition(from)
				if saveErr := saveCDCState(state); saveErr != nil {
					return saveErr
				}
				emit(jobs.Event{Phase: jobs.PhaseProgress, Message: "CDC initial snapshot complete; streaming changes"})
			}

			applied := 0
			emit2 := func(ch canonical.CanonicalChange) error {
				// Honor --table: the streamer surfaces changes for every table (the
				// source's publication/binlog is database-wide), so skip any change
				// outside the requested set before applying it to the target.
				if !tableAllowed(ch.Table, opt.Tables) {
					return nil
				}
				if applyErr := dstXfer.ApplyChange(ctx, ch); applyErr != nil {
					return applyErr
				}
				applied++
				emit(jobs.Event{
					Phase:   jobs.PhaseProgress,
					Message: fmt.Sprintf("applied %s on %s (%d total)", ch.Op, ch.Table, applied),
				})
				// No periodic mid-stream checkpoint here: srcPositioner.CurrentPosition
				// returns the source's CURRENT WAL/binlog end, which is ahead of the
				// change we just applied. Persisting it would, after a crash, resume
				// PAST changes that streamed but were never applied — silent data loss.
				// We checkpoint only the streamer's returned final position on exit
				// (below), which is tied to what was actually delivered; at-least-once
				// redelivery on resume is safe because ApplyChange is idempotent.
				return nil
			}

			finalPos, streamErr := srcStreamer.StreamChanges(ctx, from, emit2)

			// On any exit, persist the furthest position we know about. The
			// streamer returns the final position even on ctx-cancel.
			if finalPos.LSN != "" || finalPos.BinlogFile != "" {
				state.setPosition(finalPos)
			}
			_ = saveCDCState(state)

			// ctx cancellation is the normal stop signal — report clean.
			if ctx.Err() != nil {
				emit(jobs.Event{Phase: jobs.PhaseProgress, Message: "CDC stopped"})
				return nil
			}
			return streamErr
		},
	})
}

// cdcSnapshot performs the initial schema+data copy from source to target using
// the canonical transfer pipe (the same pattern as runCrossEngineSync). Both the
// source's SchemaInspector+CanonicalTransfer and the target's CanonicalTransfer
// are required.
func cdcSnapshot(ctx context.Context, srcConn driver.Conn, dstXfer driver.CanonicalTransfer, srcDriverName string, tables []string) error {
	srcInsp, ok := srcConn.(driver.SchemaInspector)
	if !ok {
		return cdcUnsupported(srcDriverName, "SchemaInspector")
	}
	srcXfer, ok := srcConn.(driver.CanonicalTransfer)
	if !ok {
		return cdcUnsupported(srcDriverName, "CanonicalTransfer")
	}

	schema, err := srcInsp.InspectSchema(ctx)
	if err != nil {
		return err
	}
	// Snapshot only the requested tables; the streamed-change phase applies the
	// same filter in emit2.
	schema = filterSchemaTables(schema, tables)

	stream := jobs.NewStream(64)
	errCh := make(chan error, 1)

	go func() {
		emitErr := srcXfer.EmitCanonical(ctx, schema, stream)
		_ = stream.CloseErr(emitErr)
		errCh <- emitErr
	}()

	consumeErr := dstXfer.ConsumeCanonical(ctx, stream)
	_ = stream.Close()
	emitErr := <-errCh

	if emitErr != nil {
		return emitErr
	}
	return consumeErr
}

func cdcUnsupported(driverName, iface string) error {
	return &errs.Error{
		Op:    "cdc",
		Code:  errs.CodeUser,
		Cause: errs.ErrDriverUnsupported,
		Hint:  driverName + " driver does not implement " + iface,
	}
}
