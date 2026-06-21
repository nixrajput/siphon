package mysqlcommon

import (
	"reflect"
	"testing"

	"github.com/nixrajput/siphon/internal/driver"
)

func TestBuildDumpArgs_Defaults(t *testing.T) {
	p := driver.Profile{Host: "db.local", Port: 3306, User: "root", Database: "shop"}
	got := BuildDumpArgs(p, driver.BackupOpts{})
	want := []string{
		"-h", "db.local",
		"-P", "3306",
		"-u", "root",
		"--single-transaction",
		"--routines",
		"--triggers",
		"--events",
		"--no-tablespaces",
		"--skip-comments",
		"shop",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BuildDumpArgs() = %v\nwant %v", got, want)
	}
}

func TestBuildDumpArgs_Tables(t *testing.T) {
	p := driver.Profile{Host: "db.local", Port: 3306, User: "root", Database: "shop"}
	opt := driver.BackupOpts{
		SchemaOnly:    true,
		IncludeTables: []string{"orders", "items"},
		ExcludeTables: []string{"sessions"},
	}
	got := BuildDumpArgs(p, opt)
	want := []string{
		"-h", "db.local",
		"-P", "3306",
		"-u", "root",
		"--single-transaction",
		"--routines",
		"--triggers",
		"--events",
		"--no-tablespaces",
		"--skip-comments",
		"shop",
		"--no-data",
		"orders", "items",
		"--ignore-table=shop.sessions",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BuildDumpArgs() = %v\nwant %v", got, want)
	}
}

func TestDSN(t *testing.T) {
	p := driver.Profile{Host: "db.local", Port: 3306, User: "root", Password: "pw", Database: "shop"}
	got := DSN(p)
	want := "root:pw@tcp(db.local:3306)/shop?parseTime=true&tls=preferred"
	if got != want {
		t.Fatalf("DSN() = %q\nwant %q", got, want)
	}
}

func TestTLSParam(t *testing.T) {
	cases := map[string]string{
		"disable":     "false",
		"require":     "true",
		"verify-ca":   "true",
		"verify-full": "true",
		"":            "preferred",
		"prefer":      "preferred",
	}
	for mode, want := range cases {
		if got := tlsParam(mode); got != want {
			t.Errorf("tlsParam(%q) = %q; want %q", mode, got, want)
		}
	}
}
