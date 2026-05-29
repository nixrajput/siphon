package postgres

import (
	"context"
	"io"
	"os"
	"os/exec"
	"strconv"

	"github.com/nixrajput/siphon/internal/driver"
)

func (c *Conn) Restore(ctx context.Context, opt driver.RestoreOpts, r io.Reader) error {
	args := buildRestoreArgs(c.p, opt)
	cmd := exec.CommandContext(ctx, "pg_restore", args...)
	cmd.Env = append(os.Environ(), "PGPASSWORD="+c.p.Password)
	cmd.Stdin = r
	cmd.Stderr = os.Stderr // surface pg_restore diagnostics directly; Phase F captures these for structured progress

	if err := cmd.Run(); err != nil {
		return toolMissingOrSystemErr("postgres.restore", err)
	}
	return nil
}

func buildRestoreArgs(p driver.Profile, opt driver.RestoreOpts) []string {
	args := []string{
		"-h", p.Host,
		"-p", strconv.Itoa(p.Port),
		"-U", p.User,
		"-d", p.Database,
		"--no-owner", "--no-acl",
		"--verbose",
	}
	if opt.Clean {
		args = append(args, "--clean", "--if-exists")
	}
	if opt.SchemaOnly {
		args = append(args, "--schema-only")
	}
	if opt.DataOnly {
		args = append(args, "--data-only")
	}
	for _, t := range opt.TargetTables {
		args = append(args, "-t", t)
	}
	return args
}
