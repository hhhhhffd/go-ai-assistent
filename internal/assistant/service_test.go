package assistant

import (
	"context"
	"testing"

	"github.com/hhhhhffd/go-gpt-tg-bot/internal/ai"
)

type fakeAIClient struct {
	lastRequest ai.CompletionRequest
	answer      string
	err         error
}

func (f *fakeAIClient) Complete(_ context.Context, request ai.CompletionRequest) (string, error) {
	f.lastRequest = request
	return f.answer, f.err
}

func TestServiceBuildsStructuredMessages(t *testing.T) {
	fake := &fakeAIClient{answer: "ok"}
	service, err := NewService(Config{
		SystemPrompt:        "system",
		MaxOutputTokens:     700,
		DeepMaxOutputTokens: 1800,
		Temperature:         0.2,
	}, fake)
	if err != nil {
		t.Fatalf("new service failed: %v", err)
	}

	_, err = service.Answer(context.Background(), Request{
		Question:       "Что такое Git?",
		PreviousAnswer: "Git — это система контроля версий.",
	})
	if err != nil {
		t.Fatalf("answer failed: %v", err)
	}

	if fake.lastRequest.SystemPrompt != "system" {
		t.Fatalf("unexpected system prompt: %q", fake.lastRequest.SystemPrompt)
	}
	if len(fake.lastRequest.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(fake.lastRequest.Messages))
	}
	if fake.lastRequest.Messages[0].Role != ai.RoleAssistant {
		t.Fatalf("expected first message role assistant, got %q", fake.lastRequest.Messages[0].Role)
	}
	if fake.lastRequest.Messages[1].Role != ai.RoleUser {
		t.Fatalf("expected second message role user, got %q", fake.lastRequest.Messages[1].Role)
	}
}

func TestServiceUsesDeepConfiguration(t *testing.T) {
	fake := &fakeAIClient{answer: "ok"}
	service, err := NewService(Config{
		SystemPrompt:        "system",
		DeepSystemPrompt:    "deep",
		MaxOutputTokens:     700,
		DeepMaxOutputTokens: 1800,
		Temperature:         0.2,
	}, fake)
	if err != nil {
		t.Fatalf("new service failed: %v", err)
	}

	_, err = service.Answer(context.Background(), Request{
		Question: "Разбери проблему",
		Deep:     true,
	})
	if err != nil {
		t.Fatalf("answer failed: %v", err)
	}

	if fake.lastRequest.SystemPrompt != "deep" {
		t.Fatalf("expected deep prompt, got %q", fake.lastRequest.SystemPrompt)
	}
	if fake.lastRequest.MaxOutputTokens != 1800 {
		t.Fatalf("expected deep max tokens, got %d", fake.lastRequest.MaxOutputTokens)
	}
}
