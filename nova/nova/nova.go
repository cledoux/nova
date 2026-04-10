// Package nova is the top-level coordinator for the Nova Discord bot.
package nova

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"nova/config"
	"nova/nova/db"
	discordhelper "nova/nova/discord"
	"nova/nova/nova/commands"
	"nova/nova/session"
	"nova/nova/swarm"

	"github.com/bwmarrin/discordgo"
)

const systemPrompt = `You are an agent in a Discord-native swarm. Your responses are posted to a Discord channel.
Always end every response with {"type":"done"} on its own line.

To issue directives to the swarm, emit one JSON object per line with a "type" field.
Directives are intercepted by the bot and not posted to Discord.

Available directive types:
  {"type":"spawn","name":"<name>","task":"<initial message>"}
  {"type":"send","to":"<name>","message":"<msg>"}
  {"type":"create_channel","name":"<name>"}
  {"type":"done"}

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
	if err := writeSystemPrompt(); err != nil {
		return nil, nil, fmt.Errorf("write system prompt: %w", err)
	}

	guildID := cfg.GuildID

	// 2. Ensure control channel.
	_, err := discordhelper.EnsureChannel(dg, guildID, "", cfg.ControlChannelName)
	if err != nil {
		log.Printf("warn: could not ensure control channel: %v", err)
	}

	// 3. Ensure fixed categories.
	soloCatID, err := discordhelper.EnsureCategory(dg, guildID, "Nova: solo")
	if err != nil {
		return nil, nil, fmt.Errorf("ensure solo category: %w", err)
	}
	archiveCatID, err := discordhelper.EnsureCategory(dg, guildID, "Nova: archived")
	if err != nil {
		return nil, nil, fmt.Errorf("ensure archive category: %w", err)
	}

	// 4. Build managers.
	sessionMgr := session.NewManager(store, dg, cfg, soloCatID, archiveCatID)
	swarmMgr := swarm.NewManager(store, dg, sessionMgr, guildID)

	// 5. Register message router.
	RegisterMessageRouter(dg, sessionMgr)

	// 6. Register slash commands.
	commands.Register(dg, sessionMgr, swarmMgr, store, guildID)
	if err := commands.RegisterCommands(dg, guildID); err != nil {
		return nil, nil, fmt.Errorf("register commands: %w", err)
	}

	log.Println("Nova startup complete.")
	return sessionMgr, swarmMgr, nil
}

// RegisterMessageRouter installs the handler that routes Discord messages to
// the appropriate Claude session's stdin.
func RegisterMessageRouter(dg *discordgo.Session, mgr *session.Manager) {
	dg.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		// Ignore the bot's own messages.
		if m.Author.ID == s.State.User.ID {
			return
		}
		sess := mgr.ByChannel(m.ChannelID)
		if sess == nil {
			return
		}
		ctx := context.Background()
		if sess.Status == session.StatusCold {
			if err := mgr.WarmIfCold(ctx, sess.ID); err != nil {
				log.Printf("WarmIfCold %s: %v", sess.Name, err)
				return
			}
		}
		if err := sess.Send(m.Content); err != nil {
			log.Printf("Send to %s: %v", sess.Name, err)
		}
	})
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
