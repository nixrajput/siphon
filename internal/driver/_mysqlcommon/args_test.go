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
		"--ssl-mode=PREFERRED",
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
		"--ssl-mode=PREFERRED",
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

func TestBuildDumpArgs_DataOnly(t *testing.T) {
	p := driver.Profile{Host: "db.local", Port: 3306, User: "root", Database: "shop"}
	got := BuildDumpArgs(p, driver.BackupOpts{DataOnly: true})
	want := []string{
		"-h", "db.local",
		"-P", "3306",
		"-u", "root",
		"--ssl-mode=PREFERRED",
		"--single-transaction",
		"--routines",
		"--triggers",
		"--events",
		"--no-tablespaces",
		"--skip-comments",
		"shop",
		"--no-create-info",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BuildDumpArgs() = %v\nwant %v", got, want)
	}
}

func TestBuildDumpArgs_MultiExclude(t *testing.T) {
	p := driver.Profile{Host: "db.local", Port: 3306, User: "root", Database: "shop"}
	opt := driver.BackupOpts{ExcludeTables: []string{"sessions", "cache"}}
	got := BuildDumpArgs(p, opt)
	want := []string{
		"-h", "db.local",
		"-P", "3306",
		"-u", "root",
		"--ssl-mode=PREFERRED",
		"--single-transaction",
		"--routines",
		"--triggers",
		"--events",
		"--no-tablespaces",
		"--skip-comments",
		"shop",
		"--ignore-table=shop.sessions",
		"--ignore-table=shop.cache",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BuildDumpArgs() = %v\nwant %v", got, want)
	}
}

func TestBuildRestoreArgs_Defaults(t *testing.T) {
	p := driver.Profile{Host: "db.local", Port: 3306, User: "root", Database: "shop"}
	got := BuildRestoreArgs(p, driver.RestoreOpts{})
	want := []string{
		"-h", "db.local",
		"-P", "3306",
		"-u", "root",
		"--ssl-mode=PREFERRED",
		"--default-character-set=utf8mb4",
		"shop",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BuildRestoreArgs() = %v\nwant %v", got, want)
	}
}

// TestBuildArgs_SSLModePropagated verifies the profile's SSLMode reaches the
// dump/restore CLI tools as --ssl-mode (not just the connect DSN).
func TestBuildArgs_SSLModePropagated(t *testing.T) {
	p := driver.Profile{Host: "h", Port: 3306, User: "u", Database: "d", SSLMode: "verify-full"}
	dump := BuildDumpArgs(p, driver.BackupOpts{})
	if !contains(dump, "--ssl-mode=VERIFY_IDENTITY") {
		t.Fatalf("BuildDumpArgs missing --ssl-mode=VERIFY_IDENTITY: %v", dump)
	}
	restore := BuildRestoreArgs(p, driver.RestoreOpts{})
	if !contains(restore, "--ssl-mode=VERIFY_IDENTITY") {
		t.Fatalf("BuildRestoreArgs missing --ssl-mode=VERIFY_IDENTITY: %v", restore)
	}
}

func TestCLISSLMode(t *testing.T) {
	cases := map[string]string{
		"disable":     "DISABLED",
		"require":     "REQUIRED",
		"verify-ca":   "VERIFY_CA",
		"verify-full": "VERIFY_IDENTITY",
		"":            "PREFERRED",
		"prefer":      "PREFERRED",
	}
	for mode, want := range cases {
		if got := cliSSLMode(mode); got != want {
			t.Errorf("cliSSLMode(%q) = %q; want %q", mode, got, want)
		}
	}
}

func contains(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
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
