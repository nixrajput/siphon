package app

import (
	"github.com/nixrajput/siphon/internal/driver"
	_ "github.com/nixrajput/siphon/internal/driver/mariadb"  // register the mariadb driver
	_ "github.com/nixrajput/siphon/internal/driver/mysql"    // register the mysql driver
	_ "github.com/nixrajput/siphon/internal/driver/postgres" // register the postgres driver
)

// DefaultDrivers returns a DriverGetter backed by the global driver registry.
// Presentation layers (CLI, TUI) use this so they never import internal/driver
// directly — dependency flows through the application layer.
func DefaultDrivers() DriverGetter {
	return registryGetter{}
}

type registryGetter struct{}

func (registryGetter) Get(name string) (driver.Driver, error) {
	return driver.Get(name)
}
