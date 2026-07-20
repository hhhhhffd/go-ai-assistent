package telegram

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/hhhhhffd/go-gpt-tg-bot/internal/config"
	"github.com/hhhhhffd/go-gpt-tg-bot/internal/logger"
)

type fakeSettingsExecutor struct {
	mu       sync.Mutex
	commands []string
}

func (f *fakeSettingsExecutor) Execute(input string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.commands = append(f.commands, input)
	return "Сохранено"
}

func (f *fakeSettingsExecutor) PrivateProvidersView() string { return "api_key=secret" }

func TestSettingsBotEditsValueAfterCallback(t *testing.T) {
	var methods []string
	var methodsMu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		methodsMu.Lock()
		methods = append(methods, request.URL.Path)
		methodsMu.Unlock()
		writer.Header().Set("Content-Type", "application/json")
		if _, err := writer.Write([]byte(`{"ok":true,"result":true}`)); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer server.Close()

	executor := &fakeSettingsExecutor{}
	bot := NewSettingsBot(config.TelegramConfig{BotToken: "token", OwnerID: 42}, executor, logger.New("error"))
	bot.baseURL = server.URL

	callback := &botCallbackQuery{
		ID:      "callback",
		From:    botUser{ID: 42},
		Data:    "settings:edit:primary:model",
		Message: &botMessage{MessageID: 100},
	}
	callback.Message.Chat.ID = 42
	callback.Message.Chat.Type = "private"
	bot.handleCallback(context.Background(), callback)

	message := &botMessage{Text: "gpt-4.1"}
	message.Chat.ID = 42
	message.Chat.Type = "private"
	message.From.ID = 42
	bot.handleMessage(context.Background(), message)

	executor.mu.Lock()
	defer executor.mu.Unlock()
	if len(executor.commands) != 1 || executor.commands[0] != "/settings set primary model gpt-4.1" {
		t.Fatalf("unexpected commands: %#v", executor.commands)
	}

	methodsMu.Lock()
	defer methodsMu.Unlock()
	joined := strings.Join(methods, " ")
	for _, method := range []string{"answerCallbackQuery", "editMessageText", "sendChatAction", "sendMessage"} {
		if !strings.Contains(joined, method) {
			t.Fatalf("expected %s call, got %v", method, methods)
		}
	}
}

func TestSettingsBotIgnoresNonOwnerUpdates(t *testing.T) {
	executor := &fakeSettingsExecutor{}
	bot := NewSettingsBot(config.TelegramConfig{BotToken: "token", OwnerID: 42}, executor, logger.New("error"))
	message := &botMessage{Text: "/settings"}
	message.Chat.ID = 7
	message.Chat.Type = "private"
	message.From.ID = 7

	bot.handleUpdate(context.Background(), botUpdate{Message: message})

	executor.mu.Lock()
	defer executor.mu.Unlock()
	if len(executor.commands) != 0 {
		t.Fatalf("non-owner executed commands: %#v", executor.commands)
	}
}

func TestEditAction(t *testing.T) {
	prefix, _, ok := editAction("settings:access:add:telegram")
	if !ok || prefix != "/settings allow telegram" {
		t.Fatalf("unexpected edit action: prefix=%q ok=%t", prefix, ok)
	}
	if _, _, ok := editAction("settings:unknown"); ok {
		t.Fatal("unknown action must not create an edit")
	}
}
