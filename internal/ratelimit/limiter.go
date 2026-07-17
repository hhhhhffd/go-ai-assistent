// Package ratelimit provides a concurrency-safe fixed-window request limiter.
package ratelimit

import (
	"sync"
	"time"
)

type Config struct {
	PerUser int
	Global  int
	Window  time.Duration
}

type Limiter struct {
	mu     sync.Mutex
	cfg    Config
	window time.Time
	global int
	users  map[string]int
}

func New(cfg Config) *Limiter {
	return &Limiter{cfg: cfg, window: time.Now(), users: make(map[string]int)}
}

// Allow reserves one request for userID. An empty user ID is never allowed.
func (l *Limiter) Allow(userID string) bool {
	if userID == "" {
		return false
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	if now.Sub(l.window) >= l.cfg.Window {
		l.window = now
		l.global = 0
		l.users = make(map[string]int)
	}
	if l.global >= l.cfg.Global || l.users[userID] >= l.cfg.PerUser {
		return false
	}

	l.global++
	l.users[userID]++
	return true
}
