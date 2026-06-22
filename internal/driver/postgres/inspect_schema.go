package postgres

import (
	"context"
	"strings"

	"github.com/nixrajput/siphon/internal/canonical"
)

// InspectSchema queries information_schema for tables (public schema) and their
// primary key columns, returning a CanonicalSchema.
func (c *Conn) InspectSchema(ctx context.Context) (*canonical.CanonicalSchema, error) {
	const colQuery = `
SELECT table_name, column_name, data_type, is_nullable, character_maximum_length, numeric_precision, numeric_scale
FROM information_schema.columns
WHERE table_schema = 'public'
ORDER BY table_name, ordinal_position
`
	rows, err := c.db.QueryContext(ctx, colQuery)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	tableMap := make(map[string]*canonical.CanonicalTable)
	var tableOrder []string

	for rows.Next() {
		var (
			tableName  string
			colName    string
			dataType   string
			isNullable string
			charMaxLen *int
			numPrec    *int
			numScale   *int
		)
		if err := rows.Scan(&tableName, &colName, &dataType, &isNullable, &charMaxLen, &numPrec, &numScale); err != nil {
			return nil, err
		}
		if _, ok := tableMap[tableName]; !ok {
			tableMap[tableName] = &canonical.CanonicalTable{Name: tableName}
			tableOrder = append(tableOrder, tableName)
		}
		col := canonical.CanonicalColumn{
			Name:     colName,
			Type:     mapPGType(dataType),
			Nullable: isNullable == "YES",
		}
		if charMaxLen != nil {
			col.Precision = *charMaxLen
		}
		if numPrec != nil {
			col.Precision = *numPrec
		}
		if numScale != nil {
			col.Scale = *numScale
		}
		tableMap[tableName].Columns = append(tableMap[tableName].Columns, col)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Fetch primary key columns
	const pkQuery = `
SELECT tc.table_name, kcu.column_name
FROM information_schema.table_constraints tc
JOIN information_schema.key_column_usage kcu
  ON kcu.constraint_name = tc.constraint_name
  AND kcu.table_schema = tc.table_schema
  AND kcu.table_name = tc.table_name
WHERE tc.constraint_type = 'PRIMARY KEY'
  AND tc.table_schema = 'public'
ORDER BY tc.table_name, kcu.ordinal_position
`
	pkRows, err := c.db.QueryContext(ctx, pkQuery)
	if err != nil {
		return nil, err
	}
	defer func() { _ = pkRows.Close() }()

	pkSet := make(map[string]map[string]bool)
	for pkRows.Next() {
		var tbl, col string
		if err := pkRows.Scan(&tbl, &col); err != nil {
			return nil, err
		}
		if pkSet[tbl] == nil {
			pkSet[tbl] = make(map[string]bool)
		}
		pkSet[tbl][col] = true
	}
	if err := pkRows.Err(); err != nil {
		return nil, err
	}

	// Mark PrimaryKey on columns
	for tblName, tbl := range tableMap {
		if pks, ok := pkSet[tblName]; ok {
			for i := range tbl.Columns {
				if pks[tbl.Columns[i].Name] {
					tbl.Columns[i].PrimaryKey = true
				}
			}
		}
	}

	schema := &canonical.CanonicalSchema{}
	for _, name := range tableOrder {
		schema.Tables = append(schema.Tables, *tableMap[name])
	}
	return schema, nil
}

// mapPGType maps a Postgres information_schema data_type string to a CanonicalType.
func mapPGType(dataType string) canonical.CanonicalType {
	switch strings.ToLower(dataType) {
	case "integer", "smallint":
		return canonical.CTInt
	case "bigint":
		return canonical.CTBigInt
	case "boolean":
		return canonical.CTBoolean
	case "character varying":
		return canonical.CTVarchar
	case "text":
		return canonical.CTText
	case "numeric", "decimal":
		return canonical.CTNumeric
	case "uuid":
		return canonical.CTUUID
	case "timestamp with time zone":
		return canonical.CTTimestampTZ
	case "json", "jsonb":
		return canonical.CTJSON
	default:
		return canonical.CTText
	}
}
