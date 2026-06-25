package cli

import (
	"context"
	"encoding/base32"
	"strings"
	"testing"
	"time"

	"github.com/nixrajput/siphon/internal/audit"
	"github.com/nixrajput/siphon/internal/config"
	"github.com/nixrajput/siphon/internal/secrets"
	"github.com/nixrajput/siphon/internal/twofactor"
)

func gateCfg(group config.GroupConfig) *config.Config {
	return &config.Config{
		Profiles: map[string]config.ProfileConfig{
			"prod":    {Name: "prod", Group: "critical"},
			"sandbox": {Name: "sandbox"}, // no group
		},
		Groups: map[string]config.GroupConfig{"critical": group},
	}
}

func newGate(cfg *config.Config, in string, now time.Time) *promptGate {
	g := newPromptGate(cfg, secrets.NewResolver(secrets.Env{}, secrets.Passthrough{}),
		strings.NewReader(in), &strings.Builder{})
	g.now = func() time.Time { return now }
	return g
}

func TestGate_NoGroupAuthorizesSilently(t *testing.T) {
	g := newGate(gateCfg(config.GroupConfig{ConfirmDestructive: true}), "", time.Now())
	if err := g.Authorize(context.Background(), audit.OpBackup, "sandbox"); err != nil {
		t.Errorf("ungrouped profile gated: %v", err)
	}
}

func TestGate_ConfirmDestructive(t *testing.T) {
	cfg := gateCfg(config.GroupConfig{ConfirmDestructive: true})

	// Correct confirmation (types the profile name) passes.
	if err := newGate(cfg, "prod\n", time.Now()).Authorize(context.Background(), audit.OpRestore, "prod"); err != nil {
		t.Errorf("correct confirmation rejected: %v", err)
	}
	// Wrong confirmation blocks.
	if err := newGate(cfg, "nope\n", time.Now()).Authorize(context.Background(), audit.OpRestore, "prod"); err == nil {
		t.Error("wrong confirmation was allowed through")
	}
}

func TestGate_Require2FA(t *testing.T) {
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte("12345678901234567890"))
	cfg := gateCfg(config.GroupConfig{Require2FA: true, TOTPSecret: secret})
	now := time.Unix(10_000, 0)
	good := totpAt(t, secret, now)

	if err := newGate(cfg, good+"\n", now).Authorize(context.Background(), audit.OpSync, "prod"); err != nil {
		t.Errorf("valid TOTP rejected: %v", err)
	}
	if err := newGate(cfg, "000000\n", now).Authorize(context.Background(), audit.OpSync, "prod"); err == nil {
		t.Error("wrong TOTP was allowed through")
	}
}

func TestGate_ConfirmAndRequire2FA_SharedReader(t *testing.T) {
	// Both gates enabled: the name and the TOTP code arrive on one piped stdin
	// ("prod\n<code>\n"). A single shared reader must not swallow the code while
	// reading the name (the bug a per-prompt reader would cause).
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte("12345678901234567890"))
	cfg := gateCfg(config.GroupConfig{ConfirmDestructive: true, Require2FA: true, TOTPSecret: secret})
	now := time.Unix(20_000, 0)
	code := totpAt(t, secret, now)

	if err := newGate(cfg, "prod\n"+code+"\n", now).Authorize(context.Background(), audit.OpRestore, "prod"); err != nil {
		t.Errorf("scripted name+code rejected (reader split the input?): %v", err)
	}
}

func TestGate_Require2FA_MissingSecret(t *testing.T) {
	cfg := gateCfg(config.GroupConfig{Require2FA: true}) // no TOTPSecret
	if err := newGate(cfg, "123456\n", time.Now()).Authorize(context.Background(), audit.OpSync, "prod"); err == nil {
		t.Error("require_2fa with no secret should fail closed (block), not pass")
	}
}

// totpAt returns a valid code for secret at now using the package's generator.
func totpAt(t *testing.T, secret string, now time.Time) string {
	t.Helper()
	code, err := twofactor.Generate(secret, now)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	return code
}
