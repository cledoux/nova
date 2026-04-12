package discord_test

import (
	"strings"
	"testing"

	"nova/internal/testdiscord"
	"nova/discord"
)

const guildID = "guild-1"

func TestEnsureCategory_creates(t *testing.T) {
	fake := testdiscord.New()
	id, err := discord.EnsureCategory(fake, guildID, "Nova: solo")
	if err != nil {
		t.Fatalf("EnsureCategory: %v", err)
	}
	if id == "" {
		t.Error("expected non-empty category ID")
	}
}

func TestEnsureCategory_idempotent(t *testing.T) {
	fake := testdiscord.New()
	id1, _ := discord.EnsureCategory(fake, guildID, "Nova: solo")
	id2, _ := discord.EnsureCategory(fake, guildID, "Nova: solo")
	if id1 != id2 {
		t.Errorf("EnsureCategory created duplicate: %q vs %q", id1, id2)
	}
}

func TestEnsureChannel_creates(t *testing.T) {
	fake := testdiscord.New()
	catID, _ := discord.EnsureCategory(fake, guildID, "Nova: solo")
	chID, err := discord.EnsureChannel(fake, guildID, catID, "worker-1")
	if err != nil {
		t.Fatalf("EnsureChannel: %v", err)
	}
	if chID == "" {
		t.Error("expected non-empty channel ID")
	}
}

func TestEnsureChannel_idempotent(t *testing.T) {
	fake := testdiscord.New()
	catID, _ := discord.EnsureCategory(fake, guildID, "Nova: solo")
	id1, _ := discord.EnsureChannel(fake, guildID, catID, "worker-1")
	id2, _ := discord.EnsureChannel(fake, guildID, catID, "worker-1")
	if id1 != id2 {
		t.Errorf("EnsureChannel created duplicate: %q vs %q", id1, id2)
	}
}

func TestArchiveChannel(t *testing.T) {
	fake := testdiscord.New()
	catID, _ := discord.EnsureCategory(fake, guildID, "Nova: solo")
	archID, _ := discord.EnsureCategory(fake, guildID, "Nova: archived")
	chID, _ := discord.EnsureChannel(fake, guildID, catID, "worker-1")

	if err := discord.ArchiveChannel(fake, guildID, chID, archID); err != nil {
		t.Fatalf("ArchiveChannel: %v", err)
	}
	ch, ok := fake.GetChannel(chID)
	if !ok {
		t.Fatal("channel not found after archive")
	}
	if !strings.HasPrefix(ch.Name, "✓-") {
		t.Errorf("channel name = %q, want ✓- prefix", ch.Name)
	}
	if ch.ParentID != archID {
		t.Errorf("ParentID = %q, want %q", ch.ParentID, archID)
	}
}

func TestPostMessage_short(t *testing.T) {
	fake := testdiscord.New()
	if err := discord.PostMessage(fake, "ch-1", "hello"); err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	if len(fake.Messages) != 1 {
		t.Errorf("got %d messages, want 1", len(fake.Messages))
	}
}

func TestPostMessage_splits(t *testing.T) {
	fake := testdiscord.New()
	long := strings.Repeat("x", 4001)
	if err := discord.PostMessage(fake, "ch-1", long); err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	if len(fake.Messages) != 3 {
		t.Errorf("got %d messages, want 3", len(fake.Messages))
	}
}
