package config

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type ProviderConfig struct {
	Name    string
	APIKey  string
	BaseURL string
	Model   string
}

type AIConfig struct {
	PrimaryProvider  ProviderConfig
	FallbackProvider ProviderConfig

	SystemPrompt        string
	DeepSystemPrompt    string
	MaxOutputTokens     int
	DeepMaxOutputTokens int
	Temperature         float64
	RequestTimeout      time.Duration
}

type TelegramConfig struct {
	Enabled        bool
	BotToken       string
	APIID          int
	APIHash        string
	AllowedUserIDs map[int64]struct{}
}

type DiscordConfig struct {
	Enabled         bool
	ApplicationID   string
	PublicKey       string
	BotToken        string
	AllowedUserIDs  map[string]struct{}
	ProcessingLimit time.Duration
}

type HTTPConfig struct {
	Addr          string
	PublicBaseURL string
}

type RateLimitConfig struct {
	PerUser int
	Global  int
	Window  time.Duration
}

type Config struct {
	LogLevel  string
	AI        AIConfig
	Telegram  TelegramConfig
	Discord   DiscordConfig
	HTTP      HTTPConfig
	RateLimit RateLimitConfig
}

type DiscordRegistrationConfig struct {
	ApplicationID  string
	BotToken       string
	RequestTimeout time.Duration
	LogLevel       string
}

func Load() (Config, error) {
	_ = godotenv.Load()

	cfg := Config{
		LogLevel: strings.ToLower(envOrDefault("LOG_LEVEL", "info")),
	}

	telegramEnabled, err := resolveTelegramEnabled()
	if err != nil {
		return Config{}, err
	}
	discordEnabled, err := resolveDiscordEnabled()
	if err != nil {
		return Config{}, err
	}

	if !telegramEnabled && !discordEnabled {
		return Config{}, errors.New("at least one platform must be enabled: TELEGRAM_ENABLED or DISCORD_ENABLED")
	}

	cfg.Telegram.Enabled = telegramEnabled
	cfg.Discord.Enabled = discordEnabled

	cfg.HTTP = HTTPConfig{
		Addr:          envOrDefault("HTTP_ADDR", ":8080"),
		PublicBaseURL: strings.TrimSpace(os.Getenv("PUBLIC_BASE_URL")),
	}
	cfg.RateLimit, err = loadRateLimitConfig()
	if err != nil {
		return Config{}, err
	}

	cfg.AI, err = loadAIConfig()
	if err != nil {
		return Config{}, err
	}

	if cfg.Telegram.Enabled {
		cfg.Telegram, err = loadTelegramConfig()
		if err != nil {
			return Config{}, err
		}
	}

	if cfg.Discord.Enabled {
		cfg.Discord, err = loadDiscordConfig(cfg.AI.RequestTimeout)
		if err != nil {
			return Config{}, err
		}

		if strings.TrimSpace(cfg.HTTP.PublicBaseURL) == "" {
			return Config{}, errors.New("PUBLIC_BASE_URL is required when DISCORD_ENABLED=true")
		}
	}

	return cfg, nil
}

func loadRateLimitConfig() (RateLimitConfig, error) {
	perUser, err := envInt("RATE_LIMIT_PER_USER", 10)
	if err != nil || perUser <= 0 {
		return RateLimitConfig{}, errors.New("RATE_LIMIT_PER_USER must be > 0")
	}
	global, err := envInt("RATE_LIMIT_GLOBAL", 60)
	if err != nil || global <= 0 {
		return RateLimitConfig{}, errors.New("RATE_LIMIT_GLOBAL must be > 0")
	}
	window, err := envDuration("RATE_LIMIT_WINDOW", time.Minute)
	if err != nil {
		return RateLimitConfig{}, err
	}
	return RateLimitConfig{PerUser: perUser, Global: global, Window: window}, nil
}

func LoadDiscordRegistrationConfig() (DiscordRegistrationConfig, error) {
	_ = godotenv.Load()

	applicationID, err := requireEnv("DISCORD_APPLICATION_ID")
	if err != nil {
		return DiscordRegistrationConfig{}, err
	}

	botToken, err := requireEnv("DISCORD_BOT_TOKEN")
	if err != nil {
		return DiscordRegistrationConfig{}, err
	}

	requestTimeout, err := envDuration("REQUEST_TIMEOUT", 30*time.Second)
	if err != nil {
		return DiscordRegistrationConfig{}, err
	}

	return DiscordRegistrationConfig{
		ApplicationID:  applicationID,
		BotToken:       botToken,
		RequestTimeout: requestTimeout,
		LogLevel:       strings.ToLower(envOrDefault("LOG_LEVEL", "info")),
	}, nil
}

