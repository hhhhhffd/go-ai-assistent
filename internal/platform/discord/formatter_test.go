package discord

import (
	"strings"
	"testing"
)

func TestSplitDiscordText(t *testing.T) {
	input := strings.Repeat("a", 4500)
	parts := splitDiscordText(input)
	if len(parts) < 3 {
		t.Fatalf("expected multiple parts, got %d", len(parts))
	}

	for i, part := range parts {
		if len([]rune(part)) > DiscordMessageLimit {
			t.Fatalf("part %d exceeds discord limit", i)
		}
	}
}

func TestSanitizeDiscordMentions(t *testing.T) {
	text := "ping @everyone and @here and role <@&123>"
	safe := sanitizeDiscordText(text)

	if strings.Contains(safe, "@everyone") {
		t.Fatalf("must sanitize @everyone")
	}
	if strings.Contains(safe, "@here") {
		t.Fatalf("must sanitize @here")
	}
	if strings.Contains(safe, "<@&123>") {
		t.Fatalf("must sanitize role mention")
	}
}
