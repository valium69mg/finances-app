-include .env
export

MIGRATIONS_DIR := migrations

.PHONY: db-up db-down migrate-up migrate-down run test

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

run: ## Start the API
	cd backend && go run ./cmd/api

test: ## Run backend tests
	cd backend && go test ./...
