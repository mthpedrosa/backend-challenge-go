SHELL := /bin/bash
GO ?= go
PKG := ./...

.DEFAULT_GOAL := help

.PHONY: help
help: ## List available targets
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'

.env:
	cp .env.example .env

.PHONY: up
up: .env ## Build and start the full stack
	docker compose up --build -d

.PHONY: down
down: ## Stop the stack (keeps volumes)
	docker compose down

.PHONY: clean
clean: ## Stop the stack and remove volumes
	docker compose down -v

.PHONY: logs
logs: ## Follow application logs
	docker compose logs -f app

.PHONY: run
run: ## Run the service locally (outside Docker)
	$(GO) run ./cmd/wallet

.PHONY: build
build: ## Build the binary into ./bin
	CGO_ENABLED=0 $(GO) build -trimpath -o bin/wallet ./cmd/wallet

.PHONY: test
test: ## Run unit tests
	$(GO) test -count=1 $(PKG)

.PHONY: test-race
test-race: ## Run unit tests with the race detector
	$(GO) test -race -count=1 $(PKG)

.PHONY: fmt
fmt: ## Format code
	gofmt -w .

.PHONY: lint
lint: ## gofmt check + go vet
	@test -z "$$(gofmt -l .)" || (echo "gofmt needed on:"; gofmt -l .; exit 1)
	$(GO) vet $(PKG)

.PHONY: tidy
tidy: ## Tidy go.mod/go.sum
	$(GO) mod tidy
