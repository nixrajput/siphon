package mysqlcommon

import (
	"context"
	"fmt"
	"strings"

	"github.com/nixrajput/siphon/internal/canonical"
)

// InspectSchema queries information_schema for tables and primary keys,
// returning a CanonicalSchema.
func (c *Conn) InspectSchema(ctx context.Context) (*canonical.CanonicalSchema, error) {
	const colQuery = `
SELECT TABLE_NAME, COLUMN_NAME, DATA_TYPE, COLUMN_TYPE, IS_NULLABLE,
       CHARACTER_MAXIMUM_LENGTH, NUMERIC_PRECISION, NUMERIC_SCALE
FROM information_schema.COLUMNS
WHERE TABLE_SCHEMA = ?
ORDER BY TABLE_NAME, ORDINAL_POSITION
`
	rows, err := c.db.QueryContext(ctx, colQuery, c.p.Database)
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
			columnType string
			isNullable string
			charMaxLen *int64
			numPrec    *int64
			numScale   *int64
		)
		if err := rows.Scan(&tableName, &colName, &dataType, &columnType, &isNullable, &charMaxLen, &numPrec, &numScale); err != nil {
			return nil, err
		}
		if _, ok := tableMap[tableName]; !ok {
			tableMap[tableName] = &canonical.CanonicalTable{Name: tableName}
			tableOrder = append(tableOrder, tableName)
		}
		ctype, ok := mapMySQLType(dataType, columnType)
		if !ok {
			return nil, fmt.Errorf("cross-engine: unsupported mysql type %q (%q) on %s.%s", dataType, columnType, tableName, colName)
		}
		col := canonical.CanonicalColumn{
			Name:     colName,
			Type:     ctype,
			Nullable: strings.EqualFold(isNullable, "YES"),
		}
		if charMaxLen != nil {
			col.Precision = int(*charMaxLen)
		}
		if numPrec != nil {
			col.Precision = int(*numPrec)
		}
		if numScale != nil {
			col.Scale = int(*numScale)
		}
		tableMap[tableName].Columns = append(tableMap[tableName].Columns, col)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Fetch primary keys
	const pkQuery = `
SELECT TABLE_NAME, COLUMN_NAME
FROM information_schema.KEY_COLUMN_USAGE
WHERE TABLE_SCHEMA = ?
  AND CONSTRAINT_NAME = 'PRIMARY'
ORDER BY TABLE_NAME, ORDINAL_POSITION
`
	pkRows, err := c.db.QueryContext(ctx, pkQuery, c.p.Database)
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

// mapMySQLType maps MySQL/MariaDB information_schema DATA_TYPE and COLUMN_TYPE
// to a CanonicalType. The bool is false for an unmapped type: the caller turns
// that into an explicit error rather than silently coercing to text, which would
// rewrite schema semantics for binary/BLOB families and corrupt data fidelity
// during cross-engine transfer.
func mapMySQLType(dataType, columnType string) (canonical.CanonicalType, bool) {
	dt := strings.ToLower(dataType)
	ct := strings.ToLower(columnType)
	switch dt {
	case "int", "smallint", "mediumint":
		return canonical.CTInt, true
	case "tinyint":
		if ct == "tinyint(1)" {
			return canonical.CTBoolean, true
		}
		return canonical.CTInt, true
	case "bigint":
		return canonical.CTBigInt, true
	case "varchar":
		return canonical.CTVarchar, true
	case "text", "longtext", "mediumtext":
		return canonical.CTText, true
	case "decimal":
		return canonical.CTNumeric, true
	case "char":
		if ct == "char(36)" {
			return canonical.CTUUID, true
		}
		return canonical.CTText, true
	case "datetime", "timestamp":
		return canonical.CTTimestampTZ, true
	case "json":
		return canonical.CTJSON, true
	default:
		return "", false
	}
}
