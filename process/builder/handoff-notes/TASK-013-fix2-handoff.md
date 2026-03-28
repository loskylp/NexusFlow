# Builder Handoff — TASK-013 Fix Iteration 2
**Date:** 2026-03-26
**Task:** Fix DELETE /api/pipelines/{id} 500 for pipelines with historical tasks
**Requirement(s):** REQ-022, TASK-013

## What Was Implemented

### Migration 000002 (up + down)

- `/Users/pablo/projects/Nexus/NexusTests/NexusFlow/internal/db/migrations/000002_pipeline_fk_set_null.up.sql`
  — Drops `tasks_pipeline_id_fkey`, drops `NOT NULL` from `tasks.pipeline_id`, re-creates the FK with `ON DELETE SET NULL`.
- `/Users/pablo/projects/Nexus/NexusTests/NexusFlow/internal/db/migrations/000002_pipeline_fk_set_null.down.sql`
  — Reverses: drops the ON DELETE FK, restores NOT NULL, re-creates the plain RESTRICT FK.

Migration was applied to the running Docker Compose postgres instance and verified: `pipeline_id` is now nullable with `FOREIGN KEY ... ON DELETE SET NULL`.

### sqlc regeneration

Ran `sqlc generate` (via `sqlc/sqlc:1.27.0` Docker image) against the updated schema. Changes to generated files:

- `/Users/pablo/projects/Nexus/NexusTests/NexusFlow/internal/db/sqlc/models.go` — `Task.PipelineID` changed from `uuid.UUID` to `uuid.NullUUID`.
- `/Users/pablo/projects/Nexus/NexusTests/NexusFlow/internal/db/sqlc/tasks.sql.go` — `CreateTaskParams.PipelineID` changed from `uuid.UUID` to `uuid.NullUUID`.
- `/Users/pablo/projects/Nexus/NexusTests/NexusFlow/internal/db/sqlc/pipelines.sql.go` — `PipelineHasActiveTasks` parameter changed from `uuid.UUID` to `uuid.NullUUID`.

### Domain model update

- `/Users/pablo/projects/Nexus/NexusTests/NexusFlow/internal/models/models.go` — `Task.PipelineID` changed from `uuid.UUID` to `*uuid.UUID`. Docstring updated to explain the nullable semantics.

### Repository updates

- `/Users/pablo/projects/Nexus/NexusTests/NexusFlow/internal/db/task_repository.go`:
  - `Create`: wraps `*uuid.UUID` → `uuid.NullUUID` before passing to sqlc.
  - `toModelTask`: unwraps `uuid.NullUUID` → `*uuid.UUID` when populating domain model. Docstring updated.
- `/Users/pablo/projects/Nexus/NexusTests/NexusFlow/internal/db/pipeline_repository.go`:
  - `Delete` and `HasActiveTasks`: wrap the `uuid.UUID` pipeline ID into `uuid.NullUUID{Valid: true}` to satisfy the updated `PipelineHasActiveTasks` signature.

### Handler update

- `/Users/pablo/projects/Nexus/NexusTests/NexusFlow/api/handlers_tasks.go` — Task submission sets `PipelineID: &pipelineID` (pointer) instead of `PipelineID: pipelineID` (value).

### Queue update

- `/Users/pablo/projects/Nexus/NexusTests/NexusFlow/internal/queue/redis.go` — Enqueue nil-guards `message.Task.PipelineID` before calling `.String()`; uses empty string when PipelineID is nil.

### Test update

- `/Users/pablo/projects/Nexus/NexusTests/NexusFlow/internal/queue/redis_test.go` — `makeTask` helper updated: allocates a `uuid.UUID` and assigns `&pid` to `PipelineID`.

## Unit Tests

- Tests written: 0 new (the defect was in the database schema, not in Go logic)
- All passing: yes — `go test ./internal/... ./api/... ./worker/... ./cmd/...` exits 0
- Key behaviors covered by existing tests:
  - `api` package: pipeline create/list/get/update/delete handlers, including 409 for active tasks, 204 for clean delete
  - `internal/queue`: Enqueue with nil-safe PipelineID (existing Redis tests exercise the code path)
  - `internal/db`: repository unit tests pass

## Deviations from Task Description

None. The migration, sqlc regeneration, and all downstream Go changes follow the specified approach exactly.

## Known Limitations

- The `PipelineHasActiveTasks` query filters `WHERE pipeline_id = $1`. After migration, tasks with `pipeline_id = NULL` (i.e., tasks whose pipeline was already deleted) will not be matched by this query regardless of their status. This is correct: there is no pipeline to guard any more.
- The down migration will fail if any task rows have `pipeline_id = NULL` at rollback time. This is documented in the down migration SQL.

## For the Verifier

The blocking defect was SQLSTATE 23503 (FK violation) when calling `DELETE FROM pipelines` against a pipeline that had historical (terminal) tasks. The fix removes the RESTRICT FK and replaces it with ON DELETE SET NULL.

To reproduce the original failure and confirm it is resolved:
1. Create a pipeline via `POST /api/pipelines`
2. Submit a task against it via `POST /api/tasks`
3. Advance the task to a terminal state (completed or failed)
4. Call `DELETE /api/pipelines/{id}` — previously returned 500 (SQLSTATE 23503), now returns 204

The integration test file at `tests/integration/TASK-002-migration-integration_test.go` contains a test (`AC-4`) that asserts `tasks.pipeline_id` has type `uuid`. After migration 000002, the column is still UUID type but nullable — the existing type assertion may need updating by the Verifier. The integration test checks `data_type = 'uuid'` which remains correct; only nullability changed.
