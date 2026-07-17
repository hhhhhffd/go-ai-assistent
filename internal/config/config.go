package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v2"
)

const (
	DefaultConfigPath    = "config.yaml"
	DefaultProvidersPath = "providers.yaml"
)

type ProviderConfig struct {
	Name    string `yaml:"name"`
	APIKey  string `yaml:"api_key"`
	BaseURL string `yaml:"base_url"`
	Model   string `yaml:"model"`
}
type ProvidersConfig struct {
	Primary  ProviderConfig `yaml:"primary"`
	Fallback ProviderConfig `yaml:"fallback"`
}
type AIConfig struct {
	PrimaryProvider     ProviderConfig
	FallbackProvider    ProviderConfig
	SystemPrompt        string  `yaml:"system_prompt"`
	DeepSystemPrompt    string  `yaml:"deep_system_prompt"`
	MaxOutputTokens     int     `yaml:"max_output_tokens"`
	DeepMaxOutputTokens int     `yaml:"deep_max_output_tokens"`
	Temperature         float64 `yaml:"temperature"`
	RequestTimeout      time.Duration
}
type TelegramConfig struct {
	Enabled        bool `yaml:"enabled"`
	BotToken       string
	APIID          int
	APIHash        string
	OwnerID        int64
	AllowedUserIDs map[int64]struct{}
}
type DiscordConfig struct {
	Enabled         bool `yaml:"enabled"`
	ApplicationID   string
	PublicKey       string
	BotToken        string
	OwnerID         string
	AllowedUserIDs  map[string]struct{}
	ProcessingLimit time.Duration
}
type RateLimitConfig struct {
	PerUser int `yaml:"per_user"`
	Global  int `yaml:"global"`
	Window  time.Duration
}
type Config struct {
	LogLevel  string          `yaml:"log_level"`
	AI        AIConfig        `yaml:"ai"`
	Telegram  TelegramConfig  `yaml:"telegram"`
	Discord   DiscordConfig   `yaml:"discord"`
	RateLimit RateLimitConfig `yaml:"rate_limit"`
}
type DiscordRegistrationConfig struct {
	ApplicationID  string
	BotToken       string
	RequestTimeout time.Duration
	LogLevel       string
}

type fileConfig struct {
	LogLevel string `yaml:"log_level"`
	AI       struct {
		SystemPrompt        string  `yaml:"system_prompt"`
		DeepSystemPrompt    string  `yaml:"deep_system_prompt"`
		MaxOutputTokens     int     `yaml:"max_output_tokens"`
		DeepMaxOutputTokens int     `yaml:"deep_max_output_tokens"`
		Temperature         float64 `yaml:"temperature"`
		RequestTimeout      string  `yaml:"request_timeout"`
	} `yaml:"ai"`
	Telegram struct {
		Enabled        bool    `yaml:"enabled"`
		AllowedUserIDs []int64 `yaml:"allowed_user_ids"`
	} `yaml:"telegram"`
	Discord struct {
		Enabled        bool     `yaml:"enabled"`
		AllowedUserIDs []string `yaml:"allowed_user_ids"`
	} `yaml:"discord"`
	RateLimit struct {
		PerUser int    `yaml:"per_user"`
		Global  int    `yaml:"global"`
		Window  string `yaml:"window"`
	} `yaml:"rate_limit"`
}

func Load() (Config, error) {
	_ = godotenv.Load()
	if err := EnsureFiles(); err != nil {
		return Config{}, err
	}
	file, err := readConfigFile(DefaultConfigPath)
	if err != nil {
		return Config{}, err
	}
	providers, err := readProvidersFile(DefaultProvidersPath)
	if err != nil {
		return Config{}, err
	}
	return buildConfig(file, providers)
}

// EnsureFiles creates local editable configuration files on first startup.
func EnsureFiles() error {
	if _, err := os.Stat(DefaultConfigPath); errors.Is(err, os.ErrNotExist) {
		data, marshalErr := yaml.Marshal(defaultFileConfig())
		if marshalErr != nil {
			return marshalErr
		}
		if writeErr := writePrivateFile(DefaultConfigPath, data); writeErr != nil {
			return fmt.Errorf("create %s: %w", DefaultConfigPath, writeErr)
		}
	} else if err != nil {
		return fmt.Errorf("stat %s: %w", DefaultConfigPath, err)
	}
	if _, err := os.Stat(DefaultProvidersPath); errors.Is(err, os.ErrNotExist) {
		data, marshalErr := yaml.Marshal(ProvidersConfig{})
		if marshalErr != nil {
			return marshalErr
		}
		if writeErr := writePrivateFile(DefaultProvidersPath, data); writeErr != nil {
			return fmt.Errorf("create %s: %w", DefaultProvidersPath, writeErr)
		}
	} else if err != nil {
		return fmt.Errorf("stat %s: %w", DefaultProvidersPath, err)
	}
	return nil
}

