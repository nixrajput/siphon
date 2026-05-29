package postgres

import (
	"reflect"
	"testing"

	"github.com/nixrajput/siphon/internal/driver"
)

func TestBuildRestoreArgs_Defaults(t *testing.T) {
	got := buildRestoreArgs(
		driver.Profile{Host: "h", Port: 5432, User: "u", Database: "d"},
		driver.RestoreOpts{},
	)
	want := []string{
		"-h", "h", "-p", "5432", "-U", "u", "-d", "d",
		"--no-owner", "--no-acl", "--verbose",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v; want %v", got, want)
	}
}

func TestBuildRestoreArgs_CleanAndTables(t *testing.T) {
	got := buildRestoreArgs(
		driver.Profile{Host: "h", Port: 5432, User: "u", Database: "d"},
		driver.RestoreOpts{
			Clean:        true,
			TargetTables: []string{"users", "orders"},
		},
	)
	want := []string{
		"-h", "h", "-p", "5432", "-U", "u", "-d", "d",
		"--no-owner", "--no-acl", "--verbose",
		"--clean", "--if-exists",
		"-t", "users", "-t", "orders",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v; want %v", got, want)
	}
}
