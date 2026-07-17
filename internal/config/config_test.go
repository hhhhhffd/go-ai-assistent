package config

import (
	"os"
	"testing"
)

func TestParseTelegramAllowlist(t *testing.T) {
	got, err := parseTelegramAllowlist("123, 456,789")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("expected 3 ids, got %d", len(got))
	}
	if _, ok := got[456]; !ok {
		t.Fatalf("expected id 456 in allowlist")
	}
}

func TestParseDiscordAllowlist(t *testing.T) {
	got, err := parseDiscordAllowlist("111,222,333")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("expected 3 ids, got %d", len(got))
	}
	if _, ok := got["222"]; !ok {
		t.Fatalf("expected id 222 in allowlist")
	}
}

func TestLoad_DisabledTelegramPlatform(t *testing.T) {
	env := baseAIEnv()
	env["TELEGRAM_ENABLED"] = "false"
	env["DISCORD_ENABLED"] = "true"
	env["DISCORD_APPLICATION_ID"] = "123456789012345678"
	env["DISCORD_PUBLIC_KEY"] = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	env["ALLOWED_DISCORD_USER_IDS"] = "123456789012345678"
	env["PUBLIC_BASE_URL"] = "https://example.com"
	withEnv(t, env, func() {
		cfg, err := Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Telegram.Enabled {
			t.Fatalf("telegram should be disabled")
		}
		if !cfg.Discord.Enabled {
			t.Fatalf("discord should be enabled")
		}
	})
}

func TestLoad_DisabledDiscordPlatform(t *testing.T) {
	env := baseAIEnv()
	env["TELEGRAM_ENABLED"] = "true"
	env["DISCORD_ENABLED"] = "false"
	env["TELEGRAM_BOT_TOKEN"] = "telegram-token"
	env["TELEGRAM_API_ID"] = "123"
	env["TELEGRAM_API_HASH"] = "telegram-api-hash"
	env["ALLOWED_TELEGRAM_USER_IDS"] = "111"
	withEnv(t, env, func() {
		cfg, err := Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !cfg.Telegram.Enabled {
			t.Fatalf("telegram should be enabled")
		}
		if cfg.Discord.Enabled {
			t.Fatalf("discord should be disabled")
		}
	})
}

func TestLoad_ErrorsWhenBothPlatformsDisabled(t *testing.T) {
	env := baseAIEnv()
	env["TELEGRAM_ENABLED"] = "false"
	env["DISCORD_ENABLED"] = "false"
	withEnv(t, env, func() {
		_, err := Load()
		if err == nil {
			t.Fatalf("expected error when both platforms disabled")
		}
	})
}

func TestLoad_PrioritizesNewTelegramVariables(t *testing.T) {
	env := baseAIEnv()
	env["TELEGRAM_ENABLED"] = "true"
	env["DISCORD_ENABLED"] = "false"
	env["TELEGRAM_BOT_TOKEN"] = "new-token"
	env["TELEGRAM_API_ID"] = "123"
	env["TELEGRAM_API_HASH"] = "telegram-api-hash"
	env["BOT_TOKEN"] = "old-token"
	env["ALLOWED_TELEGRAM_USER_IDS"] = "111"
	env["ALLOWED_USER_IDS"] = "999"
	withEnv(t, env, func() {
		cfg, err := Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Telegram.BotToken != "new-token" {
			t.Fatalf("expected new TELEGRAM_BOT_TOKEN to have priority")
		}
		if _, ok := cfg.Telegram.AllowedUserIDs[111]; !ok {
			t.Fatalf("expected new allowlist to have priority")
		}
	})
}

func baseAIEnv() map[string]string {
	return map[string]string{
		"PRIMARY_PROVIDER":  "cerebras",
		"PRIMARY_API_KEY":   "pk",
		"PRIMARY_BASE_URL":  "https://primary.example/v1",
		"PRIMARY_MODEL":     "model-primary",
		"FALLBACK_PROVIDER": "groq",
		"FALLBACK_API_KEY":  "fk",
		"FALLBACK_BASE_URL": "https://fallback.example/v1",
		"FALLBACK_MODEL":    "model-fallback",
		"SYSTEM_PROMPT":     "system",
	}
}

func withEnv(t *testing.T, vars map[string]string, fn func()) {
	t.Helper()

	original := map[string]*string{}
	for key, value := range vars {
		v, ok := os.LookupEnv(key)
		if ok {
			valueCopy := v
			original[key] = &valueCopy
		} else {
			original[key] = nil
		}
		if err := os.Setenv(key, value); err != nil {
			t.Fatalf("set env %s: %v", key, err)
		}
	}

	defer func() {
		for key, value := range original {
			if value == nil {
				_ = os.Unsetenv(key)
				continue
			}
			_ = os.Setenv(key, *value)
		}
	}()

	fn()
}
