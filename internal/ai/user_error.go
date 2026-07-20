package ai

import "errors"

// UserErrorMessage converts provider failures into safe, actionable text for chat users.
func UserErrorMessage(err error) string {
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) {
		return "Не удалось получить ответ от AI. Попробуйте ещё раз позже."
	}

	switch providerErr.Kind {
	case ErrorKindConfig:
		return "AI ещё не настроен. Проверьте провайдера, API key, base URL и модель в настройках."
	case ErrorKindUnauthorized, ErrorKindForbidden:
		return "AI-провайдер отклонил API key (401/403). Проверьте ключ в настройках."
	case ErrorKindUnknownModel:
		return "Указанная AI-модель недоступна. Проверьте название модели в настройках."
	case ErrorKindRateLimited:
		return "AI-провайдер временно ограничил запросы (429). Попробуйте немного позже."
	case ErrorKindServer:
		return "AI-провайдер временно недоступен (5xx). Попробуйте немного позже."
	case ErrorKindTimeout:
		return "AI-провайдер не ответил вовремя. Попробуйте ещё раз позже."
	case ErrorKindNetwork:
		return "Не удалось подключиться к AI-провайдеру. Попробуйте ещё раз позже."
	case ErrorKindBadRequest:
		return "AI-провайдер отклонил запрос. Проверьте модель и настройки провайдера."
	case ErrorKindInvalidResponse:
		return "AI-провайдер вернул некорректный ответ. Попробуйте ещё раз позже."
	default:
		return "AI-провайдер временно недоступен. Попробуйте ещё раз позже."
	}
}
