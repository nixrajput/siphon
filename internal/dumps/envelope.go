package dumps

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"
)

const (
	EnvelopeMagic = "SIPH"
	EnvelopeSize  = 4096
)

type EnvelopeType string

const (
	EnvelopeBase        EnvelopeType = "base"
	EnvelopeIncremental EnvelopeType = "incremental"
)

// Envelope is the 4 KB JSON header prepended to every dump. The native
// dump bytes follow immediately after.
type Envelope struct {
	Siphon        string       `json:"siphon"`
	Type          EnvelopeType `json:"type"`
	Driver        string       `json:"driver"`
	EngineVersion string       `json:"engine_version,omitempty"`
	BaseID        string       `json:"base_id,omitempty"`
	ParentID      string       `json:"parent_id,omitempty"`
	WALStart      string       `json:"wal_start,omitempty"`
	WALEnd        string       `json:"wal_end,omitempty"`
	BinlogFile    string       `json:"binlog_file,omitempty"`
	BinlogStart   uint64       `json:"binlog_start,omitempty"`
	BinlogEnd     uint64       `json:"binlog_end,omitempty"`
	Checksum      string       `json:"checksum,omitempty"`
	Tables        []TableEntry `json:"tables,omitempty"`
	Created       time.Time    `json:"created"`
}

var ErrInvalidEnvelope = errors.New("invalid siphon envelope")

// WriteEnvelope writes a padded 4 KB header to w. Returns the number of
// bytes written (always 4096 on success).
func WriteEnvelope(w io.Writer, e *Envelope) (int, error) {
	if e.Siphon == "" {
		e.Siphon = "1.0"
	}
	if e.Created.IsZero() {
		e.Created = time.Now().UTC()
	}
	body, err := json.Marshal(e)
	if err != nil {
		return 0, err
	}
	if 4+len(body) > EnvelopeSize-1 {
		return 0, fmt.Errorf("envelope JSON is %d bytes; max %d", len(body), EnvelopeSize-5)
	}

	buf := make([]byte, EnvelopeSize)
	copy(buf[0:4], []byte(EnvelopeMagic))
	copy(buf[4:], body)
	for i := 4 + len(body); i < EnvelopeSize-1; i++ {
		buf[i] = ' '
	}
	buf[EnvelopeSize-1] = '\n'
	return w.Write(buf)
}

// ReadEnvelope reads and validates the 4 KB header from r. Returns the
// parsed Envelope and a reader positioned at the start of the native
// dump bytes.
func ReadEnvelope(r io.Reader) (*Envelope, io.Reader, error) {
	header := make([]byte, EnvelopeSize)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, nil, fmt.Errorf("%w: short read", ErrInvalidEnvelope)
	}
	if string(header[0:4]) != EnvelopeMagic {
		return nil, nil, fmt.Errorf("%w: missing magic", ErrInvalidEnvelope)
	}
	body := bytes.TrimRight(header[4:EnvelopeSize-1], " ")
	e := &Envelope{}
	if err := json.Unmarshal(body, e); err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrInvalidEnvelope, err)
	}
	return e, r, nil
}