func loadAIConfig() (AIConfig, error) {
	primaryProvider, err := loadProvider("PRIMARY")
	if err != nil {
		return AIConfig{}, err
	}

	fallbackProvider, err := loadProvider("FALLBACK")
	if err != nil {
		return AIConfig{}, err
	}

	maxOutputTokens, err := envInt("MAX_OUTPUT_TOKENS", 700)
	if err != nil {
		return AIConfig{}, err
	}
	if maxOutputTokens <= 0 {
		return AIConfig{}, errors.New("MAX_OUTPUT_TOKENS must be > 0")
	}

	deepMaxOutputTokens, err := envInt("DEEP_MAX_OUTPUT_TOKENS", 1800)
	if err != nil {
		return AIConfig{}, err
	}
	if deepMaxOutputTokens <= 0 {
		return AIConfig{}, errors.New("DEEP_MAX_OUTPUT_TOKENS must be > 0")
	}

	temperature, err := envFloat("TEMPERATURE", 0.2)
	if err != nil {
		return AIConfig{}, err
	}
	if temperature < 0 || temperature > 2 {
		return AIConfig{}, errors.New("TEMPERATURE must be between 0 and 2")
	}

	requestTimeout, err := envDuration("REQUEST_TIMEOUT", 30*time.Second)
	if err != nil {
		return AIConfig{}, err
	}
	if requestTimeout <= 0 {
		return AIConfig{}, errors.New("REQUEST_TIMEOUT must be > 0")
	}

	systemPrompt := envOrDefault(
		"SYSTEM_PROMPT",
		"Ты краткий и точный ассистент. Отвечай по делу, структурно и без лишней воды.",
	)
	if strings.TrimSpace(systemPrompt) == "" {
		return AIConfig{}, errors.New("SYSTEM_PROMPT must not be empty")
	}

	return AIConfig{
		PrimaryProvider:     primaryProvider,
		FallbackProvider:    fallbackProvider,
		SystemPrompt:        systemPrompt,
		DeepSystemPrompt:    strings.TrimSpace(os.Getenv("DEEP_SYSTEM_PROMPT")),
		MaxOutputTokens:     maxOutputTokens,
		DeepMaxOutputTokens: deepMaxOutputTokens,
		Temperature:         temperature,
		RequestTimeout:      requestTimeout,
	}, nil
}

func loadTelegramConfig() (TelegramConfig, error) {
	apiID, err := envInt("TELEGRAM_API_ID", 0)
	if err != nil || apiID <= 0 {
		return TelegramConfig{}, errors.New("TELEGRAM_API_ID must be a positive integer when TELEGRAM_ENABLED=true")
	}
	apiHash, err := requireEnv("TELEGRAM_API_HASH")
	if err != nil {
		return TelegramConfig{}, err
	}

	botToken := firstNonEmpty("TELEGRAM_BOT_TOKEN", "BOT_TOKEN")
	if strings.TrimSpace(botToken) == "" {
		return TelegramConfig{}, errors.New("TELEGRAM_BOT_TOKEN is required when TELEGRAM_ENABLED=true (BOT_TOKEN is accepted for backward compatibility)")
	}

	allowlistRaw := firstNonEmpty("ALLOWED_TELEGRAM_USER_IDS", "ALLOWED_USER_IDS")
	if strings.TrimSpace(allowlistRaw) == "" {
		return TelegramConfig{}, errors.New("ALLOWED_TELEGRAM_USER_IDS is required when TELEGRAM_ENABLED=true (ALLOWED_USER_IDS is accepted for backward compatibility)")
	}

	allowedIDs, err := parseTelegramAllowlist(allowlistRaw)
	if err != nil {
		return TelegramConfig{}, fmt.Errorf("parse telegram allowlist: %w", err)
	}

	return TelegramConfig{
		Enabled:        true,
		BotToken:       strings.TrimSpace(botToken),
		APIID:          apiID,
		APIHash:        strings.TrimSpace(apiHash),
		AllowedUserIDs: allowedIDs,
	}, nil
}

