package cli

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/nixrajput/siphon/internal/config"
	"github.com/nixrajput/siphon/internal/errs"
)

// newTunnelCmd opens an SSH local-forward to a profile's database through its
// configured bastion, using the system ssh client. It runs in the foreground
// and holds the tunnel open until interrupted (Ctrl-C) — run it in one terminal
// and point siphon (or any client) at the printed local address in another.
func newTunnelCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tunnel <profile>",
		Short: "Open an SSH tunnel to a profile's database via its bastion",
		Long: "tunnel opens `ssh -L <local>:<dbhost>:<dbport> <bastion>` using your " +
			"system ssh client (your ssh config, keys, and agent apply) and holds it " +
			"open until you press Ctrl-C. Configure a profile's `tunnel.bastion` first.",
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			prof, ok := cfg.Profiles[args[0]]
			if !ok {
				return &errs.Error{Op: "tunnel", Code: errs.CodeUser, Cause: errs.ErrProfileNotFound, Hint: "unknown profile " + args[0]}
			}
			if prof.Tunnel == nil || prof.Tunnel.Bastion == "" {
				return &errs.Error{Op: "tunnel", Code: errs.CodeUser, Hint: "profile " + args[0] + " has no tunnel.bastion configured"}
			}

			localPort := prof.Tunnel.LocalPort
			if localPort == 0 {
				localPort = prof.Port
			}
			sshArgs := tunnelArgs(localPort, prof.Host, prof.Port, prof.Tunnel.Bastion)

			_, _ = fmt.Fprintf(c.OutOrStdout(),
				"tunnel open: localhost:%d → %s:%d via %s (Ctrl-C to close)\n",
				localPort, prof.Host, prof.Port, prof.Tunnel.Bastion)

			cmd := exec.CommandContext(c.Context(), "ssh", sshArgs...)
			cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, c.OutOrStdout(), c.ErrOrStderr()
			if err := cmd.Run(); err != nil {
				// A clean Ctrl-C (ssh exits non-zero on signal) is the normal stop,
				// not an error worth a non-zero siphon exit beyond ssh's own.
				return &errs.Error{Op: "tunnel", Code: errs.CodeSystem, Cause: err, Hint: "ssh tunnel exited"}
			}
			return nil
		},
	}
}

// tunnelArgs builds the `ssh` argument list for a local forward. Pure and
// testable: -N (no remote command), -L localPort:dbHost:dbPort, then the
// bastion. ExitOnForwardFailure makes ssh fail fast if the local bind fails
// rather than silently opening a useless session.
func tunnelArgs(localPort int, dbHost string, dbPort int, bastion string) []string {
	forward := strconv.Itoa(localPort) + ":" + dbHost + ":" + strconv.Itoa(dbPort)
	return []string{
		"-N",
		"-o", "ExitOnForwardFailure=yes",
		"-L", forward,
		bastion,
	}
}
