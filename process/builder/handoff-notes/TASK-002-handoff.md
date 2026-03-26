# Builder Handoff — TASK-002
**Date:** 2026-03-26
**Task:** Database schema and migration foundation
**Requirement(s):** REQ-009, REQ-019, REQ-020, ADR-008

## What Was Implemented

### Migration files

**`internal/db/migrations/000001_initial_schema.up.sql`** — Full initial schema.
Creates tables in dependency order: `users`, `workers`, `pipelines`, `pipeline_chains`, `tasks`, `task_state_log`, `task_logs`. All columns, types, CHECK constraints, foreign key constraints, and indexes match the data model in ADR-008. Notable decisions:
- All three pipeline phase configs (`data_source_config`, `process_config`, `sink_config`) stored as JSONB in the `pipelines` table, enabling schema evolution without join complexity.
- `retry_config` in `tasks` stored as JSONB (`{"maxRetries":3,"backoff":"exponential"}`) to match the `RetryConfig` domain model.
- `task_logs` is a partitioned table (`PARTITION BY RANGE(timestamp)`). A `task_logs_default` default partition catches inserts that don't match an explicit weekly partition, preventing insert failures before partition creation.
- State transition trigger `enforce_task_state_transition()` fires `BEFORE INSERT ON task_state_log` and raises a `check_violation` error on any invalid `(from_state, to_state)` pair. All valid transitions from Domain Invariant 1 (ADR-008) are encoded.
- All `CREATE TABLE` and `CREATE INDEX` statements use `IF NOT EXISTS` for safe re-application.

**`internal/db/migrations/000001_initial_schema.down.sql`** — Drops all objects in reverse dependency order. Drops trigger and function first, then tables from most-dependent to least. The parent `task_logs` table drop cascades to all partitions including `task_logs_default`.

### sqlc configuration and generated code

**`internal/db/sqlc.yaml`** — sqlc v2 configuration.
- Engine: postgresql, using pgx/v5 SQL package.
- Type overrides: `uuid` -> `github.com/google/uuid.UUID`, nullable uuid -> `uuid.NullUUID`, `jsonb` -> `encoding/json.RawMessage`.
- Generated package: `sqlcdb`, output directory: `internal/db/sqlc/`.

**`internal/db/queries/users.sql`** — 5 named queries: `CreateUser`, `GetUserByID`, `GetUserByUsername`, `ListUsers`, `DeactivateUser`.

**`internal/db/queries/workers.sql`** — 4 named queries: `RegisterWorker` (ON CONFLICT upsert), `GetWorkerByID`, `ListWorkers` (with correlated subquery for `current_task_id`), `UpdateWorkerStatus`.

**`internal/db/queries/pipelines.sql`** — 7 named queries: `CreatePipeline`, `GetPipelineByID`, `ListPipelinesByUser`, `ListAllPipelines`, `UpdatePipeline`, `DeletePipeline`, `PipelineHasActiveTasks`.

**`internal/db/queries/tasks.sql`** — 9 named queries: `CreateTask`, `GetTaskByID`, `ListTasksByUser`, `ListAllTasks`, `UpdateTaskStatus`, `IncrementTaskRetryCount`, `CancelTask`, `GetTaskStateLog`, `InsertTaskStateLog`.

**`internal/db/queries/logs.sql`** — 2 named queries: `BatchInsertLogs` (single-row insert; callers loop), `ListLogsByTask` (with `id > $2` for Last-Event-ID replay).

**`internal/db/sqlc/`** — 7 generated files: `db.go`, `models.go`, `users.sql.go`, `workers.sql.go`, `pipelines.sql.go`, `tasks.sql.go`, `logs.sql.go`. These files are generated artifacts committed for CI reproducibility.

### db.go implementation

**`internal/db/db.go`** — `New(ctx, dsn)` and `RunMigrations(dsn)` implemented.
- `RunMigrations` uses `embed.FS` to embed `internal/db/migrations/*.sql` at compile time (zero runtime file I/O dependency). Uses `golang-migrate/migrate/v4` with `iofs` source driver and `pgx/v5` database driver. Converts standard `postgresql://` / `postgres://` DSNs to the `pgx5://` scheme required by the driver via `toPgx5DSN`.
- `New` calls `RunMigrations` first (migrations run before the pool opens), then opens and pings a `pgxpool.Pool`.
- `errors.Is(err, migrate.ErrNoChange)` is handled — "no change" is not an error.

### cmd/api/main.go wiring

**`cmd/api/main.go`** — PostgreSQL pool now wired at startup.
`db.New(ctx, cfg.DatabaseURL)` is called with a 30-second startup context. The returned pool is passed to `api.NewServer` (replacing the prior `nil`). A startup log line confirms connection and migration status. Comment updated to reflect the TASK-002 completion state for remaining nil repositories.

### golang-migrate dependency

Added `github.com/golang-migrate/migrate/v4 v4.18.1` to `go.mod`. Version pinned to v4.18.1 because v4.19.0+ requires Go 1.24; the project module uses Go 1.23. `go mod tidy` was run via Docker to update `go.sum`.

## Unit Tests

