// Package profile provides CRUD operations on named connection profiles
// persisted in siphon's config file.
package profile

import (
	"errors"
	"sort"

	"github.com/nixrajput/siphon/internal/config"
	"github.com/nixrajput/siphon/internal/driver"
	"github.com/nixrajput/siphon/internal/errs"
	"github.com/nixrajput/siphon/internal/secrets"
)

// Store is the runtime API for managing profiles.
type Store struct {
	cfg  *config.Config
	res  *secrets.Resolver
	save func(*config.Config) error
}

// New creates a Store backed by cfg. The save function controls
// persistence; pass config.Save in production, or a no-op for tests.
func New(cfg *config.Config, res *secrets.Resolver, save func(*config.Config) error) *Store {
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]config.ProfileConfig{}
	}
	return &Store{cfg: cfg, res: res, save: save}
}

// List returns all profile names, sorted alphabetically.
func (s *Store) List() []string {
	names := make([]string, 0, len(s.cfg.Profiles))
	for n := range s.cfg.Profiles {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Get returns the unresolved profile (Password is still a SecretRef).
func (s *Store) Get(name string) (config.ProfileConfig, error) {
	p, ok := s.cfg.Profiles[name]
	if !ok {
		return config.ProfileConfig{}, &errs.Error{
			Op:    "profile.get",
			Code:  errs.CodeUser,
			Cause: errs.ErrProfileNotFound,
			Hint:  "run `siphon profile list` to see available profiles",
		}
	}
	return p, nil
}

// Resolve returns a driver.Profile with secrets materialized.
func (s *Store) Resolve(name string) (driver.Profile, error) {
	p, err := s.Get(name)
	if err != nil {
		return driver.Profile{}, err
	}
	pass, err := s.res.Resolve(p.Password)
	if err != nil {
		return driver.Profile{}, err
	}
	return driver.Profile{
		Name:     name,
		Driver:   p.Driver,
		Host:     p.Host,
		Port:     p.Port,
		User:     p.User,
		Password: pass,
		Database: p.Database,
		SSLMode:  p.SSLMode,
	}, nil
}

// Add stores a new profile. Returns an error if name already exists.
// Always sets p.Name = name to keep the Name field consistent with the
// map key (which Load() also enforces on reload).
func (s *Store) Add(name string, p config.ProfileConfig) error {
	if _, exists := s.cfg.Profiles[name]; exists {
		return &errs.Error{Op: "profile.add", Code: errs.CodeUser, Cause: errors.New("profile already exists: " + name)}
	}
	p.Name = name
	s.cfg.Profiles[name] = p
	return s.save(s.cfg)
}

// Update overwrites an existing profile. Sets p.Name = name for the same
// reason as Add.
func (s *Store) Update(name string, p config.ProfileConfig) error {
	if _, exists := s.cfg.Profiles[name]; !exists {
		return &errs.Error{Op: "profile.update", Code: errs.CodeUser, Cause: errs.ErrProfileNotFound}
	}
	p.Name = name
	s.cfg.Profiles[name] = p
	return s.save(s.cfg)
}

// Remove deletes a profile by name.
func (s *Store) Remove(name string) error {
	if _, exists := s.cfg.Profiles[name]; !exists {
		return &errs.Error{Op: "profile.remove", Code: errs.CodeUser, Cause: errs.ErrProfileNotFound}
	}
	delete(s.cfg.Profiles, name)
	return s.save(s.cfg)
}
