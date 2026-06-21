package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

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

// RunCDC starts (or resumes) a continuous sync. It is capability-gated: both
// the source and target driver must advertise CapCDC. No driver does today
// (CDC streaming via logical replication / binlog tailing is a Phase F
// follow-up), so this returns ErrDriverUnsupported — the honest scaffold state.
// When a driver gains CDC support, the polling loop below is replaced by real
// logical-replication streaming (pglogrepl for Postgres).
func RunCDC(parent context.Context, d Deps, opt SyncOpts) (<-chan jobs.Event, string, error) {
	if _, err := d.Profiles.Resolve(opt.From); err != nil {
		return nil, "", err
	}
	if _, err := d.Profiles.Resolve(opt.To); err != nil {
		return nil, "", err
	}
	if err := RequireCapability(d, opt.From, CapCDC); err != nil {
		return nil, "", err
	}
	if err := RequireCapability(d, opt.To, CapCDC); err != nil {
		return nil, "", err
	}

	return d.Runner.Run(parent, jobs.Job{
		Stage: "cdc",
		Func: func(ctx context.Context, emit func(jobs.Event)) error {
			emit(jobs.Event{Phase: jobs.PhaseProgress, Message: "CDC mode started"})
			// Phase F scaffold: a polling heartbeat that persists state each tick.
			// A future revision replaces this with real logical-replication
			// streaming (pglogrepl for Postgres, binlog tailing for MySQL/MariaDB).
			const jobID = "cdc"
			state := &CDCState{JobID: jobID, Source: opt.From, Target: opt.To}
			// Resume from a prior run's persisted position when one exists.
			if prev, err := loadCDCState(jobID); err == nil {
				state = prev
			}
			ticker := time.NewTicker(10 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					_ = saveCDCState(state)
					return ctx.Err()
				case <-ticker.C:
					if err := saveCDCState(state); err != nil {
						return err
					}
					emit(jobs.Event{Phase: jobs.PhaseProgress, Message: "CDC tick"})
				}
			}
		},
	})
}
