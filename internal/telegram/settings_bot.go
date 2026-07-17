package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/hhhhhffd/go-gpt-tg-bot/internal/config"
)

type SettingsExecutor interface{ Execute(input string) string }
type SettingsBot struct {
	token    string
	ownerID  int64
	settings SettingsExecutor
	logger   *slog.Logger
	client   *http.Client
}
type botUpdate struct {
	UpdateID int64       `json:"update_id"`
	Message  *botMessage `json:"message"`
}
type botMessage struct {
	Chat struct {
		ID   int64  `json:"id"`
		Type string `json:"type"`
	} `json:"chat"`
	From struct {
		ID int64 `json:"id"`
	} `json:"from"`
	Text string `json:"text"`
}
type botAPIResponse[T any] struct {
	OK          bool   `json:"ok"`
	Result      T      `json:"result"`
	Description string `json:"description"`
}

func NewSettingsBot(cfg config.TelegramConfig, settings SettingsExecutor, logger *slog.Logger) *SettingsBot {
	return &SettingsBot{token: cfg.BotToken, ownerID: cfg.OwnerID, settings: settings, logger: logger, client: &http.Client{Timeout: 35 * time.Second}}
}
func (b *SettingsBot) Start(ctx context.Context) error {
	var offset int64
	for ctx.Err() == nil {
		updates, err := b.updates(ctx, offset)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			b.logger.Error("telegram settings polling failed", "error", err)
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(2 * time.Second):
			}
			continue
		}
		for _, update := range updates {
			offset = update.UpdateID + 1
			if update.Message == nil || update.Message.Chat.Type != "private" || update.Message.From.ID != b.ownerID {
				continue
			}
			text := strings.TrimSpace(update.Message.Text)
			if !strings.HasPrefix(text, "/settings") {
				continue
			}
			if err := b.send(ctx, update.Message.Chat.ID, b.settings.Execute(text)); err != nil {
				b.logger.Error("send telegram settings response", "error", err)
			}
		}
	}
	return nil
}
func (b *SettingsBot) updates(ctx context.Context, offset int64) ([]botUpdate, error) {
	endpoint := "https://api.telegram.org/bot" + b.token + "/getUpdates"
	body, _ := json.Marshal(map[string]any{"offset": offset, "timeout": 30, "allowed_updates": []string{"message"}})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	var payload botAPIResponse[[]botUpdate]
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	if !payload.OK {
		return nil, fmt.Errorf("telegram api: %s", payload.Description)
	}
	return payload.Result, nil
}
func (b *SettingsBot) send(ctx context.Context, chatID int64, text string) error {
	endpoint := "https://api.telegram.org/bot" + b.token + "/sendMessage"
	body, _ := json.Marshal(map[string]any{"chat_id": strconv.FormatInt(chatID, 10), "text": text})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("telegram send status %d", resp.StatusCode)
	}
	return nil
}
