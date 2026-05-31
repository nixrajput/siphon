package cli

import (
	"github.com/spf13/cobra"

	"github.com/nixrajput/siphon/internal/app"
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
	)
	cmd := &cobra.Command{
		Use:   "backup [profile]",
		Short: "Dump a database to a file",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) == 1 {
				profileName = args[0]
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
	return cmd
}
