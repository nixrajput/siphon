package postgres

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"

	"github.com/nixrajput/siphon/internal/driver"
	"github.com/nixrajput/siphon/internal/errs"
)

// Backup spawns pg_dump and streams the output to w. ctx cancellation
// propagates to pg_dump via exec.CommandContext.
func (c *Conn) Backup(ctx context.Context, opt driver.BackupOpts, w io.Writer) error {
	args := buildBackupArgs(c.p, opt)
	cmd := exec.CommandContext(ctx, "pg_dump", args...)
	cmd.Env = append(os.Environ(), "PGPASSWORD="+c.p.Password)
	cmd.Stdout = w

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return toolMissingOrSystemErr("postgres.backup", err)
	}

	// Drain stderr (parsed into progress events later — see progress.go).
	go func() { _, _ = io.Copy(io.Discard, stderr) }()

	if err := cmd.Wait(); err != nil {
		return &errs.Error{Op: "postgres.backup", Code: errs.CodeSystem, Cause: err}
	}
	return nil
}

func buildBackupArgs(p driver.Profile, opt driver.BackupOpts) []string {
	args := []string{
		"-h", p.Host,
		"-p", strconv.Itoa(p.Port),
		"-U", p.User,
		"-d", p.Database,
		"-Fc", // custom binary format
		"-Z", strconv.Itoa(compressionLevel(opt.CompressionLevel)),
		"--no-owner", "--no-acl",
		"--verbose",
	}
	// NOTE: pg_dump's -j (parallel) requires directory format (-Fd); it is
	// incompatible with the single-stream custom format (-Fc) siphon writes
	// to the catalog. opt.Parallel is honored by pg_restore (see restore.go),
	// not by pg_dump. Do not add -j here.
	if opt.SchemaOnly {
		args = append(args, "--schema-only")
	}
	if opt.DataOnly {
		args = append(args, "--data-only")
	}
	for _, t := range opt.IncludeTables {
		args = append(args, "-t", t)
	}
	for _, t := range opt.ExcludeTables {
		args = append(args, "-T", t)
	}
	for _, t := range opt.ExcludeDataFrom {
		args = append(args, "--exclude-table-data", t)
	}
	return args
}

func compressionLevel(n int) int {
	if n <= 0 {
		return 1
	}
	if n > 9 {
		return 9
	}
	return n
}

func toolMissingOrSystemErr(op string, err error) error {
	if execErr, ok := err.(*exec.Error); ok && execErr.Err == exec.ErrNotFound {
		return &errs.Error{
			Op:    op,
			Code:  errs.CodeSystem,
			Cause: errs.ErrToolMissing,
			Hint:  "install postgresql client tools (brew install postgresql, apt install postgresql-client)",
		}
	}
	return &errs.Error{Op: op, Code: errs.CodeSystem, Cause: err}
}

// progress.go intentionally separate so future stderr parsing has a clear home.
var _ = fmt.Sprintf
