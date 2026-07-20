package ai

import (
	"testing"
	"time"

	"github.com/hhhhhffd/go-gpt-tg-bot/internal/config"
	"github.com/hhhhhffd/go-gpt-tg-bot/internal/logger"
)

func TestServiceUpdateTimeout(t *testing.T) {
	service := NewService(config.Config{AI: config.AIConfig{RequestTimeout: time.Second}}, logger.New("error"))
	want := 5 * time.Second

	service.UpdateTimeout(want)

	service.httpClientMu.RLock()
	got := service.httpClient.Timeout
	service.httpClientMu.RUnlock()
	if got != want {
		t.Fatalf("timeout = %s, want %s", got, want)
	}
}
