package canonical

import (
	"encoding/json"
	"strings"
	"testing"
)

// --- MapToNative -----------------------------------------------------------

func TestMapToNative_CoreTypes(t *testing.T) {
	coreTypes := []CanonicalType{
		CTInt, CTBigInt, CTText, CTBoolean, CTUUID,
		CTVarchar, CTNumeric, CTTimestampTZ, CTJSON,
	}
	for _, engine := range []string{"postgres", "mysql", "mariadb"} {
		for _, ct := range coreTypes {
			native, err := MapToNative(engine, CanonicalColumn{Name: "c", Type: ct})
			if err != nil {
				t.Errorf("MapToNative(%q, %q): unexpected error: %v", engine, ct, err)
			}
			if native == "" {
				t.Errorf("MapToNative(%q, %q): empty native type", engine, ct)
			}
		}
	}
}

func TestMapToNative_UnknownEngine(t *testing.T) {
	if _, err := MapToNative("oracle", CanonicalColumn{Type: CTInt}); err == nil {
		t.Fatal("MapToNative with unknown engine: want error, got nil")
	}
}

func TestMapToNative_UnknownType(t *testing.T) {
	if _, err := MapToNative("postgres", CanonicalColumn{Type: CanonicalType("geometry")}); err == nil {
		t.Fatal("MapToNative with unknown type: want error, got nil")
	}
}

func TestMapToNative_PrecisionDecoration(t *testing.T) {
	// VARCHAR with length.
	got, err := MapToNative("postgres", CanonicalColumn{Type: CTVarchar, Precision: 255})
	if err != nil {
		t.Fatal(err)
	}
	if got != "varchar(255)" {
		t.Errorf("varchar(255): got %q", got)
	}
	// NUMERIC with precision+scale (MySQL DECIMAL).
	got, err = MapToNative("mysql", CanonicalColumn{Type: CTNumeric, Precision: 10, Scale: 2})
	if err != nil {
		t.Fatal(err)
	}
	if got != "DECIMAL(10,2)" {
		t.Errorf("DECIMAL(10,2): got %q", got)
	}
	// Bare when precision is zero (documented v1 behavior).
	got, err = MapToNative("postgres", CanonicalColumn{Type: CTVarchar})
	if err != nil {
		t.Fatal(err)
	}
	if got != "varchar" {
		t.Errorf("bare varchar: got %q", got)
	}
}

// --- QuoteIdent (INJECTION GUARD) ------------------------------------------

func TestQuoteIdent_Postgres(t *testing.T) {
	got, err := QuoteIdent("postgres", "users")
	if err != nil {
		t.Fatal(err)
	}
	if got != `"users"` {
		t.Errorf("postgres QuoteIdent: got %q want %q", got, `"users"`)
	}
}

func TestQuoteIdent_PostgresEscapesQuote(t *testing.T) {
	got, err := QuoteIdent("postgres", `we"ird`)
	if err != nil {
		t.Fatal(err)
	}
	if got != `"we""ird"` {
		t.Errorf("postgres quote-doubling: got %q want %q", got, `"we""ird"`)
	}
}

func TestQuoteIdent_MySQL(t *testing.T) {
	got, err := QuoteIdent("mysql", "users")
	if err != nil {
		t.Fatal(err)
	}
	if got != "`users`" {
		t.Errorf("mysql QuoteIdent: got %q want %q", got, "`users`")
	}
}

func TestQuoteIdent_MySQLEscapesBacktick(t *testing.T) {
	got, err := QuoteIdent("mysql", "we`ird")
	if err != nil {
		t.Fatal(err)
	}
	if got != "`we``ird`" {
		t.Errorf("mysql backtick-doubling: got %q want %q", got, "`we``ird`")
	}
}

func TestQuoteIdent_UnknownEngine(t *testing.T) {
	if _, err := QuoteIdent("oracle", "users"); err == nil {
		t.Fatal("QuoteIdent with unknown engine: want error, got nil")
	}
}

