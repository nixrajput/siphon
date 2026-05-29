package postgres

import (
	"context"

	"github.com/nixrajput/siphon/internal/driver"
)

// inspectQuery lists user tables with row estimates and on-disk sizes.
// schemaname/relname are qualified with the pg_stat_user_tables alias (s.)
// because pg_class (c) also exposes a relname column — an unqualified
// reference is ambiguous (SQLSTATE 42702). reltuples is -1 for a table that
// has never been ANALYZEd; GREATEST clamps that to 0 so the row estimate is
// never reported as a negative number.
const inspectQuery = `
SELECT
  s.schemaname || '.' || s.relname AS name,
  GREATEST(COALESCE((SELECT reltuples::bigint FROM pg_class WHERE oid = c.oid), 0), 0) AS rows,
  pg_total_relation_size(c.oid) AS size_bytes
FROM pg_stat_user_tables s
JOIN pg_class c ON c.oid = s.relid
ORDER BY size_bytes DESC
`

func (c *Conn) Inspect(ctx context.Context) (*driver.Schema, error) {
	rows, err := c.db.QueryContext(ctx, inspectQuery)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := &driver.Schema{}
	for rows.Next() {
		var t driver.TableMeta
		if err := rows.Scan(&t.Name, &t.Rows, &t.SizeBytes); err != nil {
			return nil, err
		}
		out.Tables = append(out.Tables, t)
	}
	return out, rows.Err()
}
