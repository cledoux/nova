package session_test

import (
	"context"
	"testing"
	"time"

	"nova/config"
	"nova/internal/testdiscord"
	"nova/db"
	"nova/session"
)

func newTestManager(t *testing.T) (*session.Manager, *db.Store, *testdiscord.Session) {
	t.Helper()
	store, err := db.New(":memory:")
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	fake := testdiscord.New()
	cfg := &config.Config{
		GuildID:            "g-1",
		SessionRoot:        t.TempDir(),
		IdleTimeoutMinutes: 1,
		ClaudeBin:          fakeClaude(t),
	}
	mgr := session.NewManager(store, fake, cfg, "cat-solo", "cat-archive")
	return mgr, store, fake
}

func TestManager_Spawn(t *testing.T) {
	mgr, store, fake := newTestManager(t)

	sess, err := mgr.Spawn(context.Background(), session.SpawnOpts{Name: "alpha"})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if sess.Status != session.StatusHot {
		t.Errorf("Status = %q, want hot", sess.Status)
	}
	// Verify DB record
	dbSess, err := store.GetSessionByName("alpha")
	if err != nil {
		t.Fatalf("GetSessionByName: %v", err)
	}
	if dbSess.Status != session.StatusHot {
		t.Errorf("DB status = %q, want hot", dbSess.Status)
	}
	// Verify Discord channel created
	if len(fake.Messages) == 0 && sess.ChannelID == "" {
		t.Error("expected Discord channel to be created")
	}
}

func TestManager_ByChannel(t *testing.T) {
	mgr, _, _ := newTestManager(t)
	sess, _ := mgr.Spawn(context.Background(), session.SpawnOpts{Name: "beta"})
	got := mgr.ByChannel(sess.ChannelID)
	if got == nil || got.ID != sess.ID {
		t.Errorf("ByChannel returned wrong session")
	}
}

func TestManager_Kill(t *testing.T) {
	mgr, store, _ := newTestManager(t)
	sess, _ := mgr.Spawn(context.Background(), session.SpawnOpts{Name: "gamma"})

	if err := mgr.Kill("gamma"); err != nil {
		t.Fatalf("Kill: %v", err)
	}
	if sess.Status != session.StatusTerminated {
		t.Errorf("Status = %q, want terminated", sess.Status)
	}
	dbSess, _ := store.GetSessionByName("gamma")
	if dbSess.Status != session.StatusTerminated {
		t.Errorf("DB status = %q, want terminated", dbSess.Status)
	}
}

func TestManager_WarmIfCold(t *testing.T) {
	mgr, _, _ := newTestManager(t)
	sess, _ := mgr.Spawn(context.Background(), session.SpawnOpts{Name: "delta"})

	// Force cold
	sess.Terminate()
	// Reset to cold manually for test
	sess.Status = session.StatusCold

	if err := mgr.WarmIfCold(context.Background(), sess.ID); err != nil {
		t.Fatalf("WarmIfCold: %v", err)
	}
	time.Sleep(50 * time.Millisecond) // let subprocess start
	if sess.Status != session.StatusHot {
		t.Errorf("Status after WarmIfCold = %q, want hot", sess.Status)
	}
}
