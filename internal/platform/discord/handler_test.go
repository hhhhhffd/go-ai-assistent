package discord

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	err         error
	delay       time.Duration
}

func (f *fakeAssistant) Answer(ctx context.Context, request assistant.Request) (string, error) {
	f.mu.Lock()
	f.calls++
	f.lastRequest = request
	answer, err, delay := f.answer, f.err, f.delay
	f.mu.Unlock()

	if delay > 0 {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	return answer, err
}

type fakeInteractionClient struct {
	mu        sync.Mutex
	edited    []EditWebhookMessageRequest
	followups []FollowupMessageRequest
}

func (f *fakeInteractionClient) EditOriginalInteractionResponse(_ context.Context, _ string, _ string, payload EditWebhookMessageRequest) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.edited = append(f.edited, payload)
	return nil
}

func (f *fakeInteractionClient) CreateFollowupMessage(_ context.Context, _ string, _ string, payload FollowupMessageRequest) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.followups = append(f.followups, payload)
	return nil
}

func TestDiscordPing(t *testing.T) {
	handler, privateKey := newTestHandler(t)
	body := []byte(`{"type":1}`)

	response := performSignedRequest(t, handler, privateKey, body)
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}

	var payload InteractionResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if payload.Type != InteractionCallbackTypePong {
		t.Fatalf("expected pong response")
	}
}

func TestDiscordAskCommand(t *testing.T) {
	handler, privateKey, fakeAI, fakeClient := newTestHandlerWithFakes(t)
	fakeAI.answer = "Answer"

	interaction := map[string]any{
		"type":           InteractionTypeApplicationCommand,
		"application_id": "app",
		"token":          "token",
		"context":        InteractionContextGuild,
		"member": map[string]any{
			"user": map[string]any{
				"id": "42",
			},
		},
		"data": map[string]any{
			"type": ApplicationCommandTypeChatInput,
			"name": CommandAsk,
			"options": []map[string]any{
				{"name": "question", "type": ApplicationCommandOptionTypeString, "value": "Что такое Git?"},
			},
		},
	}
	body, _ := json.Marshal(interaction)

	response := performSignedRequest(t, handler, privateKey, body)
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}

	var deferred InteractionResponse
	if err := json.Unmarshal(response.Body.Bytes(), &deferred); err != nil {
		t.Fatalf("decode deferred: %v", err)
	}
	if deferred.Type != InteractionCallbackTypeDeferredChannelMessageWithSource {
		t.Fatalf("expected deferred response")
	}
	if deferred.Data == nil || deferred.Data.Flags != MessageFlagEphemeral {
		t.Fatalf("expected default ephemeral response")
	}
	handler.Wait()

	if fakeAI.calls != 1 {
		t.Fatalf("expected assistant call")
	}
	if fakeAI.lastRequest.Question != "Что такое Git?" || fakeAI.lastRequest.Deep {
		t.Fatalf("unexpected assistant request: %+v", fakeAI.lastRequest)
	}
	if len(fakeClient.edited) != 1 {
		t.Fatalf("expected one edit call")
	}
}

func TestDiscordDeferredResponseDoesNotWaitForAssistant(t *testing.T) {
	handler, privateKey, fakeAI, _ := newTestHandlerWithFakes(t)
	fakeAI.answer = "Answer"
	fakeAI.delay = 200 * time.Millisecond

	body := []byte(`{"type":2,"application_id":"app","token":"token","context":0,"member":{"user":{"id":"42"}},"data":{"type":1,"name":"ask","options":[{"name":"question","type":3,"value":"Q"}]}}`)
	started := time.Now()
	response := performSignedRequest(t, handler, privateKey, body)
	if elapsed := time.Since(started); elapsed >= fakeAI.delay {
		t.Fatalf("interaction response waited for assistant: %s", elapsed)
	}
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}
	handler.Wait()
}

func TestDiscordDeepCommand(t *testing.T) {
	handler, privateKey, fakeAI, _ := newTestHandlerWithFakes(t)
	fakeAI.answer = "Deep answer"

	interaction := map[string]any{
		"type":           InteractionTypeApplicationCommand,
		"application_id": "app",
		"token":          "token",
		"context":        InteractionContextGuild,
		"member": map[string]any{
			"user": map[string]any{
				"id": "42",
			},
		},
		"data": map[string]any{
			"type": ApplicationCommandTypeChatInput,
			"name": CommandDeep,
			"options": []map[string]any{
				{"name": "question", "type": ApplicationCommandOptionTypeString, "value": "Разбери глубоко"},
			},
		},
	}
	body, _ := json.Marshal(interaction)

	response := performSignedRequest(t, handler, privateKey, body)
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}
	handler.Wait()
	if !fakeAI.lastRequest.Deep {
		t.Fatalf("expected deep request")
	}
}

