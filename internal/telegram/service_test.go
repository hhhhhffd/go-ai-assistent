package telegram

import (
	"testing"

	"github.com/gotd/td/tg"
)

func TestGuestUpdatesExtractsGuestQuery(t *testing.T) {
	guest := &tg.UpdateBotGuestChatQuery{QueryID: 7}
	updates := guestUpdates(&tg.Updates{Updates: []tg.UpdateClass{guest}})
	if len(updates) != 1 || updates[0].QueryID != 7 {
		t.Fatalf("unexpected guest updates: %#v", updates)
	}
}

func TestFirstMessageText(t *testing.T) {
	text := firstMessageText([]tg.MessageClass{
		&tg.Message{Message: "  previous answer  "},
	})
	if text != "previous answer" {
		t.Fatalf("unexpected message text %q", text)
	}
}

func TestTruncateRunes(t *testing.T) {
	text := truncateRunes("абвг", 3)
	if text != "аб…" {
		t.Fatalf("unexpected truncated text %q", text)
	}
}
