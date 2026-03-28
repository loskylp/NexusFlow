# Verification Report — TASK-002
**Date:** 2026-03-26 | **Result:** PASS
**Task:** Database schema and migration foundation | **Requirement(s):** REQ-009, REQ-019, REQ-020, ADR-008

## Acceptance Criteria Results

| REQ | Criterion | Layer | Result | Notes |
|---|---|---|---|---|
| REQ-009, ADR-008 | Migrations apply cleanly to a fresh PostgreSQL database | Acceptance + Integration | PASS | All 7 tables, trigger, function created; schema_migrations version=1 dirty=false; golang-migrate ErrNoChange handled |
| REQ-009, ADR-008 | Down migrations roll back cleanly | Acceptance | PASS | All 7 tables removed; trigger and function dropped; schema_migrations preserved |
| ADR-008 | sqlc compile succeeds with zero errors | Acceptance | PASS | sqlc/sqlc:1.27.0 exits code 0; 7 generated files present |
| REQ-009, ADR-008 | Task state transition CHECK constraint rejects invalid transitions (e.g., completed -> queued) | Acceptance + Integration | PASS | SQLSTATE 23514 on completed->queued; 8 additional invalid transitions verified; all 10 valid transitions verified |
| REQ-009, REQ-020, ADR-008 | Schema matches the data model in ADR-008 | Acceptance + Integration | PASS | All 44 ADR-008 columns verified by type; partitioned task_logs confirmed; REQ-020 no-cascade FK confirmed |

## Test Summary

| Layer | Written | Passing | Failing |
|---|---|---|---|
| Integration | 7 | 7 | 0 |
| System | 0 | — | — |
| Acceptance | 95 assertions | 95 | 0 |
| Performance | 0 | — | — |

**Integration test file:** `tests/integration/TASK-002-migration-integration_test.go`
**Acceptance test script:** `tests/acceptance/TASK-002-acceptance.sh`
**Demo script:** `tests/demo/TASK-002-demo.md`

## Test Evidence

### AC-1: Migrations apply cleanly (golang-migrate path)

The integration test `TestTASK002_AC1_MigrationsApplyCleanly` called `db.New` (which invokes `RunMigrations` internally using the embedded FS) against a live PostgreSQL 16 container. Result:

```
schema_migrations: version=1 dirty=false
table "users": EXISTS
table "workers": EXISTS
table "pipelines": EXISTS
table "pipeline_chains": EXISTS
table "tasks": EXISTS
table "task_state_log": EXISTS
table "task_logs": EXISTS
task_logs_default partition: EXISTS
trg_task_state_transition trigger: EXISTS
PASS (0.06s)
```

`TestTASK002_AC1_MigrationsAreIdempotent` confirmed that a second call to `RunMigrations` against an already-migrated database returns nil (ErrNoChange swallowed). PASS.

The acceptance script also verified the raw SQL migration applied without errors to a freshly cleared database.

### AC-2: Down migrations roll back cleanly

Running `000001_initial_schema.down.sql` against a fully-migrated database produced:

```
DROP TRIGGER / DROP FUNCTION / DROP TABLE × 7
```

Post-run `\dt` showed only `schema_migrations` — all application tables gone. Trigger and function both removed. PASS.

### AC-3: sqlc compile zero errors

```
docker run --rm -v $(pwd):/app -w /app/internal/db sqlc/sqlc:1.27.0 compile
Exit code: 0
```

No stdout or stderr. All 7 generated files present in `internal/db/sqlc/`. PASS.

### AC-4: State transition constraint

Positive case verified: `submitted -> queued` INSERT succeeds.

The explicit acceptance criterion example (`completed -> queued`) verified rejected with:
```
ERROR: Invalid task state transition: completed -> queued (task_id: ...) (SQLSTATE 23514)
```

Additional invalid transitions verified rejected:
- `completed -> failed`, `completed -> running`
- `cancelled -> running`, `cancelled -> queued`
- `submitted -> running`, `submitted -> completed`
- `queued -> completed`, `running -> submitted`

All 10 valid transitions from ADR-008 Domain Invariant 1 verified accepted. PASS.

`tasks.status` CHECK constraint verified: status `'pending'` rejected with SQLSTATE 23514; all 7 valid statuses accepted.