- Tests written: 6 (in `internal/db/db_test.go`)
- All passing: yes (confirmed by `go test ./internal/db/...`)
- Key behaviors covered:
  - `toPgx5DSN` converts `postgresql://` scheme to `pgx5://` correctly
  - `toPgx5DSN` converts `postgres://` scheme to `pgx5://` correctly
  - `toPgx5DSN` returns a `pgx5://` DSN unchanged
  - `toPgx5DSN` preserves query parameters (e.g., `?sslmode=disable`)
  - `toPgx5DSN` returns an empty string unchanged
  - `toPgx5DSN` returns an unrecognised scheme (e.g., `mysql://`) unchanged

Note: `New` and `RunMigrations` require a live PostgreSQL database and are covered by the Verifier's integration tests, not by unit tests. The migration was verified manually against a fresh PostgreSQL 16 container during this session.

## Deviations from Task Description

1. **`InsertTaskStateLog` query added to tasks.sql** — The scaffold listed `UpdateTaskStatus :exec` which updates the `tasks` table. However, `TaskRepository.UpdateStatus` must also record the transition in `task_state_log` (per ADR-008 and the repository interface contract). A separate `InsertTaskStateLog :exec` query was added so TASK-005 can call both in a single transaction. This is not a deviation from the ADR — it is the correct implementation of the `UpdateStatus` contract.

2. **`BatchInsertLogs` is a single-row insert** — The scaffold spec said `:exec` for batch insert. sqlc does not support variable-length bulk insert from a Go slice natively with pgx/v5. The generated function inserts one row; the caller loops. For TASK-016, the repository implementation should use `pool.SendBatch` or `pgx.CopyFrom` for performance on large batches. The query file documents this constraint.

3. **golang-migrate pinned to v4.18.1** — The task description said "add golang-migrate"; the latest (v4.19.x) requires Go 1.24 which exceeds the module's `go 1.23` constraint. v4.18.1 is the most recent compatible version.

4. **`task_logs` UUID primary key not explicit** — The `task_logs` partitioned table does not declare `id` as a PRIMARY KEY because PostgreSQL requires the partition key (`timestamp`) to be part of any primary key on a partitioned table. `id` is declared NOT NULL with DEFAULT; the uniqueness guarantee is enforced at the application layer by using `gen_random_uuid()` in inserts. Future TASK-016 builders should be aware of this.

## Known Limitations

1. **Weekly partition creation not automated** — ADR-008 specifies weekly partitions with 30-day retention. The current migration creates only a default catch-all partition (`task_logs_default`). Weekly partition creation and partition pruning are TASK-016 concerns (background goroutine in the log sync worker). Until then, all log inserts go to `task_logs_default`, which is correct behaviour.

2. **sqlc generates `pgtype.Text` for TEXT[] columns** — The `workers.tags` and `pipeline_chains.pipeline_ids` columns are `TEXT[]` and `UUID[]` respectively. sqlc generates `pgtype.Text` or `pgtype.Array` types for these. The TASK-006 Builder implementing `WorkerRepository.Register` will need to use `pgtype.Array[pgtype.Text]` for tags and handle the generated type. The domain models use `[]string` and `[]uuid.UUID`; a conversion layer is needed in the repository implementation.

3. **`current_task_id` in `ListWorkers` returns a nullable UUID** — The correlated subquery in `workers.sql` returns `NULL` when the worker has no active task. sqlc generates a nullable type for this column. The `WorkerRepository.List` implementer must handle this nullable-to-pointer conversion.

4. **Health endpoint now requires PostgreSQL at startup** — `cmd/api/main.go` now calls `db.New` (which runs migrations) before starting the HTTP server. If PostgreSQL is not reachable at startup, the API process exits with a fatal error rather than starting in a degraded state. This is the correct production behaviour but means the Docker Compose `api` service will restart until PostgreSQL is healthy — the `depends_on: postgres: condition: service_healthy` in `docker-compose.yml` already enforces this ordering.

## For the Verifier

**Acceptance criteria verification:**

1. **Migrations apply cleanly to a fresh PostgreSQL database** — Run `docker compose up postgres -d && go run ./cmd/api` (or use `RunMigrations` in a test). The schema_migrations table will show version 1 dirty=false. Verified during this session against a fresh PostgreSQL 16 container.

2. **Down migrations roll back cleanly** — Run the down SQL directly (`psql ... < internal/db/migrations/000001_initial_schema.down.sql`) or use the golang-migrate CLI (`migrate -database 'pgx5://...' -path ./internal/db/migrations down 1`). After rollback, only `schema_migrations` table remains. Verified during this session.

3. **sqlc compile succeeds with zero errors** — Run `sqlc compile` from `internal/db/` after installing `sqlc@v1.27.0`. Verified during this session.

4. **Task state transition CHECK constraint rejects invalid transitions** — Insert a row into `task_state_log` with `from_state='completed', to_state='queued'`. The trigger should raise `ERROR: Invalid task state transition: completed -> queued`. Verified during this session.

5. **Schema matches the data model in ADR-008** — Compare the table definitions in the up migration against ADR-008's "Core data model" section. All seven entities are present with the correct fields.

**CI commands to run:**
```
go build ./...
go vet ./...
staticcheck ./...   (use v0.5.1 — v0.6+ requires Go 1.24+)
go test ./...
cd internal/db && sqlc compile
```

**Docker Compose integration note:** `docker compose up` will now apply migrations on API container startup. The first startup after TASK-002 will apply migration 000001; subsequent startups are idempotent (`ErrNoChange` is not an error).
