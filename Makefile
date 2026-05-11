SHELL := /bin/bash
.DEFAULT_GOAL := help

ROOT_DIR := $(abspath $(dir $(lastword $(MAKEFILE_LIST))))
DOCKER_COMPOSE := docker compose
GO_RUN_ENV := set -a && source .env && set +a

.PHONY: help env deps-up deps-wait deps-down deps-reset saas saas-docker-up saas-docker-logs web-install web agent-config agent build test race smoke dev

help: ## Show available development commands
	@awk 'BEGIN {FS = ": .*## "}; /^[a-zA-Z0-9_.-]+: .*## / {printf "\033[36m%-18s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

env: ## Create .env from .env.example if it does not exist
	@"$(ROOT_DIR)/scripts/ensure-env.sh"

deps-up: env ## Start Postgres and Redis for local development
	@$(DOCKER_COMPOSE) up -d postgres redis

deps-wait: ## Wait until Postgres and Redis are healthy
	@"$(ROOT_DIR)/scripts/wait-for-deps.sh"

deps-down: ## Stop local Postgres and Redis containers
	@$(DOCKER_COMPOSE) down

deps-reset: ## Remove local Postgres and Redis containers and volumes
	@$(DOCKER_COMPOSE) down -v --remove-orphans

saas: env ## Run the SaaS backend locally against .env
	@bash -lc '$(GO_RUN_ENV) && go run ./cmd/saas -config config.yaml'

saas-docker-up: env ## Run the SaaS backend inside Docker instead of locally
	@$(DOCKER_COMPOSE) --profile container up -d --build saas

saas-docker-logs: ## Tail logs from the containerized SaaS backend
	@$(DOCKER_COMPOSE) logs -f saas

web-install: ## Install frontend dependencies with npm ci
	@cd web-frontend && npm ci

web: ## Run the Vite frontend locally on 127.0.0.1:4173
	@bash -lc '$(GO_RUN_ENV) && cd web-frontend && npm run dev -- --host "$${QUANTSAAS_WEB_HOST:-127.0.0.1}" --port "$${QUANTSAAS_WEB_PORT:-4173}"'

agent-config: ## Create config.agent.yaml from the example if it does not exist
	@bash -lc 'if [[ -f config.agent.yaml ]]; then echo "config.agent.yaml already exists"; else cp config.agent.yaml.example config.agent.yaml; echo "created config.agent.yaml from example"; fi'

agent: agent-config ## Run the local agent using config.agent.yaml
	@bash -lc '$(GO_RUN_ENV) && go run ./cmd/agent -config config.agent.yaml'

build: ## Build Go packages and the frontend production bundle
	@go build ./...
	@cd web-frontend && npm run build

test: ## Run the Go test suite
	@go test ./...

race: ## Run the Go test suite with the race detector
	@go test ./... -race -timeout 300s

smoke: ## Check the local SaaS health endpoint
	@"$(ROOT_DIR)/scripts/wait-for-http.sh" http://127.0.0.1:8080/healthz 60 >/dev/null
	@curl -fsS http://127.0.0.1:8080/healthz

dev: env ## Start deps, run SaaS locally, and launch the Vite frontend
	@"$(ROOT_DIR)/scripts/dev.sh"
