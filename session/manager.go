package session

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
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
	Name      string // auto-generated if empty
	Task      string // injected as the first message if non-empty
	ChannelID string // attach to existing channel instead of creating one
	Workspace string // override session workspace directory; defaults to SessionRoot/<id>
}

// Manager owns all active sessions and handles their lifecycle.
type Manager struct {
	mu       sync.RWMutex
	sessions map[string]*Session // id → session
	byName   map[string]*Session
	byChan   map[string]*Session // channelID → session

	store   *db.Store
	discord discordhelper.Client
	cfg     *config.Config

	// typingMu guards typingCancels.
	typingMu      sync.Mutex
	typingCancels map[string]context.CancelFunc // channelID → cancel func for typing loop

	// RestartFn is called when a restart directive is received. Defaults to
	// os.Exit(0) so Docker / the process supervisor restarts the binary.
	RestartFn func()
}

// NewManager creates a Manager.
func NewManager(store *db.Store, discord discordhelper.Client, cfg *config.Config) *Manager {
	return &Manager{
		sessions:      make(map[string]*Session),
		byName:        make(map[string]*Session),
		byChan:        make(map[string]*Session),
		typingCancels: make(map[string]context.CancelFunc),
		store:         store,
		discord:       discord,
		cfg:           cfg,
		RestartFn:     func() { os.Exit(0) },
	}
}

// SpawnOrRevive creates a new session, or if a matching session already exists
// in the DB (e.g. after a restart), loads and warms that existing record.
// Lookup is by Name first, then by ChannelID if Name is empty.
func (m *Manager) SpawnOrRevive(ctx context.Context, opts SpawnOpts) (*Session, error) {
	var dbSess db.Session
	var found bool

	if opts.Name != "" {
		if s, err := m.store.GetSessionByName(opts.Name); err == nil {
			dbSess, found = s, true
		}
	} else if opts.ChannelID != "" {
		if s, err := m.store.GetSessionByChannelID(opts.ChannelID); err == nil {
			dbSess, found = s, true
		}
	}

	if found {
		// Check if already in memory (e.g. same session, different code path).
		m.mu.RLock()
		existing := m.sessions[dbSess.ID]
		m.mu.RUnlock()
		if existing != nil {
			return existing, nil
		}
		slog.Info("reviving existing session", "name", dbSess.Name, "id", dbSess.ID)
		if err := m.revive(ctx, dbSess); err != nil {
			return nil, err
		}
		m.mu.RLock()
		sess := m.sessions[dbSess.ID]
		m.mu.RUnlock()
		return sess, nil
	}

	return m.Spawn(ctx, opts)
}

// revive loads a DB session record into memory and warms it as a resume.
func (m *Manager) revive(ctx context.Context, dbSess db.Session) error {
	sess := New(dbSess.ID, dbSess.Name, dbSess.Workspace, dbSess.ChannelID)
	sess.ClaudeSID = dbSess.ClaudeSID
	// gen=1 so that Warm uses --resume: the Claude session already exists on disk.
	sess.gen = 1

	m.mu.Lock()
	m.sessions[dbSess.ID] = sess
	m.byName[dbSess.Name] = sess
	m.byChan[dbSess.ChannelID] = sess
	m.mu.Unlock()

	idleTimeout := time.Duration(m.cfg.IdleTimeoutMinutes) * time.Minute
	if err := sess.Warm(ctx, m.cfg.ClaudeBin, systemPromptPath(), idleTimeout, m.makeCallbacks(ctx)); err != nil {
		return fmt.Errorf("warm: %w", err)
	}
	if err := m.store.UpdateSessionStatus(dbSess.ID, StatusHot); err != nil {
		return err
	}
	slog.Debug("sending resume prompt", "session", dbSess.Name)
	if err := sess.Send(m.resumePrompt()); err != nil {
		return fmt.Errorf("send resume prompt: %w", err)
	}
	return nil
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

	slog.Info("spawning session", "name", name)

	workspace := opts.Workspace
	if workspace == "" {
		workspace = filepath.Join(m.cfg.SessionRoot, id)
	}
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		return nil, fmt.Errorf("create workspace: %w", err)
	}

	channelID := opts.ChannelID
	if channelID == "" {
		slog.Debug("creating Discord channel", "session", name)
		var err error
		channelID, err = discordhelper.CreateChannel(m.discord, m.cfg.GuildID, "", name)
		if err != nil {
			return nil, fmt.Errorf("create channel: %w", err)
		}
	} else {
		slog.Debug("attaching session to existing channel", "session", name, "channel_id", channelID)
	}

	dbSess := db.Session{
		ID:        id,
		Name:      name,
		ClaudeSID: claudeSID,
		Workspace: workspace,
		ChannelID: channelID,
		Status:    StatusCold,
	}
	if err := m.store.CreateSession(dbSess); err != nil {
		return nil, fmt.Errorf("create session record: %w", err)
	}

	sess := New(id, name, workspace, channelID)
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
	slog.Info("session ready", "name", name, "channel_id", channelID, "workspace", workspace)

	slog.Debug("sending boot prompt", "session", name)
	if err := sess.Send(m.bootPrompt()); err != nil {
		return nil, fmt.Errorf("send boot prompt: %w", err)
	}

	if opts.Task != "" {
		slog.Debug("sending initial task to session", "session", name, "task_len", len(opts.Task))
		if err := sess.Send(opts.Task); err != nil {
			return nil, fmt.Errorf("send initial task: %w", err)
		}
	}

	return sess, nil
}

