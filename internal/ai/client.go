package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"

	"github.com/hhhhhffd/go-gpt-tg-bot/internal/config"
)

const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
)

type Message struct {
	Role    string
	Content string
}

type CompletionRequest struct {
	SystemPrompt    string
	Messages        []Message
	MaxOutputTokens int
	Temperature     float64
}

type Client interface {
	Complete(ctx context.Context, request CompletionRequest) (string, error)
}

type ErrorKind string

const (
	ErrorKindConfig          ErrorKind = "config"
	ErrorKindBadRequest      ErrorKind = "bad_request"
	ErrorKindUnauthorized    ErrorKind = "unauthorized"
	ErrorKindForbidden       ErrorKind = "forbidden"
	ErrorKindUnknownModel    ErrorKind = "unknown_model"
	ErrorKindRateLimited     ErrorKind = "rate_limited"
	ErrorKindServer          ErrorKind = "server"
	ErrorKindNetwork         ErrorKind = "network"
	ErrorKindTimeout         ErrorKind = "timeout"
	ErrorKindInvalidResponse ErrorKind = "invalid_response"
	ErrorKindUnknown         ErrorKind = "unknown"
)

type ProviderError struct {
	Provider   string
	Kind       ErrorKind
	StatusCode int
	Err        error
	Message    string
}

func (e *ProviderError) Error() string {
	switch {
	case e.Err != nil && strings.TrimSpace(e.Message) != "":
		return fmt.Sprintf("%s provider %q error: %s: %v", e.Kind, e.Provider, e.Message, e.Err)
	case e.Err != nil:
		return fmt.Sprintf("%s provider %q error: %v", e.Kind, e.Provider, e.Err)
	case strings.TrimSpace(e.Message) != "":
		return fmt.Sprintf("%s provider %q error: %s", e.Kind, e.Provider, e.Message)
	default:
		return fmt.Sprintf("%s provider %q error", e.Kind, e.Provider)
	}
}

func (e *ProviderError) Unwrap() error {
	return e.Err
}

func ShouldFallback(err error) bool {
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) {
		return false
	}

	switch providerErr.Kind {
	case ErrorKindNetwork, ErrorKindTimeout, ErrorKindRateLimited, ErrorKindServer, ErrorKindInvalidResponse:
		return true
	default:
		return false
	}
}

type Service struct {
	httpClient *http.Client
	logger     *slog.Logger

	primaryProvider  config.ProviderConfig
	fallbackProvider config.ProviderConfig
	providersMu      sync.RWMutex
}

func NewService(cfg config.Config, logger *slog.Logger) *Service {
	return &Service{
		httpClient: &http.Client{
			Timeout: cfg.AI.RequestTimeout,
		},
		logger:           logger,
		primaryProvider:  cfg.AI.PrimaryProvider,
		fallbackProvider: cfg.AI.FallbackProvider,
	}
}

func (s *Service) Complete(ctx context.Context, request CompletionRequest) (string, error) {
	if err := validateRequest(request); err != nil {
		return "", &ProviderError{
			Provider: "local",
			Kind:     ErrorKindConfig,
			Err:      err,
			Message:  "invalid completion request",
		}
	}

	s.providersMu.RLock()
	primary, fallback := s.primaryProvider, s.fallbackProvider
	s.providersMu.RUnlock()
	answer, err := s.askProvider(ctx, primary, request)
	if err == nil {
		return answer, nil
	}

	if !ShouldFallback(err) {
		return "", err
	}

	s.logger.Warn("primary provider failed, switching to fallback",
		"provider", primary.Name,
		"error_kind", providerErrorKind(err),
	)

	answer, fallbackErr := s.askProvider(ctx, fallback, request)
	if fallbackErr != nil {
		return "", fmt.Errorf("primary failed: %w; fallback failed: %v", err, fallbackErr)
	}

	return answer, nil
}

// UpdateProviders applies owner-saved settings to subsequent requests.
func (s *Service) UpdateProviders(providers config.ProvidersConfig) {
	s.providersMu.Lock()
	defer s.providersMu.Unlock()
	s.primaryProvider = providers.Primary
	s.fallbackProvider = providers.Fallback
}

func validateRequest(request CompletionRequest) error {
	if strings.TrimSpace(request.SystemPrompt) == "" {
		return errors.New("system prompt is empty")
	}

	if request.MaxOutputTokens <= 0 {
		return errors.New("max output tokens must be > 0")
	}

	if len(request.Messages) == 0 {
		return errors.New("messages are empty")
	}

	for i, message := range request.Messages {
		if message.Role != RoleUser && message.Role != RoleAssistant {
			return fmt.Errorf("message #%d has unsupported role %q", i, message.Role)
		}
		if strings.TrimSpace(message.Content) == "" {
			return fmt.Errorf("message #%d content is empty", i)
		}
	}

	return nil
}

