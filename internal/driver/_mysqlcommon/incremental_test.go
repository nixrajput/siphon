package mysqlcommon

import (
	"reflect"
	"slices"
	"testing"

	"github.com/nixrajput/siphon/internal/driver"
)

func TestBinlogArgs(t *testing.T) {
	p := driver.Profile{Host: "db.local", Port: 3306, User: "root", Database: "shop"}
	since := BinlogPosition{File: "mysql-bin.000123", Position: 4096}
	got := binlogArgs(p, since, "mysqlbinlog")
	want := []string{
		"-h", "db.local",
		"-P", "3306",
		"-u", "root",
		"--read-from-remote-server",
		"--to-last-log",
		"--start-position=4096",
		"--ssl-mode=PREFERRED", // no SSLMode set => PREFERRED for mysqlbinlog
		"mysql-bin.000123",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("binlogArgs() = %v\nwant %v", got, want)
	}
	// The starting binlog file must remain the final positional arg.
	if got[len(got)-1] != "mysql-bin.000123" {
		t.Fatalf("binlog file not last arg: %v", got)
	}
}

// TestBinlogArgs_SSL asserts the fork-specific TLS flags: mysqlbinlog maps
// SSLMode to --ssl-mode=<level>, while mariadb-binlog uses --ssl / --skip-ssl.
func TestBinlogArgs_SSL(t *testing.T) {
	p := driver.Profile{Host: "h", Port: 3306, User: "u", SSLMode: "require"}
	since := BinlogPosition{File: "bin.1", Position: 0}

	mysql := binlogArgs(p, since, "mysqlbinlog")
	if !slices.Contains(mysql, "--ssl-mode=REQUIRED") {
		t.Fatalf("mysqlbinlog with require: missing --ssl-mode=REQUIRED; got %v", mysql)
	}

	maria := binlogArgs(p, since, "mariadb-binlog")
	if !slices.Contains(maria, "--ssl") {
		t.Fatalf("mariadb-binlog with require: missing --ssl; got %v", maria)
	}

	// disable maps to --skip-ssl for mariadb-binlog and DISABLED for mysqlbinlog.
	pDis := driver.Profile{Host: "h", Port: 3306, User: "u", SSLMode: "disable"}
	if got := binlogArgs(pDis, since, "mariadb-binlog"); !slices.Contains(got, "--skip-ssl") {
		t.Fatalf("mariadb-binlog with disable: missing --skip-ssl; got %v", got)
	}
	if got := binlogArgs(pDis, since, "mysqlbinlog"); !slices.Contains(got, "--ssl-mode=DISABLED") {
		t.Fatalf("mysqlbinlog with disable: missing --ssl-mode=DISABLED; got %v", got)
	}
}
