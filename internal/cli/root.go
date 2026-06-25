// Package cli builds the Cobra command tree. Bare invocation routes to
// the TUI; subcommands wire into the application verbs.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/nixrajput/siphon/internal/app"
	"github.com/nixrajput/siphon/internal/config"
	"github.com/nixrajput/siphon/internal/dumps"
	"github.com/nixrajput/siphon/internal/errs"
	"github.com/nixrajput/siphon/internal/jobs"
	"github.com/nixrajput/siphon/internal/profile"
	"github.com/nixrajput/siphon/internal/secrets"
	"github.com/nixrajput/siphon/internal/storage"
	"github.com/nixrajput/siphon/internal/tui"
)

// Version is overwritten at build time via -ldflags "-X github.com/nixrajput/siphon/internal/cli.Version=..."
var Version = "0.0.1-dev"

// NewRoot builds a fresh root command. Out and Err allow tests to capture output.
func NewRoot(out, err io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "siphon",
		Short:         "Sync any database, anywhere.",
		Long:          "siphon — backup, restore, and synchronize databases across engines.",
		Version:       Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) == 0 {
				deps, depErr := buildDeps()
				if depErr != nil {
					return depErr
				}
				return tui.Run(deps)
			}
			return c.Help()
		},
	}
	cmd.SetOut(out)
	cmd.SetErr(err)

	cmd.AddCommand(
		newBackupCmd(),
		newRestoreCmd(),
		newSyncCmd(),
		newCdcCmd(),
		newVerifyCmd(),
		newInspectCmd(),
		newProfileCmd(),
		newDumpsCmd(),
		newConfigCmd(),
		newScheduleCmd(),
		newTunnelCmd(),
	)
	return cmd
}

// buildDeps builds the Deps used by every verb.
func buildDeps() (app.Deps, error) {
	cfg, err := config.Load()
	if err != nil {
		return app.Deps{}, err
	}
	res := secrets.NewResolver(secrets.Env{}, secrets.Passthrough{})
	ps := profile.New(cfg, res, config.Save)

	store, err := buildStore(cfg)
	if err != nil {
		return app.Deps{}, err
	}
	cat := dumps.New(store)

	return app.Deps{
		Profiles: ps,
		Dumps:    cat,
		Runner:   jobs.NewRunner(),
		Drivers:  app.DefaultDrivers(),
	}, nil
}

// buildStore selects the dump-catalog storage backend from config. Type "s3"
// builds an S3-backed store; anything else (the default) uses the local
// filesystem rooted at Defaults.DumpDir (or the XDG share dir when unset).
func buildStore(cfg *config.Config) (storage.Store, error) {
	if cfg.Storage.Type == "s3" {
		st, err := storage.NewS3(context.Background(), storage.S3Options{
			Bucket:   cfg.Storage.Bucket,
			Prefix:   cfg.Storage.Prefix,
			Region:   cfg.Storage.Region,
			Endpoint: cfg.Storage.Endpoint,
		})
		if err != nil {
			return nil, fmt.Errorf("init s3 storage: %w", err)
		}
		return st, nil
	}

	dumpDir := cfg.Defaults.DumpDir
	if dumpDir == "" {
		home, homeErr := os.UserHomeDir()
		if homeErr != nil {
			return nil, fmt.Errorf("resolve home dir for default dump_dir: %w", homeErr)
		}
		dumpDir = filepath.Join(home, ".local", "share", "siphon", "dumps")
	}
	return storage.NewLocal(dumpDir)
}

func newScheduleCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "schedule",
		Short: "Cron-managed recurring backups (Phase G)",
		RunE:  func(*cobra.Command, []string) error { return fmt.Errorf("schedule: not implemented (Phase G)") },
	}
}

func newTunnelCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tunnel",
		Short: "SSH tunnel helper (Phase G)",
		RunE:  func(*cobra.Command, []string) error { return fmt.Errorf("tunnel: not implemented (Phase G)") },
	}
}

// Execute runs the root command using stdout/stderr and returns the POSIX
// exit code. It is the only function main() calls.
//
// If the verb returns a typed *errs.Error, Execute reports its Code via
// the exit-code taxonomy (§4.2 of the spec). Untyped errors fall back to
// CodeUser (1).
func Execute() int {
	root := NewRoot(os.Stdout, os.Stderr)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "✗", err)
		var e *errs.Error
		if errors.As(err, &e) {
			return e.Code.ExitCode()
		}
		return 1
	}
	return 0
}
