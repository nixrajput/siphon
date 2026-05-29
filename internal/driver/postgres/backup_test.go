package postgres

import (
	"reflect"
	"testing"

	"github.com/nixrajput/siphon/internal/driver"
)

func TestBuildBackupArgs_Defaults(t *testing.T) {
	got := buildBackupArgs(
		driver.Profile{Host: "h", Port: 5432, User: "u", Database: "d"},
		driver.BackupOpts{},
	)
	want := []string{
		"-h", "h", "-p", "5432", "-U", "u", "-d", "d",
		"-Fc", "-Z", "1", "--no-owner", "--no-acl", "--verbose",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v; want %v", got, want)
	}
}

// TestBuildBackupArgs_TablesAndParallelIgnored verifies that Parallel is
// accepted in opts but NOT forwarded to pg_dump (-j is directory-format only).
func TestBuildBackupArgs_TablesAndParallelIgnored(t *testing.T) {
	got := buildBackupArgs(
		driver.Profile{Host: "h", Port: 5432, User: "u", Database: "d"},
		driver.BackupOpts{
			IncludeTables:   []string{"users", "orders"},
			ExcludeTables:   []string{"logs"},
			ExcludeDataFrom: []string{"audit"},
			Parallel:        4, // intentionally ignored by buildBackupArgs; used by pg_restore
		},
	)
	want := []string{
		"-h", "h", "-p", "5432", "-U", "u", "-d", "d",
		"-Fc", "-Z", "1", "--no-owner", "--no-acl", "--verbose",
		"-t", "users", "-t", "orders",
		"-T", "logs",
		"--exclude-table-data", "audit",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v; want %v", got, want)
	}
}

func TestCompressionLevel_Clamps(t *testing.T) {
	cases := map[int]int{-1: 1, 0: 1, 1: 1, 5: 5, 9: 9, 10: 9}
	for in, want := range cases {
		if got := compressionLevel(in); got != want {
			t.Fatalf("compressionLevel(%d) = %d; want %d", in, got, want)
		}
	}
}
