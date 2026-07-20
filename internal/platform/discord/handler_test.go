package discord

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/hhhhhffd/go-gpt-tg-bot/internal/assistant"
	"github.com/hhhhhffd/go-gpt-tg-bot/internal/config"
	"github.com/hhhhhffd/go-gpt-tg-bot/internal/logger"
)

type fakeAssistant struct {
	mu          sync.Mutex
	lastRequest assistant.Request
	calls       int
	answer      string
	delay       time.Duration
}

func (f *fakeAssistant) Answer(ctx context.Context, request assistant.Request) (string, error) {
	f.mu.Lock()
	f.calls++
	f.lastRequest = request
	answer, delay := f.answer, f.delay
	f.mu.Unlock()

	if delay > 0 {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	return answer, nil
}

type fakeInteractionClient struct {
	mu     sync.Mutex
	edited []EditWebhookMessageRequest
}

func (f *fakeInteractionClient) EditOriginalInteractionResponse(_ context.Context, _ string, _ string, payload EditWebhookMessageRequest) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.edited = append(f.edited, payload)
	return nil
}

func (f *fakeInteractionClient) CreateFollowupMessage(context.Context, string, string, FollowupMessageRequest) error {
	return nil
}

func TestPrepareGatewayInteractionAsk(t *testing.T) {
	handler, fakeAI, fakeClient := newTestHandler(t)
	fakeAI.answer = "Answer"

	response, err := handler.PrepareGatewayInteraction(testChatInteraction(t, CommandAsk, "Что такое Git?", false))
	if err != nil {
		t.Fatalf("prepare interaction: %v", err)
	}
	if response.Type != InteractionCallbackTypeDeferredChannelMessageWithSource {
		t.Fatalf("expected deferred response, got %d", response.Type)
	}
	if response.Data == nil || response.Data.Flags != MessageFlagEphemeral {
		t.Fatal("expected an ephemeral response by default")
	}
	handler.Wait()

	if fakeAI.calls != 1 || fakeAI.lastRequest.Question != "Что такое Git?" || fakeAI.lastRequest.Deep {
		t.Fatalf("unexpected assistant request: %+v", fakeAI.lastRequest)
	}
	if len(fakeClient.edited) != 1 {
		t.Fatalf("expected one edited response, got %d", len(fakeClient.edited))
	}
}

func TestPrepareGatewayInteractionDoesNotWaitForAssistant(t *testing.T) {
	handler, fakeAI, _ := newTestHandler(t)
	fakeAI.answer = "Answer"
	fakeAI.delay = 200 * time.Millisecond

	started := time.Now()
	if _, err := handler.PrepareGatewayInteraction(testChatInteraction(t, CommandAsk, "Q", false)); err != nil {
		t.Fatalf("prepare interaction: %v", err)
	}
	if elapsed := time.Since(started); elapsed >= fakeAI.delay {
		t.Fatalf("gateway response waited for assistant: %s", elapsed)
	}
	handler.Wait()
}

func TestPrepareGatewayInteractionDeepAndPublic(t *testing.T) {
	handler, fakeAI, _ := newTestHandler(t)
	fakeAI.answer = "Deep answer"

	response, err := handler.PrepareGatewayInteraction(testChatInteraction(t, CommandDeep, "Разбери глубоко", true))
	if err != nil {
		t.Fatalf("prepare interaction: %v", err)
	}
	if response.Data == nil || response.Data.Flags == MessageFlagEphemeral {
		t.Fatal("expected a public response in a guild")
	}
	handler.Wait()
	if !fakeAI.lastRequest.Deep {
		t.Fatal("expected deep request")
	}
}

func TestPrepareGatewayInteractionExplain(t *testing.T) {
	handler, fakeAI, _ := newTestHandler(t)
	fakeAI.answer = "Explanation"
	interaction := Interaction{
		ApplicationID: "app",
		Token:         "token",
		Context:       InteractionContextGuild,
		Member:        &InteractionMember{User: &User{ID: "42"}},
		Data: &ApplicationCommandData{
			Type:     ApplicationCommandTypeMessage,
			Name:     CommandExplain,
			TargetID: "message",
			Resolved: &ResolvedData{Messages: map[string]ResolvedMessage{"message": {Content: "Текст для объяснения"}}},
		},
	}

	if _, err := handler.PrepareGatewayInteraction(interaction); err != nil {
		t.Fatalf("prepare interaction: %v", err)
	}
	handler.Wait()
	if fakeAI.lastRequest.Question != "Текст для объяснения" {
		t.Fatalf("unexpected question: %q", fakeAI.lastRequest.Question)
	}
}

func TestPrepareGatewayInteractionRejectsUnauthorizedUser(t *testing.T) {
	handler, fakeAI, _ := newTestHandler(t)
	interaction := testChatInteraction(t, CommandAsk, "Q", false)
	interaction.Member.User.ID = "999"

	response, err := handler.PrepareGatewayInteraction(interaction)
	if err != nil {
		t.Fatalf("prepare interaction: %v", err)
	}
	if response.Data == nil || response.Data.Flags != MessageFlagEphemeral {
		t.Fatal("expected private access denial")
	}
	if fakeAI.calls != 0 {
		t.Fatal("assistant must not be called for an unauthorized user")
	}
}

func newTestHandler(t *testing.T) (*Handler, *fakeAssistant, *fakeInteractionClient) {
	t.Helper()
	fakeAI := &fakeAssistant{}
	fakeClient := &fakeInteractionClient{}
	handler, err := NewHandler(
		context.Background(),
		config.DiscordConfig{ApplicationID: "app", AllowedUserIDs: map[string]struct{}{"42": {}}, ProcessingLimit: time.Second},
		fakeAI,
		fakeClient,
		logger.New("error"),
	)
	if err != nil {
		t.Fatalf("new handler: %v", err)
	}
	return handler, fakeAI, fakeClient
}

func testChatInteraction(t *testing.T, command, question string, public bool) Interaction {
	t.Helper()
	return Interaction{
		ApplicationID: "app",
		Token:         "token",
		Context:       InteractionContextGuild,
		Member:        &InteractionMember{User: &User{ID: "42"}},
		Data: &ApplicationCommandData{
			Type: ApplicationCommandTypeChatInput,
			Name: command,
			Options: []ApplicationCommandOption{
				{Name: "question", Value: rawJSON(t, question)},
				{Name: "public", Value: rawJSON(t, public)},
			},
		},
	}
}

func rawJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal test JSON: %v", err)
	}
	return data
}