// SaveProviders persists provider credentials atomically. It must only be called
// by authenticated owner-only settings flows.
func SaveProviders(providers ProvidersConfig) error {
	data, err := yaml.Marshal(providers)
	if err != nil {
		return fmt.Errorf("marshal providers: %w", err)
	}
	return writePrivateFile(DefaultProvidersPath, data)
}

// SaveConfig persists editable, non-bootstrap settings. Tokens and owner IDs
// remain in .env and are intentionally never written here.
func SaveConfig(cfg Config) error {
	file := fileConfig{}
	file.LogLevel = cfg.LogLevel
	file.AI.SystemPrompt, file.AI.DeepSystemPrompt = cfg.AI.SystemPrompt, cfg.AI.DeepSystemPrompt
	file.AI.MaxOutputTokens, file.AI.DeepMaxOutputTokens, file.AI.Temperature = cfg.AI.MaxOutputTokens, cfg.AI.DeepMaxOutputTokens, cfg.AI.Temperature
	file.AI.RequestTimeout = cfg.AI.RequestTimeout.String()
	file.Telegram.Enabled, file.Telegram.AllowedUserIDs = cfg.Telegram.Enabled, telegramIDsSlice(cfg.Telegram.AllowedUserIDs, cfg.Telegram.OwnerID)
	file.Discord.Enabled, file.Discord.AllowedUserIDs = cfg.Discord.Enabled, discordIDsSlice(cfg.Discord.AllowedUserIDs, cfg.Discord.OwnerID)
	file.RateLimit.PerUser, file.RateLimit.Global, file.RateLimit.Window = cfg.RateLimit.PerUser, cfg.RateLimit.Global, cfg.RateLimit.Window.String()
	data, err := yaml.Marshal(file)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	return writePrivateFile(DefaultConfigPath, data)
}

