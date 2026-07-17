package ai

import "testing"

func TestShouldFallbackByStatusPolicy(t *testing.T) {
	cases := []struct {
		name       string
		statusCode int
		body       string
		want       bool
	}{
		{name: "rate limit", statusCode: 429, body: `{"error":{"message":"too many requests"}}`, want: true},
		{name: "server error", statusCode: 500, body: `{"error":{"message":"internal"}}`, want: true},
		{name: "bad request", statusCode: 400, body: `{"error":{"message":"bad request"}}`, want: false},
		{name: "unauthorized", statusCode: 401, body: `{"error":{"message":"unauthorized"}}`, want: false},
		{name: "forbidden", statusCode: 403, body: `{"error":{"message":"forbidden"}}`, want: false},
		{name: "unknown model", statusCode: 404, body: `{"error":{"message":"model not found"}}`, want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := classifyAPIError("provider", tc.statusCode, []byte(tc.body))
			if got := ShouldFallback(err); got != tc.want {
				t.Fatalf("unexpected fallback decision for status %d: want=%v got=%v", tc.statusCode, tc.want, got)
			}
		})
	}
}

func TestShouldFallbackOnInvalidResponse(t *testing.T) {
	err := &ProviderError{
		Provider: "provider",
		Kind:     ErrorKindInvalidResponse,
		Message:  "empty response",
	}
	if !ShouldFallback(err) {
		t.Fatalf("invalid response must trigger fallback")
	}
}
