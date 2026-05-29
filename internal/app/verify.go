package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"time"

	"github.com/nixrajput/siphon/internal/driver"
	"github.com/nixrajput/siphon/internal/errs"
)

// Verify checks the integrity of a dump entry by recomputing the sha256 of the
// dump file and comparing it against the checksum recorded in the meta sidecar.
// It is stateless — no DB connection is required (Phase B checksums only; Phase
// F adds envelope-header validation via the driver's Verify method).
func Verify(_ context.Context, d Deps, dumpID string) (*driver.VerifyReport, error) {
	meta, err := d.Dumps.ReadMeta(dumpID)
	if err != nil {
		return nil, err
	}

	f, err := os.Open(d.Dumps.Path(dumpID))
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	started := time.Now()
	if _, err := io.Copy(h, f); err != nil {
		return nil, err
	}
	finished := time.Now()

	computed := "sha256:" + hex.EncodeToString(h.Sum(nil))
	report := &driver.VerifyReport{
		Checksum: computed,
		Started:  started,
		Finished: finished,
	}

	// Older dumps may not have a stored checksum — nothing to compare against,
	// so treat as a pass.
	if meta.Checksum == "" {
		report.OK = true
		return report, nil
	}

	if computed == meta.Checksum {
		report.OK = true
		return report, nil
	}

	report.OK = false
	return report, &errs.Error{
		Op:    "verify",
		Code:  errs.CodeIntegrity,
		Cause: errs.ErrChecksumMismatch,
		Hint:  "dump file does not match its recorded checksum (corruption or tampering)",
	}
}
