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
	"sync"
	"time"

	"github.com/hhhhhffd/go-gpt-tg-bot/internal/config"
)

const telegramAPIBaseURL = "https://api.telegram.org"

type SettingsExecutor interface{ Execute(input string) string }

type SettingsBot struct {
	token    string
	ownerID  int64
	settings SettingsExecutor
	logger   *slog.Logger
	client   *http.Client
	baseURL  string

	pendingMu sync.Mutex
	pending   map[int64]string
}

type botUpdate struct {
	UpdateID      int64             `json:"update_id"`
	Message       *botMessage       `json:"message"`
	CallbackQuery *botCallbackQuery `json:"callback_query"`
}

type botCallbackQuery struct {
	ID      string      `json:"id"`
	From    botUser     `json:"from"`
	Message *botMessage `json:"message"`
	Data    string      `json:"data"`
}

type botUser struct {
	ID int64 `json:"id"`
}

type botMessage struct {
	MessageID int64 `json:"message_id"`
	Chat      struct {
		ID   int64  `json:"id"`
		Type string `json:"type"`
	} `json:"chat"`
	From struct {
		ID int64 `json:"id"`
	} `json:"from"`
	Text string `json:"text"`
}

type inlineKeyboard struct {
	InlineKeyboard [][]inlineButton `json:"inline_keyboard"`
}

type inlineButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data"`
}

type botAPIResponse[T any] struct {
	OK          bool   `json:"ok"`
	Result      T      `json:"result"`
	Description string `json:"description"`
}

func NewSettingsBot(cfg config.TelegramConfig, settings SettingsExecutor, logger *slog.Logger) *SettingsBot {
	return &SettingsBot{
		token:    cfg.BotToken,
		ownerID:  cfg.OwnerID,
		settings: settings,
		logger:   logger,
		client:   &http.Client{Timeout: 35 * time.Second},
		baseURL:  telegramAPIBaseURL,
		pending:  make(map[int64]string),
	}
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
			b.handleUpdate(ctx, update)
		}
	}
	return nil
}

func (b *SettingsBot) handleUpdate(ctx context.Context, update botUpdate) {
	if update.Message != nil && b.isOwnerPrivateMessage(update.Message) {
		b.handleMessage(ctx, update.Message)
	}
	if update.CallbackQuery != nil && b.isOwnerPrivateCallback(update.CallbackQuery) {
		b.handleCallback(ctx, update.CallbackQuery)
	}
}

func (b *SettingsBot) isOwnerPrivateMessage(message *botMessage) bool {
	return message.Chat.Type == "private" && message.From.ID == b.ownerID
}

func (b *SettingsBot) isOwnerPrivateCallback(callback *botCallbackQuery) bool {
	return callback.Message != nil && callback.Message.Chat.Type == "private" && callback.From.ID == b.ownerID
}

func (b *SettingsBot) handleMessage(ctx context.Context, message *botMessage) {
	text := strings.TrimSpace(message.Text)
	if text == "/settings" || text == "/start" {
		b.clearPending(message.Chat.ID)
		if err := b.showPanel(ctx, message.Chat.ID, 0, panelMain()); err != nil {
			b.logger.Error("show settings panel", "error", err)
		}
		return
	}
	if strings.HasPrefix(text, "/settings ") {
		b.clearPending(message.Chat.ID)
		if err := b.sendTyping(ctx, message.Chat.ID); err != nil {
			b.logger.Warn("send settings typing action", "error", err)
		}
		result := b.settings.Execute(text)
		if isAPIKeyCommand(text) {
			b.deleteInputMessage(ctx, message)
		}
		if err := b.sendPanel(ctx, message.Chat.ID, panelResult(result)); err != nil {
			b.logger.Error("send settings command result", "error", err)
		}
		return
	}

	prefix, waiting := b.takePending(message.Chat.ID)
	if !waiting {
		if text != "" {
			if err := b.sendPanel(ctx, message.Chat.ID, panelHelp()); err != nil {
				b.logger.Error("send settings help", "error", err)
			}
		}
		return
	}

	if text == "/cancel" {
		if err := b.sendPanel(ctx, message.Chat.ID, panelMain()); err != nil {
			b.logger.Error("cancel settings edit", "error", err)
		}
		return
	}
	if err := b.sendTyping(ctx, message.Chat.ID); err != nil {
		b.logger.Warn("send settings typing action", "error", err)
	}
	result := b.settings.Execute(prefix + " " + text)
	if isAPIKeyCommand(prefix) {
		b.deleteInputMessage(ctx, message)
	}
	if err := b.sendPanel(ctx, message.Chat.ID, panelResult(result)); err != nil {
		b.logger.Error("send settings edit result", "error", err)
	}
}