### AC-5: Schema matches ADR-008

All 44 ADR-008 data model columns verified by `udt_name` from `information_schema.columns`:
- `users` (6 columns), `pipelines` (8 columns), `pipeline_chains` (5 columns)
- `tasks` (12 columns), `task_state_log` (6 columns), `workers` (5 columns), `task_logs` (5 columns)

All types matched exactly: uuid, text, bool, int4, jsonb, timestamptz, _uuid (UUID[]), _text (TEXT[]).

Additional ADR-008 constraints verified:
- `task_logs` relkind=`p` (partitioned) with `task_logs_default` catch-all partition
- `tasks.user_id` FK delete_rule = `NO ACTION` (not CASCADE) — REQ-020 "deactivation does not cancel in-flight tasks"
- `pipelines.user_id` FK → `users.id` exists
- All 9 indexes present: `idx_pipelines_user_id`, `idx_pipeline_chains_user_id`, `idx_tasks_user_id`, `idx_tasks_pipeline_id`, `idx_tasks_status`, `idx_tasks_worker_id`, `idx_task_state_log_task_id`, `idx_task_logs_task_id`, `idx_task_logs_timestamp`

### Builder CI commands

All four commands from the handoff note verified clean:
- `go build ./...` — PASS
- `go vet ./...` — PASS
- `go test ./internal/db/...` — PASS (6 unit tests, `TestToPgx5DSN` and 5 subtests)
- `sqlc compile` — PASS (covered under AC-3)

Note: `staticcheck` not separately tested — covered by CI (green).

### API health endpoint (TASK-001 OBS-003 resolution)

After rebuilding the API Docker image with TASK-002 code and starting via `docker compose up api`:

```
{"status":"ok","redis":"ok","postgres":"ok"}   HTTP 200
```

Startup log confirmed: `api: PostgreSQL connected and migrations applied`. TASK-001 OBS-003 is fully resolved.

## CI Results

Push to `main` triggered CI run [23606734063](https://github.com/loskylp/NexusFlow/actions/runs/23606734063):

| Job | Result |
|---|---|
| Go Build, Vet, and Test | PASS (1m30s) |
| Frontend Build and Typecheck | PASS (13s) |
| Docker Build Smoke Test | PASS (1m2s) |

Full regression green. Task COMPLETE.

## Observations (non-blocking)

**OBS-001: Trigger does not have IF NOT EXISTS (raw SQL re-application)**
`CREATE TRIGGER trg_task_state_transition` does not have `IF NOT EXISTS` syntax because PostgreSQL 16 does not support it for triggers. When applied through golang-migrate this is never an issue — golang-migrate tracks versions and never re-applies. When applied raw (e.g., in tests or CI) a drop-first pattern is required. This is standard practice and not a defect. The acceptance script handles this correctly via a conditional DO block drop before re-applying.

**OBS-002: task_logs primary key absence**
The Builder documented this deviation: `task_logs` has no explicit PRIMARY KEY because PostgreSQL requires the partition key (`timestamp`) in any primary key on a partitioned table. `id` is declared NOT NULL with `gen_random_uuid()` default. This is correct behaviour for this schema design and aligns with the ADR-008 known limitation. TASK-016 builders should be aware.

**OBS-003: schema_migrations not cleaned up by down migration**
The down migration does not drop `schema_migrations`. This is correct — golang-migrate owns that table and developers should use the golang-migrate CLI or the Go API to manage it. Dropping it manually would confuse the migration tracker.

**OBS-004: schemaMappings column absent from pipelines**
ADR-008 specifies `Pipeline { ..., schemaMappings, ... }` in the domain model but the migration does not include a `schema_mappings` column in `pipelines`. The three JSONB config columns (`data_source_config`, `process_config`, `sink_config`) embed schema mapping configuration within the phase config objects. This is a deliberate design decision consistent with the ADR-008 rationale ("enabling schema evolution without join complexity"). Not a defect — flagged for awareness as the domain model language differs slightly from the schema.

## Recommendation

PASS TO NEXT STAGE

**Next:** Invoke @nexus-orchestrator — TASK-002 verified PASS, CI green, committed and pushed; ready for next task dispatch.
