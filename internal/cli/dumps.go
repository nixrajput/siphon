package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/nixrajput/siphon/internal/dumps"
)

func newDumpsCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "dumps", Short: "List, inspect, and prune saved dumps"}
	cmd.AddCommand(dumpsListCmd(), dumpsInspectCmd(), dumpsPruneCmd())
	return cmd
}

func dumpsListCmd() *cobra.Command {
	return &cobra.Command{
		Use: "list", Short: "List saved dumps newest first",
		RunE: func(c *cobra.Command, _ []string) error {
			deps, err := buildDeps()
			if err != nil {
				return err
			}
			all, err := deps.Dumps.List()
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(c.OutOrStdout(), "%-28s %-12s %-20s %10s\n", "ID", "PROFILE", "CREATED", "SIZE")
			for _, m := range all {
				_, _ = fmt.Fprintf(c.OutOrStdout(), "%-28s %-12s %-20s %10d\n", m.ID, m.Profile, m.Created.Format(time.RFC3339), m.SizeBytes)
			}
			return nil
		},
	}
}

func dumpsInspectCmd() *cobra.Command {
	return &cobra.Command{
		Use: "inspect <id>", Short: "Show metadata for a dump", Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			deps, err := buildDeps()
			if err != nil {
				return err
			}
			m, err := deps.Dumps.ReadMeta(args[0])
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(c.OutOrStdout(), "id:       %s\nprofile:  %s\ndriver:   %s\nformat:   %s\nsize:     %d bytes\nchecksum: %s\ncreated:  %s\n",
				m.ID, m.Profile, m.Driver, m.DumpFormat, m.SizeBytes, m.Checksum, m.Created.Format(time.RFC3339))
			return nil
		},
	}
}

func dumpsPruneCmd() *cobra.Command {
	var maxAge time.Duration
	var apply bool
	cmd := &cobra.Command{
		Use: "prune", Short: "Show or apply retention policy",
		RunE: func(c *cobra.Command, _ []string) error {
			deps, err := buildDeps()
			if err != nil {
				return err
			}
			rep, err := deps.Dumps.PruneDryRun(dumps.RetentionPolicy{MaxAge: maxAge})
			if err != nil {
				return err
			}
			for _, m := range rep.Would {
				if apply {
					_ = deps.Dumps.Delete(m.ID)
					_, _ = fmt.Fprintf(c.OutOrStdout(), "  ✗ deleted %s\n", m.ID)
				} else {
					_, _ = fmt.Fprintf(c.OutOrStdout(), "  would delete %s\n", m.ID)
				}
			}
			return nil
		},
	}
	cmd.Flags().DurationVar(&maxAge, "max-age", 0, "Delete dumps older than this duration")
	cmd.Flags().BoolVar(&apply, "apply", false, "Actually delete (default is dry-run)")
	return cmd
}