func writePrivateFile(path string, data []byte) error {
	temporary, err := os.CreateTemp(".", ".config-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func LoadDiscordRegistrationConfig() (DiscordRegistrationConfig, error) {
	cfg, err := Load()
	if err != nil {
		return DiscordRegistrationConfig{}, err
	}
	if !cfg.Discord.Enabled {
		return DiscordRegistrationConfig{}, errors.New("discord is disabled in config.yaml")
	}
	return DiscordRegistrationConfig{ApplicationID: cfg.Discord.ApplicationID, BotToken: cfg.Discord.BotToken, RequestTimeout: cfg.AI.RequestTimeout, LogLevel: cfg.LogLevel}, nil
}

func readConfigFile(path string) (fileConfig, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return defaultFileConfig(), nil
	}
	if err != nil {
		return fileConfig{}, fmt.Errorf("read %s: %w", path, err)
	}
	var cfg fileConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return fileConfig{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return cfg, nil
}

func readProvidersFile(path string) (ProvidersConfig, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return ProvidersConfig{}, nil
	}
	if err != nil {
		return ProvidersConfig{}, fmt.Errorf("read %s: %w", path, err)
	}
	var cfg ProvidersConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return ProvidersConfig{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return cfg, nil
}

func buildConfig(file fileConfig, providers ProvidersConfig) (Config, error) {
	timeout, err := parseDuration(file.AI.RequestTimeout, 30*time.Second)
	if err != nil {
		return Config{}, fmt.Errorf("ai.request_timeout: %w", err)
	}
	window, err := parseDuration(file.RateLimit.Window, time.Minute)
	if err != nil {
		return Config{}, fmt.Errorf("rate_limit.window: %w", err)
	}
	cfg := Config{LogLevel: defaultString(file.LogLevel, "info"), AI: AIConfig{PrimaryProvider: providers.Primary, FallbackProvider: providers.Fallback, SystemPrompt: defaultString(file.AI.SystemPrompt, "Ты краткий и точный ассистент. Отвечай по делу, структурно и без лишней воды."), DeepSystemPrompt: file.AI.DeepSystemPrompt, MaxOutputTokens: defaultInt(file.AI.MaxOutputTokens, 700), DeepMaxOutputTokens: defaultInt(file.AI.DeepMaxOutputTokens, 1800), Temperature: file.AI.Temperature, RequestTimeout: timeout}, RateLimit: RateLimitConfig{PerUser: defaultInt(file.RateLimit.PerUser, 10), Global: defaultInt(file.RateLimit.Global, 60), Window: window}}
	if cfg.AI.MaxOutputTokens <= 0 || cfg.AI.DeepMaxOutputTokens <= 0 || cfg.RateLimit.PerUser <= 0 || cfg.RateLimit.Global <= 0 || cfg.AI.Temperature < 0 || cfg.AI.Temperature > 2 {
		return Config{}, errors.New("invalid numeric value in config.yaml")
	}
	if file.Telegram.Enabled {
		id, err := envInt("TELEGRAM_API_ID")
		if err != nil {
			return Config{}, err
		}
		hash, err := requireEnv("TELEGRAM_API_HASH")
		if err != nil {
			return Config{}, err
		}
		token, err := requireEnv("TELEGRAM_BOT_TOKEN")
		if err != nil {
			return Config{}, err
		}
		owner, err := envInt64("OWNER_TELEGRAM_ID")
		if err != nil {
			return Config{}, err
		}
		cfg.Telegram = TelegramConfig{Enabled: true, BotToken: token, APIID: id, APIHash: hash, OwnerID: owner, AllowedUserIDs: telegramIDs(file.Telegram.AllowedUserIDs, owner)}
	}
	if file.Discord.Enabled {
		token, err := requireEnv("DISCORD_BOT_TOKEN")
		if err != nil {
			return Config{}, err
		}
		appID, err := requireEnv("DISCORD_CLIENT_ID")
		if err != nil {
			return Config{}, err
		}
		owner, err := requireEnv("OWNER_DISCORD_ID")
		if err != nil {
			return Config{}, err
		}
		cfg.Discord = DiscordConfig{Enabled: true, BotToken: token, ApplicationID: appID, OwnerID: owner, AllowedUserIDs: discordIDs(file.Discord.AllowedUserIDs, owner), ProcessingLimit: maxDuration(timeout*2+5*time.Second, 20*time.Second)}
	}
	if !cfg.Telegram.Enabled && !cfg.Discord.Enabled {
		return Config{}, errors.New("enable telegram or discord in config.yaml")
	}
	return cfg, nil
}

func defaultFileConfig() fileConfig {
	var c fileConfig
	c.Telegram.Enabled = true
	c.AI.RequestTimeout = "30s"
	c.RateLimit.Window = "1m"
	return c
}
func parseDuration(raw string, fallback time.Duration) (time.Duration, error) {
	if strings.TrimSpace(raw) == "" {
		return fallback, nil
	}
	d, e := time.ParseDuration(raw)
	if e != nil || d <= 0 {
		return 0, errors.New("must be positive duration")
	}
	return d, nil
}
func defaultString(v, f string) string {
	if strings.TrimSpace(v) == "" {
		return f
	}
	return strings.TrimSpace(v)
}
func defaultInt(v, f int) int {
	if v == 0 {
		return f
	}
	return v
}
func maxDuration(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}
func requireEnv(key string) (string, error) {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return "", fmt.Errorf("required env %s is not set", key)
	}
	return v, nil
}
func envInt(key string) (int, error) {
	v, e := requireEnv(key)
	if e != nil {
		return 0, e
	}
	n, e := strconv.Atoi(v)
	if e != nil || n <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", key)
	}
	return n, nil
}
func envInt64(key string) (int64, error) {
	v, e := requireEnv(key)
	if e != nil {
		return 0, e
	}
	n, e := strconv.ParseInt(v, 10, 64)
	if e != nil || n <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", key)
	}
	return n, nil
}
func telegramIDs(ids []int64, owner int64) map[int64]struct{} {
	result := map[int64]struct{}{owner: {}}
	for _, id := range ids {
		if id > 0 {
			result[id] = struct{}{}
		}
	}
	return result
}
func discordIDs(ids []string, owner string) map[string]struct{} {
	result := map[string]struct{}{owner: {}}
	for _, id := range ids {
		if strings.TrimSpace(id) != "" {
			result[id] = struct{}{}
		}
	}
	return result
}
func telegramIDsSlice(ids map[int64]struct{}, owner int64) []int64 {
	result := make([]int64, 0, len(ids))
	for id := range ids {
		if id != owner {
			result = append(result, id)
		}
	}
	return result
}
func discordIDsSlice(ids map[string]struct{}, owner string) []string {
	result := make([]string, 0, len(ids))
	for id := range ids {
		if id != owner {
			result = append(result, id)
		}
	}
	return result
}
func parseTelegramAllowlist(raw string) (map[int64]struct{}, error) {
	result := map[int64]struct{}{}
	for _, item := range strings.Split(raw, ",") {
		if strings.TrimSpace(item) == "" {
			continue
		}
		id, err := strconv.ParseInt(strings.TrimSpace(item), 10, 64)
		if err != nil {
			return nil, err
		}
		result[id] = struct{}{}
	}
	if len(result) == 0 {
		return nil, errors.New("allowlist is empty")
	}
	return result, nil
}
func parseDiscordAllowlist(raw string) (map[string]struct{}, error) {
	result := map[string]struct{}{}
	for _, item := range strings.Split(raw, ",") {
		id := strings.TrimSpace(item)
		if id == "" {
			continue
		}
		if _, err := strconv.ParseInt(id, 10, 64); err != nil {
			return nil, err
		}
		result[id] = struct{}{}
	}
	if len(result) == 0 {
		return nil, errors.New("allowlist is empty")
	}
	return result, nil
}
