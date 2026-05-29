package cli

import (
	"github.com/spf13/cobra"

	"github.com/nixrajput/siphon/internal/app"
)

func newSyncCmd() *cobra.Command {
	var (
		fromName, toName string
		stream           bool
		tables           []string
	)
	cmd := &cobra.Command{
		Use:   "sync [from] [to]",
		Short: "Backup + restore in one pass",
		Args:  cobra.MaximumNArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) >= 1 {
				fromName = args[0]
			}
			if len(args) >= 2 {
				toName = args[1]
			}
			deps, err := buildDeps()
			if err != nil {
				return err
			}
			ch, _, err := app.Sync(c.Context(), deps, app.SyncOpts{
				From: fromName, To: toName, Stream: stream, Tables: tables,
			})
			if err != nil {
				return err
			}
			return Heartbeat(c.ErrOrStderr(), ch)
		},
	}
	cmd.Flags().StringVar(&fromName, "from", "", "Source profile")
	cmd.Flags().StringVar(&toName, "to", "", "Target profile")
	cmd.Flags().BoolVar(&stream, "stream", true, "Stream source→target without intermediate file")
	cmd.Flags().StringSliceVar(&tables, "table", nil, "Limit to these tables")
	return cmd
}