// TestQuoteIdent_InjectionIsNeutralized is the core security assertion: a
// malicious identifier is wrapped+escaped, never passed through raw, so the
// embedded statement terminator and quote cannot break out of the identifier.
func TestQuoteIdent_InjectionIsNeutralized(t *testing.T) {
	evil := `x"; DROP TABLE y; --`

	pg, err := QuoteIdent("postgres", evil)
	if err != nil {
		t.Fatal(err)
	}
	// The inner " must be doubled, and the whole thing wrapped in quotes.
	want := `"x""; DROP TABLE y; --"`
	if pg != want {
		t.Errorf("postgres injection: got %q want %q", pg, want)
	}
	// The raw (un-doubled) attacker substring must NOT appear verbatim.
	if strings.Contains(pg, `x"; DROP`) {
		t.Errorf("postgres injection: raw quote leaked through: %q", pg)
	}

	my, err := QuoteIdent("mysql", "x`; DROP TABLE y; --")
	if err != nil {
		t.Fatal(err)
	}
	wantMy := "`x``; DROP TABLE y; --`"
	if my != wantMy {
		t.Errorf("mysql injection: got %q want %q", my, wantMy)
	}
}

// --- Placeholder -----------------------------------------------------------

func TestPlaceholder(t *testing.T) {
	cases := []struct {
		engine string
		n      int
		want   string
	}{
		{"postgres", 1, "$1"},
		{"postgres", 3, "$3"},
		{"mysql", 1, "?"},
		{"mariadb", 2, "?"},
	}
	for _, c := range cases {
		if got := Placeholder(c.engine, c.n); got != c.want {
			t.Errorf("Placeholder(%q, %d): got %q want %q", c.engine, c.n, got, c.want)
		}
	}
}

// --- SQL builders (pure, DB-free) ------------------------------------------

func twoColTable() CanonicalTable {
	return CanonicalTable{
		Name: "t",
		Columns: []CanonicalColumn{
			{Name: "id", Type: CTInt, Nullable: false},
			{Name: "name", Type: CTText, Nullable: true},
		},
	}
}

func TestBuildCreateTableSQL_Postgres(t *testing.T) {
	got, err := BuildCreateTableSQL("postgres", twoColTable())
	if err != nil {
		t.Fatal(err)
	}
	want := `CREATE TABLE IF NOT EXISTS "t" ("id" integer NOT NULL, "name" text)`
	if got != want {
		t.Errorf("postgres CREATE:\n got %q\nwant %q", got, want)
	}
}

func TestBuildCreateTableSQL_MySQL(t *testing.T) {
	got, err := BuildCreateTableSQL("mysql", twoColTable())
	if err != nil {
		t.Fatal(err)
	}
	want := "CREATE TABLE IF NOT EXISTS `t` (`id` INT NOT NULL, `name` TEXT)"
	if got != want {
		t.Errorf("mysql CREATE:\n got %q\nwant %q", got, want)
	}
}

func TestBuildCreateTableSQL_UnknownEngine(t *testing.T) {
	if _, err := BuildCreateTableSQL("oracle", twoColTable()); err == nil {
		t.Fatal("BuildCreateTableSQL unknown engine: want error, got nil")
	}
}

// TestBuildCreateTableSQL_InjectionGuard proves a hostile column name flows
// through the builder quoted+escaped, not breaking out of the DDL.
func TestBuildCreateTableSQL_InjectionGuard(t *testing.T) {
	tbl := CanonicalTable{
		Name:    "t",
		Columns: []CanonicalColumn{{Name: `evil"col`, Type: CTInt, Nullable: true}},
	}
	got, err := BuildCreateTableSQL("postgres", tbl)
	if err != nil {
		t.Fatal(err)
	}
	want := `CREATE TABLE IF NOT EXISTS "t" ("evil""col" integer)`
	if got != want {
		t.Errorf("injection-guarded CREATE:\n got %q\nwant %q", got, want)
	}
	if strings.Contains(got, `evil"col" integer)`) && !strings.Contains(got, `evil""col`) {
		t.Errorf("raw quote leaked into DDL: %q", got)
	}
}

func TestBuildInsertSQL_Postgres(t *testing.T) {
	got, err := BuildInsertSQL("postgres", "t", []string{"id", "name"})
	if err != nil {
		t.Fatal(err)
	}
	want := `INSERT INTO "t" ("id","name") VALUES ($1,$2)`
	if got != want {
		t.Errorf("postgres INSERT:\n got %q\nwant %q", got, want)
	}
}

func TestBuildInsertSQL_MySQL(t *testing.T) {
	got, err := BuildInsertSQL("mysql", "t", []string{"id", "name"})
	if err != nil {
		t.Fatal(err)
	}
	want := "INSERT INTO `t` (`id`,`name`) VALUES (?,?)"
	if got != want {
		t.Errorf("mysql INSERT:\n got %q\nwant %q", got, want)
	}
}

