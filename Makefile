APP_NAME := go-ai-assistant
GO_IMAGE ?= golang:1.25
DOCKER_GO := docker run --rm -v $(PWD):/app -w /app $(GO_IMAGE)

.PHONY: tidy fmt vet test test-race build check run compose-up compose-discord-up compose-down register-discord

tidy:
	$(DOCKER_GO) sh -c "go mod tidy"

fmt:
	$(DOCKER_GO) sh -c "find . -type f -name '*.go' -not -path './vendor/*' -print0 | xargs -0 gofmt -w"

vet:
	$(DOCKER_GO) sh -c "go vet ./..."

test:
	$(DOCKER_GO) sh -c "go test ./..."

test-race:
	$(DOCKER_GO) sh -c "go test -race ./..."

build:
	$(DOCKER_GO) sh -c "CGO_ENABLED=0 go build -o ./bin/$(APP_NAME) ./cmd/bot"

check: fmt vet test test-race build

run:
	docker compose up --build

compose-up:
	docker compose up -d --build

compose-discord-up:
	docker compose -f docker-compose.yml -f docker-compose.discord.yml up -d --build

compose-down:
	docker compose down

register-discord:
	$(DOCKER_GO) sh -c "go run ./cmd/register-discord"
