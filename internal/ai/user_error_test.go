package ai

import (
	"strings"
	"testing"
)

func TestUserErrorMessage(t *testing.T) {
	cases := []struct {
		kind ErrorKind
		want string
	}{
		{kind: ErrorKindUnauthorized, want: "401/403"},
		{kind: ErrorKindRateLimited, want: "429"},
		{kind: ErrorKindServer, want: "5xx"},
		{kind: ErrorKindTimeout, want: "не ответил вовремя"},
		{kind: ErrorKindConfig, want: "ещё не настроен"},
	}
	for _, test := range cases {
		t.Run(string(test.kind), func(t *testing.T) {
			got := UserErrorMessage(&ProviderError{Kind: test.kind})
			if !strings.Contains(got, test.want) {
				t.Fatalf("message %q does not contain %q", got, test.want)
			}
		})
	}
}
