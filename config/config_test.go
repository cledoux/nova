package config_test

import (
	"os"
	"testing"

	"nova/config"
)

func TestLoad_defaults(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "*.toml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`bot_token = "tok"`); err != nil {
		t.Fatal(err)
	}
	f.Close()

	cfg, err := config.Load(f.Name())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.BotToken != "tok" {
		t.Errorf("BotToken = %q, want %q", cfg.BotToken, "tok")
	}
	if cfg.ControlChannelName != "nova" {
		t.Errorf("ControlChannelName = %q, want nova", cfg.ControlChannelName)
	}
	if cfg.IdleTimeoutMinutes != 10 {
		t.Errorf("IdleTimeoutMinutes = %d, want 10", cfg.IdleTimeoutMinutes)
	}
	if cfg.ClaudeBin != "claude" {
		t.Errorf("ClaudeBin = %q, want claude", cfg.ClaudeBin)
	}
}

func TestLoad_overrides(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "*.toml")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString("bot_token = \"x\"\nidle_timeout_minutes = 5\ndebug = true\n")
	f.Close()

	cfg, err := config.Load(f.Name())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.IdleTimeoutMinutes != 5 {
		t.Errorf("IdleTimeoutMinutes = %d, want 5", cfg.IdleTimeoutMinutes)
	}
	if !cfg.Debug {
		t.Error("Debug should be true")
	}
}

func TestLoad_missingFile(t *testing.T) {
	_, err := config.Load("/nonexistent/nova-config.toml")
	if err == nil {
		t.Error("expected error for missing file")
	}
}
