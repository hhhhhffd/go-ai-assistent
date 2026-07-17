package discord

import (
	"context"
	"encoding/json"
	"errors"
	"expvar"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"github.com/hhhhhffd/go-gpt-tg-bot/internal/assistant"
	"github.com/hhhhhffd/go-gpt-tg-bot/internal/config"
	"github.com/hhhhhffd/go-gpt-tg-bot/internal/metrics"
	"github.com/hhhhhffd/go-gpt-tg-bot/internal/ratelimit"
)

const maxInteractionBodySize = 1 << 20

type Handler struct {
	baseCtx context.Context
	logger  *slog.Logger

	assistant assistant.Service
	client    InteractionClient
	verifier  *SignatureVerifier
	cfg       config.DiscordConfig
	limiter   *ratelimit.Limiter
	jobs      sync.WaitGroup
}

func NewHandler(
	baseCtx context.Context,
	cfg config.DiscordConfig,
	assistantService assistant.Service,
	client InteractionClient,
	logger *slog.Logger,
	limiters ...*ratelimit.Limiter,
) (*Handler, error) {
	verifier, err := NewSignatureVerifier(cfg.PublicKey)
	if err != nil {
		return nil, err
	}

	var limiter *ratelimit.Limiter
	if len(limiters) > 0 {
		limiter = limiters[0]
	}

	return &Handler{
		baseCtx:   baseCtx,
		logger:    logger,
		assistant: assistantService,
		client:    client,
		verifier:  verifier,
		cfg:       cfg,
		limiter:   limiter,
	}, nil
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/discord/interactions", h.handleInteractions)
	mux.HandleFunc("/healthz", h.handleHealthz)
	mux.HandleFunc("/readyz", h.handleReadyz)
	mux.Handle("/metrics", expvar.Handler())
}

func (h *Handler) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) handleReadyz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (h *Handler) handleInteractions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	body, err := readLimitedBody(w, r)
	if err != nil {
		h.logger.Warn("discord interaction body read failed", "error", err)
		return
	}

	signature := r.Header.Get("X-Signature-Ed25519")
	timestamp := r.Header.Get("X-Signature-Timestamp")
	if err := h.verifier.Verify(timestamp, signature, body); err != nil {
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}

	var interaction Interaction
	if err := json.Unmarshal(body, &interaction); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	switch interaction.Type {
	case InteractionTypePing:
		writeJSON(w, http.StatusOK, InteractionResponse{
			Type: InteractionCallbackTypePong,
		})
		return
	case InteractionTypeApplicationCommand:
		h.handleApplicationCommand(w, interaction)
		return
	default:
		writeJSON(w, http.StatusOK, InteractionResponse{
			Type: InteractionCallbackTypeChannelMessageWithSource,
			Data: &InteractionCallbackData{
				Content:         "Неподдерживаемый тип interaction.",
				Flags:           MessageFlagEphemeral,
				AllowedMentions: safeAllowedMentions(),
			},
		})
		return
	}
}

func (h *Handler) handleApplicationCommand(w http.ResponseWriter, interaction Interaction) {
	metrics.DiscordRequests.Add(1)
	userID := interactionUserID(interaction)
	if !h.isAllowedUser(userID) {
		writeJSON(w, http.StatusOK, InteractionResponse{
			Type: InteractionCallbackTypeChannelMessageWithSource,
			Data: &InteractionCallbackData{
				Content:         "У вас нет доступа к этому приложению.",
				Flags:           MessageFlagEphemeral,
				AllowedMentions: safeAllowedMentions(),
			},
		})
		return
	}
	if h.limiter != nil && !h.limiter.Allow("discord:"+userID) {
		metrics.RateLimitRejected.Add(1)
		writeJSON(w, http.StatusOK, InteractionResponse{
			Type: InteractionCallbackTypeChannelMessageWithSource,
			Data: &InteractionCallbackData{
				Content:         "Слишком много запросов. Попробуйте позже.",
				Flags:           MessageFlagEphemeral,
				AllowedMentions: safeAllowedMentions(),
			},
		})
		return
	}

	request, publicRequested, err := parseAssistantRequest(interaction)
	if err != nil {
		writeJSON(w, http.StatusOK, InteractionResponse{
			Type: InteractionCallbackTypeChannelMessageWithSource,
			Data: &InteractionCallbackData{
				Content:         sanitizeDiscordText(err.Error()),
				Flags:           MessageFlagEphemeral,
				AllowedMentions: safeAllowedMentions(),
			},
		})
		return
	}

	ephemeral := true
	publicUnavailableReason := ""
	if publicRequested {
		if canPublic(interaction) {
			ephemeral = false
		} else {
			ephemeral = true
			publicUnavailableReason = "Публичный ответ недоступен в этом контексте, отправляю приватно."
		}
	}

	flags := 0
	if ephemeral {
		flags = MessageFlagEphemeral
	}

	writeJSON(w, http.StatusOK, InteractionResponse{
		Type: InteractionCallbackTypeDeferredChannelMessageWithSource,
		Data: &InteractionCallbackData{
			Flags: flags,
		},
	})

	// Discord requires the initial callback promptly. The deferred result is
	// delivered separately through the interaction webhook.
	h.jobs.Add(1)
	go func() {
		defer h.jobs.Done()
		h.processDeferredResponse(interaction, request, ephemeral, publicUnavailableReason)
	}()
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

func readLimitedBody(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	reader := http.MaxBytesReader(w, r.Body, maxInteractionBodySize)
	defer reader.Close()

	body, err := io.ReadAll(reader)
	if err != nil {
		http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
		return nil, err
	}

	return body, nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
