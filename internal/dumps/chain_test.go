package dumps

import (
	"context"
	"testing"
	"time"
)

func TestResolveChain_SingleBase(t *testing.T) {
	c, _ := NewCatalog(t.TempDir())
	base := &Meta{ID: "base", BaseID: "base", Created: time.Now()}
	_ = c.WriteMeta(context.Background(), base)

	chain, err := c.ResolveChain(context.Background(), "base")
	if err != nil {
		t.Fatal(err)
	}
	if len(chain) != 1 || chain[0].ID != "base" {
		t.Fatalf("got %v; want [base]", chain)
	}
}

func TestResolveChain_BaseAndOneIncremental(t *testing.T) {
	c, _ := NewCatalog(t.TempDir())
	_ = c.WriteMeta(context.Background(), &Meta{ID: "base", BaseID: "base", Created: time.Now()})
	_ = c.WriteMeta(context.Background(), &Meta{ID: "inc1", BaseID: "base", ParentID: "base", Created: time.Now()})

	chain, err := c.ResolveChain(context.Background(), "inc1")
	if err != nil {
		t.Fatal(err)
	}
	if len(chain) != 2 || chain[0].ID != "base" || chain[1].ID != "inc1" {
		t.Fatalf("got %v; want [base, inc1]", chain)
	}
}

func TestResolveChain_MultiIncremental(t *testing.T) {
	c, _ := NewCatalog(t.TempDir())
	_ = c.WriteMeta(context.Background(), &Meta{ID: "base", BaseID: "base", Created: time.Now()})
	_ = c.WriteMeta(context.Background(), &Meta{ID: "inc1", BaseID: "base", ParentID: "base", Created: time.Now()})
	_ = c.WriteMeta(context.Background(), &Meta{ID: "inc2", BaseID: "base", ParentID: "inc1", Created: time.Now()})

	chain, err := c.ResolveChain(context.Background(), "inc2")
	if err != nil {
		t.Fatal(err)
	}
	if len(chain) != 3 || chain[0].ID != "base" || chain[1].ID != "inc1" || chain[2].ID != "inc2" {
		t.Fatalf("got %v; want [base, inc1, inc2]", chain)
	}
}

func TestResolveChain_DetectsCycle(t *testing.T) {
	c, _ := NewCatalog(t.TempDir())
	_ = c.WriteMeta(context.Background(), &Meta{ID: "a", BaseID: "x", ParentID: "b"})
	_ = c.WriteMeta(context.Background(), &Meta{ID: "b", BaseID: "x", ParentID: "a"})

	if _, err := c.ResolveChain(context.Background(), "a"); err == nil {
		t.Fatal("expected cycle error")
	}
}

func TestResolveChain_BrokenChain_MissingParent(t *testing.T) {
	c, _ := NewCatalog(t.TempDir())
	// inc1 claims a parent that was never written.
	_ = c.WriteMeta(context.Background(), &Meta{ID: "inc1", BaseID: "base", ParentID: "base", Created: time.Now()})

	if _, err := c.ResolveChain(context.Background(), "inc1"); err == nil {
		t.Fatal("expected broken-chain error (missing base)")
	}
}
