package mysqlcommon

import (
	"reflect"
	"testing"

	"github.com/nixrajput/siphon/internal/driver"
)

func TestBinlogArgs(t *testing.T) {
	p := driver.Profile{Host: "db.local", Port: 3306, User: "root", Database: "shop"}
	since := BinlogPosition{File: "mysql-bin.000123", Position: 4096}
	got := binlogArgs(p, since)
	want := []string{
		"-h", "db.local",
		"-P", "3306",
		"-u", "root",
		"--read-from-remote-server",
		"--to-last-log",
		"--start-position=4096",
		"mysql-bin.000123",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("binlogArgs() = %v\nwant %v", got, want)
	}
}