func (b *SettingsBot) handleCallback(ctx context.Context, callback *botCallbackQuery) {
	if err := b.answerCallback(ctx, callback.ID); err != nil {
		b.logger.Warn("answer settings callback", "error", err)
	}

	chatID := callback.Message.Chat.ID
	messageID := callback.Message.MessageID
	action := callback.Data
	if action == "settings:main" {
		b.clearPending(chatID)
	}
	if prefix, prompt, ok := editAction(action); ok {
		b.setPending(chatID, prefix)
		panel := panelInput(prompt)
		if err := b.showPanel(ctx, chatID, messageID, panel); err != nil {
			b.logger.Error("show settings input", "error", err)
		}
		return
	}

	var panel settingsPanel
	switch action {
	case "settings:main":
		panel = panelMain()
	case "settings:ai":
		panel = panelAI(b.settings.Execute("/settings config"))
	case "settings:primary":
		panel = panelProvider("Основной провайдер", "primary", b.privateProvidersView())
	case "settings:fallback":
		panel = panelProvider("Резервный провайдер", "fallback", b.privateProvidersView())
	case "settings:limits":
		panel = panelLimits(b.settings.Execute("/settings config"))
	case "settings:access":
		panel = panelAccess(b.settings.Execute("/settings config"))
	case "settings:runtime":
		panel = panelRuntime(b.settings.Execute("/settings config"))
	case "settings:help":
		panel = panelHelp()
	case "settings:set:telegram:on":
		panel = panelResult(b.settings.Execute("/settings set config telegram enabled true"))
	case "settings:set:telegram:off":
		panel = panelResult(b.settings.Execute("/settings set config telegram enabled false"))
	case "settings:set:discord:on":
		panel = panelResult(b.settings.Execute("/settings set config discord enabled true"))
	case "settings:set:discord:off":
		panel = panelResult(b.settings.Execute("/settings set config discord enabled false"))
	case "settings:set:log:debug":
		panel = panelResult(b.settings.Execute("/settings set config app log_level debug"))
	case "settings:set:log:info":
		panel = panelResult(b.settings.Execute("/settings set config app log_level info"))
	default:
		panel = panelMain()
	}
	if err := b.showPanel(ctx, chatID, messageID, panel); err != nil {
		b.logger.Error("show settings panel", "error", err)
	}
}

func (b *SettingsBot) privateProvidersView() string {
	return b.settings.Execute("/settings providers")
}

func isAPIKeyCommand(input string) bool {
	fields := strings.Fields(input)
	return len(fields) >= 4 && fields[0] == "/settings" && fields[1] == "set" && fields[3] == "api_key"
}

func (b *SettingsBot) deleteInputMessage(ctx context.Context, message *botMessage) {
	if err := b.call(ctx, "deleteMessage", map[string]any{
		"chat_id":    strconv.FormatInt(message.Chat.ID, 10),
		"message_id": message.MessageID,
	}, nil); err != nil {
		b.logger.Warn("delete API key message", "error", err)
	}
}

