package config

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config holds all Nova bot configuration loaded from config.toml.
type Config struct {
	BotToken           string `toml:"bot_token"`
	GuildID            string `toml:"guild_id"`
	ControlChannelName string `toml:"control_channel_name"`
	SessionRoot        string `toml:"session_root"`
	RepoPath           string `toml:"repo_path"`
	IdleTimeoutMinutes int    `toml:"idle_timeout_minutes"`
	ClaudeBin          string `toml:"claude_bin"`
	Debug              bool   `toml:"debug"`
}

// Load reads the TOML config file at path. Fields absent from the file keep
// their default values.
func Load(path string) (*Config, error) {
	home, _ := os.UserHomeDir()
	cfg := &Config{
		ControlChannelName: "nova",
		SessionRoot:        filepath.Join(home, ".nova", "sessions"),
		RepoPath:           "/home/agent/workspace",
		IdleTimeoutMinutes: 10,
		ClaudeBin:          "claude",
	}
	if _, err := toml.DecodeFile(path, cfg); err != nil {
		return nil, err
	}
	// Expand leading ~ in SessionRoot.
	if strings.HasPrefix(cfg.SessionRoot, "~/") {
		cfg.SessionRoot = filepath.Join(home, cfg.SessionRoot[2:])
	}
	return cfg, nil
}

// DefaultPath returns ~/.nova/config.toml.
func DefaultPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".nova", "config.toml")
}
