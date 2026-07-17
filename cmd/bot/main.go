package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/hhhhhffd/go-gpt-tg-bot/internal/ai"
	"github.com/hhhhhffd/go-gpt-tg-bot/internal/assistant"
	"github.com/hhhhhffd/go-gpt-tg-bot/internal/config"
	ilog "github.com/hhhhhffd/go-gpt-tg-bot/internal/logger"
	"github.com/hhhhhffd/go-gpt-tg-bot/internal/platform/discord"
	"github.com/hhhhhffd/go-gpt-tg-bot/internal/ratelimit"
	"github.com/hhhhhffd/go-gpt-tg-bot/internal/telegram"
	"github.com/hhhhhffd/go-gpt-tg-bot/pkg/version"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "healthcheck" {
		if err := runHealthcheck(); err != nil {
			log.Fatal(err)
		}
		return
	}

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

	var telegramService *telegram.Service
	if cfg.Telegram.Enabled {
		service, initErr := telegram.NewService(ctx, cfg, assistantService, logger, requestLimiter)
		if initErr != nil {
			logger.Error("telegram init failed", "error", initErr)
			os.Exit(1)
		}
		telegramService = service
	}

	var discordServer *discord.HTTPServer
	if cfg.Discord.Enabled {
		discordClient := discord.NewAPIClient(cfg.Discord.BotToken, cfg.AI.RequestTimeout, logger)
		discordHandler, initErr := discord.NewHandler(ctx, cfg.Discord, assistantService, discordClient, logger, requestLimiter)
		if initErr != nil {
			logger.Error("discord init failed", "error", initErr)
			os.Exit(1)
		}
		discordServer = discord.NewHTTPServer(cfg.HTTP.Addr, discordHandler, logger)
	}

	errCh := make(chan error, 2)
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

	if discordServer != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := discordServer.Start(); err != nil {
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

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if discordServer != nil {
		if err := discordServer.Shutdown(shutdownCtx); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("discord shutdown failed", "error", err)
		}
		discordServer.Wait()
	}

	wg.Wait()
}

func runHealthcheck() error {
	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("parse HTTP_ADDR for healthcheck: %w", err)
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}

	client := &http.Client{Timeout: 3 * time.Second}
	response, err := client.Get("http://" + net.JoinHostPort(host, port) + "/healthz")
	if err != nil {
		return fmt.Errorf("healthcheck request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("healthcheck returned status %d", response.StatusCode)
	}
	return nil
}
