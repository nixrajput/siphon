package app

import (
	"context"
	"errors"
	"testing"

	"github.com/nixrajput/siphon/internal/config"
	"github.com/nixrajput/siphon/internal/driver"
	"github.com/nixrajput/siphon/internal/errs"
	"github.com/nixrajput/siphon/internal/profile"
	"github.com/nixrajput/siphon/internal/secrets"
)

// capDriver is a driver whose Capabilities are fully controllable per-test.
// (The package-level fakeDriver in backup_test.go hardcodes its caps, so it
// can't exercise the capability mapping.)
type capDriver struct {
	name string
	caps driver.Capabilities
}

func (d capDriver) Name() string                      { return d.name }
func (d capDriver) Capabilities() driver.Capabilities { return d.caps }
func (d capDriver) Connect(_ context.Context, _ driver.Profile) (driver.Conn, error) {
	return nil, nil
}

// capDeps builds a Deps with a real *profile.Store holding one profile "p"
// bound to driver "fake", whose driver reports the given capabilities.
func capDeps(caps driver.Capabilities) Deps {
	cfg := &config.Config{Profiles: map[string]config.ProfileConfig{
		"p": {Driver: "fake", Host: "h", User: "u", Database: "d", Password: "pw"},
	}}
	res := secrets.NewResolver(secrets.Passthrough{})
	ps := profile.New(cfg, res, func(*config.Config) error { return nil })
	return Deps{
		Profiles: ps,
		Drivers:  fakeGetter{d: capDriver{name: "fake", caps: caps}},
	}
}

func TestRequireCapability_Supported(t *testing.T) {
	deps := capDeps(driver.Capabilities{Parallel: true})
	if err := RequireCapability(deps, "p", CapParallel); err != nil {
		t.Fatalf("RequireCapability(CapParallel) = %v; want nil", err)
	}
}

func TestRequireCapability_Unsupported_ReturnsErrDriverUnsupported(t *testing.T) {
	deps := capDeps(driver.Capabilities{Parallel: false})
	err := RequireCapability(deps, "p", CapParallel)
	if err == nil {
		t.Fatal("RequireCapability(CapParallel) = nil; want error")
	}
	if !errors.Is(err, errs.ErrDriverUnsupported) {
		t.Fatalf("errors.Is(err, ErrDriverUnsupported) = false; err = %v", err)
	}
	var ee *errs.Error
	if !errors.As(err, &ee) {
		t.Fatalf("err is not *errs.Error; got %T", err)
	}
	if ee.Code != errs.CodeUser {
		t.Fatalf("Code = %v; want CodeUser", ee.Code)
	}
}

func TestRequireCapability_MappingMatchesFlags(t *testing.T) {
	cases := []struct {
		name string
		cap  Capability
		caps driver.Capabilities
		want bool
	}{
		{"native-stream supported", CapNativeStream, driver.Capabilities{NativeStream: true}, true},
		{"native-stream unsupported", CapNativeStream, driver.Capabilities{}, false},
		{"incremental supported", CapIncremental, driver.Capabilities{Incremental: true}, true},
		{"cdc unsupported", CapCDC, driver.Capabilities{Incremental: true}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := RequireCapability(capDeps(tc.caps), "p", tc.cap)
			got := err == nil
			if got != tc.want {
				t.Fatalf("RequireCapability(%s) supported = %v; want %v (err=%v)", tc.cap, got, tc.want, err)
			}
		})
	}
}