// Kill terminates a session by name.
func (m *Manager) Kill(name string) error {
	m.mu.RLock()
	sess := m.byName[name]
	m.mu.RUnlock()
	if sess == nil {
		return fmt.Errorf("session %q not found", name)
	}

	slog.Info("killing session", "session", name)
	sess.Terminate()

	if err := m.store.UpdateSessionStatus(sess.ID, StatusTerminated); err != nil {
		return err
	}
	slog.Info("session terminated", "session", name)
	return nil
}

// postThinkingThread creates a thread on messageID and posts thinking blocks into it.
func (m *Manager) postThinkingThread(channelID, messageID string, thinking []string) {
	combined := strings.Join(thinking, "\n\n---\n\n")
	slog.Info("posting thinking thread", "channel_id", channelID, "message_id", messageID, "len", len(combined))
	if err := discordhelper.PostThread(m.discord, channelID, messageID, "💭 Thinking", combined); err != nil {
		slog.Error("failed to post thinking thread", "channel_id", channelID, "err", err)
	}
}

// Restart posts a notice to channelID and then calls RestartFn (default: os.Exit(0)).
// Docker's restart: unless-stopped policy brings the process back up.
func (m *Manager) Restart(channelID string) {
	slog.Info("restart requested", "channel_id", channelID)
	_, _ = discordhelper.PostMessage(m.discord, channelID, "Restarting nova... brb")
	if m.RestartFn != nil {
		m.RestartFn()
	}
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
		slog.Debug("session already hot, skipping warm", "session", sess.Name)
		return nil
	}
	slog.Info("warming cold session", "session", sess.Name)
	// Reload ClaudeSID from DB in case it was updated.
	dbSess, err := m.store.GetSession(id)
	if err != nil {
		return err
	}
	sess.ClaudeSID = dbSess.ClaudeSID

	idleTimeout := time.Duration(m.cfg.IdleTimeoutMinutes) * time.Minute
	if err := sess.Warm(ctx, m.cfg.ClaudeBin, systemPromptPath(), idleTimeout, m.makeCallbacks(ctx)); err != nil {
		return err
	}
	slog.Info("session warmed", "session", sess.Name)
	return nil
}

// ChannelStats returns the most recent turn stats for the session bound to
// channelID. The second return value is false if no session is found or if no
// turn has completed yet.
func (m *Manager) ChannelStats(channelID string) (Stats, bool) {
	m.mu.RLock()
	sess := m.byChan[channelID]
	m.mu.RUnlock()
	if sess == nil {
		return Stats{}, false
	}
	st := sess.GetStats()
	if st.UpdatedAt.IsZero() {
		return Stats{}, false
	}
	return st, true
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

// List returns all active sessions.
func (m *Manager) List() []*Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []*Session
	for _, s := range m.sessions {
		if s.Status != StatusTerminated {
			out = append(out, s)
		}
	}
	return out
}

func (m *Manager) makeCallbacks(ctx context.Context) Callbacks {
	return Callbacks{
		OnTurnStart: func(channelID string) {
			m.startTyping(ctx, channelID)
		},
		OnContent: func(channelID, content string, thinking []string) {
			m.stopTyping(channelID)
			slog.Info("posting response to Discord", "channel_id", channelID,
				"content_len", len(content), "thinking_blocks", len(thinking))
			msgID, err := discordhelper.PostMessage(m.discord, channelID, content)
			if err != nil {
				slog.Error("failed to post message", "channel_id", channelID, "err", err)
				return
			}
			if len(thinking) > 0 && msgID != "" {
				go m.postThinkingThread(channelID, msgID, thinking)
			}
		},
		OnDirective: func(sess *Session, d directive.Directive) {
			slog.Info("handling directive", "session", sess.Name, "type", d.Type)
			m.handleDirective(sess, d)
		},
		OnIdle: func(sessID string) {
			m.mu.RLock()
			sess := m.sessions[sessID]
			m.mu.RUnlock()
			name := sessID
			if sess != nil {
				name = sess.Name
			}
			slog.Info("session went idle", "session", name)
			_ = m.store.UpdateSessionStatus(sessID, StatusCold)
		},
	}
}

