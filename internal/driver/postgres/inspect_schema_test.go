package postgres

import (
	"testing"

	"github.com/nixrajput/siphon/internal/canonical"
)

func TestMapPGType(t *testing.T) {
	cases := []struct {
		input string
		want  canonical.CanonicalType
	}{
		{"integer", canonical.CTInt},
		{"smallint", canonical.CTInt},
		{"bigint", canonical.CTBigInt},
		{"boolean", canonical.CTBoolean},
		{"character varying", canonical.CTVarchar},
		{"text", canonical.CTText},
		{"numeric", canonical.CTNumeric},
		{"decimal", canonical.CTNumeric},
		{"uuid", canonical.CTUUID},
		{"timestamp with time zone", canonical.CTTimestampTZ},
		{"json", canonical.CTJSON},
		{"jsonb", canonical.CTJSON},
		{"bytea", canonical.CTText}, // fallback
	}
	for _, tc := range cases {
		got := mapPGType(tc.input)
		if got != tc.want {
			t.Errorf("mapPGType(%q) = %q; want %q", tc.input, got, tc.want)
		}
	}
}
