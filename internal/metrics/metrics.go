// Package metrics exposes process counters through the standard expvar registry.
package metrics

import "expvar"

var (
	AIRequests        = expvar.NewInt("ai_requests_total")
	AIFailures        = expvar.NewInt("ai_failures_total")
	RateLimitRejected = expvar.NewInt("rate_limit_rejected_total")
	TelegramGuests    = expvar.NewInt("telegram_guest_queries_total")
	DiscordRequests   = expvar.NewInt("discord_interactions_total")
)
