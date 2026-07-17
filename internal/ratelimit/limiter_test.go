package ratelimit

import (
	"testing"
	"time"
)

func TestLimiterEnforcesPerUserAndGlobalLimits(t *testing.T) {
	limiter := New(Config{PerUser: 1, Global: 2, Window: time.Minute})
	if !limiter.Allow("user-1") {
		t.Fatal("first request must be allowed")
	}
	if limiter.Allow("user-1") {
		t.Fatal("per-user limit was not enforced")
	}
	if !limiter.Allow("user-2") {
		t.Fatal("second user request must be allowed")
	}
	if limiter.Allow("user-3") {
		t.Fatal("global limit was not enforced")
	}
}
