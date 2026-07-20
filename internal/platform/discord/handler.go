package discord

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/hhhhhffd/go-gpt-tg-bot/internal/assistant"
	"github.com/hhhhhffd/go-gpt-tg-bot/internal/config"
	"github.com/hhhhhffd/go-gpt-tg-bot/internal/metrics"
	"github.com/hhhhhffd/go-gpt-tg-bot/internal/ratelimit"
)

type Handler struct {
	baseCtx context.Context
	logger  *slog.Logger

	assistant assistant.Service
	client    InteractionClient
	cfg       config.DiscordConfig
	limiter   *ratelimit.Limiter
	jobs      sync.WaitGroup
	settings  SettingsExecutor
}

type SettingsExecutor interface{ Execute(input string) string }

func NewHandler(
	baseCtx context.Context,
	cfg config.DiscordConfig,
	assistantService assistant.Service,
	client InteractionClient,
	logger *slog.Logger,
	limiters ...*ratelimit.Limiter,
) (*Handler, error) {
	var limiter *ratelimit.Limiter
	if len(limiters) > 0 {
		limiter = limiters[0]
	}

	return &Handler{
		baseCtx:   baseCtx,
		logger:    logger,
		assistant: assistantService,
		client:    client,
		cfg:       cfg,
		limiter:   limiter,
	}, nil
}

func (h *Handler) SetSettingsService(service SettingsExecutor) { h.settings = service }

// PrepareGatewayInteraction creates the initial response for an interaction
// received through Discord Gateway and starts deferred AI work when required.
func (h *Handler) PrepareGatewayInteraction(interaction Interaction) (InteractionResponse, error) {
	metrics.DiscordRequests.Add(1)
	userID := interactionUserID(interaction)
	if interaction.Data != nil && strings.EqualFold(interaction.Data.Name, CommandSettings) {
		if userID != h.cfg.OwnerID {
			return privateResponse("Настройки доступны только owner."), nil
		}
		if h.settings == nil {
			return privateResponse("Сервис настроек недоступен."), nil
		}
		command := "/settings"
		for _, option := range interaction.Data.Options {
			if option.Name == "command" {
				var value string
				if err := json.Unmarshal(option.Value, &value); err == nil && strings.TrimSpace(value) != "" {
					command += " " + value
				}
			}
		}
		return privateResponse(h.settings.Execute(command)), nil
	}
	if !h.isAllowedUser(userID) {
		return privateResponse("У вас нет доступа к этому приложению."), nil
	}
	if h.limiter != nil && !h.limiter.Allow("discord:"+userID) {
		metrics.RateLimitRejected.Add(1)
		return privateResponse("Слишком много запросов. Попробуйте позже."), nil
	}
	request, publicRequested, err := parseAssistantRequest(interaction)
	if err != nil {
		return privateResponse(sanitizeDiscordText(err.Error())), nil
	}
	ephemeral := !publicRequested || !canPublic(interaction)
	flags := 0
	if ephemeral {
		flags = MessageFlagEphemeral
	}
	h.jobs.Add(1)
	go func() { defer h.jobs.Done(); h.processDeferredResponse(interaction, request, ephemeral, "") }()
	return InteractionResponse{Type: InteractionCallbackTypeDeferredChannelMessageWithSource, Data: &InteractionCallbackData{Flags: flags}}, nil
}

func privateResponse(content string) InteractionResponse {
	return InteractionResponse{Type: InteractionCallbackTypeChannelMessageWithSource, Data: &InteractionCallbackData{Content: content, Flags: MessageFlagEphemeral, AllowedMentions: safeAllowedMentions()}}
}

// Wait blocks until all accepted deferred interaction jobs finish. Their
// contexts are bounded by ProcessingLimit and are cancelled with baseCtx.
func (h *Handler) Wait() {
	h.jobs.Wait()
}

