package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/nixrajput/siphon/internal/app"
)

func newRestoreCmd() *cobra.Command {
	var (
		profileName string
		dumpID      string
		tables      []string
		schemaOnly  bool
		dataOnly    bool
		clean       bool
	)
	cmd := &cobra.Command{
		Use:   "restore [dump-id]",
		Short: "Load a dump into a database",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) == 1 {
				dumpID = args[0]
			}
			if dumpID == "" {
				return fmt.Errorf("dump-id is required (positional or --dump)")
			}
			deps, err := buildDeps()
			if err != nil {
				return err
			}
			ch, _, err := app.Restore(c.Context(), deps, app.RestoreOpts{
				Profile:      profileName,
				DumpID:       dumpID,
				TargetTables: tables,
				SchemaOnly:   schemaOnly,
				DataOnly:     dataOnly,
				Clean:        clean,
			})
			if err != nil {
				return err
			}
			return Heartbeat(c.ErrOrStderr(), ch)
		},
	}
	cmd.Flags().StringVar(&profileName, "profile", "", "Target profile to restore into")
	cmd.Flags().StringVar(&dumpID, "dump", "", "Dump ID (alternative to positional)")
	cmd.Flags().StringSliceVar(&tables, "table", nil, "Restore only these tables")
	cmd.Flags().BoolVar(&schemaOnly, "schema-only", false, "Schema, no data")
	cmd.Flags().BoolVar(&dataOnly, "data-only", false, "Data, no schema")
	cmd.Flags().BoolVar(&clean, "clean", false, "DROP objects before recreating")
	return cmd
}
