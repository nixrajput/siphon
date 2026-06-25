package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/nixrajput/siphon/internal/app"
	"github.com/nixrajput/siphon/internal/audit"
	"github.com/nixrajput/siphon/internal/config"
	"github.com/nixrajput/siphon/internal/errs"
	"github.com/nixrajput/siphon/internal/secrets"
	"github.com/nixrajput/siphon/internal/twofactor"
)

// promptGate implements app.Gate by enforcing a profile's group policy before a
// destructive op: ConfirmDestructive prompts the operator to retype the profile
// name, and Require2FA prompts for a TOTP code verified against the group's
// shared secret. Both read from in / write to out, so the gate is wired with the
// command's stdin/stdout (and can be driven by a scripted reader in tests).
//
// A profile with no group, or a group with neither flag, authorizes silently.
type promptGate struct {
	cfg *config.Config
	res *secrets.Resolver
	in  io.Reader
	out io.Writer
	now func() time.Time // TOTP clock; defaults to time.Now
}

// newPromptGate builds a gate from config + a secret resolver, prompting on in
// and reporting on out.
func newPromptGate(cfg *config.Config, res *secrets.Resolver, in io.Reader, out io.Writer) *promptGate {
	return &promptGate{cfg: cfg, res: res, in: in, out: out, now: time.Now}
}

func (g *promptGate) Authorize(_ context.Context, op audit.Op, profile string) error {
	grp, ok := g.groupFor(profile)
	if !ok {
		return nil // no group → no policy
	}
	if grp.ConfirmDestructive {
		if err := g.confirmName(op, profile); err != nil {
			return err
		}
	}
	if grp.Require2FA {
		if err := g.confirmTOTP(op, profile, grp.TOTPSecret); err != nil {
			return err
		}
	}
	return nil
}

// groupFor returns the group config for a profile's group, if it has one.
func (g *promptGate) groupFor(profile string) (config.GroupConfig, bool) {
	p, ok := g.cfg.Profiles[profile]
	if !ok || p.Group == "" {
		return config.GroupConfig{}, false
	}
	grp, ok := g.cfg.Groups[p.Group]
	return grp, ok
}

func (g *promptGate) confirmName(op audit.Op, profile string) error {
	_, _ = fmt.Fprintf(g.out, "%s on %q is destructive. Type the profile name to confirm: ", op, profile)
	line, _ := bufio.NewReader(g.in).ReadString('\n')
	if strings.TrimSpace(line) != profile {
		return &errs.Error{Op: "gate", Code: errs.CodeUser, Hint: "confirmation did not match the profile name; aborted"}
	}
	return nil
}

func (g *promptGate) confirmTOTP(op audit.Op, profile, secretRef string) error {
	secret, err := g.res.Resolve(secretRef)
	if err != nil || strings.TrimSpace(secret) == "" {
		return &errs.Error{Op: "gate", Code: errs.CodeUser, Hint: "group requires 2FA but its totp_secret is unset or unresolvable"}
	}
	_, _ = fmt.Fprintf(g.out, "%s on %q requires 2FA. Enter your 6-digit code: ", op, profile)
	line, _ := bufio.NewReader(g.in).ReadString('\n')
	ok, err := twofactor.Verify(secret, line, g.now())
	if err != nil {
		return &errs.Error{Op: "gate", Code: errs.CodeUser, Cause: err, Hint: "invalid TOTP secret configured for this group"}
	}
	if !ok {
		return &errs.Error{Op: "gate", Code: errs.CodeUser, Hint: "incorrect 2FA code; aborted"}
	}
	return nil
}

// compile-time assertion that promptGate satisfies the app.Gate seam.
var _ app.Gate = (*promptGate)(nil)
