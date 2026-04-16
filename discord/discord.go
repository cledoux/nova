// Package discord provides channel and category management helpers for Nova.
package discord

import (
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"
)

// Client wraps the discordgo methods used by Nova.
// *discordgo.Session satisfies this interface directly.
type Client interface {
	GuildChannels(guildID string, options ...discordgo.RequestOption) ([]*discordgo.Channel, error)
	GuildChannelCreateComplex(guildID string, data discordgo.GuildChannelCreateData, options ...discordgo.RequestOption) (*discordgo.Channel, error)
	ChannelEdit(channelID string, data *discordgo.ChannelEdit, options ...discordgo.RequestOption) (*discordgo.Channel, error)
	ChannelMessageSend(channelID, content string, options ...discordgo.RequestOption) (*discordgo.Message, error)
	ChannelTyping(channelID string, options ...discordgo.RequestOption) error
	ChannelPermissionSet(channelID, targetID string, targetType discordgo.PermissionOverwriteType, allow, deny int64, options ...discordgo.RequestOption) error
	MessageThreadStart(channelID, messageID string, name string, archiveDuration int, options ...discordgo.RequestOption) (*discordgo.Channel, error)
}

// EnsureCategory returns the ID of the category named name in guildID,
// creating it if it does not exist.
func EnsureCategory(c Client, guildID, name string) (string, error) {
	channels, err := c.GuildChannels(guildID)
	if err != nil {
		return "", fmt.Errorf("GuildChannels: %w", err)
	}
	for _, ch := range channels {
		if ch.Type == discordgo.ChannelTypeGuildCategory && ch.Name == name {
			return ch.ID, nil
		}
	}
	ch, err := c.GuildChannelCreateComplex(guildID, discordgo.GuildChannelCreateData{
		Name: name,
		Type: discordgo.ChannelTypeGuildCategory,
	})
	if err != nil {
		return "", fmt.Errorf("create category %q: %w", name, err)
	}
	return ch.ID, nil
}

// EnsureChannel returns the ID of the text channel named name in categoryID,
// creating it if it does not exist. If categoryID is empty, the search matches
// any channel with the given name regardless of its category, and new channels
// are created at the top level.
func EnsureChannel(c Client, guildID, categoryID, name string) (string, error) {
	channels, err := c.GuildChannels(guildID)
	if err != nil {
		return "", fmt.Errorf("GuildChannels: %w", err)
	}
	for _, ch := range channels {
		if ch.Type != discordgo.ChannelTypeGuildText || ch.Name != name {
			continue
		}
		if categoryID == "" || ch.ParentID == categoryID {
			return ch.ID, nil
		}
	}
	ch, err := c.GuildChannelCreateComplex(guildID, discordgo.GuildChannelCreateData{
		Name:     name,
		Type:     discordgo.ChannelTypeGuildText,
		ParentID: categoryID,
	})
	if err != nil {
		return "", fmt.Errorf("create channel %q: %w", name, err)
	}
	return ch.ID, nil
}

// CreateChannel creates a new text channel in categoryID and returns its ID.
// Unlike EnsureChannel, this always creates a new channel.
func CreateChannel(c Client, guildID, categoryID, name string) (string, error) {
	ch, err := c.GuildChannelCreateComplex(guildID, discordgo.GuildChannelCreateData{
		Name:     name,
		Type:     discordgo.ChannelTypeGuildText,
		ParentID: categoryID,
	})
	if err != nil {
		return "", fmt.Errorf("create channel %q: %w", name, err)
	}
	return ch.ID, nil
}

// ArchiveChannel renames channelID to "✓-<name>", moves it to
// archiveCategoryID, and makes it read-only.
func ArchiveChannel(c Client, guildID, channelID, archiveCategoryID string) error {
	channels, err := c.GuildChannels(guildID)
	if err != nil {
		return err
	}
	var current *discordgo.Channel
	for _, ch := range channels {
		if ch.ID == channelID {
			current = ch
			break
		}
	}
	if current == nil {
		return fmt.Errorf("channel %s not found", channelID)
	}

	newName := "✓-" + strings.TrimPrefix(current.Name, "✓-")
	if _, err := c.ChannelEdit(channelID, &discordgo.ChannelEdit{
		Name:     newName,
		ParentID: archiveCategoryID,
	}); err != nil {
		return fmt.Errorf("edit channel: %w", err)
	}

	// Deny SEND_MESSAGES for @everyone (targetID = guildID for @everyone role).
	const sendMessages = 0x0000000000000800
	if err := c.ChannelPermissionSet(channelID, guildID, discordgo.PermissionOverwriteTypeRole, 0, sendMessages); err != nil {
		return fmt.Errorf("set permissions: %w", err)
	}
	return nil
}

// SetChannelTopic updates a channel's topic string.
func SetChannelTopic(c Client, channelID, topic string) error {
	_, err := c.ChannelEdit(channelID, &discordgo.ChannelEdit{Topic: topic})
	return err
}

// PostMessage sends content to channelID, splitting into 2000-char chunks.
// Returns the ID of the first message posted (useful for threading).
func PostMessage(c Client, channelID, content string) (string, error) {
	const limit = 2000
	var firstID string
	for len(content) > 0 {
		chunk := content
		if len(chunk) > limit {
			chunk = content[:limit]
		}
		content = content[len(chunk):]
		msg, err := c.ChannelMessageSend(channelID, chunk)
		if err != nil {
			return firstID, err
		}
		if firstID == "" && msg != nil {
			firstID = msg.ID
		}
	}
	return firstID, nil
}

// PostThread creates a thread named name on messageID in channelID, then posts
// content into it split into 2000-char chunks. The thread auto-archives after
// 60 minutes of inactivity.
func PostThread(c Client, channelID, messageID, name, content string) error {
	thread, err := c.MessageThreadStart(channelID, messageID, name, 60)
	if err != nil {
		return fmt.Errorf("create thread: %w", err)
	}
	_, err = PostMessage(c, thread.ID, content)
	return err
}
