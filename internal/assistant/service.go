package assistant

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/hhhhhffd/go-gpt-tg-bot/internal/ai"
)

type AIClient interface {
	Complete(ctx context.Context, request ai.CompletionRequest) (string, error)
}

type Config struct {
	SystemPrompt        string
	DeepSystemPrompt    string
	MaxOutputTokens     int
	DeepMaxOutputTokens int
	Temperature         float64
}

type DefaultService struct {
	aiClient AIClient
	cfg      Config
	mu       sync.RWMutex
}

func NewService(cfg Config, aiClient AIClient) (*DefaultService, error) {
	if aiClient == nil {
		return nil, errors.New("assistant ai client is required")
	}

	if strings.TrimSpace(cfg.SystemPrompt) == "" {
		return nil, errors.New("assistant system prompt is required")
	}

	if cfg.MaxOutputTokens <= 0 {
		return nil, errors.New("assistant max output tokens must be > 0")
	}

	if cfg.DeepMaxOutputTokens <= 0 {
		return nil, errors.New("assistant deep max output tokens must be > 0")
	}

	return &DefaultService{
		aiClient: aiClient,
		cfg:      cfg,
	}, nil
}

func (s *DefaultService) Answer(ctx context.Context, request Request) (string, error) {
	question := strings.TrimSpace(request.Question)
	if question == "" {
		return "", errors.New("question is empty")
	}

	s.mu.RLock()
	cfg := s.cfg
	s.mu.RUnlock()
	systemPrompt := cfg.SystemPrompt
	maxTokens := cfg.MaxOutputTokens

	if request.Deep {
		maxTokens = cfg.DeepMaxOutputTokens
		if strings.TrimSpace(cfg.DeepSystemPrompt) != "" {
			systemPrompt = cfg.DeepSystemPrompt
		} else {
			systemPrompt = fmt.Sprintf(
				"%s\n\nДополнительно: внимательно проанализируй вопрос, покажи ход рассуждения структурно, приведи ограничения и практические рекомендации.",
				cfg.SystemPrompt,
			)
		}
	}

	messages := make([]ai.Message, 0, 2)
	if strings.TrimSpace(request.PreviousAnswer) != "" {
		messages = append(messages, ai.Message{
			Role:    ai.RoleAssistant,
			Content: strings.TrimSpace(request.PreviousAnswer),
		})
	}
	messages = append(messages, ai.Message{
		Role:    ai.RoleUser,
		Content: question,
	})

	return s.aiClient.Complete(ctx, ai.CompletionRequest{
		SystemPrompt:    systemPrompt,
		Messages:        messages,
		MaxOutputTokens: maxTokens,
		Temperature:     cfg.Temperature,
	})
}

func (s *DefaultService) UpdateConfig(cfg Config) error {
	if strings.TrimSpace(cfg.SystemPrompt) == "" || cfg.MaxOutputTokens <= 0 || cfg.DeepMaxOutputTokens <= 0 {
		return errors.New("invalid assistant configuration")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg = cfg
	return nil
}