func TestBuildIdempotentInsertSQL_Postgres(t *testing.T) {
	got, err := BuildIdempotentInsertSQL("postgres", "t", []string{"id", "name"})
	if err != nil {
		t.Fatal(err)
	}
	want := `INSERT INTO "t" ("id","name") VALUES ($1,$2) ON CONFLICT DO NOTHING`
	if got != want {
		t.Errorf("postgres idempotent INSERT:\n got %q\nwant %q", got, want)
	}
}

func TestBuildIdempotentInsertSQL_MySQL(t *testing.T) {
	got, err := BuildIdempotentInsertSQL("mysql", "t", []string{"id", "name"})
	if err != nil {
		t.Fatal(err)
	}
	want := "INSERT IGNORE INTO `t` (`id`,`name`) VALUES (?,?)"
	if got != want {
		t.Errorf("mysql idempotent INSERT:\n got %q\nwant %q", got, want)
	}
}

func TestBuildIdempotentInsertSQL_UnknownEngine(t *testing.T) {
	if _, err := BuildIdempotentInsertSQL("oracle", "t", []string{"id"}); err == nil {
		t.Fatal("BuildIdempotentInsertSQL unknown engine: want error, got nil")
	}
}

func TestBuildInsertSQL_EmptyColumns(t *testing.T) {
	if _, err := BuildInsertSQL("postgres", "t", nil); err == nil {
		t.Fatal("BuildInsertSQL with no columns: want error, got nil")
	}
}

func TestBuildInsertSQL_UnknownEngine(t *testing.T) {
	if _, err := BuildInsertSQL("oracle", "t", []string{"id"}); err == nil {
		t.Fatal("BuildInsertSQL unknown engine: want error, got nil")
	}
}

func TestBuildSelectSQL_Postgres(t *testing.T) {
	got, err := BuildSelectSQL("postgres", twoColTable())
	if err != nil {
		t.Fatal(err)
	}
	want := `SELECT "id", "name" FROM "t"`
	if got != want {
		t.Errorf("postgres SELECT:\n got %q\nwant %q", got, want)
	}
}

func TestBuildSelectSQL_MySQL(t *testing.T) {
	got, err := BuildSelectSQL("mysql", twoColTable())
	if err != nil {
		t.Fatal(err)
	}
	want := "SELECT `id`, `name` FROM `t`"
	if got != want {
		t.Errorf("mysql SELECT:\n got %q\nwant %q", got, want)
	}
}

// --- WriteJSONL framing ----------------------------------------------------

func TestWriteJSONL_SchemaHeaderFirst(t *testing.T) {
	var sb strings.Builder
	schema := &CanonicalSchema{Tables: []CanonicalTable{twoColTable()}}
	if err := WriteJSONL(&sb, map[string]*CanonicalSchema{"schema": schema}); err != nil {
		t.Fatal(err)
	}
	out := sb.String()
	if !strings.HasPrefix(out, `{"schema":`) {
		t.Errorf("first line must start with schema key, got %q", out)
	}
	if !strings.HasSuffix(out, "\n") {
		t.Error("WriteJSONL must terminate the line with a newline")
	}
}

func TestCanonicalRow_JSONRoundTrip(t *testing.T) {
	orig := CanonicalRow{
		Table: "users",
		Values: map[string]any{
			"id":   float64(7), // JSON numbers decode as float64
			"name": "alice",
			"ok":   true,
			"nil":  nil,
		},
	}
	b, err := json.Marshal(orig)
	if err != nil {
		t.Fatal(err)
	}
	// Compact keys keep the stream small.
	if !strings.Contains(string(b), `"t":"users"`) || !strings.Contains(string(b), `"v":`) {
		t.Errorf("CanonicalRow uses compact keys t/v, got %s", b)
	}
	var back CanonicalRow
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if back.Table != orig.Table {
		t.Errorf("table mismatch: got %q want %q", back.Table, orig.Table)
	}
	if len(back.Values) != len(orig.Values) {
		t.Fatalf("values length mismatch: got %d want %d", len(back.Values), len(orig.Values))
	}
	for k, v := range orig.Values {
		if back.Values[k] != v {
			t.Errorf("value %q mismatch: got %v want %v", k, back.Values[k], v)
		}
	}
}