func TestDiscordContextMenuExplain(t *testing.T) {
	handler, privateKey, fakeAI, _ := newTestHandlerWithFakes(t)
	fakeAI.answer = "Explain answer"

	interaction := map[string]any{
		"type":           InteractionTypeApplicationCommand,
		"application_id": "app",
		"token":          "token",
		"context":        InteractionContextGuild,
		"member": map[string]any{
			"user": map[string]any{
				"id": "42",
			},
		},
		"data": map[string]any{
			"type":      ApplicationCommandTypeMessage,
			"name":      CommandExplain,
			"target_id": "m1",
			"resolved": map[string]any{
				"messages": map[string]any{
					"m1": map[string]any{
						"content": "Объясни этот текст",
					},
				},
			},
		},
	}
	body, _ := json.Marshal(interaction)

	response := performSignedRequest(t, handler, privateKey, body)
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}
	handler.Wait()
	if fakeAI.lastRequest.Question != "Объясни этот текст" {
		t.Fatalf("unexpected context menu question: %q", fakeAI.lastRequest.Question)
	}
}

func TestDiscordRejectsUnauthorizedUser(t *testing.T) {
	handler, privateKey, fakeAI, _ := newTestHandlerWithFakes(t)
	fakeAI.answer = "ignored"

	interaction := map[string]any{
		"type":           InteractionTypeApplicationCommand,
		"application_id": "app",
		"token":          "token",
		"context":        InteractionContextGuild,
		"member": map[string]any{
			"user": map[string]any{
				"id": "999",
			},
		},
		"data": map[string]any{
			"type": ApplicationCommandTypeChatInput,
			"name": CommandAsk,
			"options": []map[string]any{
				{"name": "question", "type": ApplicationCommandOptionTypeString, "value": "Q"},
			},
		},
	}
	body, _ := json.Marshal(interaction)

	response := performSignedRequest(t, handler, privateKey, body)
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}
	handler.Wait()
	if fakeAI.calls != 0 {
		t.Fatalf("assistant should not be called for unauthorized user")
	}

	var payload InteractionResponse
	_ = json.Unmarshal(response.Body.Bytes(), &payload)
	if payload.Data == nil || payload.Data.Flags != MessageFlagEphemeral {
		t.Fatalf("expected ephemeral deny response")
	}
}

func TestDiscordPublicFlagBehavior(t *testing.T) {
	handler, privateKey, fakeAI, _ := newTestHandlerWithFakes(t)
	fakeAI.answer = "public"

	interaction := map[string]any{
		"type":           InteractionTypeApplicationCommand,
		"application_id": "app",
		"token":          "token",
		"context":        InteractionContextGuild,
		"member": map[string]any{
			"user": map[string]any{
				"id": "42",
			},
		},
		"data": map[string]any{
			"type": ApplicationCommandTypeChatInput,
			"name": CommandAsk,
			"options": []map[string]any{
				{"name": "question", "type": ApplicationCommandOptionTypeString, "value": "Q"},
				{"name": "public", "type": ApplicationCommandOptionTypeBoolean, "value": true},
			},
		},
	}
	body, _ := json.Marshal(interaction)

	response := performSignedRequest(t, handler, privateKey, body)
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}

	var deferred InteractionResponse
	_ = json.Unmarshal(response.Body.Bytes(), &deferred)
	if deferred.Data != nil && deferred.Data.Flags == MessageFlagEphemeral {
		t.Fatalf("public request in guild should not be ephemeral")
	}
	handler.Wait()
}

func newTestHandler(t *testing.T) (*Handler, ed25519.PrivateKey) {
	handler, privateKey, _, _ := newTestHandlerWithFakes(t)
	return handler, privateKey
}

func newTestHandlerWithFakes(t *testing.T) (*Handler, ed25519.PrivateKey, *fakeAssistant, *fakeInteractionClient) {
	t.Helper()

	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate keys: %v", err)
	}

	fakeAI := &fakeAssistant{}
	fakeClient := &fakeInteractionClient{}
	logg := logger.New("error")

	handler, err := NewHandler(
		context.Background(),
		config.DiscordConfig{
			Enabled:         true,
			ApplicationID:   "app",
			PublicKey:       hex.EncodeToString(publicKey),
			AllowedUserIDs:  map[string]struct{}{"42": {}},
			ProcessingLimit: 5 * time.Second,
		},
		fakeAI,
		fakeClient,
		logg,
	)
	if err != nil {
		t.Fatalf("new handler: %v", err)
	}

	return handler, privateKey, fakeAI, fakeClient
}

func performSignedRequest(t *testing.T, handler *Handler, privateKey ed25519.PrivateKey, body []byte) *httptest.ResponseRecorder {
	t.Helper()

	timestamp := "1720000000"
	signature := ed25519.Sign(privateKey, append([]byte(timestamp), body...))

	request := httptest.NewRequest(http.MethodPost, "/discord/interactions", bytes.NewReader(body))
	request.Header.Set("X-Signature-Timestamp", timestamp)
	request.Header.Set("X-Signature-Ed25519", hex.EncodeToString(signature))
	request.Header.Set("Content-Type", "application/json")

	response := httptest.NewRecorder()
	handler.handleInteractions(response, request)
	return response
}