func editAction(action string) (prefix, prompt string, ok bool) {
	actions := map[string]struct {
		prefix string
		prompt string
	}{
		"settings:edit:primary:name":              {"/settings set primary name", "Введите название основного провайдера."},
		"settings:edit:primary:api_key":           {"/settings set primary api_key", "Введите API key основного провайдера. Он останется в истории личного чата."},
		"settings:edit:primary:base_url":          {"/settings set primary base_url", "Введите base URL основного провайдера."},
		"settings:edit:primary:model":             {"/settings set primary model", "Введите модель основного провайдера."},
		"settings:edit:fallback:name":             {"/settings set fallback name", "Введите название резервного провайдера."},
		"settings:edit:fallback:api_key":          {"/settings set fallback api_key", "Введите API key резервного провайдера. Он останется в истории личного чата."},
		"settings:edit:fallback:base_url":         {"/settings set fallback base_url", "Введите base URL резервного провайдера."},
		"settings:edit:fallback:model":            {"/settings set fallback model", "Введите модель резервного провайдера."},
		"settings:edit:ai:system_prompt":          {"/settings set config ai system_prompt", "Введите основной системный промпт."},
		"settings:edit:ai:deep_system_prompt":     {"/settings set config ai deep_system_prompt", "Введите системный промпт для глубоких ответов."},
		"settings:edit:ai:max_output_tokens":      {"/settings set config ai max_output_tokens", "Введите лимит токенов обычного ответа."},
		"settings:edit:ai:deep_max_output_tokens": {"/settings set config ai deep_max_output_tokens", "Введите лимит токенов глубокого ответа."},
		"settings:edit:ai:temperature":            {"/settings set config ai temperature", "Введите температуру от 0 до 2."},
		"settings:edit:ai:request_timeout":        {"/settings set config ai request_timeout", "Введите timeout, например 30s или 1m."},
		"settings:edit:limits:per_user":           {"/settings set config rate_limit per_user", "Введите лимит запросов на пользователя."},
		"settings:edit:limits:global":             {"/settings set config rate_limit global", "Введите общий лимит запросов."},
		"settings:edit:limits:window":             {"/settings set config rate_limit window", "Введите окно лимита, например 1m."},
		"settings:access:add:telegram":            {"/settings allow telegram", "Введите Telegram ID пользователя для доступа."},
		"settings:access:remove:telegram":         {"/settings deny telegram", "Введите Telegram ID пользователя для удаления."},
		"settings:access:add:discord":             {"/settings allow discord", "Введите Discord ID пользователя для доступа."},
		"settings:access:remove:discord":          {"/settings deny discord", "Введите Discord ID пользователя для удаления."},
	}
	target, exists := actions[action]
	if !exists {
		return "", "", false
	}
	return target.prefix, target.prompt, true
}

func (b *SettingsBot) setPending(chatID int64, prefix string) {
	b.pendingMu.Lock()
	defer b.pendingMu.Unlock()
	b.pending[chatID] = prefix
}

func (b *SettingsBot) takePending(chatID int64) (string, bool) {
	b.pendingMu.Lock()
	defer b.pendingMu.Unlock()
	prefix, ok := b.pending[chatID]
	delete(b.pending, chatID)
	return prefix, ok
}

func (b *SettingsBot) clearPending(chatID int64) {
	b.pendingMu.Lock()
	defer b.pendingMu.Unlock()
	delete(b.pending, chatID)
}

func (b *SettingsBot) updates(ctx context.Context, offset int64) ([]botUpdate, error) {
	var response []botUpdate
	err := b.call(ctx, "getUpdates", map[string]any{
		"offset":          offset,
		"timeout":         30,
		"allowed_updates": []string{"message", "callback_query"},
	}, &response)
	return response, err
}

func (b *SettingsBot) showPanel(ctx context.Context, chatID, messageID int64, panel settingsPanel) error {
	if messageID == 0 {
		return b.sendPanel(ctx, chatID, panel)
	}
	return b.call(ctx, "editMessageText", map[string]any{
		"chat_id":      strconv.FormatInt(chatID, 10),
		"message_id":   messageID,
		"text":         panel.text,
		"reply_markup": panel.keyboard,
	}, nil)
}

func (b *SettingsBot) sendPanel(ctx context.Context, chatID int64, panel settingsPanel) error {
	return b.call(ctx, "sendMessage", map[string]any{
		"chat_id":      strconv.FormatInt(chatID, 10),
		"text":         panel.text,
		"reply_markup": panel.keyboard,
	}, nil)
}

func (b *SettingsBot) sendTyping(ctx context.Context, chatID int64) error {
	return b.call(ctx, "sendChatAction", map[string]any{
		"chat_id": strconv.FormatInt(chatID, 10),
		"action":  "typing",
	}, nil)
}

func (b *SettingsBot) answerCallback(ctx context.Context, callbackID string) error {
	return b.call(ctx, "answerCallbackQuery", map[string]any{"callback_query_id": callbackID}, nil)
}

func (b *SettingsBot) call(ctx context.Context, method string, payload any, result any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal telegram %s request: %w", method, err)
	}
	endpoint := strings.TrimRight(b.baseURL, "/") + "/bot" + b.token + "/" + method
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create telegram %s request: %w", method, err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := b.client.Do(req)
	if err != nil {
		return fmt.Errorf("telegram %s request: %w", method, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read telegram %s response: %w", method, err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("telegram %s status %d", method, resp.StatusCode)
	}
	var response botAPIResponse[json.RawMessage]
	if err := json.Unmarshal(data, &response); err != nil {
		return fmt.Errorf("decode telegram %s response: %w", method, err)
	}
	if !response.OK {
		return fmt.Errorf("telegram %s: %s", method, response.Description)
	}
	if result != nil && len(response.Result) > 0 {
		if err := json.Unmarshal(response.Result, result); err != nil {
			return fmt.Errorf("decode telegram %s result: %w", method, err)
		}
	}
	return nil
}

