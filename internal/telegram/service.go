package telegram

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/hhhhhffd/go-gpt-tg-bot/internal/assistant"
	"github.com/hhhhhffd/go-gpt-tg-bot/internal/config"
	"github.com/hhhhhffd/go-gpt-tg-bot/internal/metrics"
	"github.com/hhhhhffd/go-gpt-tg-bot/internal/ratelimit"
	"github.com/hhhhhffd/go-gpt-tg-bot/internal/utils"
)

const guestMessageLimit = 4096

type Service struct {
	cfg       config.TelegramConfig
	assistant assistant.Service
	logger    *slog.Logger
	limiter   *ratelimit.Limiter
	username  string
	api       *tg.Client
}

func NewService(_ context.Context, cfg config.Config, assistantService assistant.Service, logger *slog.Logger, limiter *ratelimit.Limiter) (*Service, error) {
	if assistantService == nil {
		return nil, errors.New("telegram assistant service is required")
	}
	return &Service{cfg: cfg.Telegram, assistant: assistantService, logger: logger, limiter: limiter}, nil
}

func (s *Service) Start(ctx context.Context) error {
	client := telegram.NewClient(s.cfg.APIID, s.cfg.APIHash, telegram.Options{UpdateHandler: telegram.UpdateHandlerFunc(s.handleUpdates)})
	return client.Run(ctx, func(ctx context.Context) error {
		auth, err := client.Auth().Bot(ctx, s.cfg.BotToken)
		if err != nil {
			return fmt.Errorf("authorize telegram bot: %w", err)
		}
		user, ok := auth.User.(*tg.User)
		if !ok {
			return errors.New("telegram authorization returned unexpected user")
		}
		s.username = user.Username
		s.api = client.API()
		s.logger.Info("telegram guest mode connected", "bot_id", user.ID, "username", user.Username)
		<-ctx.Done()
		return nil
	})
}

func (s *Service) handleUpdates(ctx context.Context, updates tg.UpdatesClass) error {
	for _, update := range guestUpdates(updates) {
		if err := s.handleGuestQuery(ctx, update); err != nil {
			s.logger.Error("telegram guest query failed", "error", err)
		}
	}
	return nil
}

func guestUpdates(updates tg.UpdatesClass) []*tg.UpdateBotGuestChatQuery {
	var all []tg.UpdateClass
	switch value := updates.(type) {
	case *tg.Updates:
		all = value.Updates
	case *tg.UpdatesCombined:
		all = value.Updates
	case *tg.UpdateShort:
		all = []tg.UpdateClass{value.Update}
	}

	result := make([]*tg.UpdateBotGuestChatQuery, 0, len(all))
	for _, update := range all {
		if guest, ok := update.(*tg.UpdateBotGuestChatQuery); ok {
			result = append(result, guest)
		}
	}
	return result
}

func (s *Service) handleGuestQuery(ctx context.Context, update *tg.UpdateBotGuestChatQuery) error {
	message, ok := update.Message.(*tg.Message)
	if !ok {
		return errors.New("guest query contains unsupported message")
	}
	userID, ok := peerUserID(message.FromID)
	if !ok || !s.isAllowedUser(userID) {
		return s.reply(ctx, update.QueryID, "У вас нет доступа к этому приложению.")
	}
	if s.limiter != nil && !s.limiter.Allow(fmt.Sprintf("telegram:%d", userID)) {
		metrics.RateLimitRejected.Add(1)
		return s.reply(ctx, update.QueryID, "Слишком много запросов. Попробуйте позже.")
	}

	question := strings.TrimSpace(utils.StripMention(message.Message, s.username))
	if question == "" {
		return s.reply(ctx, update.QueryID, "Добавьте вопрос после упоминания бота.")
	}

	request := assistant.Request{Question: question}
	if references, exists := update.GetReferenceMessages(); exists {
		request.PreviousAnswer = firstMessageText(references)
	}

	metrics.TelegramGuests.Add(1)
	metrics.AIRequests.Add(1)
	answer, err := s.assistant.Answer(ctx, request)
	if err != nil {
		metrics.AIFailures.Add(1)
		s.logger.Error("telegram assistant request failed", "error", err, "user_id", userID)
		answer = "Временная ошибка AI-провайдера. Попробуйте чуть позже."
	}
	return s.reply(ctx, update.QueryID, answer)
}

func (s *Service) reply(ctx context.Context, queryID int64, text string) error {
	if s.api == nil {
		return errors.New("telegram api client is not initialized")
	}
	text = truncateRunes(strings.TrimSpace(text), guestMessageLimit)
	if text == "" {
		text = "Пустой ответ от AI-провайдера."
	}
	_, err := s.api.MessagesSetBotGuestChatResult(ctx, &tg.MessagesSetBotGuestChatResultRequest{
		QueryID: queryID,
		Result: &tg.InputBotInlineResult{
			ID:          fmt.Sprintf("%d", queryID),
			Type:        "article",
			Title:       "Ответ ассистента",
			SendMessage: &tg.InputBotInlineMessageText{Message: text},
		},
	})
	if err != nil {
		return fmt.Errorf("send guest response: %w", err)
	}
	return nil
}

func (s *Service) isAllowedUser(userID int64) bool {
	_, ok := s.cfg.AllowedUserIDs[userID]
	return ok
}

func peerUserID(peer tg.PeerClass) (int64, bool) {
	user, ok := peer.(*tg.PeerUser)
	if !ok {
		return 0, false
	}
	return user.UserID, true
}

func firstMessageText(messages []tg.MessageClass) string {
	for _, message := range messages {
		if text, ok := message.(*tg.Message); ok && strings.TrimSpace(text.Message) != "" {
			return strings.TrimSpace(text.Message)
		}
	}
	return ""
}

func truncateRunes(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return strings.TrimSpace(string(runes[:limit-1])) + "…"
}
