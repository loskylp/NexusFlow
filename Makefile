# NexusFlow development task runner.
# See: ADR-004, ADR-005, TASK-001, TASK-029

.PHONY: help up down build test vet lint sqlc migrate seed logs staging-up staging-down staging-logs staging-pull staging-tag

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
	@echo ""
	@echo "Staging commands (run on staging host or via SSH):"
	@echo "  make staging-up      Deploy staging stack from registry images"
	@echo "  make staging-pull    Pull latest images from registry without restarting"
	@echo "  make staging-down    Stop and remove staging containers (preserves volumes)"
	@echo "  make staging-logs    Tail staging service logs"
	@echo "  make staging-tag V=v1.0  Push a demo/vN.N git tag to trigger CD pipeline"

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

# --- Staging targets (TASK-029) ---
# These targets operate on the staging compose stack at deploy/staging/docker-compose.yml.
# Run on the staging host (/opt/nexusflow) or prefix commands with SSH as needed.

# STAGING_COMPOSE is the compose file path relative to the project root.
STAGING_COMPOSE := deploy/staging/docker-compose.yml

# staging-up: pull latest images and start (or restart) the staging stack.
staging-up:
	docker compose -f $(STAGING_COMPOSE) pull
	docker compose -f $(STAGING_COMPOSE) up -d

# staging-pull: pull the latest images from the registry without restarting services.
# Use this to pre-warm the cache before a rolling redeploy.
staging-pull:
	docker compose -f $(STAGING_COMPOSE) pull

# staging-down: stop and remove staging containers while preserving volumes.
staging-down:
	docker compose -f $(STAGING_COMPOSE) down

# staging-logs: tail logs for all staging services.
staging-logs:
	docker compose -f $(STAGING_COMPOSE) logs -f

# staging-tag: push a demo/vN.N git tag to trigger the CD pipeline.
# Usage: make staging-tag V=v1.0
# The CD workflow builds all images, tags them with V and "latest", and pushes to ghcr.io.
# Watchtower on staging then redeploys within 5 minutes.
staging-tag:
	@test -n "$(V)" || (echo "Error: V is required. Usage: make staging-tag V=v1.0" && exit 1)
	git tag demo/$(V)
	git push origin demo/$(V)
	@echo "Tag demo/$(V) pushed. CD pipeline will build and push images."
	@echo "Watchtower will redeploy staging within 5 minutes."
