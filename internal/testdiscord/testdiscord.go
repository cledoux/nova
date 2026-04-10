// Package testdiscord provides a fake discord.Client for use in tests.
package testdiscord

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/bwmarrin/discordgo"
)

// Message records a message sent via ChannelMessageSend.
type Message struct {
	ChannelID string
	Content   string
}

// Session is a fake discord.Client.
type Session struct {
	mu       sync.Mutex
	channels map[string]*discordgo.Channel
	Messages []Message
	idSeq    atomic.Int64
}

// New returns a new fake Session.
func New() *Session {
	return &Session{channels: make(map[string]*discordgo.Channel)}
}

func (s *Session) nextID() string {
	return fmt.Sprintf("fake-%d", s.idSeq.Add(1))
}

func (s *Session) GuildChannels(guildID string, options ...discordgo.RequestOption) ([]*discordgo.Channel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*discordgo.Channel
	for _, ch := range s.channels {
		if ch.GuildID == guildID {
			out = append(out, ch)
		}
	}
	return out, nil
}

func (s *Session) GuildChannelCreateComplex(guildID string, data discordgo.GuildChannelCreateData, options ...discordgo.RequestOption) (*discordgo.Channel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ch := &discordgo.Channel{
		ID:       s.nextID(),
		GuildID:  guildID,
		Name:     data.Name,
		Type:     data.Type,
		ParentID: data.ParentID,
		Topic:    data.Topic,
	}
	s.channels[ch.ID] = ch
	return ch, nil
}

func (s *Session) ChannelEdit(channelID string, data *discordgo.ChannelEdit, options ...discordgo.RequestOption) (*discordgo.Channel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ch, ok := s.channels[channelID]
	if !ok {
		return nil, fmt.Errorf("channel %s not found", channelID)
	}
	if data.Name != "" {
		ch.Name = data.Name
	}
	if data.ParentID != "" {
		ch.ParentID = data.ParentID
	}
	if data.Topic != "" {
		ch.Topic = data.Topic
	}
	return ch, nil
}

func (s *Session) ChannelMessageSend(channelID, content string, options ...discordgo.RequestOption) (*discordgo.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Messages = append(s.Messages, Message{ChannelID: channelID, Content: content})
	return &discordgo.Message{ID: s.nextID()}, nil
}

func (s *Session) ChannelPermissionSet(channelID, targetID string, targetType discordgo.PermissionOverwriteType, allow, deny int64, options ...discordgo.RequestOption) error {
	return nil
}

// GetChannel returns a channel by ID (test helper).
func (s *Session) GetChannel(id string) (*discordgo.Channel, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ch, ok := s.channels[id]
	return ch, ok
}
