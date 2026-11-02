BACKEND  := backend
FRONTEND := frontend
COMPOSE  := docker compose -f $(BACKEND)/docker-compose.yml
GOOSE    := go run github.com/pressly/goose/v3/cmd/goose@latest

# The Go service reads config straight from the environment, so export
# backend/.env into the shell before running anything that needs it.
LOAD_ENV := set -a && . ./.env && set +a

.PHONY: help setup env reset install dev dev-backend dev-frontend db-up db-down migrate migrate-down seed admin sqlc build lint typecheck test

help: ## Show available targets
	@grep -E '^[a-zA-Z_-]+:.*## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*## "}; {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'

setup: ## First-time setup: .env files with generated secrets, deps, database, seed
	@./scripts/setup.sh

env: ## Only create/fill the .env files (never overwrites values you've set)
	@./scripts/setup.sh --env-only

reset: ## Wipe the local database and seed fresh demo data (asks first; FORCE=1 to skip)
	@./scripts/setup.sh --reset

install: ## Install backend and frontend dependencies
	cd $(BACKEND) && go mod download
	cd $(FRONTEND) && npm install

dev: ## Run backend and frontend together (Ctrl-C stops both)
	@$(MAKE) -j2 --no-print-directory dev-backend dev-frontend

dev-backend: db-up ## Start Postgres and the Go API with live reload (air)
	cd $(BACKEND) && $(LOAD_ENV) && air

dev-frontend: ## Start the Next.js dev server
	cd $(FRONTEND) && npm run dev

db-up: ## Start Postgres in Docker
	$(COMPOSE) up -d

db-down: ## Stop Postgres
	$(COMPOSE) down

migrate: ## Apply database migrations
	cd $(BACKEND) && $(LOAD_ENV) && $(GOOSE) -dir internal/db/migrations postgres "$$DATABASE_URL" up

migrate-down: ## Roll back the last migration
	cd $(BACKEND) && $(LOAD_ENV) && $(GOOSE) -dir internal/db/migrations postgres "$$DATABASE_URL" down

seed: ## Seed the database
	cd $(BACKEND) && $(LOAD_ENV) && go run ./cmd/seed

admin: ## Create a platform admin (make admin EMAIL=you@example.com NAME="Your Name")
	@test -n "$(EMAIL)" -a -n "$(NAME)" || { echo 'usage: make admin EMAIL=you@example.com NAME="Your Name"'; exit 1; }
	@cd $(BACKEND) && $(LOAD_ENV) && { stty -echo; trap 'stty echo' EXIT; printf 'Password (min 12 characters): '; read -r pw; stty echo; echo; printf '%s\n' "$$pw" | go run ./cmd/admin create --email "$(EMAIL)" --name "$(NAME)"; }

sqlc: ## Regenerate Go code from SQL queries
	cd $(BACKEND) && sqlc generate

build: ## Build both apps
	cd $(BACKEND) && go build -o bin/api ./cmd/api
	cd $(FRONTEND) && npm run build

lint: ## Lint both apps
	cd $(BACKEND) && go vet ./...
	cd $(FRONTEND) && npm run lint

typecheck: ## Typecheck the frontend
	cd $(FRONTEND) && npm run typecheck

test: ## Run backend tests
	cd $(BACKEND) && go test ./...
