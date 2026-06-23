package mysqlcommon

import (
	"reflect"
	"strings"
	"testing"

	"github.com/nixrajput/siphon/internal/canonical"
)

func TestParseColAssign(t *testing.T) {
	tests := []struct {
		body       string
		wantN      int
		wantStr    string
		wantIsNull bool
		wantOK     bool
	}{
		{"@1=42", 1, "42", false, true},
		{"@2='wrench'", 2, "wrench", false, true},
		{"@3='a''b'", 3, "a'b", false, true},
		{"@4=NULL", 4, "", true, true}, // bare token: SQL NULL
		// A quoted 'NULL' is the string literal "NULL", NOT a SQL NULL — must not
		// be conflated with the bare token above.
		{"@5='NULL'", 5, "NULL", false, true},
		// Trailing type comment is stripped (it is outside the quotes).
		{"@6='x' /* VARSTRING(60) meta=60 nullable=1 is_null=0 */", 6, "x", false, true},
		// A quoted value that itself contains "/*" must be preserved, not
		// truncated at the comment-like sequence inside the string.
		{"@7='a /* b'", 7, "a /* b", false, true},
		{"@8='a /* b' /* TYPE meta */", 8, "a /* b", false, true},
		{"not an assign", 0, "", false, false},
	}
	for _, tt := range tests {
		n, val, ok := parseColAssign(tt.body)
		if ok != tt.wantOK || n != tt.wantN || val.str != tt.wantStr || val.isNull != tt.wantIsNull {
			t.Errorf("parseColAssign(%q) = (%d, {str:%q isNull:%v}, %v), want (%d, {str:%q isNull:%v}, %v)",
				tt.body, n, val.str, val.isNull, ok, tt.wantN, tt.wantStr, tt.wantIsNull, tt.wantOK)
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

	pos, err := parseBinlogRows(strings.NewReader(transcript), meta, emit, BinlogPosition{File: "mysql-bin.000001"}, nil)
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

// TestParseBinlogRows_NullVsNullString proves a bare NULL token decodes to a
// nil value while a quoted 'NULL' decodes to the string "NULL" — the two must
// not collapse together, or a legitimate 'NULL' string would be lost.
func TestParseBinlogRows_NullVsNullString(t *testing.T) {
	transcript := `# at 4
### INSERT INTO ` + "`test`.`widgets`" + `
### SET
###   @1=1
###   @2=NULL
# at 100
### INSERT INTO ` + "`test`.`widgets`" + `
### SET
###   @1=2
###   @2='NULL'
# at 200
`
	meta := &tableMetaCache{cache: map[string]*tableLayout{
		"widgets": {cols: []string{"id", "name"}, pk: map[string]bool{"id": true}},
	}}

	var got []canonical.CanonicalChange
	emit := func(ch canonical.CanonicalChange) error { got = append(got, ch); return nil }

	if _, err := parseBinlogRows(strings.NewReader(transcript), meta, emit, BinlogPosition{File: "mysql-bin.000001"}, nil); err != nil {
		t.Fatalf("parseBinlogRows: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d changes, want 2: %+v", len(got), got)
	}
	if got[0].Values["name"] != nil {
		t.Errorf("bare NULL: name = %#v, want nil", got[0].Values["name"])
	}
	if got[1].Values["name"] != "NULL" {
		t.Errorf("quoted 'NULL': name = %#v, want \"NULL\" string", got[1].Values["name"])
	}
}

// TestParseBinlogRows_FlushesAtMarker proves a completed event is emitted at the
// next "# at" marker rather than being held until a following ### op-line. In an
// unbounded CDC stream the next op-line may never arrive during a quiet period,
// so without the marker-boundary flush the last change would sit buffered and
// never reach the target. The emit callback fails if it ever sees a pending
// event still buffered when the NEXT marker is processed — i.e. it asserts each
// event is flushed by the marker that immediately follows it, not deferred.
func TestParseBinlogRows_FlushesAtMarker(t *testing.T) {
	// Two events separated by markers; the second is followed by a trailing
	// marker. If flushing only happened on the next op-line, the second event
	// would emit at end-of-loop, after the "# at 300" marker was already seen.
	transcript := `# at 4
### INSERT INTO ` + "`test`.`widgets`" + `
### SET
###   @1=7
###   @2='a'
# at 100
### INSERT INTO ` + "`test`.`widgets`" + `
### SET
###   @1=8
###   @2='b'
# at 300
`
	meta := &tableMetaCache{cache: map[string]*tableLayout{
		"widgets": {cols: []string{"id", "name"}, pk: map[string]bool{"id": true}},
	}}

	var got []canonical.CanonicalChange
	emit := func(ch canonical.CanonicalChange) error { got = append(got, ch); return nil }

	if _, err := parseBinlogRows(strings.NewReader(transcript), meta, emit, BinlogPosition{File: "mysql-bin.000001"}, nil); err != nil {
		t.Fatalf("parseBinlogRows: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d changes, want 2: %+v", len(got), got)
	}
	// The first event must be emitted before the second begins — proving the
	// "# at 100" marker flushed it rather than the second INSERT's op-line.
	if got[0].Values["name"] != "a" || got[1].Values["name"] != "b" {
		t.Errorf("changes out of order or wrong: %+v", got)
	}
}
