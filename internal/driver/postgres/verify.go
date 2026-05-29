package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"time"

	"github.com/nixrajput/siphon/internal/driver"
)

// Verify currently performs a checksum-only check on the dump stream.
// Header-format checks land in Phase F when the siphon envelope exists.
func (c *Conn) Verify(ctx context.Context, r io.Reader) (*driver.VerifyReport, error) {
	started := time.Now()
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return nil, err
	}
	return &driver.VerifyReport{
		Checksum: "sha256:" + hex.EncodeToString(h.Sum(nil)),
		OK:       true,
		Started:  started,
		Finished: time.Now(),
	}, nil
}
