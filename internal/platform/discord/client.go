package discord

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

const (
	defaultDiscordAPIBaseURL = "https://discord.com/api/v10"
)

type InteractionClient interface {
	EditOriginalInteractionResponse(ctx context.Context, applicationID string, interactionToken string, payload EditWebhookMessageRequest) error
	CreateFollowupMessage(ctx context.Context, applicationID string, interactionToken string, payload FollowupMessageRequest) error
}

type APIClient struct {
	baseURL    string
	botToken   string
	httpClient *http.Client
	logger     *slog.Logger
}

func NewAPIClient(botToken string, timeout time.Duration, logger *slog.Logger) *APIClient {
	return &APIClient{
		baseURL:  defaultDiscordAPIBaseURL,
		botToken: strings.TrimSpace(botToken),
		httpClient: &http.Client{
			Timeout: timeout,
		},
		logger: logger,
	}
}

func (c *APIClient) RegisterGlobalCommands(ctx context.Context, applicationID string, commands []ApplicationCommand) error {
	if strings.TrimSpace(applicationID) == "" {
		return errors.New("discord application id is empty")
	}
	if strings.TrimSpace(c.botToken) == "" {
		return errors.New("discord bot token is empty")
	}

	endpoint := fmt.Sprintf("%s/applications/%s/commands", c.baseURL, applicationID)
	return c.doRequest(ctx, http.MethodPut, endpoint, true, commands, nil)
}

func (c *APIClient) EditOriginalInteractionResponse(ctx context.Context, applicationID string, interactionToken string, payload EditWebhookMessageRequest) error {
	endpoint := fmt.Sprintf("%s/webhooks/%s/%s/messages/@original", c.baseURL, applicationID, interactionToken)
	return c.doRequest(ctx, http.MethodPatch, endpoint, false, payload, nil)
}

func (c *APIClient) CreateFollowupMessage(ctx context.Context, applicationID string, interactionToken string, payload FollowupMessageRequest) error {
	endpoint := fmt.Sprintf("%s/webhooks/%s/%s", c.baseURL, applicationID, interactionToken)
	return c.doRequest(ctx, http.MethodPost, endpoint, false, payload, nil)
}

func (c *APIClient) doRequest(
	ctx context.Context,
	method string,
	url string,
	withBotAuth bool,
	payload any,
	out any,
) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal discord payload: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create discord request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")

	if withBotAuth {
		request.Header.Set("Authorization", "Bot "+c.botToken)
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("discord request failed: %w", err)
	}
	defer response.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read discord response: %w", err)
	}

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("discord api returned status %d", response.StatusCode)
	}

	if out != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("decode discord response: %w", err)
		}
	}

	return nil
}
