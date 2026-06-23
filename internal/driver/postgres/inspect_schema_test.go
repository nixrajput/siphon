package postgres

import (
	"testing"

	"github.com/nixrajput/siphon/internal/canonical"
)

func TestMapPGType(t *testing.T) {
	cases := []struct {
		input  string
		want   canonical.CanonicalType
		wantOK bool
	}{
		{"integer", canonical.CTInt, true},
		{"smallint", canonical.CTInt, true},
		{"bigint", canonical.CTBigInt, true},
		{"boolean", canonical.CTBoolean, true},
		{"character varying", canonical.CTVarchar, true},
		{"text", canonical.CTText, true},
		{"numeric", canonical.CTNumeric, true},
		{"decimal", canonical.CTNumeric, true},
		{"uuid", canonical.CTUUID, true},
		{"timestamp with time zone", canonical.CTTimestampTZ, true},
		{"json", canonical.CTJSON, true},
		{"jsonb", canonical.CTJSON, true},
		{"bytea", "", false},    // unmapped: surfaces as an error, not silent text
		{"geometry", "", false}, // unmapped custom type
	}
	for _, tc := range cases {
		got, ok := mapPGType(tc.input)
		if got != tc.want || ok != tc.wantOK {
			t.Errorf("mapPGType(%q) = (%q, %v); want (%q, %v)", tc.input, got, ok, tc.want, tc.wantOK)
		}
	}
}
