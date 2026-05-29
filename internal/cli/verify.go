package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/nixrajput/siphon/internal/app"
)

func newVerifyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "verify <dump-id>",
		Short: "Checksum + integrity check on a dump",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			deps, err := buildDeps()
			if err != nil {
				return err
			}
			report, err := app.Verify(c.Context(), deps, args[0])
			if report != nil {
				if report.OK {
					_, _ = fmt.Fprintf(c.OutOrStdout(), "  ✓ checksum verified %s\n", report.Checksum)
				} else {
					_, _ = fmt.Fprintf(c.OutOrStdout(), "  ✗ checksum MISMATCH %s\n", report.Checksum)
				}
			}
			return err // nil on success; *errs.Error{CodeIntegrity} on mismatch → exit 3
		},
	}
	return cmd
}
