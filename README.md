<div align="center">

<h1>go-ai-assistant</h1>
<hr>

**An AI assistant bot in one place: Telegram guest mode, Discord slash commands, LLM provider failover**

![Go](https://img.shields.io/badge/GO-1.25-00ADD8?style=flat-square&logo=go&logoColor=white)
![Telegram](https://img.shields.io/badge/TELEGRAM-MTProto-26A5E4?style=flat-square&logo=telegram&logoColor=white)
![Discord](https://img.shields.io/badge/DISCORD-Gateway-5865F2?style=flat-square&logo=discord&logoColor=white)
![License](https://img.shields.io/badge/LICENSE-MIT-green?style=flat-square)

[Stack](#technical-stack) · [Quick start](#quick-start) · [License](#license)

</div>

> [!NOTE]
> Archived — this project is no longer maintained.

## What it is

A Go service that answers questions with any OpenAI-compatible chat completions API. It works in Telegram (guest mode, plus an owner-only settings panel in a direct chat) and in Discord (`/ask`, `/deep` and an "explain" message command).

There is no HTTP server and no database. The service makes outbound connections only and keeps its settings in YAML files.

## Highlights

- Primary and fallback LLM providers; the fallback is used on transient errors only.
- One rate limiter shared by Telegram and Discord (per-user and global limits).
- Runtime settings edited from chat and written to `config.yaml` / `providers.yaml`.
- Allowlists of user IDs per platform.
- Unit tests for the AI client, assistant service, rate limiter, config and both platform adapters.
- Systemd unit and an Ubuntu deployment script.

## Technical stack

| Layer | Technology |
|-------|------------|
| Language | Go 1.25, standard library `net/http` |
| Telegram | `gotd/td` (MTProto guest mode), Bot API long polling for settings |
| Discord | `coder/websocket` (Gateway), REST interactions |
| Config | YAML (`yaml.v2`), `.env` via `godotenv` |
| Deploy | systemd, `scripts/deploy.sh` |

## Quick start

Requirements: Go 1.25, a Telegram bot token with API ID and hash, optionally a Discord application, and an API key for an OpenAI-compatible provider.

```bash
cp .env.example .env                       # tokens and owner IDs
mkdir -p data
cp config.example.yaml data/config.yaml    # prompts, limits, platform switches
cp providers.example.yaml data/providers.yaml   # primary / fallback provider
make build && ./bin/go-ai-assistant
```

Other targets: `make check` (fmt, vet, tests, race tests, build) and `make register-discord` (registers the slash commands). Remote install: `./scripts/deploy.sh user@host`.

| Variable | Purpose |
|----------|---------|
| `CONFIG_DIR` | Directory with `config.yaml` and `providers.yaml` (`./data` in `.env.example`) |
| `OWNER_TELEGRAM_ID`, `OWNER_DISCORD_ID` | Who may change settings |
| `TELEGRAM_BOT_TOKEN`, `TELEGRAM_API_ID`, `TELEGRAM_API_HASH` | Telegram credentials |
| `DISCORD_BOT_TOKEN`, `DISCORD_CLIENT_ID` | Discord credentials |

## Project structure

```text
cmd/        bot (main service), register-discord (slash command setup)
internal/   ai, assistant, config, ratelimit, settings, telegram, platform/discord
deploy/     systemd unit
scripts/    deploy.sh
```

## License

MIT, see [LICENSE](LICENSE).