func (s *Service) askProvider(ctx context.Context, provider config.ProviderConfig, request CompletionRequest) (string, error) {
	if err := validateProvider(provider); err != nil {
		return "", &ProviderError{
			Provider: provider.Name,
			Kind:     ErrorKindConfig,
			Err:      err,
			Message:  "invalid provider configuration",
		}
	}

	requestBody := openAIChatCompletionRequest{
		Model:       provider.Model,
		Messages:    buildOpenAIMessages(request),
		MaxTokens:   request.MaxOutputTokens,
		Temperature: request.Temperature,
	}

	payload, err := json.Marshal(requestBody)
	if err != nil {
		return "", &ProviderError{
			Provider: provider.Name,
			Kind:     ErrorKindConfig,
			Err:      err,
			Message:  "marshal request failed",
		}
	}

	endpoint := buildChatCompletionsURL(provider.BaseURL)
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", &ProviderError{
			Provider: provider.Name,
			Kind:     ErrorKindConfig,
			Err:      err,
			Message:  "create request failed",
		}
	}

	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Authorization", "Bearer "+provider.APIKey)

	httpResponse, err := s.httpClient.Do(httpRequest)
	if err != nil {
		return "", classifyTransportError(provider.Name, err)
	}
	defer httpResponse.Body.Close()

	body, err := io.ReadAll(io.LimitReader(httpResponse.Body, 1<<20))
	if err != nil {
		return "", &ProviderError{
			Provider: provider.Name,
			Kind:     ErrorKindInvalidResponse,
			Err:      err,
			Message:  "read response failed",
		}
	}

	if httpResponse.StatusCode < http.StatusOK || httpResponse.StatusCode >= http.StatusMultipleChoices {
		return "", classifyAPIError(provider.Name, httpResponse.StatusCode, body)
	}

	var completionResponse openAIChatCompletionResponse
	if err := json.Unmarshal(body, &completionResponse); err != nil {
		return "", &ProviderError{
			Provider: provider.Name,
			Kind:     ErrorKindInvalidResponse,
			Err:      err,
			Message:  "decode response failed",
		}
	}

	if len(completionResponse.Choices) == 0 {
		return "", &ProviderError{
			Provider: provider.Name,
			Kind:     ErrorKindInvalidResponse,
			Message:  "empty choices in response",
		}
	}

	content := strings.TrimSpace(completionResponse.Choices[0].Message.Content)
	if content == "" {
		return "", &ProviderError{
			Provider: provider.Name,
			Kind:     ErrorKindInvalidResponse,
			Message:  "empty content in response",
		}
	}

	return content, nil
}

func buildOpenAIMessages(request CompletionRequest) []openAIChatMessage {
	messages := make([]openAIChatMessage, 0, len(request.Messages)+1)
	messages = append(messages, openAIChatMessage{
		Role:    RoleSystem,
		Content: request.SystemPrompt,
	})

	for _, message := range request.Messages {
		messages = append(messages, openAIChatMessage{
			Role:    message.Role,
			Content: message.Content,
		})
	}

	return messages
}

func validateProvider(provider config.ProviderConfig) error {
	if strings.TrimSpace(provider.Name) == "" {
		return errors.New("provider name is empty")
	}
	if strings.TrimSpace(provider.APIKey) == "" {
		return errors.New("provider api key is empty")
	}
	if strings.TrimSpace(provider.BaseURL) == "" {
		return errors.New("provider base url is empty")
	}
	if strings.TrimSpace(provider.Model) == "" {
		return errors.New("provider model is empty")
	}
	return nil
}

func classifyTransportError(providerName string, err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return &ProviderError{
			Provider: providerName,
			Kind:     ErrorKindTimeout,
			Err:      err,
			Message:  "request timed out",
		}
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return &ProviderError{
				Provider: providerName,
				Kind:     ErrorKindTimeout,
				Err:      err,
				Message:  "network timeout",
			}
		}

		return &ProviderError{
			Provider: providerName,
			Kind:     ErrorKindNetwork,
			Err:      err,
			Message:  "network error",
		}
	}

	return &ProviderError{
		Provider: providerName,
		Kind:     ErrorKindNetwork,
		Err:      err,
		Message:  "transport error",
	}
}

func classifyAPIError(providerName string, statusCode int, body []byte) error {
	var apiErr struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.Unmarshal(body, &apiErr)

	message := strings.TrimSpace(apiErr.Error.Message)
	kind := ErrorKindUnknown

	switch {
	case statusCode == http.StatusTooManyRequests:
		kind = ErrorKindRateLimited
	case statusCode >= http.StatusInternalServerError:
		kind = ErrorKindServer
	case statusCode == http.StatusBadRequest:
		kind = ErrorKindBadRequest
	case statusCode == http.StatusUnauthorized:
		kind = ErrorKindUnauthorized
	case statusCode == http.StatusForbidden:
		kind = ErrorKindForbidden
	case statusCode == http.StatusNotFound && strings.Contains(strings.ToLower(message), "model"):
		kind = ErrorKindUnknownModel
	case strings.Contains(strings.ToLower(message), "model"):
		kind = ErrorKindUnknownModel
	}

	if message == "" {
		message = fmt.Sprintf("http status %d", statusCode)
	}

	return &ProviderError{
		Provider:   providerName,
		Kind:       kind,
		StatusCode: statusCode,
		Message:    message,
	}
}

func buildChatCompletionsURL(baseURL string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if strings.HasSuffix(base, "/chat/completions") {
		return base
	}
	return base + "/chat/completions"
}

func providerErrorKind(err error) ErrorKind {
	var providerErr *ProviderError
	if errors.As(err, &providerErr) {
		return providerErr.Kind
	}
	return ErrorKindUnknown
}

type openAIChatCompletionRequest struct {
	Model       string              `json:"model"`
	Messages    []openAIChatMessage `json:"messages"`
	MaxTokens   int                 `json:"max_tokens,omitempty"`
	Temperature float64             `json:"temperature,omitempty"`
}

type openAIChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIChatCompletionResponse struct {
	Choices []struct {
		Message openAIChatMessage `json:"message"`
	} `json:"choices"`
}
