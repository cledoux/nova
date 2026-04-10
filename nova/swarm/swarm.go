// Package swarm manages groups of Nova sessions.
package swarm

import (
	"context"
	"fmt"

	"nova/nova/db"
	discordhelper "nova/nova/discord"
	"nova/nova/session"

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
	for _, s := range sessions {
		_ = m.sessions.Kill(s.Name) // best-effort
	}
	return m.store.DeleteSwarm(sw.ID)
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
	for _, s := range dbSessions {
		_ = m.sessions.WarmIfCold(ctx, s.ID)
		sess := m.sessions.ByName(s.Name)
		if sess != nil {
			_ = sess.Send(message)
		}
	}
	return nil
}
