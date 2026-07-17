package assistant

import (
	"context"
	"errors"
	"fmt"
	"strings"

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

	systemPrompt := s.cfg.SystemPrompt
	maxTokens := s.cfg.MaxOutputTokens

	if request.Deep {
		maxTokens = s.cfg.DeepMaxOutputTokens
		if strings.TrimSpace(s.cfg.DeepSystemPrompt) != "" {
			systemPrompt = s.cfg.DeepSystemPrompt
		} else {
			systemPrompt = fmt.Sprintf(
				"%s\n\nДополнительно: внимательно проанализируй вопрос, покажи ход рассуждения структурно, приведи ограничения и практические рекомендации.",
				s.cfg.SystemPrompt,
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
		Temperature:     s.cfg.Temperature,
	})
}