// TestSchemaHeader_RoundTrip confirms a schema header marshals and unmarshals
// through the same envelope ConsumeCanonical expects.
func TestSchemaHeader_RoundTrip(t *testing.T) {
	schema := &CanonicalSchema{Tables: []CanonicalTable{
		{Name: "users", Columns: []CanonicalColumn{
			{Name: "id", Type: CTBigInt, Nullable: false},
			{Name: "email", Type: CTVarchar, Nullable: true, Precision: 320},
		}},
	}}
	var sb strings.Builder
	if err := WriteJSONL(&sb, map[string]*CanonicalSchema{"schema": schema}); err != nil {
		t.Fatal(err)
	}
	var header struct {
		Schema *CanonicalSchema `json:"schema"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(sb.String())), &header); err != nil {
		t.Fatal(err)
	}
	if header.Schema == nil || len(header.Schema.Tables) != 1 {
		t.Fatalf("schema header did not round-trip: %+v", header.Schema)
	}
	got := header.Schema.Tables[0]
	if got.Name != "users" || len(got.Columns) != 2 || got.Columns[1].Precision != 320 {
		t.Errorf("schema fields lost in round-trip: %+v", got)
	}
}

// --- NormalizeScanned ------------------------------------------------------

// TestNormalizeScanned_BytesBecomeString proves a []byte value scanned from
// database/sql is converted to a string so it marshals as a JSON string and
// round-trips intact, rather than base64-encoding (which would corrupt the
// value before ConsumeCanonical re-inserts it).
func TestNormalizeScanned_BytesBecomeString(t *testing.T) {
	in := []byte("hello-world")
	got := NormalizeScanned(in)
	s, ok := got.(string)
	if !ok {
		t.Fatalf("NormalizeScanned([]byte) returned %T; want string", got)
	}
	if s != "hello-world" {
		t.Fatalf("NormalizeScanned = %q; want %q", s, "hello-world")
	}

	// Round-trip through the row marshal: a []byte value must come back as the
	// original text, NOT base64.
	row := CanonicalRow{Table: "t", Values: map[string]any{"c": NormalizeScanned([]byte("café"))}}
	b, err := json.Marshal(row)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "Y2Fmw") { // base64 prefix of "café" would leak in
		t.Fatalf("[]byte was base64-encoded, not stringified: %s", b)
	}
	var back CanonicalRow
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if back.Values["c"] != "café" {
		t.Fatalf("round-trip = %v; want %q", back.Values["c"], "café")
	}
}

// TestNormalizeScanned_NonBytesUnchanged confirms non-[]byte values pass
// through untouched.
func TestNormalizeScanned_NonBytesUnchanged(t *testing.T) {
	if got := NormalizeScanned(int64(42)); got != int64(42) {
		t.Fatalf("NormalizeScanned(int64) = %v; want 42", got)
	}
	if got := NormalizeScanned(nil); got != nil {
		t.Fatalf("NormalizeScanned(nil) = %v; want nil", got)
	}
}

// --- CanonicalChange JSON round-trip ----------------------------------------

func TestCanonicalChange_JSONRoundTrip(t *testing.T) {
	orig := CanonicalChange{
		Op:    OpUpdate,
		Table: "orders",
		Key:   map[string]any{"id": float64(42)},
		Values: map[string]any{
			"id":     float64(42),
			"status": "shipped",
		},
	}
	b, err := json.Marshal(orig)
	if err != nil {
		t.Fatal(err)
	}
	var back CanonicalChange
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if back.Op != orig.Op {
		t.Errorf("Op: got %q want %q", back.Op, orig.Op)
	}
	if back.Table != orig.Table {
		t.Errorf("Table: got %q want %q", back.Table, orig.Table)
	}
	if back.Key["id"] != orig.Key["id"] {
		t.Errorf("Key[id]: got %v want %v", back.Key["id"], orig.Key["id"])
	}
	if back.Values["status"] != orig.Values["status"] {
		t.Errorf("Values[status]: got %v want %v", back.Values["status"], orig.Values["status"])
	}
}

// --- BuildUpdateSQL ----------------------------------------------------------

func TestBuildUpdateSQL_Postgres(t *testing.T) {
	got, err := BuildUpdateSQL("postgres", "users", []string{"name", "email"}, []string{"id"})
	if err != nil {
		t.Fatal(err)
	}
	want := `UPDATE "users" SET "name" = $1, "email" = $2 WHERE "id" = $3`
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestBuildUpdateSQL_MySQL_CompositeKey(t *testing.T) {
	got, err := BuildUpdateSQL("mysql", "t", []string{"v"}, []string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	want := "UPDATE `t` SET `v` = ? WHERE `a` = ? AND `b` = ?"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestBuildUpdateSQL_InjectionGuard(t *testing.T) {
	got, err := BuildUpdateSQL("postgres", "t", []string{`evil"col`}, []string{"id"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, `"evil""col"`) {
		t.Fatalf("identifier not quoted/escaped: %s", got)
	}
}

