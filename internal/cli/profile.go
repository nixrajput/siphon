package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/nixrajput/siphon/internal/config"
	"github.com/nixrajput/siphon/internal/errs"
)

// Swapped in tests: a test process has no terminal to read a password from.
var (
	isTerminal   = term.IsTerminal
	readPassword = term.ReadPassword
)

func newProfileCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "profile", Short: "Manage named connection profiles"}
	cmd.AddCommand(
		profileListCmd(),
		profileAddCmd(),
		profileRmCmd(),
		profileShowCmd(),
	)
	return cmd
}

func profileListCmd() *cobra.Command {
	return &cobra.Command{
		Use: "list", Short: "List profiles",
		RunE: func(c *cobra.Command, _ []string) error {
			deps, err := buildDeps()
			if err != nil {
				return err
			}
			for _, n := range deps.Profiles.List() {
				p, _ := deps.Profiles.Get(n)
				_, _ = fmt.Fprintf(c.OutOrStdout(), "%-20s %-10s %s\n", n, p.Driver, p.Host)
			}
			return nil
		},
	}
}

func profileAddCmd() *cobra.Command {
	var p config.ProfileConfig
	var name string
	var passwordStdin bool
	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Add a new profile",
		Long: `Add a new profile.

Give the password in one of these ways:
  --password-stdin     read it from stdin: printf '%s' "$PASS" | siphon profile add ...
  (no password flag)   siphon prompts for it without echo when stdin is a terminal
  --password env:VAR   store a secret ref (env:, keychain://, awssm://), not the secret

A literal --password value is visible to other users in the process list
and stays in your shell history.`,
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			name = args[0]
			deps, err := buildDeps()
			if err != nil {
				return err
			}
			if p.Password, err = profilePassword(c, p.Password, passwordStdin); err != nil {
				return err
			}
			return deps.Profiles.Add(name, p)
		},
	}
	cmd.Flags().StringVar(&p.Driver, "driver", "postgres", "Driver name")
	cmd.Flags().StringVar(&p.Host, "host", "", "Host")
	cmd.Flags().IntVar(&p.Port, "port", 5432, "Port")
	cmd.Flags().StringVar(&p.User, "user", "", "User")
	cmd.Flags().StringVar(&p.Password, "password", "", "SecretRef (e.g. env:VAR) or literal password (visible in ps and shell history)")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "Read the password from stdin")
	cmd.Flags().StringVar(&p.Database, "database", "", "Database name")
	cmd.Flags().StringVar(&p.SSLMode, "sslmode", "prefer", "SSL mode")
	cmd.MarkFlagsMutuallyExclusive("password", "password-stdin")
	return cmd
}

// profilePassword returns the password for `profile add`: read from stdin with
// --password-stdin, else the --password value (with a warning for a literal),
// else a no-echo prompt when stdin is a terminal.
func profilePassword(c *cobra.Command, flag string, fromStdin bool) (string, error) {
	switch {
	case fromStdin:
		b, err := io.ReadAll(c.InOrStdin())
		if err != nil {
			return "", &errs.Error{Op: "profile.add", Code: errs.CodeUser, Cause: err, Hint: "could not read the password from stdin"}
		}
		pw := strings.TrimRight(string(b), "\r\n")
		if pw == "" {
			return "", &errs.Error{Op: "profile.add", Code: errs.CodeUser, Hint: "--password-stdin read an empty password"}
		}
		return pw, nil
	case c.Flags().Changed("password"):
		if flag != "" && !isSecretRef(flag) {
			_, _ = fmt.Fprintln(c.ErrOrStderr(), "warning: a literal --password is visible to other users in the process list and stays in your shell history; use --password-stdin, a secret ref (env:VAR), or omit it to be prompted")
		}
		return flag, nil
	}
	f, ok := c.InOrStdin().(*os.File)
	if !ok || !isTerminal(int(f.Fd())) {
		return "", nil
	}
	_, _ = fmt.Fprint(c.ErrOrStderr(), "Password: ")
	b, err := readPassword(int(f.Fd()))
	_, _ = fmt.Fprintln(c.ErrOrStderr()) // ReadPassword leaves the cursor on the prompt line
	if err != nil {
		return "", &errs.Error{Op: "profile.add", Code: errs.CodeUser, Cause: err, Hint: "could not read the password"}
	}
	return string(b), nil
}

func profileRmCmd() *cobra.Command {
	return &cobra.Command{
		Use: "rm <name>", Short: "Remove a profile", Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			deps, err := buildDeps()
			if err != nil {
				return err
			}
			return deps.Profiles.Remove(args[0])
		},
	}
}

func profileShowCmd() *cobra.Command {
	return &cobra.Command{
		Use: "show <name>", Short: "Show a profile (secret refs shown; literal passwords masked)", Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			deps, err := buildDeps()
			if err != nil {
				return err
			}
			p, err := deps.Profiles.Get(args[0])
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(c.OutOrStdout(), "driver:   %s\nhost:     %s\nport:     %d\nuser:     %s\ndatabase: %s\nsslmode:  %s\npassword: %s\n",
				p.Driver, p.Host, p.Port, p.User, p.Database, p.SSLMode, maskSecret(p.Password))
			return nil
		},
	}
}

// maskSecret returns ref-style secrets (e.g. "env:VAR") verbatim — they are
// pointers, not the secret itself — but masks literal passwords so `show`
// never leaks a cleartext credential to stdout / shell scrollback.
func maskSecret(v string) string {
	if v == "" || isSecretRef(v) {
		return v
	}
	return "••••••••"
}

func isSecretRef(v string) bool {
	for _, scheme := range []string{"env:", "keychain:", "awssm:", "vault:"} {
		if strings.HasPrefix(v, scheme) {
			return true
		}
	}
	return false
}
