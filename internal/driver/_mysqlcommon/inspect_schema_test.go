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
		wantOK     bool
	}{
		{"int", "int", canonical.CTInt, true},
		{"smallint", "smallint(6)", canonical.CTInt, true},
		{"mediumint", "mediumint(9)", canonical.CTInt, true},
		{"tinyint", "tinyint(1)", canonical.CTBoolean, true},
		{"tinyint", "tinyint(4)", canonical.CTInt, true},
		{"bigint", "bigint(20)", canonical.CTBigInt, true},
		{"varchar", "varchar(255)", canonical.CTVarchar, true},
		{"text", "text", canonical.CTText, true},
		{"longtext", "longtext", canonical.CTText, true},
		{"mediumtext", "mediumtext", canonical.CTText, true},
		{"decimal", "decimal(10,2)", canonical.CTNumeric, true},
		{"char", "char(36)", canonical.CTUUID, true},
		{"char", "char(10)", canonical.CTText, true},
		{"datetime", "datetime", canonical.CTTimestampTZ, true},
		{"timestamp", "timestamp", canonical.CTTimestampTZ, true},
		{"json", "json", canonical.CTJSON, true},
		{"blob", "blob", "", false},               // unmapped: surfaces as an error, not silent text
		{"varbinary", "varbinary(16)", "", false}, // unmapped binary family
	}
	for _, tc := range cases {
		got, ok := mapMySQLType(tc.dataType, tc.columnType)
		if got != tc.want || ok != tc.wantOK {
			t.Errorf("mapMySQLType(%q, %q) = (%q, %v); want (%q, %v)", tc.dataType, tc.columnType, got, ok, tc.want, tc.wantOK)
		}
	}
}
