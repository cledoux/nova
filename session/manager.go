package session

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"time"

	"nova/config"
	"nova/db"
	"nova/directive"
	discordhelper "nova/discord"

	"github.com/google/uuid"
)

// SpawnOpts carries options for creating a new session.
type SpawnOpts struct {
	Name       string // auto-generated if empty
	SwarmID    string
	Task       string // injected as the first message if non-empty
	CategoryID string // Discord category; uses soloCategoryID if empty
}

// Manager owns all active sessions and handles their lifecycle.
type Manager struct {
	mu       sync.RWMutex
	sessions map[string]*Session // id → session
	byName   map[string]*Session
	byChan   map[string]*Session // channelID → session

	store             *db.Store
	discord           discordhelper.Client
	cfg               *config.Config
	soloCategoryID    string
	archiveCategoryID string
}

// NewManager creates a Manager. soloCategoryID and archiveCategoryID are the
// Discord category IDs for solo sessions and archived sessions respectively.
func NewManager(store *db.Store, discord discordhelper.Client, cfg *config.Config, soloCategoryID, archiveCategoryID string) *Manager {
	return &Manager{
		sessions:          make(map[string]*Session),
		byName:            make(map[string]*Session),
		byChan:            make(map[string]*Session),
		store:             store,
		discord:           discord,
		cfg:               cfg,
		soloCategoryID:    soloCategoryID,
		archiveCategoryID: archiveCategoryID,
	}
}

// Spawn creates a new session: workspace, Discord channel, DB record, subprocess.
func (m *Manager) Spawn(ctx context.Context, opts SpawnOpts) (*Session, error) {
	id := uuid.New().String()
	// Pre-assign the Claude session ID so we don't need directory diffing.
	claudeSID := uuid.New().String()

	name := opts.Name
	if name == "" {
		name = generateName()
	}

	workspace := filepath.Join(m.cfg.SessionRoot, id)
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		return nil, fmt.Errorf("create workspace: %w", err)
	}

	catID := opts.CategoryID
	if catID == "" {
		catID = m.soloCategoryID
	}

	channelID, err := discordhelper.CreateChannel(m.discord, m.cfg.GuildID, catID, name)
	if err != nil {
		return nil, fmt.Errorf("create channel: %w", err)
	}

	dbSess := db.Session{
		ID:        id,
		Name:      name,
		ClaudeSID: claudeSID,
		Workspace: workspace,
		ChannelID: channelID,
		SwarmID:   opts.SwarmID,
		Status:    StatusCold,
	}
	if err := m.store.CreateSession(dbSess); err != nil {
		return nil, fmt.Errorf("create session record: %w", err)
	}

	sess := New(id, name, workspace, channelID, opts.SwarmID)
	sess.ClaudeSID = claudeSID

	m.mu.Lock()
	m.sessions[id] = sess
	m.byName[name] = sess
	m.byChan[channelID] = sess
	m.mu.Unlock()

	idleTimeout := time.Duration(m.cfg.IdleTimeoutMinutes) * time.Minute
	if err := sess.Warm(ctx, m.cfg.ClaudeBin, systemPromptPath(), idleTimeout, m.makeCallbacks(ctx)); err != nil {
		return nil, fmt.Errorf("warm: %w", err)
	}
	if err := m.store.UpdateSessionStatus(id, StatusHot); err != nil {
		return nil, err
	}

	if opts.Task != "" {
		if err := sess.Send(opts.Task); err != nil {
			return nil, fmt.Errorf("send initial task: %w", err)
		}
	}

	return sess, nil
}

// Kill terminates a session by name and archives its Discord channel.
func (m *Manager) Kill(name string) error {
	m.mu.RLock()
	sess := m.byName[name]
	m.mu.RUnlock()
	if sess == nil {
		return fmt.Errorf("session %q not found", name)
	}

	sess.Terminate()

	if err := m.store.UpdateSessionStatus(sess.ID, StatusTerminated); err != nil {
		return err
	}

	return discordhelper.ArchiveChannel(m.discord, m.cfg.GuildID, sess.ChannelID, m.archiveCategoryID)
}

// WarmIfCold warms a cold session by ID. No-op if already hot.
func (m *Manager) WarmIfCold(ctx context.Context, id string) error {
	m.mu.RLock()
	sess := m.sessions[id]
	m.mu.RUnlock()
	if sess == nil {
		return fmt.Errorf("session %s not found", id)
	}
	if sess.Status == StatusHot {
		return nil
	}
	// Reload ClaudeSID from DB in case it was updated.
	dbSess, err := m.store.GetSession(id)
	if err != nil {
		return err
	}
	sess.ClaudeSID = dbSess.ClaudeSID

	idleTimeout := time.Duration(m.cfg.IdleTimeoutMinutes) * time.Minute
	return sess.Warm(ctx, m.cfg.ClaudeBin, systemPromptPath(), idleTimeout, m.makeCallbacks(ctx))
}

// ByChannel returns the session whose Discord channel matches channelID, or nil.
func (m *Manager) ByChannel(channelID string) *Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.byChan[channelID]
}

// ByName returns the session with the given name, or nil.
func (m *Manager) ByName(name string) *Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.byName[name]
}

// List returns all active sessions, optionally filtered by swarm ID.
func (m *Manager) List(swarmID string) []*Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []*Session
	for _, s := range m.sessions {
		if s.Status == StatusTerminated {
			continue
		}
		if swarmID != "" && s.SwarmID != swarmID {
			continue
		}
		out = append(out, s)
	}
	return out
}

func (m *Manager) makeCallbacks(ctx context.Context) Callbacks {
	return Callbacks{
		OnContent: func(channelID, content string) {
			_ = discordhelper.PostMessage(m.discord, channelID, content)
		},
		OnDirective: func(sess *Session, d directive.Directive) {
			m.handleDirective(ctx, sess, d)
		},
		OnIdle: func(sessID string) {
			_ = m.store.UpdateSessionStatus(sessID, StatusCold)
		},
	}
}

func (m *Manager) handleDirective(ctx context.Context, src *Session, d directive.Directive) {
	switch d.Type {
	case directive.TypeSpawn:
		catID := m.soloCategoryID
		if src.SwarmID != "" {
			if sw, err := m.store.GetSwarm(src.SwarmID); err == nil {
				catID = sw.CategoryID
			}
		}
		_, _ = m.Spawn(ctx, SpawnOpts{
			Name:       d.Name,
			SwarmID:    src.SwarmID,
			Task:       d.Task,
			CategoryID: catID,
		})

	case directive.TypeSend:
		m.mu.RLock()
		target := m.byName[d.To]
		m.mu.RUnlock()
		if target == nil {
			return
		}
		_ = m.WarmIfCold(ctx, target.ID)
		_ = target.Send(d.Message)

	case directive.TypeCreateChannel:
		catID := m.soloCategoryID
		if src.SwarmID != "" {
			if sw, err := m.store.GetSwarm(src.SwarmID); err == nil {
				catID = sw.CategoryID
			}
		}
		_, _ = discordhelper.CreateChannel(m.discord, m.cfg.GuildID, catID, d.Name)
	}
}

// systemPromptPath returns ~/.nova/system-prompt.txt.
func systemPromptPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".nova", "system-prompt.txt")
}

var adjectives = []string{"amber", "bold", "calm", "deft", "keen", "sage", "swift", "vast", "wry", "zeal"}
var nouns = []string{"atlas", "bloom", "crane", "drift", "ember", "flint", "grove", "haven", "isle", "jade"}

func generateName() string {
	return adjectives[rand.Intn(len(adjectives))] + "-" + nouns[rand.Intn(len(nouns))]
}
