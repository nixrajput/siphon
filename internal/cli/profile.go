package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/nixrajput/siphon/internal/config"
)

func newProfileCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "profile", Short: "Manage named connection profiles"}
	cmd.AddCommand(
		profileListCmd(),
		profileAddCmd(),
		profileRmCmd(),
		profileShowCmd(),
	)
	return cmd
}

func profileListCmd() *cobra.Command {
	return &cobra.Command{
		Use: "list", Short: "List profiles",
		RunE: func(c *cobra.Command, _ []string) error {
			deps, err := buildDeps()
			if err != nil {
				return err
			}
			for _, n := range deps.Profiles.List() {
				p, _ := deps.Profiles.Get(n)
				_, _ = fmt.Fprintf(c.OutOrStdout(), "%-20s %-10s %s\n", n, p.Driver, p.Host)
			}
			return nil
		},
	}
}

func profileAddCmd() *cobra.Command {
	var p config.ProfileConfig
	var name string
	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Add a new profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			name = args[0]
			deps, err := buildDeps()
			if err != nil {
				return err
			}
			return deps.Profiles.Add(name, p)
		},
	}
	cmd.Flags().StringVar(&p.Driver, "driver", "postgres", "Driver name")
	cmd.Flags().StringVar(&p.Host, "host", "", "Host")
	cmd.Flags().IntVar(&p.Port, "port", 5432, "Port")
	cmd.Flags().StringVar(&p.User, "user", "", "User")
	cmd.Flags().StringVar(&p.Password, "password", "", "Password or SecretRef (e.g. env:VAR)")
	cmd.Flags().StringVar(&p.Database, "database", "", "Database name")
	cmd.Flags().StringVar(&p.SSLMode, "sslmode", "prefer", "SSL mode")
	return cmd
}

func profileRmCmd() *cobra.Command {
	return &cobra.Command{
		Use: "rm <name>", Short: "Remove a profile", Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			deps, err := buildDeps()
			if err != nil {
				return err
			}
			return deps.Profiles.Remove(args[0])
		},
	}
}

func profileShowCmd() *cobra.Command {
	return &cobra.Command{
		Use: "show <name>", Short: "Show a profile (with secret refs)", Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			deps, err := buildDeps()
			if err != nil {
				return err
			}
			p, err := deps.Profiles.Get(args[0])
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(c.OutOrStdout(), "driver:   %s\nhost:     %s\nport:     %d\nuser:     %s\ndatabase: %s\nsslmode:  %s\npassword: %s\n",
				p.Driver, p.Host, p.Port, p.User, p.Database, p.SSLMode, p.Password)
			return nil
		},
	}
}
