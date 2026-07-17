package main

import (
	"context"
	"log"

	"github.com/hhhhhffd/go-gpt-tg-bot/internal/config"
	"github.com/hhhhhffd/go-gpt-tg-bot/internal/logger"
	"github.com/hhhhhffd/go-gpt-tg-bot/internal/platform/discord"
)

func main() {
	cfg, err := config.LoadDiscordRegistrationConfig()
	if err != nil {
		log.Fatalf("load discord registration config: %v", err)
	}

	logg := logger.New(cfg.LogLevel)
	client := discord.NewAPIClient(cfg.BotToken, cfg.RequestTimeout, logg)

	if err := client.RegisterGlobalCommands(context.Background(), cfg.ApplicationID, discord.GlobalCommands()); err != nil {
		log.Fatalf("register discord commands: %v", err)
	}

	logg.Info("discord global commands registered successfully")
}
