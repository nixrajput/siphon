package cli

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/nixrajput/siphon/internal/config"
)

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "config", Short: "Read or modify global settings"}
	cmd.AddCommand(configPathCmd(), configEditCmd())
	return cmd
}

func configPathCmd() *cobra.Command {
	return &cobra.Command{
		Use: "path", Short: "Print the config file path",
		RunE: func(c *cobra.Command, _ []string) error {
			_, _ = fmt.Fprintln(c.OutOrStdout(), config.Path())
			return nil
		},
	}
}

func configEditCmd() *cobra.Command {
	return &cobra.Command{
		Use: "edit", Short: "Open the config file in $EDITOR",
		RunE: func(c *cobra.Command, _ []string) error {
			editor := defaultEditor()
			if editor == "" {
				editor = "vi"
			}
			ed := exec.Command(editor, config.Path())
			ed.Stdin = c.InOrStdin()
			ed.Stdout = c.OutOrStdout()
			ed.Stderr = c.ErrOrStderr()
			return ed.Run()
		},
	}
}

func defaultEditor() string {
	for _, env := range []string{"VISUAL", "EDITOR"} {
		if v := os.Getenv(env); v != "" {
			return v
		}
	}
	return ""
}
