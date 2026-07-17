package utils

import "testing"

func TestSplitByLimit(t *testing.T) {
	text := "12345 67890 12345 67890 12345 67890"
	parts := SplitByLimit(text, 10)
	if len(parts) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(parts))
	}

	for i, part := range parts {
		if len([]rune(part)) > 10 {
			t.Fatalf("chunk %d exceeded limit: %q", i, part)
		}
	}
}
