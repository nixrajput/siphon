// Package dumps implements the catalog of saved dump files. Dumps live
// on a Storage backend (filesystem in Phase B; cloud backends in Phase G);
// each entry has a sidecar <id>.meta.json carrying schema, checksum, etc.
package dumps

import "time"

// Meta is the sidecar JSON written next to each dump file.
type Meta struct {
	ID            string            `json:"id"`
	Profile       string            `json:"profile"`
	Driver        string            `json:"driver"`
	EngineVersion string            `json:"engine_version"`
	DumpFormat    string            `json:"dump_format"`
	SizeBytes     int64             `json:"size_bytes"`
	Checksum      string            `json:"checksum"`
	Created       time.Time         `json:"created"`
	Tables        []TableEntry      `json:"tables"`
	Annotations   map[string]string `json:"annotations,omitempty"`

	// Incremental chain (populated in Phase F; nil for full backups in B).
	BaseID   string `json:"base_id,omitempty"`
	ParentID string `json:"parent_id,omitempty"`
}

// TableEntry holds per-table statistics recorded at dump time.
type TableEntry struct {
	Name      string `json:"name"`
	Rows      int64  `json:"rows"`
	SizeBytes int64  `json:"size_bytes"`
}
