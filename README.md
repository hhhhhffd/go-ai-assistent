# go-ai-assistant

Один Go-процесс для Telegram и Discord. Платформы используют общий `internal/assistant`-сервис, единый system prompt и OpenAI-compatible primary/fallback-провайдеры. История и база данных не используются.

## Возможности

- Telegram: упоминание бота и reply на его сообщение, только для `ALLOWED_TELEGRAM_USER_IDS`.
- Discord User-Installed App: `/ask`, `/deep`, контекстная команда сообщения `Объяснить`; по умолчанию ответы ephemeral.
- Discord использует только HTTP Interactions Endpoint, без Gateway и без добавления обычного guild-бота.

## Telegram: важное ограничение Guest Mode

Telegram работает только в Guest Mode: бот не добавляется в чат. Укажите `TELEGRAM_API_ID` и `TELEGRAM_API_HASH` из [my.telegram.org](https://my.telegram.org), включите Guest Mode у бота и используйте `@bot_username вопрос` в доступном Telegram-чате. Запросы обрабатываются по MTProto `updateBotGuestChatQuery` и ответы отправляются через `messages.setBotGuestChatResult`. Пользователь должен быть в `ALLOWED_TELEGRAM_USER_IDS`.

Guest Mode недоступен для secret chats и чатов с content protection. Ответ Guest Mode — одно сообщение Telegram, поэтому ответы сверх 4096 символов сокращаются. Старые `BOT_TOKEN` и `ALLOWED_USER_IDS` временно поддерживаются, но новые переменные приоритетнее.

## Discord Developer Portal

1. Создайте Application в [Discord Developer Portal](https://discord.com/developers/applications) и откройте **Installation**.
2. В **Installation Contexts** включите **User Install**; Guild Install не нужен.
3. Для User Install выберите scope `applications.commands`. Scope `bot` не нужен.
4. В **Interactions Endpoint URL** укажите `https://your-domain.example/discord/interactions` и сохраните настройку. URL должен быть публичным HTTPS.
5. В **General Information** скопируйте Application ID и Public Key. Bot Token понадобится только для регистрации команд.
6. Добавьте свой Discord user ID в `ALLOWED_DISCORD_USER_IDS`.
7. Зарегистрируйте команды командой ниже. Повторный запуск заменяет весь набор глобальных команд и не создаёт дубликаты.
8. В разделе Installation скопируйте сгенерированную ссылку **Add to My Apps**. После установки команды доступны в guild, bot DM и group DM, если Discord разрешает данный context.

```bash
make register-discord
# или при установленном Go: go run ./cmd/register-discord
```

## Конфигурация и запуск

Скопируйте `.env.example` в `.env` и заполните ключи. Telegram переменные не требуются при `TELEGRAM_ENABLED=false`; Discord-переменные не требуются при `DISCORD_ENABLED=false`. Если отключены обе платформы, запуск завершится с ошибкой.

```bash
# Только Telegram: без открытого HTTP-порта
docker compose up --build

# Discord или обе платформы: публикует HTTP_ADDR и включает healthcheck
docker compose -f docker-compose.yml -f docker-compose.discord.yml up --build
```

`HTTP_ADDR` по умолчанию `:8080`; Discord endpoint — `${PUBLIC_BASE_URL}/discord/interactions`. При Discord обязательно задайте `PUBLIC_BASE_URL` с `https://` URL.

### Caddy (пример reverse proxy)

```caddyfile
your-domain.example {
    reverse_proxy 127.0.0.1:8080
}
```

Caddy автоматически выдаст TLS-сертификат при корректных DNS-записях. Он нужен только для публичного Discord endpoint, не для Telegram-only режима.

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
