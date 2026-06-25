package cli

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nixrajput/siphon/internal/errs"
	"github.com/nixrajput/siphon/internal/schedule"
)

// newScheduleCmd manages a siphon-owned block of recurring-backup entries in the
// user's crontab. siphon does not run a scheduler — the host cron invokes
// `siphon backup <profile>` on the given expression.
func newScheduleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "schedule",
		Short: "Manage cron-scheduled recurring backups",
		Long: "schedule maintains a siphon-owned block in your crontab that runs " +
			"`siphon backup <profile>` on a cron expression. siphon does not run a " +
			"daemon — your system's cron runs the jobs. Requires the `crontab` command.",
	}
	cmd.AddCommand(scheduleListCmd(), scheduleAddCmd(), scheduleRemoveCmd())
	return cmd
}

func scheduleListCmd() *cobra.Command {
	return &cobra.Command{
		Use: "list", Short: "List scheduled backups",
		RunE: func(c *cobra.Command, _ []string) error {
			tab, err := readCrontab()
			if err != nil {
				return err
			}
			entries := schedule.List(tab)
			if len(entries) == 0 {
				_, _ = fmt.Fprintln(c.OutOrStdout(), "no scheduled backups")
				return nil
			}
			for _, e := range entries {
				_, _ = fmt.Fprintf(c.OutOrStdout(), "%-20s %s\n", e.Profile, e.Cron)
			}
			return nil
		},
	}
}

func scheduleAddCmd() *cobra.Command {
	var cron string
	cmd := &cobra.Command{
		Use: "add <profile>", Short: "Schedule (or reschedule) a recurring backup", Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if strings.TrimSpace(cron) == "" {
				return &errs.Error{Op: "schedule.add", Code: errs.CodeUser, Hint: "--cron is required (e.g. --cron \"0 2 * * *\")"}
			}
			// The managed-block parser round-trips standard 5-field expressions
			// only; reject @daily / 6-field specs up front so an entry can't be
			// installed and then vanish from list/remove.
			if len(strings.Fields(cron)) != 5 {
				return &errs.Error{Op: "schedule.add", Code: errs.CodeUser, Hint: "--cron must be a standard 5-field cron expression (e.g. \"0 2 * * *\")"}
			}
			bin, err := os.Executable()
			if err != nil {
				bin = "siphon" // fall back to PATH lookup at cron time
			}
			tab, err := readCrontab()
			if err != nil {
				return err
			}
			updated := schedule.Add(tab, bin, schedule.Entry{Profile: args[0], Cron: cron})
			if err := writeCrontab(updated); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(c.OutOrStdout(), "scheduled %s: %s\n", args[0], cron)
			return nil
		},
	}
	cmd.Flags().StringVar(&cron, "cron", "", "Cron expression, e.g. \"0 2 * * *\" (daily at 02:00)")
	return cmd
}

func scheduleRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use: "remove <profile>", Short: "Remove a scheduled backup", Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			bin, err := os.Executable()
			if err != nil {
				bin = "siphon"
			}
			tab, err := readCrontab()
			if err != nil {
				return err
			}
			updated := schedule.Remove(tab, bin, args[0])
			if err := writeCrontab(updated); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(c.OutOrStdout(), "removed schedule for %s\n", args[0])
			return nil
		},
	}
}

// readCrontab returns the current user crontab, or "" when none is installed
// (`crontab -l` exits non-zero with "no crontab" — treated as empty, not error).
func readCrontab() (string, error) {
	out, err := exec.Command("crontab", "-l").Output()
	if err != nil {
		// Only the known "no crontab for user" path means empty. An ExitError
		// with code 1 and no stdout is cron's empty-table signal on most
		// platforms; any other failure (missing binary, permission) propagates so
		// `schedule list` never reports an empty schedule that masks a real error.
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			stderr := strings.ToLower(string(ee.Stderr))
			if strings.Contains(stderr, "no crontab") || (ee.ExitCode() == 1 && len(out) == 0) {
				return "", nil
			}
		}
		return "", &errs.Error{Op: "schedule", Code: errs.CodeSystem, Cause: err, Hint: "could not read crontab (is the `crontab` command available?)"}
	}
	return string(out), nil
}

// writeCrontab installs tab as the user crontab via `crontab -` (reads stdin).
func writeCrontab(tab string) error {
	cmd := exec.Command("crontab", "-")
	cmd.Stdin = bytes.NewReader([]byte(tab))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return &errs.Error{Op: "schedule", Code: errs.CodeSystem, Cause: err, Hint: "could not write crontab: " + strings.TrimSpace(stderr.String())}
	}
	return nil
}
