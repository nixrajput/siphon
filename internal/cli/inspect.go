package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/nixrajput/siphon/internal/app"
)

func newInspectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inspect <profile>",
		Short: "Show tables, sizes, schema",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			deps, err := buildDeps()
			if err != nil {
				return err
			}
			schema, err := app.Inspect(c.Context(), deps, args[0])
			if err != nil {
				return err
			}
			out := c.OutOrStdout()
			_, _ = fmt.Fprintf(out, "%-50s %15s %15s\n", "TABLE", "ROWS", "SIZE")
			for _, t := range schema.Tables {
				_, _ = fmt.Fprintf(out, "%-50s %15d %15d\n", t.Name, t.Rows, t.SizeBytes)
			}
			return nil
		},
	}
	return cmd
}
