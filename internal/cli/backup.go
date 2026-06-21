package cli

import (
	"github.com/spf13/cobra"

	"github.com/nixrajput/siphon/internal/app"
	"github.com/nixrajput/siphon/internal/errs"
)

func newBackupCmd() *cobra.Command {
	var (
		profileName    string
		includeTables  []string
		excludeTables  []string
		excludeData    []string
		schemaOnly     bool
		dataOnly       bool
		parallel       int
		compressionLvl int
		incremental    bool
		baseID         string
	)
	cmd := &cobra.Command{
		Use:   "backup [profile]",
		Short: "Dump a database to a file",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) == 1 {
				profileName = args[0]
			}
			// Incremental backup is scaffolded but not wired end-to-end. Full
			// wiring needs: (a) reading --base's envelope for the parent
			// WAL/binlog position, (b) an optional-interface dance to reach the
			// driver's BackupIncremental (it is on the concrete *Conn types, not
			// driver.Conn), and (c) writing an incremental-type envelope with
			// BaseID/ParentID. It also needs a live DB to verify, plus the
			// tracked runtime gates: orphan replication-slot cleanup for
			// Postgres and binlog-position validation for MySQL/MariaDB. Tracked
			// as a Phase F follow-up; rejected honestly here rather than
			// half-wired into app.Backup.
			if incremental {
				return &errs.Error{
					Op:    "backup.incremental",
					Code:  errs.CodeUser,
					Cause: errs.ErrDriverUnsupported,
					Hint:  "incremental backup is not yet wired end-to-end (Phase F follow-up); the driver-level machinery exists",
				}
			}
			deps, err := buildDeps()
			if err != nil {
				return err
			}
			ch, _, err := app.Backup(c.Context(), deps, app.BackupOpts{
				Profile:          profileName,
				IncludeTables:    includeTables,
				ExcludeTables:    excludeTables,
				ExcludeDataFrom:  excludeData,
				SchemaOnly:       schemaOnly,
				DataOnly:         dataOnly,
				Parallel:         parallel,
				CompressionLevel: compressionLvl,
			})
			if err != nil {
				return err
			}
			return Heartbeat(c.ErrOrStderr(), ch)
		},
	}
	cmd.Flags().StringVar(&profileName, "profile", "", "Profile name (alternative to positional)")
	cmd.Flags().StringSliceVar(&includeTables, "table", nil, "Only dump these tables (repeatable)")
	cmd.Flags().StringSliceVar(&excludeTables, "exclude-table", nil, "Exclude tables (repeatable)")
	cmd.Flags().StringSliceVar(&excludeData, "exclude-data", nil, "Keep schema but drop data for these tables")
	cmd.Flags().BoolVar(&schemaOnly, "schema-only", false, "Schema, no data")
	cmd.Flags().BoolVar(&dataOnly, "data-only", false, "Data, no schema")
	cmd.Flags().IntVar(&parallel, "jobs", 1, "Parallel workers (not yet effective for backup; Phase F)")
	cmd.Flags().IntVar(&compressionLvl, "compression", 1, "Compression level 0-9")
	cmd.Flags().BoolVar(&incremental, "incremental", false, "Capture only changes since --base (not yet wired end-to-end; Phase F follow-up)")
	cmd.Flags().StringVar(&baseID, "base", "", "Base dump ID for an incremental backup (used with --incremental)")
	return cmd
}
