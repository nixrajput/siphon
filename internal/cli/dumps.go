package cli

import (
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/nixrajput/siphon/internal/app"
	"github.com/nixrajput/siphon/internal/config"
	"github.com/nixrajput/siphon/internal/dumps"
	"github.com/nixrajput/siphon/internal/errs"
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
			all, err := deps.Dumps.List(c.Context())
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
			m, err := deps.Dumps.ReadMeta(c.Context(), args[0])
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
	var (
		profileName               string
		keepLast                  int
		maxAge                    time.Duration
		gfsDaily, gfsWeekly       int
		gfsMonthly                int
		apply                     bool
		keepLastSet, maxAgeSet    bool
		gfsDSet, gfsWSet, gfsMSet bool
	)
	cmd := &cobra.Command{
		Use: "prune", Short: "Apply a retention policy, pruning whole dump chains",
		Long: "prune groups the catalog into restorable chains (a base plus its " +
			"incrementals) and keeps or deletes each chain as a unit, so an " +
			"incremental is never orphaned from its base. The policy comes from the " +
			"config retention block (per-profile override, else defaults); the flags " +
			"below override it per run. Dry-run by default — pass --apply to delete.",
		RunE: func(c *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			policy := resolveRetentionPolicy(cfg, profileName)
			// CLI flags override the config-derived policy, but only when set, so
			// an unset flag does not zero out a configured rule.
			if keepLastSet {
				policy.KeepLast = keepLast
			}
			if maxAgeSet {
				policy.MaxAge = maxAge
			}
			if gfsDSet {
				policy.GFS.Daily = gfsDaily
			}
			if gfsWSet {
				policy.GFS.Weekly = gfsWeekly
			}
			if gfsMSet {
				policy.GFS.Monthly = gfsMonthly
			}

			deps, err := buildDeps()
			if err != nil {
				return err
			}
			res, err := app.Prune(c.Context(), deps, app.PruneOpts{
				Profile: profileName, Policy: policy, Apply: apply,
			})
			if err != nil {
				return err
			}
			renderPruneResult(c.OutOrStdout(), res)
			if res.Failed > 0 {
				return &errs.Error{Op: "prune", Code: errs.CodeSystem, Hint: "some dumps could not be deleted"}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&profileName, "profile", "", "Limit pruning to one profile's dumps")
	cmd.Flags().IntVar(&keepLast, "keep-last", 0, "Keep the N newest chains")
	cmd.Flags().DurationVar(&maxAge, "max-age", 0, "Keep chains younger than this duration")
	cmd.Flags().IntVar(&gfsDaily, "gfs-daily", 0, "GFS: keep newest chain in each of N recent days")
	cmd.Flags().IntVar(&gfsWeekly, "gfs-weekly", 0, "GFS: keep newest chain in each of N recent ISO weeks")
	cmd.Flags().IntVar(&gfsMonthly, "gfs-monthly", 0, "GFS: keep newest chain in each of N recent months")
	cmd.Flags().BoolVar(&apply, "apply", false, "Actually delete (default is dry-run)")
	// Record which override flags were explicitly set so unset flags don't clobber config.
	cmd.PreRun = func(c *cobra.Command, _ []string) {
		keepLastSet = c.Flags().Changed("keep-last")
		maxAgeSet = c.Flags().Changed("max-age")
		gfsDSet = c.Flags().Changed("gfs-daily")
		gfsWSet = c.Flags().Changed("gfs-weekly")
		gfsMSet = c.Flags().Changed("gfs-monthly")
	}
	return cmd
}

// resolveRetentionPolicy maps a profile's effective config retention block into
// a dumps.RetentionPolicy. A nil block (none configured) yields the zero policy
// — keep everything. The map duration is pre-validated at config load, so the
// parse here cannot fail in practice; a parse error degrades to "no max-age".
func resolveRetentionPolicy(cfg *config.Config, profile string) dumps.RetentionPolicy {
	rc := cfg.EffectiveRetention(profile)
	if rc == nil {
		return dumps.RetentionPolicy{}
	}
	var maxAge time.Duration
	if rc.MaxAge != "" {
		maxAge, _ = time.ParseDuration(rc.MaxAge)
	}
	return dumps.RetentionPolicy{
		KeepLast: rc.KeepLast,
		MaxAge:   maxAge,
		GFS:      dumps.GFSPolicy{Daily: rc.GFS.Daily, Weekly: rc.GFS.Weekly, Monthly: rc.GFS.Monthly},
	}
}

// renderPruneResult prints the plan: kept chains, then pruned chains with their
// member dumps and (on apply) deletion outcome.
func renderPruneResult(w io.Writer, res *app.PruneResult) {
	verb := "would prune"
	if res.Apply {
		verb = "pruned"
	}
	var prunedChains int
	for _, oc := range res.Outcomes {
		if !oc.Pruned {
			continue
		}
		prunedChains++
		_, _ = fmt.Fprintf(w, "%s chain %s (%d dump(s), %d bytes)\n", verb, oc.Root, len(oc.DumpIDs), oc.SizeBytes)
		for _, e := range oc.Errors {
			_, _ = fmt.Fprintf(w, "  ! %s\n", e)
		}
	}
	if prunedChains == 0 {
		_, _ = fmt.Fprintln(w, "nothing to prune under the current policy")
		return
	}
	if res.Apply {
		_, _ = fmt.Fprintf(w, "reclaimed %d bytes; %d deletion failure(s)\n", res.Reclaimed, res.Failed)
	} else {
		_, _ = fmt.Fprintf(w, "%d chain(s) would be pruned (dry-run; pass --apply to delete)\n", prunedChains)
	}
}