func loadDiscordConfig(defaultTimeout time.Duration) (DiscordConfig, error) {
	applicationID, err := requireEnv("DISCORD_APPLICATION_ID")
	if err != nil {
		return DiscordConfig{}, err
	}

	publicKey, err := requireEnv("DISCORD_PUBLIC_KEY")
	if err != nil {
		return DiscordConfig{}, err
	}

	allowlistRaw, err := requireEnv("ALLOWED_DISCORD_USER_IDS")
	if err != nil {
		return DiscordConfig{}, err
	}

	allowedIDs, err := parseDiscordAllowlist(allowlistRaw)
	if err != nil {
		return DiscordConfig{}, fmt.Errorf("parse discord allowlist: %w", err)
	}

	processingLimit := defaultTimeout*2 + 5*time.Second
	if processingLimit < 20*time.Second {
		processingLimit = 20 * time.Second
	}

	return DiscordConfig{
		Enabled:         true,
		ApplicationID:   strings.TrimSpace(applicationID),
		PublicKey:       strings.TrimSpace(publicKey),
		BotToken:        strings.TrimSpace(os.Getenv("DISCORD_BOT_TOKEN")),
		AllowedUserIDs:  allowedIDs,
		ProcessingLimit: processingLimit,
	}, nil
}

func loadProvider(prefix string) (ProviderConfig, error) {
	name := envOrDefault(prefix+"_PROVIDER", strings.ToLower(prefix))

	apiKey, err := requireEnv(prefix + "_API_KEY")
	if err != nil {
		return ProviderConfig{}, err
	}

	baseURL, err := requireEnv(prefix + "_BASE_URL")
	if err != nil {
		return ProviderConfig{}, err
	}

	model, err := requireEnv(prefix + "_MODEL")
	if err != nil {
		return ProviderConfig{}, err
	}

	return ProviderConfig{
		Name:    strings.TrimSpace(name),
		APIKey:  strings.TrimSpace(apiKey),
		BaseURL: strings.TrimSpace(baseURL),
		Model:   strings.TrimSpace(model),
	}, nil
}

func parseTelegramAllowlist(raw string) (map[int64]struct{}, error) {
	items := splitCSV(raw)
	if len(items) == 0 {
		return nil, errors.New("allowlist is empty")
	}

	result := make(map[int64]struct{}, len(items))
	for _, item := range items {
		id, err := strconv.ParseInt(item, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid telegram user id %q", item)
		}
		result[id] = struct{}{}
	}
	return result, nil
}

func parseDiscordAllowlist(raw string) (map[string]struct{}, error) {
	items := splitCSV(raw)
	if len(items) == 0 {
		return nil, errors.New("allowlist is empty")
	}

	re := regexp.MustCompile(`^\d+$`)
	result := make(map[string]struct{}, len(items))
	for _, item := range items {
		if !re.MatchString(item) {
			return nil, fmt.Errorf("invalid discord user id %q", item)
		}
		result[item] = struct{}{}
	}
	return result, nil
}

func resolveTelegramEnabled() (bool, error) {
	explicit, hasValue, err := envBoolOptional("TELEGRAM_ENABLED")
	if err != nil {
		return false, err
	}
	if hasValue {
		return explicit, nil
	}
	return strings.TrimSpace(firstNonEmpty("TELEGRAM_BOT_TOKEN", "BOT_TOKEN")) != "", nil
}

func resolveDiscordEnabled() (bool, error) {
	explicit, hasValue, err := envBoolOptional("DISCORD_ENABLED")
	if err != nil {
		return false, err
	}
	if hasValue {
		return explicit, nil
	}
	return strings.TrimSpace(os.Getenv("DISCORD_APPLICATION_ID")) != "", nil
}

func envBoolOptional(key string) (value bool, hasValue bool, err error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return false, false, nil
	}
	parsed, parseErr := strconv.ParseBool(raw)
	if parseErr != nil {
		return false, true, fmt.Errorf("%s must be boolean", key)
	}
	return parsed, true, nil
}

func requireEnv(key string) (string, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return "", fmt.Errorf("required env %s is not set", key)
	}
	return value, nil
}

func envOrDefault(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func envInt(key string, fallback int) (int, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be integer: %w", key, err)
	}
	return parsed, nil
}

func envFloat(key string, fallback float64) (float64, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be float: %w", key, err)
	}
	return parsed, nil
}

func envDuration(key string, fallback time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}

	if seconds, err := strconv.Atoi(value); err == nil {
		if seconds <= 0 {
			return 0, fmt.Errorf("%s must be > 0", key)
		}
		return time.Duration(seconds) * time.Second, nil
	}

	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be duration (e.g. 30s) or integer seconds", key)
	}
	if duration <= 0 {
		return 0, fmt.Errorf("%s must be > 0", key)
	}
	return duration, nil
}

func firstNonEmpty(keys ...string) string {
	for _, key := range keys {
		value := strings.TrimSpace(os.Getenv(key))
		if value != "" {
			return value
		}
	}
	return ""
}

func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item == "" {
			continue
		}
		result = append(result, item)
	}
	return result
}
