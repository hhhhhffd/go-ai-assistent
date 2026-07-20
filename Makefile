APP_NAME := go-ai-assistant

.PHONY: tidy fmt vet test test-race build check register-discord

tidy:
	go mod tidy

fmt:
	find . -type f -name '*.go' -not -path './vendor/*' -print0 | xargs -0 gofmt -w

vet:
	go vet ./...

test:
	go test ./...

test-race:
	go test -race ./...

build:
	go build -o ./bin/$(APP_NAME) ./cmd/bot

check: fmt vet test test-race build

register-discord:
	go run ./cmd/register-discord
