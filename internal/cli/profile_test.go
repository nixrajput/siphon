package cli

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/nixrajput/siphon/internal/config"
)

// runProfileAdd runs `siphon profile add t <args>` against a temp config, with
// in as stdin. It returns the stored password and everything written to stderr.
func runProfileAdd(t *testing.T, in io.Reader, args ...string) (string, string, error) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("SIPHON_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)        // buildDeps creates the default dump dir under home
	t.Setenv("USERPROFILE", dir) // the home dir on Windows
	var out, errb bytes.Buffer
	root := NewRoot(&out, &errb)
	root.SetIn(in)
	root.SetArgs(append([]string{"profile", "add", "t"}, args...))
	if err := root.Execute(); err != nil {
		return "", errb.String(), err
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	return cfg.Profiles["t"].Password, errb.String(), nil
}

func TestProfileAdd_PasswordStdin(t *testing.T) {
	got, stderr, err := runProfileAdd(t, strings.NewReader("s3cret\r\n"), "--password-stdin")
	if err != nil {
		t.Fatalf("profile add --password-stdin: %v", err)
	}
	if got != "s3cret" {
		t.Errorf("password = %q; want %q", got, "s3cret")
	}
	if stderr != "" {
		t.Errorf("unexpected stderr: %q", stderr)
	}
}

func TestProfileAdd_PasswordStdinRejectsEmpty(t *testing.T) {
	_, _, err := runProfileAdd(t, strings.NewReader("\n"), "--password-stdin")
	if err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("err = %v; want an empty-password error", err)
	}
}

func TestProfileAdd_PasswordFlagsAreExclusive(t *testing.T) {
	_, _, err := runProfileAdd(t, strings.NewReader("s3cret"), "--password", "x", "--password-stdin")
	if err == nil || !strings.Contains(err.Error(), "none of the others") {
		t.Fatalf("err = %v; want a mutually-exclusive flags error", err)
	}
}

func TestProfileAdd_LiteralPasswordFlagWarns(t *testing.T) {
	got, stderr, err := runProfileAdd(t, strings.NewReader(""), "--password", "s3cret")
	if err != nil {
		t.Fatalf("profile add --password: %v", err)
	}
	if got != "s3cret" {
		t.Errorf("password = %q; want %q", got, "s3cret")
	}
	if !strings.Contains(stderr, "--password-stdin") {
		t.Errorf("stderr = %q; want a warning that recommends --password-stdin", stderr)
	}
	if strings.Contains(stderr, "s3cret") {
		t.Errorf("warning echoes the password: %q", stderr)
	}
}

func TestProfileAdd_SecretRefPasswordFlagDoesNotWarn(t *testing.T) {
	for _, ref := range []string{"env:DB_PASS", "keychain://prod-db", "awssm://prod/db#password"} {
		got, stderr, err := runProfileAdd(t, strings.NewReader(""), "--password", ref)
		if err != nil {
			t.Fatalf("profile add --password %s: %v", ref, err)
		}
		if got != ref {
			t.Errorf("password = %q; want the ref stored as given", got)
		}
		if stderr != "" {
			t.Errorf("%s is a ref, not a secret, so no warning; stderr = %q", ref, stderr)
		}
	}
}

func TestProfileAdd_NoPromptWithoutTerminal(t *testing.T) {
	got, stderr, err := runProfileAdd(t, strings.NewReader("not-read\n"))
	if err != nil {
		t.Fatalf("profile add: %v", err)
	}
	if got != "" || stderr != "" {
		t.Errorf("password = %q, stderr = %q; want both empty when stdin is not a terminal", got, stderr)
	}
}

func TestProfileAdd_PromptsOnTerminal(t *testing.T) {
	r, w, err := os.Pipe() // an *os.File stdin; isTerminal is stubbed to call it a terminal
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	origIsTerminal, origRead := isTerminal, readPassword
	t.Cleanup(func() { isTerminal, readPassword = origIsTerminal, origRead })
	isTerminal = func(int) bool { return true }
	readPassword = func(int) ([]byte, error) { return []byte("typed"), nil }

	got, stderr, err := runProfileAdd(t, r)
	if err != nil {
		t.Fatalf("profile add: %v", err)
	}
	if got != "typed" {
		t.Errorf("password = %q; want the prompted value", got)
	}
	if !strings.Contains(stderr, "Password: ") || strings.Contains(stderr, "typed") {
		t.Errorf("stderr = %q; want the prompt without the password", stderr)
	}
}
