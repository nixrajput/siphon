package cli

import (
	"github.com/spf13/cobra"

	"github.com/nixrajput/siphon/internal/app"
	"github.com/nixrajput/siphon/internal/errs"
)

func newCdcCmd() *cobra.Command {
	var fromName, toName string
	cmd := &cobra.Command{
		Use:   "cdc [from] [to]",
		Short: "Continuously stream source changes to a target (CDC)",
		Long: "cdc tails the source database's logical change stream and applies each " +
			"change to the target, continuously, until interrupted. It works both " +
			"same-engine and cross-engine (changes are translated through the canonical " +
			"format). On first run it takes an initial snapshot, then follows changes; " +
			"on restart it resumes from the last saved position. Press Ctrl-C to stop cleanly.",
		Args: cobra.MaximumNArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) >= 1 {
				fromName = args[0]
			}
			if len(args) >= 2 {
				toName = args[1]
			}
			if fromName == "" || toName == "" {
				return &errs.Error{
					Op:   "cdc",
					Code: errs.CodeUser,
					Hint: "cdc requires both source and target profiles (positional args or --from/--to)",
				}
			}
			deps, err := buildDeps()
			if err != nil {
				return err
			}
			ch, _, err := app.RunCDC(c.Context(), deps, app.SyncOpts{
				From:       fromName,
				To:         toName,
				Continuous: true,
			})
			if err != nil {
				return err
			}
			return Heartbeat(c.ErrOrStderr(), ch)
		},
	}
	cmd.Flags().StringVar(&fromName, "from", "", "Source profile")
	cmd.Flags().StringVar(&toName, "to", "", "Target profile")
	return cmd
}