func TestBuildUpdateSQL_UnknownEngine(t *testing.T) {
	if _, err := BuildUpdateSQL("oracle", "t", []string{"v"}, []string{"id"}); err == nil {
		t.Fatal("BuildUpdateSQL unknown engine: want error, got nil")
	}
}

// --- BuildDeleteSQL ----------------------------------------------------------

func TestBuildDeleteSQL_Postgres(t *testing.T) {
	got, err := BuildDeleteSQL("postgres", "users", []string{"id"})
	if err != nil {
		t.Fatal(err)
	}
	want := `DELETE FROM "users" WHERE "id" = $1`
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestBuildDeleteSQL_MySQL_CompositeKey(t *testing.T) {
	got, err := BuildDeleteSQL("mysql", "t", []string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	want := "DELETE FROM `t` WHERE `a` = ? AND `b` = ?"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestBuildDeleteSQL_UnknownEngine(t *testing.T) {
	if _, err := BuildDeleteSQL("oracle", "t", []string{"id"}); err == nil {
		t.Fatal("BuildDeleteSQL unknown engine: want error, got nil")
	}
}

// --- BuildCreateTableSQL with PRIMARY KEY ------------------------------------

func TestBuildCreateTableSQL_WithPrimaryKey(t *testing.T) {
	tbl := CanonicalTable{Name: "users", Columns: []CanonicalColumn{
		{Name: "id", Type: CTInt, PrimaryKey: true},
		{Name: "name", Type: CTText},
	}}
	got, err := BuildCreateTableSQL("postgres", tbl)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, `PRIMARY KEY ("id")`) {
		t.Fatalf("missing PK clause: %s", got)
	}
}

// --- ChangeColumns -----------------------------------------------------------

func TestChangeColumns_Basic(t *testing.T) {
	ch := CanonicalChange{
		Op:     OpUpdate,
		Table:  "t",
		Key:    map[string]any{"id": 1},
		Values: map[string]any{"id": 1, "name": "x"},
	}
	set, key := ChangeColumns(ch)
	if len(key) != 1 || key[0] != "id" {
		t.Fatalf("key cols = %v want [id]", key)
	}
	if len(set) != 1 || set[0] != "name" {
		t.Fatalf("set cols = %v want [name]", set)
	}
}

func TestChangeColumns_MultipleKeyAndSet(t *testing.T) {
	ch := CanonicalChange{
		Op:     OpUpdate,
		Table:  "t",
		Key:    map[string]any{"b": 2, "a": 1},
		Values: map[string]any{"a": 1, "b": 2, "z": "v", "m": "w"},
	}
	set, key := ChangeColumns(ch)
	if len(key) != 2 || key[0] != "a" || key[1] != "b" {
		t.Fatalf("key cols = %v want [a b]", key)
	}
	if len(set) != 2 || set[0] != "m" || set[1] != "z" {
		t.Fatalf("set cols = %v want [m z]", set)
	}
}

// --- ValidateChangeKey -------------------------------------------------------

func TestValidateChangeKey_InsertNoKey_OK(t *testing.T) {
	ch := CanonicalChange{Op: OpInsert, Table: "t"}
	if err := ValidateChangeKey(ch); err != nil {
		t.Fatalf("INSERT with no key: got %v want nil", err)
	}
}

func TestValidateChangeKey_UpdateNoKey_Error(t *testing.T) {
	ch := CanonicalChange{Op: OpUpdate, Table: "orders"}
	if err := ValidateChangeKey(ch); err == nil {
		t.Fatal("UPDATE with no key: want error, got nil")
	}
}

func TestValidateChangeKey_DeleteNoKey_Error(t *testing.T) {
	ch := CanonicalChange{Op: OpDelete, Table: "orders"}
	if err := ValidateChangeKey(ch); err == nil {
		t.Fatal("DELETE with no key: want error, got nil")
	}
}

func TestValidateChangeKey_UpdateWithKey_OK(t *testing.T) {
	ch := CanonicalChange{Op: OpUpdate, Table: "t", Key: map[string]any{"id": 1}}
	if err := ValidateChangeKey(ch); err != nil {
		t.Fatalf("UPDATE with key: got %v want nil", err)
	}
}
