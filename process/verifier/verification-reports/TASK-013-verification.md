# Verification Report — TASK-013
**Date:** 2026-03-26 | **Result:** PASS
**Task:** Pipeline CRUD via REST API | **Requirement(s):** REQ-022
**Iteration:** 2 (re-verification after Builder fix)

## Acceptance Criteria Results

| REQ | Criterion | Layer | Result | Notes |
|---|---|---|---|---|
| REQ-022 | POST /api/pipelines creates a pipeline with DataSource, Process, Sink config; returns 201 | Acceptance | PASS | userId set from session, all fields present, row persisted in DB |
| REQ-022 | GET /api/pipelines returns user's own pipelines (User) or all pipelines (Admin) | Acceptance | PASS | Admin sees both users' pipelines; non-admin sees only own; isolation confirmed |
| REQ-022 | GET /api/pipelines/{id} returns pipeline details | Acceptance | PASS | 200 with all phase configs, 404 on missing, 400 on bad UUID |
| REQ-022 | PUT /api/pipelines/{id} updates pipeline config; returns 200 | Acceptance | PASS | Name updated in response and in DB; userId preserved from original record |
| REQ-022 | DELETE /api/pipelines/{id} deletes pipeline if no active tasks; returns 204 | Acceptance | PASS | 204 returned for both zero-task and terminal-task-only pipelines; FK SET NULL orphans historical rows cleanly |
| REQ-022 | DELETE /api/pipelines/{id} returns 409 if active tasks exist | Acceptance | PASS | 409 returned and pipeline row unchanged when status='running' task references it; 204 after task reaches terminal status |
| REQ-022 | Non-owner non-admin operations return 403 | Acceptance | PASS | GET, PUT, DELETE all return 403 for non-owner; admin bypasses correctly on all three verbs |

## Test Summary

| Layer | Written | Passing | Failing |
|---|---|---|---|
| Integration | 0 | 0 | 0 |
| System | 0 | 0 | 0 |
| Acceptance | 36 | 36 | 0 |
| Performance | 0 | 0 | 0 |

Note: No separate integration or system test files were written. All seven acceptance criteria are exercised end-to-end through the public HTTP interface using `curl` against the running Docker Compose stack, which constitutes system-level verification. The pipeline handler has no component seams that require a separate integration test layer beyond what the unit tests (all passing) already cover at the function boundary.

## Unit Test Results (Builder-provided)

Run via `docker run --rm -v <project>:/app -w /app golang:1.23-alpine go test ./api/... ./internal/db/... -count=1`:

- `api` package — all PASS
- `internal/db` package — all PASS
- `go build ./...` — clean
- `go vet ./...` — clean

## Migration Verification

Migration 000002 (`internal/db/migrations/000002_pipeline_fk_set_null.up.sql`) was confirmed applied:

- `schema_migrations` table: version=2, dirty=false
- `tasks.pipeline_id` column: is_nullable=YES (was NOT NULL in 000001)
- `tasks_pipeline_id_fkey` constraint: confdeltype=n (ON DELETE SET NULL)

Rollback cycle tested manually:
- Down migration applied cleanly (confdeltype=a, nullable=NO restored)
- Up migration re-applied cleanly (confdeltype=n, nullable=YES restored)
- WARNING in down migration is correctly documented: rollback fails if any task row has NULL pipeline_id (expected; must resolve orphaned tasks before reverting)

## Regression Check: TASK-005

`POST /api/tasks` tested after the nullable pipeline_id change:

- HTTP 201 returned with status "queued"
- `pipeline_id` stored correctly in the tasks table (non-null UUID matching submitted pipeline)
- `uuid.NullUUID` handling in `task_repository.go` correctly marshals *uuid.UUID to/from the nullable column
- No regression introduced

## Observations (non-blocking)

**OBS-001 (carried from iteration 1): Pipeline ownership not enforced at task submission.** `TaskHandler.Submit` does not verify that the submitting user owns the referenced pipeline. A user can submit a task referencing another user's pipeline. Cross-task concern; flagged for Planner awareness.

**OBS-002 (carried from iteration 1): Empty list returns `[]` not `null`.** Correct behaviour; consistent with REST API conventions.

**OBS-003 (carried from iteration 1): No empty-name check on PUT.** `Create` rejects empty name (400); `Update` does not. Minor inconsistency deferred to TASK-026 (Cycle 2).

**OBS-004: Down migration has a documented pre-condition.** The `000002_pipeline_fk_set_null.down.sql` file correctly documents that rollback will fail if any task rows have `pipeline_id = NULL`. This is expected and acceptable — the Builder has annotated the file with a WARNING comment. Operators must nullify or reassign orphaned tasks before running a rollback. No action required for current cycle.

## Recommendation

PASS TO NEXT STAGE.

All 36 acceptance tests pass (36/36). The previously failing criterion AC-5 is now fully resolved: DELETE returns 204 for pipelines whose only historical tasks are in terminal states, and the FK `ON DELETE SET NULL` correctly orphans those task rows without blocking the delete. AC-6 (409 guard) remains intact. TASK-005 regression check passes. Migration 000002 applies and rolls back cleanly.
