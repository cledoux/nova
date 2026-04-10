// Package commands registers and handles Nova slash commands.
package commands

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"nova/nova/db"
	"nova/nova/session"
	"nova/nova/swarm"

	"github.com/bwmarrin/discordgo"
)

type handler struct {
	sessions *session.Manager
	swarms   *swarm.Manager
	store    *db.Store
	guildID  string
}

// Register installs the /nova command and interaction handler on dg.
func Register(dg *discordgo.Session, sessions *session.Manager, swarms *swarm.Manager, store *db.Store, guildID string) {
	h := &handler{sessions: sessions, swarms: swarms, store: store, guildID: guildID}
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
	grp := discordgo.ApplicationCommandOptionSubCommandGroup
	opt := func(name, desc string, typ discordgo.ApplicationCommandOptionType, required bool) *discordgo.ApplicationCommandOption {
		return &discordgo.ApplicationCommandOption{Type: typ, Name: name, Description: desc, Required: required}
	}
	return &discordgo.ApplicationCommand{
		Name:        "nova",
		Description: "Manage Claude swarm sessions",
		Options: []*discordgo.ApplicationCommandOption{
			{Type: sub, Name: "spawn", Description: "Spawn a new Claude session", Options: []*discordgo.ApplicationCommandOption{
				opt("name", "Session name (auto-generated if omitted)", str, false),
				opt("swarm", "Swarm to add this session to", str, false),
			}},
			{Type: sub, Name: "list", Description: "List active sessions", Options: []*discordgo.ApplicationCommandOption{
				opt("swarm", "Filter by swarm name", str, false),
			}},
			{Type: sub, Name: "kill", Description: "Terminate a session", Options: []*discordgo.ApplicationCommandOption{
				opt("name", "Session name", str, true),
			}},
			{Type: sub, Name: "resume", Description: "Force-warm a cold session", Options: []*discordgo.ApplicationCommandOption{
				opt("name", "Session name", str, true),
			}},
			{Type: sub, Name: "status", Description: "Show session status", Options: []*discordgo.ApplicationCommandOption{
				opt("name", "Session name", str, false),
			}},
			{Type: sub, Name: "clean", Description: "Delete workspaces of terminated sessions"},
			{Type: sub, Name: "broadcast", Description: "Send message to all sessions in a swarm", Options: []*discordgo.ApplicationCommandOption{
				opt("swarm", "Swarm name", str, true),
				opt("message", "Message to broadcast", str, true),
			}},
			{Type: grp, Name: "swarm", Description: "Manage swarms", Options: []*discordgo.ApplicationCommandOption{
				{Type: sub, Name: "create", Description: "Create a swarm", Options: []*discordgo.ApplicationCommandOption{
					opt("name", "Swarm name", str, true),
				}},
				{Type: sub, Name: "dissolve", Description: "Dissolve a swarm", Options: []*discordgo.ApplicationCommandOption{
					opt("name", "Swarm name", str, true),
				}},
			}},
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
	ctx := context.Background()
	switch sub.Name {
	case "spawn":
		h.handleSpawn(ctx, s, i, sub)
	case "list":
		h.handleList(s, i, sub)
	case "kill":
		h.handleKill(ctx, s, i, sub)
	case "resume":
		h.handleResume(ctx, s, i, sub)
	case "status":
		h.handleStatus(s, i, sub)
	case "clean":
		h.handleClean(s, i)
	case "broadcast":
		h.handleBroadcast(ctx, s, i, sub)
	case "swarm":
		h.handleSwarmGroup(ctx, s, i, sub)
	}
}

func (h *handler) handleSpawn(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate, sub *discordgo.ApplicationCommandInteractionDataOption) {
	opts := optMap(sub.Options)
	name, _ := opts["name"]
	swarmName, _ := opts["swarm"]

	var swarmID string
	if swarmName != "" {
		sw, err := h.store.GetSwarmByName(swarmName)
		if err != nil {
			respondEphemeral(s, i, fmt.Sprintf("Swarm %q not found.", swarmName))
			return
		}
		swarmID = sw.ID
	}

	sess, err := h.sessions.Spawn(ctx, session.SpawnOpts{Name: name, SwarmID: swarmID})
	if err != nil {
		respondEphemeral(s, i, fmt.Sprintf("Failed to spawn session: %v", err))
		return
	}
	respondEphemeral(s, i, fmt.Sprintf("Spawned **%s** → <#%s>", sess.Name, sess.ChannelID))
}

func (h *handler) handleList(s *discordgo.Session, i *discordgo.InteractionCreate, sub *discordgo.ApplicationCommandInteractionDataOption) {
	opts := optMap(sub.Options)
	swarmName, _ := opts["swarm"]

	var swarmID string
	if swarmName != "" {
		sw, err := h.store.GetSwarmByName(swarmName)
		if err != nil {
			respondEphemeral(s, i, fmt.Sprintf("Swarm %q not found.", swarmName))
			return
		}
		swarmID = sw.ID
	}

	sessions, err := h.store.ListSessions(swarmID)
	if err != nil {
		respondEphemeral(s, i, "Error fetching sessions.")
		return
	}
	if len(sessions) == 0 {
		respondEphemeral(s, i, "No active sessions.")
		return
	}
	var sb strings.Builder
	sb.WriteString("```\nName            Status  Swarm           Last Active\n")
	sb.WriteString("──────────────────────────────────────────────────────\n")
	for _, sess := range sessions {
		sb.WriteString(fmt.Sprintf("%-16s%-8s%-16s%s\n",
			truncate(sess.Name, 15),
			sess.Status,
			truncate(sess.SwarmID, 15),
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
		respondEphemeral(s, i, fmt.Sprintf("Kill failed: %v", err))
		return
	}
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
	name, _ := opts["name"]
	var dbSess db.Session
	var err error
	if name != "" {
		dbSess, err = h.store.GetSessionByName(name)
	} else {
		respondEphemeral(s, i, "Specify a session name.")
		return
	}
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

func (h *handler) handleBroadcast(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate, sub *discordgo.ApplicationCommandInteractionDataOption) {
	opts := optMap(sub.Options)
	swarmName := opts["swarm"]
	message := opts["message"]
	if err := h.swarms.Broadcast(ctx, swarmName, message); err != nil {
		respondEphemeral(s, i, fmt.Sprintf("Broadcast failed: %v", err))
		return
	}
	respondEphemeral(s, i, fmt.Sprintf("Broadcast sent to **%s**.", swarmName))
}

func (h *handler) handleSwarmGroup(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate, sub *discordgo.ApplicationCommandInteractionDataOption) {
	if len(sub.Options) == 0 {
		return
	}
	cmd := sub.Options[0]
	opts := optMap(cmd.Options)
	switch cmd.Name {
	case "create":
		sw, err := h.swarms.Create(opts["name"])
		if err != nil {
			respondEphemeral(s, i, fmt.Sprintf("Create failed: %v", err))
			return
		}
		respondEphemeral(s, i, fmt.Sprintf("Swarm **%s** created.", sw.Name))
	case "dissolve":
		if err := h.swarms.Dissolve(opts["name"]); err != nil {
			respondEphemeral(s, i, fmt.Sprintf("Dissolve failed: %v", err))
			return
		}
		respondEphemeral(s, i, fmt.Sprintf("Swarm **%s** dissolved.", opts["name"]))
	}
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
