// Package commands registers and handles Nova slash commands.
package commands

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"nova/db"
	"nova/session"

	"github.com/bwmarrin/discordgo"
)

type handler struct {
	sessions *session.Manager
	store    *db.Store
	guildID  string
}

// Register installs the /nova command and interaction handler on dg.
func Register(dg *discordgo.Session, sessions *session.Manager, store *db.Store, guildID string) {
	h := &handler{sessions: sessions, store: store, guildID: guildID}
	dg.AddHandler(h.onInteraction)
}

// RegisterCommands creates the /nova application command. Must be called after
// dg.Open() so dg.State.User is populated.
func RegisterCommands(dg *discordgo.Session, guildID string) error {
	_, err := dg.ApplicationCommandCreate(dg.State.User.ID, guildID, novaCommand())
	return err
}

func novaCommand() *discordgo.ApplicationCommand {
	str := discordgo.ApplicationCommandOptionString
	sub := discordgo.ApplicationCommandOptionSubCommand
	opt := func(name, desc string, typ discordgo.ApplicationCommandOptionType, required bool) *discordgo.ApplicationCommandOption {
		return &discordgo.ApplicationCommandOption{Type: typ, Name: name, Description: desc, Required: required}
	}
	return &discordgo.ApplicationCommand{
		Name:        "nova",
		Description: "Manage Nova sessions",
		Options: []*discordgo.ApplicationCommandOption{
			{Type: sub, Name: "spawn", Description: "Spawn a new Claude session", Options: []*discordgo.ApplicationCommandOption{
				opt("name", "Session name (auto-generated if omitted)", str, false),
			}},
			{Type: sub, Name: "list", Description: "List active sessions"},
			{Type: sub, Name: "kill", Description: "Terminate a session", Options: []*discordgo.ApplicationCommandOption{
				opt("name", "Session name", str, true),
			}},
			{Type: sub, Name: "resume", Description: "Force-warm a cold session", Options: []*discordgo.ApplicationCommandOption{
				opt("name", "Session name", str, true),
			}},
			{Type: sub, Name: "status", Description: "Show session status", Options: []*discordgo.ApplicationCommandOption{
				opt("name", "Session name", str, true),
			}},
			{Type: sub, Name: "reset", Description: "Clear Claude context and re-orient from git history"},
			{Type: sub, Name: "clean", Description: "Delete workspaces of terminated sessions"},
			{Type: sub, Name: "restart", Description: "Restart the nova bot process (Docker restarts it automatically)"},
			{Type: sub, Name: "help", Description: "Show available commands"},
		},
	}
}

func (h *handler) onInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand {
		return
	}
	data := i.ApplicationCommandData()
	if data.Name != "nova" || len(data.Options) == 0 {
		return
	}
	sub := data.Options[0]
	invoker := interactionUser(i)
	slog.Info("slash command", "command", "/nova "+sub.Name, "invoker", invoker)
	ctx := context.Background()
	switch sub.Name {
	case "spawn":
		h.handleSpawn(ctx, s, i, sub)
	case "list":
		h.handleList(s, i)
	case "kill":
		h.handleKill(ctx, s, i, sub)
	case "resume":
		h.handleResume(ctx, s, i, sub)
	case "status":
		h.handleStatus(s, i, sub)
	case "reset":
		h.handleReset(ctx, s, i)
	case "clean":
		h.handleClean(s, i)
	case "restart":
		h.handleRestart(s, i)
	case "help":
		h.handleHelp(s, i)
	}
}

func (h *handler) handleSpawn(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate, sub *discordgo.ApplicationCommandInteractionDataOption) {
	opts := optMap(sub.Options)
	name, _ := opts["name"]

	sess, err := h.sessions.Spawn(ctx, session.SpawnOpts{Name: name})
	if err != nil {
		slog.Error("spawn failed", "name", name, "err", err)
		respondEphemeral(s, i, fmt.Sprintf("Failed to spawn session: %v", err))
		return
	}
	slog.Info("session spawned via command", "session", sess.Name, "channel_id", sess.ChannelID)
	respondEphemeral(s, i, fmt.Sprintf("Spawned **%s** → <#%s>", sess.Name, sess.ChannelID))
}

func (h *handler) handleList(s *discordgo.Session, i *discordgo.InteractionCreate) {
	sessions, err := h.store.ListSessions()
	if err != nil {
		respondEphemeral(s, i, "Error fetching sessions.")
		return
	}
	if len(sessions) == 0 {
		respondEphemeral(s, i, "No active sessions.")
		return
	}
	var sb strings.Builder
	sb.WriteString("```\nName            Status  Last Active\n")
	sb.WriteString("────────────────────────────────────\n")
	for _, sess := range sessions {
		sb.WriteString(fmt.Sprintf("%-16s%-8s%s\n",
			truncate(sess.Name, 15),
			sess.Status,
			sess.LastActive.Format(time.RFC822),
		))
	}
	sb.WriteString("```")
	respondEphemeral(s, i, sb.String())
}

