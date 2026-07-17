package discord

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/hhhhhffd/go-gpt-tg-bot/internal/config"
)

const gatewayURL = "wss://gateway.discord.gg/?v=10&encoding=json"

type Gateway struct {
	cfg     config.DiscordConfig
	handler *Handler
	client  GatewayInteractionClient
	logger  *slog.Logger
}

type gatewayEnvelope struct {
	Op int             `json:"op"`
	D  json.RawMessage `json:"d"`
	S  *int            `json:"s"`
	T  string          `json:"t"`
}

func NewGateway(cfg config.DiscordConfig, handler *Handler, client GatewayInteractionClient, logger *slog.Logger) (*Gateway, error) {
	if handler == nil || client == nil {
		return nil, fmt.Errorf("discord gateway dependencies are required")
	}
	return &Gateway{cfg: cfg, handler: handler, client: client, logger: logger}, nil
}

func (g *Gateway) Start(ctx context.Context) error {
	for ctx.Err() == nil {
		if err := g.run(ctx); err != nil && ctx.Err() == nil {
			g.logger.Error("discord gateway disconnected", "error", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(3 * time.Second):
		}
	}
	return nil
}

func (g *Gateway) run(ctx context.Context) error {
	conn, _, err := websocket.Dial(ctx, gatewayURL, nil)
	if err != nil {
		return fmt.Errorf("dial discord gateway: %w", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "shutdown")
	var writeMu sync.Mutex
	send := func(payload any) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return wsjson.Write(ctx, conn, payload)
	}
	var heartbeatCancel context.CancelFunc
	defer func() {
		if heartbeatCancel != nil {
			heartbeatCancel()
		}
	}()
	for {
		var event gatewayEnvelope
		if err := wsjson.Read(ctx, conn, &event); err != nil {
			return fmt.Errorf("read discord gateway: %w", err)
		}
		switch event.Op {
		case 10:
			var hello struct {
				HeartbeatInterval int `json:"heartbeat_interval"`
			}
			if err := json.Unmarshal(event.D, &hello); err != nil {
				return fmt.Errorf("decode gateway hello: %w", err)
			}
			hbCtx, cancel := context.WithCancel(ctx)
			heartbeatCancel = cancel
			go func() {
				ticker := time.NewTicker(time.Duration(hello.HeartbeatInterval) * time.Millisecond)
				defer ticker.Stop()
				for {
					select {
					case <-hbCtx.Done():
						return
					case <-ticker.C:
						_ = send(map[string]any{"op": 1, "d": nil})
					}
				}
			}()
			if err := send(map[string]any{"op": 2, "d": map[string]any{"token": g.cfg.BotToken, "intents": 0, "properties": map[string]string{"os": "linux", "browser": "go-ai-assistant", "device": "go-ai-assistant"}}}); err != nil {
				return fmt.Errorf("identify discord gateway: %w", err)
			}
		case 0:
			if event.T != "INTERACTION_CREATE" {
				continue
			}
			var interaction Interaction
			if err := json.Unmarshal(event.D, &interaction); err != nil {
				g.logger.Error("decode discord interaction", "error", err)
				continue
			}
			response, err := g.handler.PrepareGatewayInteraction(interaction)
			if err != nil {
				g.logger.Error("prepare discord interaction", "error", err)
				continue
			}
			if err := g.client.CreateInteractionResponse(ctx, interaction.ID, interaction.Token, response); err != nil {
				g.logger.Error("respond to discord interaction", "error", err)
			}
		case 7, 9:
			return fmt.Errorf("discord gateway requested reconnect")
		}
	}
}
