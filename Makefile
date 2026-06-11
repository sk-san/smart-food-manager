# Monorepo task runner. Run `make help` for the list.
.DEFAULT_GOAL := help
SHELL := /bin/bash

POSTGRES_USER ?= app
POSTGRES_DB   ?= foodapp

.PHONY: help db-up db-down migrate backend frontend tidy install build clean obs-up obs-down

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

db-up: ## Start Postgres + Adminer (migrations auto-run on first init)
	docker compose up -d db adminer

db-down: ## Stop and remove the database containers and volume
	docker compose down -v

migrate: ## Apply migrations manually against the running db
	@for f in backend/migrations/*.sql; do \
		echo "applying $$f"; \
		docker compose exec -T db psql -U $(POSTGRES_USER) -d $(POSTGRES_DB) -v ON_ERROR_STOP=1 < $$f; \
	done

backend: ## Run the Go API (hot path: go run)
	cd backend && go run ./cmd/api

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
