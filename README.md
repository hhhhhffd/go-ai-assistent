# go-ai-assistant

Один Go-процесс для Telegram и Discord. Платформы используют общий `internal/assistant`-сервис и OpenAI-compatible primary/fallback-провайдеры. История и база данных не используются.

## Возможности

- Telegram Guest Mode: упоминание бота и reply на его сообщение. Owner настраивает бот в обычном личном чате командой `/settings`.
- Discord Gateway: `/ask`, `/deep`, `/settings` и контекстная команда сообщения `Объяснить`; ответы по умолчанию ephemeral.
- Настройки и провайдеры меняются owner'ом без перезапуска.

## Telegram: важное ограничение Guest Mode

Telegram работает только в Guest Mode: бот не добавляется в чат. Укажите `TELEGRAM_API_ID` и `TELEGRAM_API_HASH` из [my.telegram.org](https://my.telegram.org), включите Guest Mode у бота и используйте `@bot_username вопрос` в доступном Telegram-чате. Запросы обрабатываются по MTProto `updateBotGuestChatQuery` и ответы отправляются через `messages.setBotGuestChatResult`. Пользователь должен быть в `ALLOWED_TELEGRAM_USER_IDS`.

Guest Mode недоступен для secret chats и чатов с content protection. Ответ Guest Mode — одно сообщение Telegram, поэтому ответы сверх 4096 символов сокращаются. Старые `BOT_TOKEN` и `ALLOWED_USER_IDS` временно поддерживаются, но новые переменные приоритетнее.

## Discord Developer Portal

1. Создайте Application в [Discord Developer Portal](https://discord.com/developers/applications) и откройте **Installation**.
2. В **Installation Contexts** включите **Guild Install**.
3. В install link выберите scopes `bot` и `applications.commands`.
4. Не задавайте **Interactions Endpoint URL**: бот подключается к Gateway сам, публичный адрес не нужен.
5. В **General Information** скопируйте Application ID; Bot Token нужен для Gateway и регистрации команд.
6. Добавьте свой Discord user ID в `OWNER_DISCORD_ID`.
7. Зарегистрируйте команды командой ниже. Повторный запуск заменяет весь набор глобальных команд и не создаёт дубликаты.
8. В разделе Installation используйте сгенерированную ссылку, чтобы добавить bot в нужный сервер.

```bash
make register-discord
# или при установленном Go: go run ./cmd/register-discord
```

## Конфигурация и запуск

Скопируйте `.env.example` в `.env` и заполните только bootstrap-данные: owner IDs, токены и Telegram API ID/hash. При первом запуске бот создаст `config.yaml` и `providers.yaml`; оба файла локальные и не коммитятся. Через `/settings` owner может настроить primary/fallback провайдеры. API keys хранятся только в `providers.yaml` с правами `0600`.

В Telegram откройте личный чат с ботом и отправьте `/settings`. В Discord используйте `/settings`; для изменения передайте поле `command`, например `set primary model gpt-4.1`. Команды одинаковы на обеих платформах:

```text
/settings providers
/settings set primary name openai
/settings set primary api_key <key>
/settings set primary base_url https://api.openai.com/v1
/settings set primary model gpt-4.1
/settings config
/settings set config ai temperature 0.3
/settings allow telegram 123456789
```

Изменения провайдеров и AI-параметров применяются сразу. Лимиты, allowlist и включение платформ сохраняются в `config.yaml` и применяются при следующем перезапуске процесса.

```bash
docker compose up --build
```

Discord Gateway и Telegram работают через исходящие соединения: открытый HTTP-порт, Caddy и публичный HTTPS URL не нужны.

## Проверки

При отсутствии локального Go используйте Docker:

```bash
make fmt
make vet
make test
make test-race
make build
docker build -t go-ai-assistant .
```

Systemd unit: `deploy/systemd/go-ai-assistant.service`. При миграции замените старый бинарник `go-gpt-tg-bot` на `go-ai-assistant`, обновите unit path и выполните `systemctl daemon-reload` перед перезапуском.
