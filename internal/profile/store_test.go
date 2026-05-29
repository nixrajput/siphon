package profile

import (
	"errors"
	"testing"

	"github.com/nixrajput/siphon/internal/config"
	"github.com/nixrajput/siphon/internal/errs"
	"github.com/nixrajput/siphon/internal/secrets"
)

func newTestStore() *Store {
	cfg := &config.Config{Profiles: map[string]config.ProfileConfig{}}
	res := secrets.NewResolver(secrets.Env{}, secrets.Passthrough{})
	return New(cfg, res, func(*config.Config) error { return nil })
}

func TestStore_AddListGetRemove_Roundtrip(t *testing.T) {
	s := newTestStore()

	p := config.ProfileConfig{Driver: "postgres", Host: "localhost", Port: 5432, User: "u", Database: "d"}
	if err := s.Add("dev", p); err != nil {
		t.Fatalf("Add: %v", err)
	}

	if names := s.List(); len(names) != 1 || names[0] != "dev" {
		t.Fatalf("List() = %v; want [dev]", names)
	}

	got, err := s.Get("dev")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Host != "localhost" {
		t.Fatalf("Host = %q; want localhost", got.Host)
	}
	if got.Name != "dev" {
		t.Fatalf("Name = %q; want 'dev' (Add should set Name = name)", got.Name)
	}

	if err := s.Remove("dev"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if names := s.List(); len(names) != 0 {
		t.Fatalf("List() after Remove = %v; want empty", names)
	}
}

func TestStore_Add_Duplicate_Errors(t *testing.T) {
	s := newTestStore()
	_ = s.Add("dev", config.ProfileConfig{Driver: "postgres"})
	err := s.Add("dev", config.ProfileConfig{Driver: "postgres"})
	if err == nil {
		t.Fatal("expected error on duplicate add")
	}
}

func TestStore_GetMissing_ReturnsErrProfileNotFound(t *testing.T) {
	s := newTestStore()
	_, err := s.Get("ghost")
	if !errors.Is(err, errs.ErrProfileNotFound) {
		t.Fatalf("err = %v; want ErrProfileNotFound", err)
	}
}

func TestStore_Resolve_MaterializesPassword(t *testing.T) {
	t.Setenv("PG_PASS", "topsecret")
	s := newTestStore()
	_ = s.Add("prod", config.ProfileConfig{
		Driver:   "postgres",
		Host:     "db",
		User:     "app",
		Password: "env:PG_PASS",
		Database: "app",
	})
	p, err := s.Resolve("prod")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if p.Password != "topsecret" {
		t.Fatalf("Password = %q; want 'topsecret'", p.Password)
	}
	if p.Name != "prod" {
		t.Fatalf("Name = %q; want 'prod'", p.Name)
	}
}
