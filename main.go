package main

import (
	"context"
	"flag"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	nova "nova/bot"
	"nova/config"
	novadb "nova/db"

	"github.com/bwmarrin/discordgo"
)

func main() {
	cfgPath := flag.String("config", config.DefaultPath(), "path to config.toml")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if cfg.BotToken == "" {
		log.Fatal("bot_token is required in config.toml")
	}
	if cfg.GuildID == "" {
		log.Fatal("guild_id is required in config.toml")
	}

	// Configure structured logger; debug mode enables slog.LevelDebug output.
	logLevel := slog.LevelInfo
	if cfg.Debug {
		logLevel = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel})))

	slog.Info("nova starting",
		"guild_id", cfg.GuildID,
		"control_channel", cfg.ControlChannelName,
		"session_root", cfg.SessionRoot,
		"db_path", cfg.DBPath,
		"idle_timeout_min", cfg.IdleTimeoutMinutes,
		"claude_bin", cfg.ClaudeBin,
		"debug", cfg.Debug,
	)

	store, err := novadb.New(cfg.DBPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer store.Close()

	if err := store.ResetActiveSessions(); err != nil {
		log.Fatalf("reset sessions: %v", err)
	}

	dg, err := discordgo.New("Bot " + cfg.BotToken)
	if err != nil {
		log.Fatalf("create discord session: %v", err)
	}
	dg.Identify.Intents = nova.Intents()

	if err := dg.Open(); err != nil {
		log.Fatalf("open discord: %v", err)
	}
	defer dg.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if _, _, err := nova.Run(ctx, dg, store, cfg); err != nil {
		log.Fatalf("nova run: %v", err)
	}

	slog.Info("nova running — press Ctrl-C to stop")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM)
	<-sc
	slog.Info("nova shutting down")
}