type settingsPanel struct {
	text     string
	keyboard inlineKeyboard
}

func panelMain() settingsPanel {
	return settingsPanel{
		text: "Панель управления\n\nВыберите раздел. Подробные настройки доступны только в этом личном чате.",
		keyboard: keyboard(
			row("🧠 AI", "settings:ai"),
			row("🔌 Основной", "settings:primary"),
			row("🛟 Резервный", "settings:fallback"),
			row("⏱ Лимиты", "settings:limits"),
			row("👥 Доступ", "settings:access"),
			row("⚙️ Runtime", "settings:runtime"),
			row("❔ Подсказки", "settings:help"),
		),
	}
}

func panelAI(status string) settingsPanel {
	return settingsPanel{text: "🧠 AI\n\n" + status, keyboard: keyboard(
		row("Системный промпт", "settings:edit:ai:system_prompt"),
		row("Deep-промпт", "settings:edit:ai:deep_system_prompt"),
		row("Токены", "settings:edit:ai:max_output_tokens", "Deep токены", "settings:edit:ai:deep_max_output_tokens"),
		row("Температура", "settings:edit:ai:temperature", "Timeout", "settings:edit:ai:request_timeout"),
		row("← Назад", "settings:main"),
	)}
}

func panelProvider(title, slot, status string) settingsPanel {
	return settingsPanel{text: "🔌 " + title + "\n\n" + status, keyboard: keyboard(
		row("Название", "settings:edit:"+slot+":name", "Модель", "settings:edit:"+slot+":model"),
		row("Base URL", "settings:edit:"+slot+":base_url"),
		row("API key", "settings:edit:"+slot+":api_key"),
		row("← Назад", "settings:main"),
	)}
}

func panelLimits(status string) settingsPanel {
	return settingsPanel{text: "⏱ Rate limit\n\n" + status, keyboard: keyboard(
		row("На пользователя", "settings:edit:limits:per_user"),
		row("Общий", "settings:edit:limits:global", "Окно", "settings:edit:limits:window"),
		row("← Назад", "settings:main"),
	)}
}

func panelAccess(status string) settingsPanel {
	return settingsPanel{text: "👥 Доступ\n\n" + status, keyboard: keyboard(
		row("+ Telegram", "settings:access:add:telegram", "− Telegram", "settings:access:remove:telegram"),
		row("+ Discord", "settings:access:add:discord", "− Discord", "settings:access:remove:discord"),
		row("← Назад", "settings:main"),
	)}
}

func panelRuntime(status string) settingsPanel {
	return settingsPanel{text: "⚙️ Runtime\n\n" + status + "\n\nИзменения платформ и log level вступят в силу после перезапуска.", keyboard: keyboard(
		row("Telegram вкл.", "settings:set:telegram:on", "выкл.", "settings:set:telegram:off"),
		row("Discord вкл.", "settings:set:discord:on", "выкл.", "settings:set:discord:off"),
		row("Логи debug", "settings:set:log:debug", "Логи info", "settings:set:log:info"),
		row("← Назад", "settings:main"),
	)}
}

func panelHelp() settingsPanel {
	return settingsPanel{text: "❔ Как пользоваться\n\n• /settings — открыть панель.\n• Нажмите кнопку поля, затем отправьте новое значение.\n• /cancel — отменить ввод.\n• В любом Guest Mode чате: @bot /settings подскажет, что панель находится в личном чате.\n\nAPI key показывается и меняется только здесь, в личном чате owner.", keyboard: keyboard(row("← В панель", "settings:main"))}
}

func panelInput(prompt string) settingsPanel {
	return settingsPanel{text: prompt + "\n\nДля отмены отправьте /cancel.", keyboard: keyboard(row("Отмена", "settings:main"))}
}

func panelResult(result string) settingsPanel {
	return settingsPanel{text: result, keyboard: keyboard(row("← В панель", "settings:main"))}
}

func keyboard(rows ...[]inlineButton) inlineKeyboard { return inlineKeyboard{InlineKeyboard: rows} }

func row(values ...string) []inlineButton {
	buttons := make([]inlineButton, 0, len(values)/2)
	for index := 0; index+1 < len(values); index += 2 {
		buttons = append(buttons, inlineButton{Text: values[index], CallbackData: values[index+1]})
	}
	return buttons
}
