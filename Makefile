SHELL := /bin/sh

COMPOSE_PROJECT_NAME ?= go-microservices-lab
VERIFY_COMPOSE_PROJECT_NAME ?= go-microservices-phase1
COMPOSE := docker compose --project-name $(COMPOSE_PROJECT_NAME) -f project/docker-compose.yml
MODULES := authentication-service broker-service front-end listener-service logger-service mail-service
GO_FILES := $(shell rg --files -g '*.go')

.DEFAULT_GOAL := help

.PHONY: help doctor fmt fmt-check tidy tidy-check test vet build config dev demo smoke \
	logs ps down clean verify-local verify-phase-1 verify-phase-4 verify-phase-5

help: ## Show available commands
	@awk 'BEGIN {FS = ":.*## "; printf "Go Microservices Communication Lab\n\nUsage:\n  make <target>\n\nTargets:\n"} /^[a-zA-Z0-9_-]+:.*## / {printf "  %-18s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

doctor: ## Check required local tools
	@command -v go >/dev/null || { echo "go is required"; exit 1; }
	@command -v docker >/dev/null || { echo "docker is required"; exit 1; }
	@docker compose version >/dev/null || { echo "docker compose is required"; exit 1; }
	@command -v curl >/dev/null || { echo "curl is required"; exit 1; }
	@command -v jq >/dev/null || { echo "jq is required"; exit 1; }
	@command -v rg >/dev/null || { echo "ripgrep is required"; exit 1; }
	@docker info >/dev/null || { echo "docker daemon is not running"; exit 1; }

fmt: ## Format all Go source
	@gofmt -w $(GO_FILES)

fmt-check: ## Fail when Go source is not formatted
	@test -z "$$(gofmt -l $(GO_FILES))" || { gofmt -l $(GO_FILES); exit 1; }

tidy: ## Tidy every Go module
	@set -e; for module in $(MODULES); do (cd $$module && go mod tidy); done

tidy-check: ## Fail when a Go module needs tidying
	@set -e; for module in $(MODULES); do (cd $$module && go mod tidy -diff); done

test: ## Run every Go test
	@set -e; for module in $(MODULES); do echo "==> $$module"; (cd $$module && go test ./...); done

vet: ## Run go vet for every module
	@set -e; for module in $(MODULES); do echo "==> $$module"; (cd $$module && go vet ./...); done

config: ## Validate Docker Compose configuration
	@$(COMPOSE) config --quiet

build: doctor ## Build all container images from source
	@$(COMPOSE) build

dev: doctor ## Build and start the complete lab
	@$(COMPOSE) up --build -d --wait --wait-timeout 180

demo: dev smoke ## Start the lab and execute all demo scenarios

smoke: ## Assert all public workflows end-to-end
	@./scripts/smoke.sh --project $(COMPOSE_PROJECT_NAME)

logs: ## Follow container logs
	@$(COMPOSE) logs -f

ps: ## Show container status
	@$(COMPOSE) ps

down: ## Stop the lab
	@$(COMPOSE) down --remove-orphans

clean: ## Stop the lab and remove its named volumes
	@$(COMPOSE) down --volumes --remove-orphans

verify-local: fmt-check tidy-check test vet config ## Run fast local verification

verify-phase-1: ## Run the complete Phase 1 acceptance gate
	@./scripts/verify-phase-1.sh --project $(VERIFY_COMPOSE_PROJECT_NAME)

verify-phase-4: ## Verify reliable RabbitMQ delivery after a logger outage
	@./scripts/verify-phase-4.sh

verify-phase-5: ## Verify tracing and protocol failure injection
	@./scripts/verify-phase-5.sh
