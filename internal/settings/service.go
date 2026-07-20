package settings

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hhhhhffd/go-gpt-tg-bot/internal/ai"
	"github.com/hhhhhffd/go-gpt-tg-bot/internal/assistant"
	"github.com/hhhhhffd/go-gpt-tg-bot/internal/config"
)

// Service owns mutable provider settings. Secrets are exposed only through the owner-only private Telegram panel.
type Service struct {
	mu        sync.Mutex
	providers config.ProvidersConfig
	ai        *ai.Service
	cfg       config.Config
	assistant *assistant.DefaultService
}

func New(
	providers config.ProvidersConfig,
	cfg config.Config,
	aiService *ai.Service,
	assistantService *assistant.DefaultService,
) *Service {
	return &Service{
		providers: providers,
		cfg:       cfg,
		ai:        aiService,
		assistant: assistantService,
	}
}

func (s *Service) Execute(input string) string {
	fields := strings.Fields(strings.TrimSpace(input))
	if len(fields) == 0 || fields[0] != "/settings" {
		return s.help()
	}
	if len(fields) == 1 || fields[1] == "providers" {
		return s.providersView()
	}
	if fields[1] == "config" {
		return s.configView()
	}
	if fields[1] == "allow" || fields[1] == "deny" {
		return s.changeAllowlist(fields)
	}
	if len(fields) >= 5 && fields[1] == "set" && fields[2] == "config" {
		return s.changeConfig(fields)
	}
	if len(fields) < 5 || fields[1] != "set" {
		return s.help()
	}
	slot, field := fields[2], fields[3]
	value := strings.TrimSpace(strings.Join(fields[4:], " "))
	if value == "" {
		return "Значение не может быть пустым."
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var provider *config.ProviderConfig
	switch slot {
	case "primary":
		provider = &s.providers.Primary
	case "fallback":
		provider = &s.providers.Fallback
	default:
		return "Провайдер: primary или fallback."
	}
	switch field {
	case "name":
		provider.Name = value
	case "api_key":
		provider.APIKey = value
	case "base_url":
		provider.BaseURL = value
	case "model":
		provider.Model = value
	default:
		return "Поле: name, api_key, base_url или model."
	}
	if err := config.SaveProviders(s.providers); err != nil {
		return fmt.Sprintf("Не удалось сохранить providers.yaml: %v", err)
	}
	s.ai.UpdateProviders(s.providers)
	return "Сохранено и применено. " + s.providersViewLocked()
}

func (s *Service) changeConfig(fields []string) string {
	section, field := fields[3], fields[4]
	value := strings.TrimSpace(strings.Join(fields[5:], " "))
	if value == "" {
		return "Значение не может быть пустым."
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	switch section {
	case "ai":
		if err := s.setAIConfig(field, value); err != nil {
			return err.Error()
		}
	case "rate_limit":
		if err := s.setRateLimit(field, value); err != nil {
			return err.Error()
		}
	case "telegram", "discord":
		if field != "enabled" {
			return "Для платформы доступно только поле enabled."
		}
		enabled, err := strconv.ParseBool(value)
		if err != nil {
			return "enabled должно быть true или false."
		}
		if section == "telegram" {
			s.cfg.Telegram.Enabled = enabled
		} else {
			s.cfg.Discord.Enabled = enabled
		}
	case "app":
		if field != "log_level" {
			return "Для app доступно только поле log_level."
		}
		s.cfg.LogLevel = value
	default:
		return "Раздел: ai, rate_limit, telegram, discord или app."
	}

	if err := config.SaveConfig(s.cfg); err != nil {
		return fmt.Sprintf("Не удалось сохранить config.yaml: %v", err)
	}
	return "Сохранено. AI-настройки применены сразу; настройки платформ и лимитов применятся после перезапуска."
}

func (s *Service) setAIConfig(field, value string) error {
	switch field {
	case "system_prompt":
		s.cfg.AI.SystemPrompt = value
	case "deep_system_prompt":
		s.cfg.AI.DeepSystemPrompt = value
	case "max_output_tokens", "deep_max_output_tokens":
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed <= 0 {
			return fmt.Errorf("%s должно быть положительным целым числом", field)
		}
		if field == "max_output_tokens" {
			s.cfg.AI.MaxOutputTokens = parsed
		} else {
			s.cfg.AI.DeepMaxOutputTokens = parsed
		}
	case "temperature":
		parsed, err := strconv.ParseFloat(value, 64)
		if err != nil || parsed < 0 || parsed > 2 {
			return fmt.Errorf("temperature должно быть числом от 0 до 2")
		}
		s.cfg.AI.Temperature = parsed
	case "request_timeout":
		parsed, err := time.ParseDuration(value)
		if err != nil || parsed <= 0 {
			return fmt.Errorf("request_timeout должно быть положительной duration, например 30s")
		}
		s.cfg.AI.RequestTimeout = parsed
		s.ai.UpdateTimeout(parsed)
	default:
		return fmt.Errorf("Неизвестное AI-поле %q", field)
	}
	return s.assistant.UpdateConfig(assistant.Config{
		SystemPrompt:        s.cfg.AI.SystemPrompt,
		DeepSystemPrompt:    s.cfg.AI.DeepSystemPrompt,
		MaxOutputTokens:     s.cfg.AI.MaxOutputTokens,
		DeepMaxOutputTokens: s.cfg.AI.DeepMaxOutputTokens,
		Temperature:         s.cfg.AI.Temperature,
	})
}

func (s *Service) setRateLimit(field, value string) error {
	switch field {
	case "per_user", "global":
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed <= 0 {
			return fmt.Errorf("%s должно быть положительным целым числом", field)
		}
		if field == "per_user" {
			s.cfg.RateLimit.PerUser = parsed
		} else {
			s.cfg.RateLimit.Global = parsed
		}
	case "window":
		parsed, err := time.ParseDuration(value)
		if err != nil || parsed <= 0 {
			return fmt.Errorf("window должно быть положительной duration, например 1m")
		}
		s.cfg.RateLimit.Window = parsed
	default:
		return fmt.Errorf("Неизвестное поле rate_limit %q", field)
	}
	return nil
}

func (s *Service) changeAllowlist(fields []string) string {
	if len(fields) != 4 {
		return "Использование: /settings allow telegram <id> или /settings deny discord <id>."
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	add := fields[1] == "allow"
	switch fields[2] {
	case "telegram":
		id, err := strconv.ParseInt(fields[3], 10, 64)
		if err != nil || id <= 0 || id == s.cfg.Telegram.OwnerID {
			return "Некорректный Telegram ID. Owner нельзя удалить."
		}
		if add {
			s.cfg.Telegram.AllowedUserIDs[id] = struct{}{}
		} else {
			delete(s.cfg.Telegram.AllowedUserIDs, id)
		}
	case "discord":
		id := strings.TrimSpace(fields[3])
		if _, err := strconv.ParseInt(id, 10, 64); err != nil || id == s.cfg.Discord.OwnerID {
			return "Некорректный Discord ID. Owner нельзя удалить."
		}
		if add {
			s.cfg.Discord.AllowedUserIDs[id] = struct{}{}
		} else {
			delete(s.cfg.Discord.AllowedUserIDs, id)
		}
	default:
		return "Платформа: telegram или discord."
	}
	if err := config.SaveConfig(s.cfg); err != nil {
		return fmt.Sprintf("Не удалось сохранить config.yaml: %v", err)
	}
	return "Сохранено. Изменение доступа применится после перезапуска."
}

func (s *Service) providersView() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.providersViewLocked()
}
func (s *Service) providersViewLocked() string {
	return fmt.Sprintf("Провайдеры:\nprimary: name=%q, base_url=%q, model=%q, api_key=%s\nfallback: name=%q, base_url=%q, model=%q, api_key=%s", s.providers.Primary.Name, s.providers.Primary.BaseURL, s.providers.Primary.Model, secretState(s.providers.Primary.APIKey), s.providers.Fallback.Name, s.providers.Fallback.BaseURL, s.providers.Fallback.Model, secretState(s.providers.Fallback.APIKey))
}

// PrivateProvidersView returns secrets for the owner-only Telegram settings chat.
func (s *Service) PrivateProvidersView() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return fmt.Sprintf("Провайдеры:\nprimary: name=%q, base_url=%q, model=%q, api_key=%q\nfallback: name=%q, base_url=%q, model=%q, api_key=%q", s.providers.Primary.Name, s.providers.Primary.BaseURL, s.providers.Primary.Model, s.providers.Primary.APIKey, s.providers.Fallback.Name, s.providers.Fallback.BaseURL, s.providers.Fallback.Model, s.providers.Fallback.APIKey)
}

func (s *Service) configView() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return fmt.Sprintf(
		"config.yaml:\nlog_level=%s\nai: max_output_tokens=%d, deep_max_output_tokens=%d, temperature=%g, request_timeout=%s\ntelegram: enabled=%t, users=%d\ndiscord: enabled=%t, users=%d\nrate_limit: per_user=%d, global=%d, window=%s",
		s.cfg.LogLevel,
		s.cfg.AI.MaxOutputTokens,
		s.cfg.AI.DeepMaxOutputTokens,
		s.cfg.AI.Temperature,
		s.cfg.AI.RequestTimeout,
		s.cfg.Telegram.Enabled,
		len(s.cfg.Telegram.AllowedUserIDs),
		s.cfg.Discord.Enabled,
		len(s.cfg.Discord.AllowedUserIDs),
		s.cfg.RateLimit.PerUser,
		s.cfg.RateLimit.Global,
		s.cfg.RateLimit.Window,
	)
}
func secretState(value string) string {
	if strings.TrimSpace(value) == "" {
		return "не задан"
	}
	return "задан"
}
func (s *Service) help() string {
	return "Настройки:\n/settings providers\n/settings config\n/settings set primary name|api_key|base_url|model <значение>\n/settings set fallback name|api_key|base_url|model <значение>\n/settings set config ai system_prompt|deep_system_prompt|max_output_tokens|deep_max_output_tokens|temperature|request_timeout <значение>\n/settings set config rate_limit per_user|global|window <значение>\n/settings set config telegram|discord enabled true|false\n/settings allow|deny telegram|discord <id>\nКлючи никогда не показываются в ответах."
}
