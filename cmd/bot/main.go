package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/hhhhhffd/go-gpt-tg-bot/internal/ai"
	"github.com/hhhhhffd/go-gpt-tg-bot/internal/assistant"
	"github.com/hhhhhffd/go-gpt-tg-bot/internal/config"
	ilog "github.com/hhhhhffd/go-gpt-tg-bot/internal/logger"
	"github.com/hhhhhffd/go-gpt-tg-bot/internal/platform/discord"
	"github.com/hhhhhffd/go-gpt-tg-bot/internal/ratelimit"
	"github.com/hhhhhffd/go-gpt-tg-bot/internal/settings"
	"github.com/hhhhhffd/go-gpt-tg-bot/internal/telegram"
	"github.com/hhhhhffd/go-gpt-tg-bot/pkg/version"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	logger := ilog.New(cfg.LogLevel)
	logger.Info("starting assistant", "version", version.Version)

	aiService := ai.NewService(cfg, logger)
	requestLimiter := ratelimit.New(ratelimit.Config{
		PerUser: cfg.RateLimit.PerUser,
		Global:  cfg.RateLimit.Global,
		Window:  cfg.RateLimit.Window,
	})
	assistantService, err := assistant.NewService(assistant.Config{
		SystemPrompt:        cfg.AI.SystemPrompt,
		DeepSystemPrompt:    cfg.AI.DeepSystemPrompt,
		MaxOutputTokens:     cfg.AI.MaxOutputTokens,
		DeepMaxOutputTokens: cfg.AI.DeepMaxOutputTokens,
		Temperature:         cfg.AI.Temperature,
	}, aiService)
	if err != nil {
		logger.Error("assistant init failed", "error", err)
		os.Exit(1)
	}
	settingsService := settings.New(
		config.ProvidersConfig{Primary: cfg.AI.PrimaryProvider, Fallback: cfg.AI.FallbackProvider},
		cfg,
		aiService,
		assistantService,
	)

	var telegramService *telegram.Service
	var telegramSettingsBot *telegram.SettingsBot
	if cfg.Telegram.Enabled {
		service, initErr := telegram.NewService(ctx, cfg, assistantService, logger, requestLimiter)
		if initErr != nil {
			logger.Error("telegram init failed", "error", initErr)
			os.Exit(1)
		}
		telegramService = service
		telegramSettingsBot = telegram.NewSettingsBot(cfg.Telegram, settingsService, logger)
	}

	var discordGateway *discord.Gateway
	if cfg.Discord.Enabled {
		discordClient := discord.NewAPIClient(cfg.Discord.BotToken, cfg.AI.RequestTimeout, logger)
		discordHandler, initErr := discord.NewHandler(ctx, cfg.Discord, assistantService, discordClient, logger, requestLimiter)
		if initErr != nil {
			logger.Error("discord init failed", "error", initErr)
			os.Exit(1)
		}
		discordHandler.SetSettingsService(settingsService)
		gateway, gatewayErr := discord.NewGateway(cfg.Discord, discordHandler, discordClient, logger)
		if gatewayErr != nil {
			logger.Error("discord gateway init failed", "error", gatewayErr)
			os.Exit(1)
		}
		discordGateway = gateway
	}

	errCh := make(chan error, 3)
	var wg sync.WaitGroup

	if telegramService != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := telegramService.Start(ctx); err != nil {
				errCh <- err
			}
		}()
	}
	if telegramSettingsBot != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := telegramSettingsBot.Start(ctx); err != nil {
				errCh <- err
			}
		}()
	}

	if discordGateway != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := discordGateway.Start(ctx); err != nil {
				errCh <- err
			}
		}()
	}

	select {
	case runErr := <-errCh:
		logger.Error("runtime error", "error", runErr)
		stop()
	case <-ctx.Done():
	}

	wg.Wait()
}
