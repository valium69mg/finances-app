-include .env
export

MIGRATIONS_DIR := migrations

.PHONY: db-up db-down migrate-up migrate-down seed run test test-integration import-config fe-dev fe-test fe-e2e

db-up: ## Start PostgreSQL and MinIO and wait until they are healthy
	docker compose up -d --wait

db-down: ## Stop PostgreSQL and MinIO (data volumes are kept)
	docker compose down

migrate-up: ## Apply all pending migrations
	@test -n "$$DATABASE_URL" || { echo "DATABASE_URL is not set (see README)"; exit 1; }
	migrate -path $(MIGRATIONS_DIR) -database "$$DATABASE_URL" up

migrate-down: ## Revert the last migration
	@test -n "$$DATABASE_URL" || { echo "DATABASE_URL is not set (see README)"; exit 1; }
	migrate -path $(MIGRATIONS_DIR) -database "$$DATABASE_URL" down 1

seed: ## Insert the admin user (idempotent)
	@test -n "$$DATABASE_URL" || { echo "DATABASE_URL is not set (see README)"; exit 1; }
	cd backend && go run ./cmd/seed

run: ## Start the API
	cd backend && go run ./cmd/api

test: ## Run backend tests
	cd backend && go test ./...

test-integration: ## Run backend tests including the database integration tests (needs db-up)
	@test -n "$$DATABASE_URL" || { echo "DATABASE_URL is not set (see README)"; exit 1; }
	cd backend && TEST_DATABASE_URL="$$DATABASE_URL" go test -count=1 ./...

import-config: ## One-time import of ~/finances/config.json (FORCE=1 to overwrite existing settings)
	@test -n "$$DATABASE_URL" || { echo "DATABASE_URL is not set (see README)"; exit 1; }
	cd backend && go run ./cmd/import-config $(if $(FORCE),--force,) $(if $(CONFIG),--file $(CONFIG),)

fe-dev: ## Start the frontend dev server
	cd frontend && npm run dev

fe-test: ## Run frontend tests
	cd frontend && npm test

fe-e2e: ## Run frontend Playwright e2e tests (API is mocked)
	cd frontend && npm run e2e
