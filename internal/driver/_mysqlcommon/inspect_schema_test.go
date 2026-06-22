package mysqlcommon

import (
	"testing"

	"github.com/nixrajput/siphon/internal/canonical"
)

func TestMapMySQLType(t *testing.T) {
	cases := []struct {
		dataType   string
		columnType string
		want       canonical.CanonicalType
	}{
		{"int", "int", canonical.CTInt},
		{"smallint", "smallint(6)", canonical.CTInt},
		{"mediumint", "mediumint(9)", canonical.CTInt},
		{"tinyint", "tinyint(1)", canonical.CTBoolean},
		{"tinyint", "tinyint(4)", canonical.CTInt},
		{"bigint", "bigint(20)", canonical.CTBigInt},
		{"varchar", "varchar(255)", canonical.CTVarchar},
		{"text", "text", canonical.CTText},
		{"longtext", "longtext", canonical.CTText},
		{"mediumtext", "mediumtext", canonical.CTText},
		{"decimal", "decimal(10,2)", canonical.CTNumeric},
		{"char", "char(36)", canonical.CTUUID},
		{"char", "char(10)", canonical.CTText},
		{"datetime", "datetime", canonical.CTTimestampTZ},
		{"timestamp", "timestamp", canonical.CTTimestampTZ},
		{"json", "json", canonical.CTJSON},
		{"blob", "blob", canonical.CTText}, // fallback
	}
	for _, tc := range cases {
		got := mapMySQLType(tc.dataType, tc.columnType)
		if got != tc.want {
			t.Errorf("mapMySQLType(%q, %q) = %q; want %q", tc.dataType, tc.columnType, got, tc.want)
		}
	}
}
