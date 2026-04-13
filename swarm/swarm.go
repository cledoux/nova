// Package swarm manages groups of Nova sessions.
package swarm

import (
	"context"
	"fmt"
	"log/slog"

	"nova/db"
	discordhelper "nova/discord"
	"nova/session"

	"github.com/google/uuid"
)

// Manager creates and manages swarms.
type Manager struct {
	store    *db.Store
	discord  discordhelper.Client
	sessions *session.Manager
	guildID  string
}

// NewManager creates a swarm Manager.
func NewManager(store *db.Store, discord discordhelper.Client, sessions *session.Manager, guildID string) *Manager {
	return &Manager{store: store, discord: discord, sessions: sessions, guildID: guildID}
}

// Create creates a new named swarm with a Discord category.
func (m *Manager) Create(name string) (db.Swarm, error) {
	if _, err := m.store.GetSwarmByName(name); err == nil {
		return db.Swarm{}, fmt.Errorf("swarm %q already exists", name)
	}
	slog.Info("creating swarm", "name", name)
	catName := "Nova: " + name
	catID, err := discordhelper.EnsureCategory(m.discord, m.guildID, catName)
	if err != nil {
		return db.Swarm{}, fmt.Errorf("create category: %w", err)
	}
	sw := db.Swarm{
		ID:         uuid.New().String(),
		Name:       name,
		CategoryID: catID,
	}
	if err := m.store.CreateSwarm(sw); err != nil {
		return db.Swarm{}, fmt.Errorf("create swarm record: %w", err)
	}
	slog.Info("swarm created", "name", name, "category_id", catID)
	return sw, nil
}

// Dissolve kills all sessions in the swarm and removes the swarm record.
func (m *Manager) Dissolve(name string) error {
	sw, err := m.store.GetSwarmByName(name)
	if err != nil {
		return fmt.Errorf("swarm %q not found: %w", name, err)
	}
	sessions, err := m.store.ListSessions(sw.ID)
	if err != nil {
		return err
	}
	slog.Info("dissolving swarm", "name", name, "session_count", len(sessions))
	for _, s := range sessions {
		slog.Debug("dissolve: killing session", "swarm", name, "session", s.Name)
		_ = m.sessions.Kill(s.Name) // best-effort
	}
	if err := m.store.DeleteSwarm(sw.ID); err != nil {
		return err
	}
	slog.Info("swarm dissolved", "name", name)
	return nil
}

// Broadcast sends message to all active sessions in the named swarm.
// Cold sessions are warmed before the message is sent.
func (m *Manager) Broadcast(ctx context.Context, swarmName, message string) error {
	sw, err := m.store.GetSwarmByName(swarmName)
	if err != nil {
		return fmt.Errorf("swarm %q not found: %w", swarmName, err)
	}
	dbSessions, err := m.store.ListSessions(sw.ID)
	if err != nil {
		return err
	}
	slog.Info("broadcasting to swarm", "swarm", swarmName, "session_count", len(dbSessions), "message_len", len(message))
	for _, s := range dbSessions {
		slog.Debug("broadcast: sending to session", "swarm", swarmName, "session", s.Name)
		_ = m.sessions.WarmIfCold(ctx, s.ID)
		sess := m.sessions.ByName(s.Name)
		if sess != nil {
			_ = sess.Send(message)
		}
	}
	return nil
}
