# NexusFlow development task runner.
# See: ADR-004, ADR-005, TASK-001

.PHONY: help up down build test vet lint sqlc migrate seed logs

# Default target: show help
help:
	@echo "NexusFlow development commands:"
	@echo ""
	@echo "  make up          Start all core services (api, worker, monitor, redis, postgres)"
	@echo "  make up-demo     Start all services including demo infrastructure (MinIO, demo-postgres)"
	@echo "  make down        Stop and remove containers (preserves volumes)"
	@echo "  make down-clean  Stop and remove containers AND volumes"
	@echo "  make build       Build all Go binaries"
	@echo "  make test        Run Go tests"
	@echo "  make vet         Run go vet on all packages"
	@echo "  make lint        Run staticcheck on all packages"
	@echo "  make sqlc        Generate sqlc query code from internal/db/queries/"
	@echo "  make migrate-up  Run pending database migrations"
	@echo "  make migrate-down Run one down migration"
	@echo "  make seed        Seed the database with the initial admin user"
	@echo "  make logs        Tail logs for all services"
	@echo "  make scale-workers N=3  Scale the worker service to N instances"

up:
	docker compose up -d

up-demo:
	docker compose --profile demo up -d

down:
	docker compose down

down-clean:
	docker compose down -v

build:
	go build ./...

test:
	go test ./... -v -count=1

vet:
	go vet ./...

lint:
	staticcheck ./...

sqlc:
	sqlc generate

migrate-up:
	migrate -path internal/db/migrations -database "$(DATABASE_URL)" up

migrate-down:
	migrate -path internal/db/migrations -database "$(DATABASE_URL)" down 1

seed:
	go run ./cmd/api --seed-only

logs:
	docker compose logs -f

scale-workers:
	docker compose up --scale worker=$(N) -d worker

# Run CI checks locally (mirrors .github/workflows/ci.yml)
ci: build vet lint test
	@echo "CI checks passed"
