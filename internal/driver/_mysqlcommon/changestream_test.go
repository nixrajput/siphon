package mysqlcommon

import (
	"reflect"
	"strings"
	"testing"

	"github.com/nixrajput/siphon/internal/canonical"
)

func TestParseColAssign(t *testing.T) {
	tests := []struct {
		body    string
		wantN   int
		wantVal string
		wantOK  bool
	}{
		{"@1=42", 1, "42", true},
		{"@2='wrench'", 2, "wrench", true},
		{"@3='a''b'", 3, "a'b", true},
		{"@4=NULL", 4, "NULL", true},
		{"@5='x' /* VARSTRING(60) meta=60 nullable=1 is_null=0 */", 5, "x", true},
		{"not an assign", 0, "", false},
	}
	for _, tt := range tests {
		n, val, ok := parseColAssign(tt.body)
		if ok != tt.wantOK || n != tt.wantN || val != tt.wantVal {
			t.Errorf("parseColAssign(%q) = (%d, %q, %v), want (%d, %q, %v)",
				tt.body, n, val, ok, tt.wantN, tt.wantVal, tt.wantOK)
		}
	}
}

func TestTableFromRef(t *testing.T) {
	tests := map[string]string{
		"`test`.`widgets`":           "widgets",
		"test.widgets":               "widgets",
		"`widgets`":                  "widgets",
		"`db`.`tbl` WHERE something": "tbl",
	}
	for in, want := range tests {
		if got := tableFromRef(in); got != want {
			t.Errorf("tableFromRef(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseAtMarker(t *testing.T) {
	if p, ok := parseAtMarker("# at 1234"); !ok || p != 1234 {
		t.Errorf("parseAtMarker = (%d,%v), want (1234,true)", p, ok)
	}
	if _, ok := parseAtMarker("### INSERT INTO x"); ok {
		t.Error("parseAtMarker matched a ### line")
	}
}

// fakeRowsReader feeds a canned mysqlbinlog --verbose transcript through the
// parser. It builds CanonicalChanges WITHOUT a DB by pre-seeding the layout
// cache, so column names and PKs resolve offline.
func TestParseBinlogRows(t *testing.T) {
	transcript := `# at 4
#250101 12:00:00 server id 1  end_log_pos 120 	Write_rows: table id 10 flags: STMT_END_F
### INSERT INTO ` + "`test`.`widgets`" + `
### SET
###   @1=1
###   @2='wrench'
# at 200
#250101 12:00:01 server id 1  end_log_pos 280 	Update_rows: table id 10 flags: STMT_END_F
### UPDATE ` + "`test`.`widgets`" + `
### WHERE
###   @1=1
###   @2='wrench'
### SET
###   @1=1
###   @2='spanner'
# at 360
#250101 12:00:02 server id 1  end_log_pos 420 	Delete_rows: table id 10 flags: STMT_END_F
### DELETE FROM ` + "`test`.`widgets`" + `
### WHERE
###   @1=1
###   @2='spanner'
`
	meta := &tableMetaCache{cache: map[string]*tableLayout{
		"widgets": {cols: []string{"id", "name"}, pk: map[string]bool{"id": true}},
	}}

	var got []canonical.CanonicalChange
	emit := func(ch canonical.CanonicalChange) error { got = append(got, ch); return nil }

	pos, err := parseBinlogRows(strings.NewReader(transcript), meta, emit, BinlogPosition{File: "mysql-bin.000001"})
	if err != nil {
		t.Fatalf("parseBinlogRows: %v", err)
	}
	if pos.Position != 360 {
		t.Errorf("final pos = %d, want 360 (last # at marker)", pos.Position)
	}
	if len(got) != 3 {
		t.Fatalf("got %d changes, want 3: %+v", len(got), got)
	}

	want := []canonical.CanonicalChange{
		{Op: canonical.OpInsert, Table: "widgets", Key: map[string]any{"id": "1"}, Values: map[string]any{"id": "1", "name": "wrench"}},
		{Op: canonical.OpUpdate, Table: "widgets", Key: map[string]any{"id": "1"}, Values: map[string]any{"id": "1", "name": "spanner"}},
		{Op: canonical.OpDelete, Table: "widgets", Key: map[string]any{"id": "1"}},
	}
	for i := range want {
		if !reflect.DeepEqual(got[i], want[i]) {
			t.Errorf("change[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestBinlogValueNull(t *testing.T) {
	if binlogValue("NULL") != nil {
		t.Error("NULL should map to nil")
	}
	if binlogValue("x") != "x" {
		t.Error("non-NULL should pass through")
	}
}
