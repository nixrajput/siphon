package app

import (
	"context"

	"github.com/nixrajput/siphon/internal/driver"
)

// Inspect returns the live schema for the named profile.
func Inspect(ctx context.Context, d Deps, profile string) (*driver.Schema, error) {
	resolved, err := d.Profiles.Resolve(profile)
	if err != nil {
		return nil, err
	}
	drv, err := d.Drivers.Get(resolved.Driver)
	if err != nil {
		return nil, err
	}
	conn, err := drv.Connect(ctx, resolved)
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close() }()
	return conn.Inspect(ctx)
}
