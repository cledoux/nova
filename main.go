package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"nova/config"
	novadb "nova/nova/db"
	nova "nova/nova/nova"

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

	store, err := novadb.New("data/nova.db")
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

	log.Println("Nova running. Ctrl-C to stop.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM)
	<-sc
}