func (h *handler) handleKill(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate, sub *discordgo.ApplicationCommandInteractionDataOption) {
	opts := optMap(sub.Options)
	name := opts["name"]
	if err := h.sessions.Kill(name); err != nil {
		slog.Error("kill failed", "name", name, "err", err)
		respondEphemeral(s, i, fmt.Sprintf("Kill failed: %v", err))
		return
	}
	slog.Info("session killed via command", "session", name)
	respondEphemeral(s, i, fmt.Sprintf("Session **%s** terminated.", name))
}

func (h *handler) handleResume(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate, sub *discordgo.ApplicationCommandInteractionDataOption) {
	opts := optMap(sub.Options)
	name := opts["name"]
	sess := h.sessions.ByName(name)
	if sess == nil {
		respondEphemeral(s, i, fmt.Sprintf("Session %q not found.", name))
		return
	}
	if err := h.sessions.WarmIfCold(ctx, sess.ID); err != nil {
		respondEphemeral(s, i, fmt.Sprintf("Resume failed: %v", err))
		return
	}
	respondEphemeral(s, i, fmt.Sprintf("Session **%s** is now hot.", name))
}

func (h *handler) handleStatus(s *discordgo.Session, i *discordgo.InteractionCreate, sub *discordgo.ApplicationCommandInteractionDataOption) {
	opts := optMap(sub.Options)
	name := opts["name"]
	dbSess, err := h.store.GetSessionByName(name)
	if err != nil {
		respondEphemeral(s, i, fmt.Sprintf("Session %q not found.", name))
		return
	}
	n, _ := h.store.CountMessages(dbSess.ID)
	msg := fmt.Sprintf("**%s**\nStatus: `%s`\nWorkspace: `%s`\nChannel: <#%s>\nMessages: %d\nLast active: %s",
		dbSess.Name, dbSess.Status, dbSess.Workspace, dbSess.ChannelID, n,
		dbSess.LastActive.Format(time.RFC1123))
	respondEphemeral(s, i, msg)
}

func (h *handler) handleReset(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate) {
	sess := h.sessions.ByChannel(i.ChannelID)
	if sess == nil {
		respondEphemeral(s, i, "No session is bound to this channel.")
		return
	}
	if err := h.sessions.Reset(ctx, sess.ID); err != nil {
		slog.Error("reset failed", "session", sess.Name, "err", err)
		respondEphemeral(s, i, fmt.Sprintf("Reset failed: %v", err))
		return
	}
	slog.Info("session reset via command", "session", sess.Name)
	respondEphemeral(s, i, fmt.Sprintf("Context reset for **%s**. Re-orienting...", sess.Name))
}

func (h *handler) handleClean(s *discordgo.Session, i *discordgo.InteractionCreate) {
	sessions, err := h.store.ListTerminatedSessions()
	if err != nil {
		respondEphemeral(s, i, "Error fetching terminated sessions.")
		return
	}
	var cleaned int
	for _, sess := range sessions {
		if err := os.RemoveAll(sess.Workspace); err == nil {
			cleaned++
		}
	}
	respondEphemeral(s, i, fmt.Sprintf("Cleaned %d workspace(s).", cleaned))
}

func (h *handler) handleRestart(s *discordgo.Session, i *discordgo.InteractionCreate) {
	respondEphemeral(s, i, "Restarting nova...")
	h.sessions.Restart(i.ChannelID)
}

func (h *handler) handleHelp(s *discordgo.Session, i *discordgo.InteractionCreate) {
	const msg = "```\n" +
		"/nova spawn [name]     Spawn a new Claude session\n" +
		"/nova list             List active sessions\n" +
		"/nova kill <name>      Terminate a session\n" +
		"/nova resume <name>    Force-warm a cold session\n" +
		"/nova status <name>    Show session status\n" +
		"/nova clean            Delete workspaces of terminated sessions\n" +
		"/nova reset            Clear Claude context and re-orient from git history\n" +
		"/nova restart          Restart the nova bot process\n" +
		"/nova help             Show this message\n" +
		"```"
	respondEphemeral(s, i, msg)
}

// interactionUser returns a display name for the user who triggered i.
func interactionUser(i *discordgo.InteractionCreate) string {
	if i.Member != nil && i.Member.User != nil {
		return i.Member.User.Username
	}
	if i.User != nil {
		return i.User.Username
	}
	return "unknown"
}

func respondEphemeral(s *discordgo.Session, i *discordgo.InteractionCreate, content string) {
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags:   discordgo.MessageFlagsEphemeral,
			Content: content,
		},
	})
}

func optMap(opts []*discordgo.ApplicationCommandInteractionDataOption) map[string]string {
	m := make(map[string]string)
	for _, o := range opts {
		if o.Value != nil {
			m[o.Name] = fmt.Sprintf("%v", o.Value)
		}
	}
	return m
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
