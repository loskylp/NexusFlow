# Routing Instruction — Builder — TASK-031

**From:** Orchestrator
**To:** @nexus-builder
**Date:** 2026-04-15
**Cycle:** 4
**Task:** TASK-031 -- Demo infrastructure: Mock-Postgres with seed data

---

## Objective

Implement the Mock-Postgres demo infrastructure:
1. Fill in `PostgreSQLSourceConnector` and `PostgreSQLSinkConnector` bodies in `worker/connector_postgres.go` (scaffold already exists with stubs).
2. Configure the `demo-postgres` container in `docker-compose.yml` under the `demo` profile with 10K pre-seeded rows.
3. Wire `RegisterPostgreSQLConnectors(reg, pgBackend)` into `cmd/worker/main.go` when `DEMO_POSTGRES_DSN` env var is set.
4. Produce the acceptance test script body in `tests/acceptance/TASK-031-acceptance.sh`.

Follow the established `DatabaseSinkConnector` pattern in `worker/sink_connectors.go` for the Sink write/transaction wrapper semantics. The Source should use a simple `SELECT * FROM <table>` pattern with batching.

## Acceptance Criteria (from Task Plan)

- `demo-postgres` starts via `docker compose --profile demo up` with 10K pre-seeded rows
- PostgreSQL DataSource can query data from `demo-postgres`
- PostgreSQL Sink can write data to `demo-postgres`
- A demo pipeline can use `demo-postgres` as both DataSource and Sink

## Required Documents

- Task definition: [process/planner/task-plan.md#TASK-031](../planner/task-plan.md) (lines 605-618)
- Scaffold manifest: [process/scaffolder/scaffold-manifest.md](../scaffolder/scaffold-manifest.md) (see Cycle 4 section — file-by-file wiring table around line 629)
- Connector stub to implement: [worker/connector_postgres.go](../../worker/connector_postgres.go)
- Reference pattern (DatabaseSinkConnector): [worker/sink_connectors.go](../../worker/sink_connectors.go)
- Main wiring point: [cmd/worker/main.go](../../cmd/worker/main.go)
- Compose file: [docker-compose.yml](../../docker-compose.yml)
- Acceptance test scaffold: [tests/acceptance/TASK-031-acceptance.sh](../../tests/acceptance/TASK-031-acceptance.sh)

## Dependencies (all satisfied)

- TASK-007 -- Tag-based task assignment and pipeline execution (COMPLETE Cycle 1)
- TASK-018 -- Sink atomicity with idempotency (COMPLETE Cycle 2; InMemoryDedupStore/InMemoryDatabase currently in use — see OBS-018-1/2/3)

## Reminders

- **Nil-wiring check:** After implementing connectors, verify `cmd/worker/main.go` actually registers the PostgreSQL connector when `DEMO_POSTGRES_DSN` is set — do not leave a `nil` pgBackend wired into `RegisterPostgreSQLConnectors`. (See MEMORY: Builder nil-wiring pattern.)
- **Transaction semantics:** The existing `DatabaseSinkConnector.Write` uses a transaction wrapper for atomicity — the PostgreSQL Sink must preserve the same guarantee against a separate PostgreSQL instance (identified as a risk on the task card).
- **Seed data:** 10K rows must be deterministic (stable across compose restarts). A seed SQL file mounted via `docker-entrypoint-initdb.d/` is the conventional approach.
- **Demo profile isolation:** The `demo-postgres` container must only start under the `demo` profile, not the default compose stack (which already has the primary `postgres` container for the app itself).
- Commit working increments. Report final commit SHA, acceptance pass summary, and explicit nil-wiring verification on completion.

## Exit Criteria for Your Handoff

- All 4 acceptance criteria implementable against the running demo-postgres container.
- `tests/acceptance/TASK-031-acceptance.sh` passes locally.
- CI green (Go + web).
- Final commit SHA reported.
- Explicit statement: "nil-wiring verified in cmd/worker/main.go for RegisterPostgreSQLConnectors."

---

**Next:** Invoke @nexus-orchestrator — on completion, report commit SHA, acceptance summary, and nil-wiring verification so Verifier can be dispatched.
