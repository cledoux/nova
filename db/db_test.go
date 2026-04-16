package db_test

import (
	"testing"
	"time"

	"nova/db"
)

func newTestStore(t *testing.T) *db.Store {
	t.Helper()
	store, err := db.New(":memory:")
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestCreateAndGetSession(t *testing.T) {
	store := newTestStore(t)
	s := db.Session{
		ID:        "id-1",
		Name:      "worker-1",
		Workspace: "/tmp/ws",
		ChannelID: "ch-1",
		Status:    "cold",
	}
	if err := store.CreateSession(s); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	got, err := store.GetSession("id-1")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got.Name != "worker-1" {
		t.Errorf("Name = %q, want worker-1", got.Name)
	}
}

func TestGetSessionByName(t *testing.T) {
	store := newTestStore(t)
	_ = store.CreateSession(db.Session{ID: "id-2", Name: "alpha", Workspace: "/w", ChannelID: "c", Status: "cold"})
	got, err := store.GetSessionByName("alpha")
	if err != nil {
		t.Fatalf("GetSessionByName: %v", err)
	}
	if got.ID != "id-2" {
		t.Errorf("ID = %q, want id-2", got.ID)
	}
}

func TestUpdateSessionStatus(t *testing.T) {
	store := newTestStore(t)
	_ = store.CreateSession(db.Session{ID: "id-3", Name: "beta", Workspace: "/w", ChannelID: "c", Status: "cold"})
	if err := store.UpdateSessionStatus("id-3", "hot"); err != nil {
		t.Fatalf("UpdateSessionStatus: %v", err)
	}
	got, _ := store.GetSession("id-3")
	if got.Status != "hot" {
		t.Errorf("Status = %q, want hot", got.Status)
	}
}

func TestResetActiveSessions(t *testing.T) {
	store := newTestStore(t)
	_ = store.CreateSession(db.Session{ID: "a", Name: "a", Workspace: "/w", ChannelID: "c", Status: "hot"})
	_ = store.CreateSession(db.Session{ID: "b", Name: "b", Workspace: "/w", ChannelID: "c2", Status: "cold"})
	if err := store.ResetActiveSessions(); err != nil {
		t.Fatalf("ResetActiveSessions: %v", err)
	}
	got, _ := store.GetSession("a")
	if got.Status != "cold" {
		t.Errorf("hot session not reset: status = %q", got.Status)
	}
	got2, _ := store.GetSession("b")
	if got2.Status != "cold" {
		t.Errorf("cold session changed: status = %q", got2.Status)
	}
}

func TestListSessions(t *testing.T) {
	store := newTestStore(t)
	_ = store.CreateSession(db.Session{ID: "x", Name: "x", Workspace: "/w", ChannelID: "c1", Status: "cold"})
	_ = store.CreateSession(db.Session{ID: "y", Name: "y", Workspace: "/w", ChannelID: "c2", Status: "cold"})
	all, err := store.ListSessions()
	if err != nil || len(all) != 2 {
		t.Errorf("ListSessions: got %d, want 2 (err=%v)", len(all), err)
	}
}

func TestInsertMessage(t *testing.T) {
	store := newTestStore(t)
	_ = store.CreateSession(db.Session{ID: "s1", Name: "s", Workspace: "/w", ChannelID: "c", Status: "cold"})
	if err := store.InsertMessage("s1", "user", "hello"); err != nil {
		t.Fatalf("InsertMessage: %v", err)
	}
	n, err := store.CountMessages("s1")
	if err != nil {
		t.Fatalf("CountMessages: %v", err)
	}
	if n != 1 {
		t.Errorf("CountMessages = %d, want 1", n)
	}
}

func TestTouchSession(t *testing.T) {
	store := newTestStore(t)
	_ = store.CreateSession(db.Session{ID: "t1", Name: "t", Workspace: "/w", ChannelID: "c", Status: "cold"})
	before, _ := store.GetSession("t1")
	time.Sleep(time.Millisecond)
	if err := store.TouchSession("t1"); err != nil {
		t.Fatalf("TouchSession: %v", err)
	}
	after, _ := store.GetSession("t1")
	if !after.LastActive.After(before.LastActive) {
		t.Error("last_active not updated")
	}
}
