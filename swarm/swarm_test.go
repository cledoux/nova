package swarm_test

import (
	"context"
	"testing"

	"nova/config"
	"nova/internal/testdiscord"
	"nova/db"
	"nova/session"
	"nova/swarm"
)

func setup(t *testing.T) (*swarm.Manager, *db.Store, *testdiscord.Session) {
	t.Helper()
	store, err := db.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	fake := testdiscord.New()
	cfg := &config.Config{GuildID: "g-1", SessionRoot: t.TempDir(), IdleTimeoutMinutes: 1, ClaudeBin: "true"}
	sessMgr := session.NewManager(store, fake, cfg, "cat-solo", "cat-arch")
	mgr := swarm.NewManager(store, fake, sessMgr, "g-1")
	return mgr, store, fake
}

func TestSwarm_Create(t *testing.T) {
	mgr, store, _ := setup(t)
	sw, err := mgr.Create("backend")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if sw.Name != "backend" {
		t.Errorf("Name = %q, want backend", sw.Name)
	}
	dbSw, err := store.GetSwarmByName("backend")
	if err != nil {
		t.Fatalf("GetSwarmByName: %v", err)
	}
	if dbSw.CategoryID == "" {
		t.Error("CategoryID should be set")
	}
}

func TestSwarm_CreateDuplicate(t *testing.T) {
	mgr, _, _ := setup(t)
	_, _ = mgr.Create("frontend")
	_, err := mgr.Create("frontend")
	if err == nil {
		t.Error("expected error for duplicate swarm name")
	}
}

func TestSwarm_Broadcast(t *testing.T) {
	mgr, _, fake := setup(t)
	sw, _ := mgr.Create("team")
	_ = sw

	// Broadcast with no sessions should succeed (no-op).
	if err := mgr.Broadcast(context.Background(), "team", "hello swarm"); err != nil {
		t.Fatalf("Broadcast: %v", err)
	}
	_ = fake
}
