---
task: TASK-002
title: Database schema and migration foundation
date: 2026-03-26
requirements: REQ-009, REQ-019, REQ-020, ADR-008
status: PASS
smoke: false
---

# Demo Script — TASK-002: Database schema and migration foundation

**Environment:** Local development via Docker Compose
**Prerequisites:** Docker running; `docker compose up postgres` started; project cloned to local disk

---

## Scenario 1 — Migrations apply cleanly to a fresh PostgreSQL database

**Traces to:** AC-1 | REQ-009, ADR-008

| Step | Action | Expected Result |
| --- | --- | --- |
| Given | A fresh PostgreSQL 16 container is running (no application schema) | `docker compose up postgres -d` returns; `pg_isready -U nexusflow` accepts connections |
| When | The up migration is applied: run `docker compose exec postgres psql -U nexusflow -d nexusflow -f /dev/stdin < internal/db/migrations/000001_initial_schema.up.sql` | psql outputs CREATE TABLE × 7, CREATE INDEX × 9, CREATE FUNCTION × 1, CREATE TRIGGER × 1 with no ERROR lines |
| Then | All 7 tables exist in the schema | `\dt` in psql shows: users, workers, pipelines, pipeline_chains, tasks, task_state_log, task_logs (partitioned), task_logs_default |
| And | The state transition trigger is active | `\df` shows `enforce_task_state_transition`; `SELECT tgname FROM pg_trigger WHERE tgrelid='task_state_log'::regclass` returns `trg_task_state_transition` |

---

## Scenario 2 — Down migrations roll back cleanly

**Traces to:** AC-2 | REQ-009, ADR-008

| Step | Action | Expected Result |
| --- | --- | --- |
| Given | Migration 000001 has been applied (all 7 tables exist) | `\dt` shows all application tables |
| When | The down migration is applied: `docker compose exec postgres psql -U nexusflow -d nexusflow -f /dev/stdin < internal/db/migrations/000001_initial_schema.down.sql` | psql outputs DROP TRIGGER, DROP FUNCTION, DROP TABLE × 7 with no ERROR lines |
| Then | All application tables are gone | `\dt` shows only `schema_migrations` — no application tables remain |
| And | The trigger and function are removed | `\df` returns no rows; `SELECT COUNT(*) FROM pg_trigger WHERE tgname='trg_task_state_transition'` returns 0 |
| And | schema_migrations remains | `SELECT * FROM schema_migrations;` succeeds (table exists for migration tracking) |

---

## Scenario 3 — sqlc compile succeeds with zero errors

**Traces to:** AC-3 | ADR-008

| Step | Action | Expected Result |
| --- | --- | --- |
| Given | `internal/db/sqlc.yaml` and all `.sql` query files in `internal/db/queries/` are present | Files are visible in the working tree |
| When | sqlc compile is run: `docker run --rm -v $(pwd):/app -w /app/internal/db sqlc/sqlc:1.27.0 compile` | Command exits with code 0; no output is produced (empty stdout/stderr means zero errors) |
| Then | All 7 generated files exist in `internal/db/sqlc/` | `ls internal/db/sqlc/` shows: db.go, models.go, users.sql.go, workers.sql.go, pipelines.sql.go, tasks.sql.go, logs.sql.go |

---

## Scenario 4 — State transition constraint rejects an invalid transition (completed → queued)

**Traces to:** AC-4 | REQ-009, ADR-008 Domain Invariant 1

| Step | Action | Expected Result |
| --- | --- | --- |
| Given | A task record exists in the database with any status | Insert via: `INSERT INTO tasks (pipeline_id, user_id, status, execution_id) VALUES (...)` with status='completed' |
| When | An invalid state transition is attempted: `INSERT INTO task_state_log (task_id, from_state, to_state) VALUES ('<task-id>', 'completed', 'queued')` | PostgreSQL raises: `ERROR: Invalid task state transition: completed -> queued (task_id: ...) (SQLSTATE 23514)` |
| Then | The row is NOT inserted into task_state_log | `SELECT COUNT(*) FROM task_state_log WHERE from_state='completed' AND to_state='queued'` returns 0 |
| And | A valid transition succeeds: `INSERT INTO task_state_log (...) VALUES ('<task-id>', 'submitted', 'queued')` | INSERT succeeds; row appears in task_state_log |

---

## Scenario 5 — Schema matches the ADR-008 data model

**Traces to:** AC-5 | REQ-009, REQ-020, ADR-008

| Step | Action | Expected Result |
| --- | --- | --- |
| Given | Migration 000001 has been applied | All tables present |
| When | Each entity's columns are inspected via `\d <table>` in psql | |
| Then (users) | `\d users` | Columns: id uuid, username text UNIQUE, password_hash text, role text CHECK IN ('admin','user'), active bool DEFAULT true, created_at timestamptz |
| Then (tasks) | `\d tasks` | Columns: id uuid, pipeline_id uuid FK, chain_id uuid nullable FK, user_id uuid FK, status text CHECK IN 7 values, retry_config jsonb, retry_count int4, execution_id text, worker_id text nullable FK, input jsonb, created_at timestamptz, updated_at timestamptz |
| Then (task_logs) | `\d task_logs` | Partitioned table (PARTITION BY RANGE timestamp); default partition task_logs_default exists |
| And (REQ-020) | `\d+ tasks` — check delete rule on user_id FK | FK tasks.user_id → users.id has NO ACTION delete rule (not CASCADE); deactivating a user does NOT cancel in-flight tasks |
| And | Run `docker run --rm -v $(pwd):/app -w /app golang:1.23-alpine go build ./...` | Exits with code 0 — generated sqlc code compiles correctly with the schema |

---

## Scenario 6 — API health endpoint returns 200 with postgres connected (TASK-001 OBS-003)

**Traces to:** AC-1 (integration via application startup) | ADR-008, ADR-005

| Step | Action | Expected Result |
| --- | --- | --- |
| Given | PostgreSQL and Redis containers are running | `docker compose up postgres redis -d` |
| When | The API container is started: `docker compose up api -d` | Container starts; logs show: `api: PostgreSQL connected and migrations applied` |
| Then | Health endpoint returns 200 | `curl http://localhost:8080/api/health` returns HTTP 200 with body: `{"status":"ok","redis":"ok","postgres":"ok"}` |
| And | Migrations ran on startup | `docker compose exec postgres psql -U nexusflow -d nexusflow -c "SELECT version, dirty FROM schema_migrations;"` returns version=1, dirty=f |
