package telegram

import (
	"strings"
	"testing"
)

func TestSplitTelegramMarkdownV2(t *testing.T) {
	input := strings.Repeat("`code` line\n", 900)
	parts := splitTelegramMarkdownV2(input)
	if len(parts) < 2 {
		t.Fatalf("expected multiple parts, got %d", len(parts))
	}

	for i, part := range parts {
		if len([]rune(part)) > telegramMessageLimit {
			t.Fatalf("part %d exceeds telegram limit", i)
		}
	}
}