// startTyping begins sending the Discord typing indicator to channelID every 8s
// until stopTyping is called. Any previous typing loop for the channel is cancelled first.
func (m *Manager) startTyping(ctx context.Context, channelID string) {
	ctx, cancel := context.WithCancel(ctx)

	m.typingMu.Lock()
	if old, ok := m.typingCancels[channelID]; ok {
		old()
	}
	m.typingCancels[channelID] = cancel
	m.typingMu.Unlock()

	go func() {
		for {
			if err := m.discord.ChannelTyping(channelID); err != nil {
				slog.Warn("ChannelTyping failed", "channel_id", channelID, "err", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(8 * time.Second):
			}
		}
	}()
}

// stopTyping cancels the typing indicator loop for channelID.
func (m *Manager) stopTyping(channelID string) {
	m.typingMu.Lock()
	defer m.typingMu.Unlock()
	if cancel, ok := m.typingCancels[channelID]; ok {
		cancel()
		delete(m.typingCancels, channelID)
	}
}

func (m *Manager) handleDirective(src *Session, d directive.Directive) {
	switch d.Type {
	case directive.TypeRestart:
		slog.Info("directive: restart requested", "from", src.Name)
		_, _ = discordhelper.PostMessage(m.discord, src.ChannelID, "Restarting nova... brb")
		if m.RestartFn != nil {
			m.RestartFn()
		}
	}
}

// bootPrompt returns the orientation message sent to Claude on fresh spawn or
// after a /reset. Claude reads the git log and posts a brief status to Discord.
func (m *Manager) bootPrompt() string {
	return fmt.Sprintf(
		"You are starting fresh. Read the git log in %s to orient yourself — "+
			"understand what the project does and what changed recently. "+
			"Post a short message to Discord summarising what you found: "+
			"what the project does, and what the most recent work was.",
		m.cfg.RepoPath,
	)
}

// resumePrompt returns the message sent after a session is revived following a
// restart. It asks Claude to check for unfinished work and always post a
// brief status update so the user knows orientation is complete.
func (m *Manager) resumePrompt() string {
	return fmt.Sprintf(
		"Nova has just restarted and your session has been revived. "+
			"Check `git log` and `git status` in %s to see if there is any work in progress "+
			"that was interrupted by the restart. "+
			"Then post a short message to Discord: if you were in the middle of something, "+
			"say what it was and pick up where you left off; "+
			"otherwise give a one-line confirmation that everything looks complete.",
		m.cfg.RepoPath,
	)
}

// Reset starts a fresh Claude session for sessID: stops the subprocess, assigns
// a new session ID (discarding conversation history), re-warms, and requeues
// the boot orientation prompt. /reset is not used because it is not recognised
// in stream-json pipe mode.
func (m *Manager) Reset(ctx context.Context, sessID string) error {
	m.mu.RLock()
	sess := m.sessions[sessID]
	m.mu.RUnlock()
	if sess == nil {
		return fmt.Errorf("session %s not found", sessID)
	}
	slog.Info("resetting session context", "session", sess.Name)

	newClaudeSID := uuid.New().String()
	sess.PrepareReset(newClaudeSID)

	if err := m.store.UpdateSessionClaudeSID(sessID, newClaudeSID); err != nil {
		return fmt.Errorf("update claude session id: %w", err)
	}

	idleTimeout := time.Duration(m.cfg.IdleTimeoutMinutes) * time.Minute
	if err := sess.Warm(ctx, m.cfg.ClaudeBin, systemPromptPath(), idleTimeout, m.makeCallbacks(ctx)); err != nil {
		return fmt.Errorf("warm after reset: %w", err)
	}
	if err := m.store.UpdateSessionStatus(sessID, StatusHot); err != nil {
		return err
	}
	if err := sess.Send(m.bootPrompt()); err != nil {
		return fmt.Errorf("send boot prompt: %w", err)
	}
	return nil
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