func (h *Handler) processDeferredResponse(
	interaction Interaction,
	request assistant.Request,
	ephemeral bool,
	publicUnavailableReason string,
) {
	ctx, cancel := context.WithTimeout(h.baseCtx, h.cfg.ProcessingLimit)
	defer cancel()

	defer func() {
		if recovered := recover(); recovered != nil {
			h.logger.Error("discord deferred handler panic", "panic", recovered)
			_ = h.client.EditOriginalInteractionResponse(ctx, interaction.ApplicationID, interaction.Token, EditWebhookMessageRequest{
				Content:         "Внутренняя ошибка обработки запроса.",
				AllowedMentions: safeAllowedMentions(),
			})
		}
	}()

	metrics.AIRequests.Add(1)
	answer, err := h.assistant.Answer(ctx, request)
	if err != nil {
		metrics.AIFailures.Add(1)
		h.logger.Error("discord assistant request failed", "error", err)
		_ = h.client.EditOriginalInteractionResponse(ctx, interaction.ApplicationID, interaction.Token, EditWebhookMessageRequest{
			Content:         "Временная ошибка AI-провайдера. Попробуйте чуть позже.",
			AllowedMentions: safeAllowedMentions(),
		})
		return
	}

	if publicUnavailableReason != "" {
		answer = publicUnavailableReason + "\n\n" + answer
	}

	parts := splitDiscordText(answer)
	if len(parts) == 0 {
		parts = []string{"Пустой ответ от AI-провайдера."}
	}

	if err := h.client.EditOriginalInteractionResponse(ctx, interaction.ApplicationID, interaction.Token, EditWebhookMessageRequest{
		Content:         parts[0],
		AllowedMentions: safeAllowedMentions(),
	}); err != nil {
		h.logger.Error("discord edit original response failed", "error", err)
		return
	}

	for _, part := range parts[1:] {
		followup := FollowupMessageRequest{
			Content:         part,
			AllowedMentions: safeAllowedMentions(),
		}
		if ephemeral {
			followup.Flags = MessageFlagEphemeral
		}

		if err := h.client.CreateFollowupMessage(ctx, interaction.ApplicationID, interaction.Token, followup); err != nil {
			h.logger.Error("discord followup send failed", "error", err)
			return
		}
	}
}

func parseAssistantRequest(interaction Interaction) (assistant.Request, bool, error) {
	if interaction.Data == nil {
		return assistant.Request{}, false, errors.New("interaction data is missing")
	}

	data := interaction.Data
	switch data.Type {
	case ApplicationCommandTypeChatInput:
		return parseChatInputCommand(data)
	case ApplicationCommandTypeMessage:
		return parseMessageCommand(data)
	default:
		return assistant.Request{}, false, fmt.Errorf("неподдерживаемая команда %q", data.Name)
	}
}

func parseChatInputCommand(data *ApplicationCommandData) (assistant.Request, bool, error) {
	name := strings.ToLower(strings.TrimSpace(data.Name))
	if name != CommandAsk && name != CommandDeep {
		return assistant.Request{}, false, fmt.Errorf("команда %q не поддерживается", data.Name)
	}

	question := ""
	public := false
	for _, option := range data.Options {
		switch option.Name {
		case "question":
			if err := json.Unmarshal(option.Value, &question); err != nil {
				return assistant.Request{}, false, errors.New("не удалось прочитать параметр question")
			}
		case "public":
			if err := json.Unmarshal(option.Value, &public); err != nil {
				return assistant.Request{}, false, errors.New("не удалось прочитать параметр public")
			}
		}
	}

	question = strings.TrimSpace(question)
	if question == "" {
		return assistant.Request{}, false, errors.New("параметр question обязателен")
	}

	return assistant.Request{
		Question: question,
		Deep:     name == CommandDeep,
	}, public, nil
}

func parseMessageCommand(data *ApplicationCommandData) (assistant.Request, bool, error) {
	if strings.TrimSpace(data.Name) != CommandExplain {
		return assistant.Request{}, false, fmt.Errorf("команда %q не поддерживается", data.Name)
	}

	if data.Resolved == nil || len(data.Resolved.Messages) == 0 {
		return assistant.Request{}, false, errors.New("выбранное сообщение не содержит доступного текста")
	}

	var target ResolvedMessage
	if data.TargetID != "" {
		message, ok := data.Resolved.Messages[data.TargetID]
		if !ok {
			return assistant.Request{}, false, errors.New("не удалось получить выбранное сообщение")
		}
		target = message
	} else {
		for _, message := range data.Resolved.Messages {
			target = message
			break
		}
	}

	content := strings.TrimSpace(target.Content)
	if content == "" {
		return assistant.Request{}, false, errors.New("в выбранном сообщении нет текста для объяснения")
	}

	return assistant.Request{
		Question: content,
	}, false, nil
}

func canPublic(interaction Interaction) bool {
	return interaction.Context == InteractionContextGuild || strings.TrimSpace(interaction.GuildID) != ""
}

func interactionUserID(interaction Interaction) string {
	if interaction.Member != nil && interaction.Member.User != nil {
		return interaction.Member.User.ID
	}
	if interaction.User != nil {
		return interaction.User.ID
	}
	return ""
}

func (h *Handler) isAllowedUser(userID string) bool {
	_, ok := h.cfg.AllowedUserIDs[strings.TrimSpace(userID)]
	return ok
}
