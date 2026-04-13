// Package nova is the top-level coordinator for the Nova Discord bot.
package bot

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"nova/bot/commands"
	"nova/config"
	"nova/db"
	discordhelper "nova/discord"
	"nova/session"
	"nova/swarm"

	"github.com/bwmarrin/discordgo"
)

const systemPrompt = `You are Nova, a Discord-native AI agent. Your responses are posted to a Discord channel.

## Nova's own codebase

Nova's source code is at /workspace (Go module: nova).
You can update nova's code and restart it yourself:

  1. Edit files under /workspace as needed.
  2. Run tests:  cd /workspace && go test ./...
  3. Rebuild:    cd /workspace && CGO_ENABLED=0 go build -o bin/nova .
  4. Restart:    emit {"type":"restart"} — nova exits and Docker restarts it with the new binary.

When nova comes back online it announces itself in the control channel.

## Swarm directives

To issue directives, emit one JSON object per line with a "type" field.
Directives are intercepted by the bot and not posted to Discord.

Available directive types:
  {"type":"spawn","name":"<name>","task":"<initial message>"}
  {"type":"send","to":"<name>","message":"<msg>"}
  {"type":"create_channel","name":"<name>"}
  {"type":"restart"}

All other output is posted to your Discord channel verbatim.`

// Intents returns the Discord gateway intents Nova requires.
func Intents() discordgo.Intent {
	return discordgo.IntentsGuilds |
		discordgo.IntentsGuildMessages |
		discordgo.IntentMessageContent
}

// Run performs the startup sequence that requires an open Discord connection.
// Returns initialized managers for use by main.
func Run(ctx context.Context, dg *discordgo.Session, store *db.Store, cfg *config.Config) (*session.Manager, *swarm.Manager, error) {
	// 1. Write system prompt file.
	slog.Debug("writing system prompt file")
	if err := writeSystemPrompt(); err != nil {
		return nil, nil, fmt.Errorf("write system prompt: %w", err)
	}

	guildID := cfg.GuildID

	// 2. Ensure control channel.
	slog.Debug("ensuring control channel", "name", cfg.ControlChannelName)
	controlChannelID, err := discordhelper.EnsureChannel(dg, guildID, "", cfg.ControlChannelName)
	if err != nil {
		return nil, nil, fmt.Errorf("ensure control channel: %w", err)
	}

	// 3. Ensure fixed categories.
	slog.Debug("ensuring Discord categories")
	soloCatID, err := discordhelper.EnsureCategory(dg, guildID, "Nova: solo")
	if err != nil {
		return nil, nil, fmt.Errorf("ensure solo category: %w", err)
	}
	archiveCatID, err := discordhelper.EnsureCategory(dg, guildID, "Nova: archived")
	if err != nil {
		return nil, nil, fmt.Errorf("ensure archive category: %w", err)
	}
	slog.Debug("Discord categories ready", "solo_cat", soloCatID, "archive_cat", archiveCatID)

	// 4. Build managers.
	sessionMgr := session.NewManager(store, dg, cfg, soloCatID, archiveCatID)
	swarmMgr := swarm.NewManager(store, dg, sessionMgr, guildID)

	// 5. Spawn (or revive on restart) the control session.
	slog.Info("ensuring control session", "name", cfg.ControlChannelName, "channel_id", controlChannelID)
	if _, err := sessionMgr.SpawnOrRevive(ctx, session.SpawnOpts{
		Name:      cfg.ControlChannelName,
		ChannelID: controlChannelID,
		Workspace: cfg.RepoPath,
	}); err != nil {
		return nil, nil, fmt.Errorf("spawn control session: %w", err)
	}

	// 6. Register message router.
	RegisterMessageRouter(dg, sessionMgr, cfg)

	// 7. Register slash commands.
	slog.Debug("registering slash commands")
	commands.Register(dg, sessionMgr, swarmMgr, store, guildID)
	if err := commands.RegisterCommands(dg, guildID); err != nil {
		return nil, nil, fmt.Errorf("register commands: %w", err)
	}

	// 8. Announce that nova is online (serves as restart-success confirmation).
	slog.Info("nova startup complete", "guild_id", guildID)
	if err := discordhelper.PostMessage(dg, controlChannelID, "Nova is online."); err != nil {
		slog.Warn("failed to post startup announcement", "err", err)
	}

	return sessionMgr, swarmMgr, nil
}

// RegisterMessageRouter installs the handler that routes Discord messages to
// the appropriate Claude session's stdin.
func RegisterMessageRouter(dg *discordgo.Session, mgr *session.Manager, cfg *config.Config) {
	dg.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		// Ignore the bot's own messages.
		if m.Author.ID == s.State.User.ID {
			return
		}

		ctx := context.Background()

		sess := mgr.ByChannel(m.ChannelID)
		if sess == nil {
			// In unmanaged channels, only respond if the bot is @mentioned or
			// the bot's name appears in the message content.
			if !botIsMentioned(s, m, cfg.ControlChannelName) {
				slog.Debug("message in unmanaged channel without bot mention, ignoring",
					"channel_id", m.ChannelID,
					"author", m.Author.Username,
				)
				return
			}
			sess = mgr.ByName(cfg.ControlChannelName)
			if sess == nil {
				slog.Warn("bot mentioned in unmanaged channel but no control session exists",
					"channel_id", m.ChannelID,
					"author", m.Author.Username,
					"control_session", cfg.ControlChannelName,
				)
				return
			}
			slog.Info("routing mention in unmanaged channel to control session",
				"channel_id", m.ChannelID,
				"author", m.Author.Username,
				"control_session", cfg.ControlChannelName,
			)
		}

		if sess.Status == session.StatusCold {
			slog.Info("warming cold session for incoming message", "session", sess.Name, "author", m.Author.Username)
			if err := mgr.WarmIfCold(ctx, sess.ID); err != nil {
				log.Printf("WarmIfCold %s: %v", sess.Name, err)
				return
			}
		}
		slog.Info("routing message to session",
			"session", sess.Name,
			"author", m.Author.Username,
			"content_len", len(m.Content),
		)
		if err := sess.Send(m.Content); err != nil {
			log.Printf("Send to %s: %v", sess.Name, err)
		}
	})
}

// botIsMentioned returns true if the bot is @mentioned in the message or if
// the bot's name (controlName) appears in the message content.
func botIsMentioned(s *discordgo.Session, m *discordgo.MessageCreate, controlName string) bool {
	for _, u := range m.Mentions {
		if u.ID == s.State.User.ID {
			return true
		}
	}
	return strings.Contains(strings.ToLower(m.Content), strings.ToLower(controlName))
}

func writeSystemPrompt() error {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".nova")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, "system-prompt.txt")
	return os.WriteFile(path, []byte(systemPrompt), 0o644)
}
