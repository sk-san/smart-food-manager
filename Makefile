# Monorepo task runner. Run `make help` for the list.
.DEFAULT_GOAL := help
SHELL := /bin/bash

# Load .env so native targets (backend, migrate, etc.) get the same config as
# the containers — including OTEL_EXPORTER_OTLP_INSECURE and DATABASE_URL.
# Without this, `go run` starts with TLS-on OTLP defaults and telemetry is
# silently dropped against the plaintext collector.
ifneq (,$(wildcard .env))
include .env
export
endif

POSTGRES_USER ?= app
POSTGRES_DB   ?= foodapp

.PHONY: help db-up db-down migrate migrate-status migrate-baseline backend frontend tidy install build clean obs-up obs-down test test-e2e test-e2e-frontend

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

db-up: ## Start Postgres + Adminer (migrations auto-run on first init)
	docker compose up -d db adminer

db-down: ## Stop and remove the database containers and volume
	docker compose down -v

migrate: ## Apply outstanding migrations (same runner the deploy uses)
	cd backend && go run ./cmd/migrate

migrate-status: ## Show which migrations would be applied, changing nothing
	cd backend && go run ./cmd/migrate -dry-run

migrate-baseline: ## Mark migrations as applied without running them (db built before the runner existed)
	cd backend && go run ./cmd/migrate -baseline

backend: ## Run the Go API (hot path: go run)
	cd backend && go run ./cmd/api

test: ## Run backend unit tests
	cd backend && go test ./...

test-e2e: ## Run end-to-end API tests (needs the db from `make db-up`)
	cd backend && go test -tags e2e -count=1 -v ./e2e

test-e2e-frontend: ## Run Playwright UI tests (boots Vite itself; no backend needed)
	cd frontend && npx playwright test

frontend: ## Run the Vite dev server
	cd frontend && npm run dev

tidy: ## Resolve Go module dependencies
	cd backend && go mod tidy

install: ## Install frontend dependencies
	cd frontend && npm install

build: ## Build backend binary and frontend bundle
	cd backend && CGO_ENABLED=0 go build -o bin/api ./cmd/api
	cd frontend && npm run build

obs-up: ## Start the LGTM observability stack (Grafana :3000, Prometheus :9090)
	docker compose --profile monitoring up -d

obs-down: ## Stop the LGTM observability stack and remove its volumes
	docker compose --profile monitoring down -v

clean: ## Remove build artifacts
	rm -rf backend/bin frontend/dist
