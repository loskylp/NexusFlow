# Builder Handoff — TASK-005
**Date:** 2026-03-26
**Task:** Task submission via REST API
**Requirement(s):** REQ-001, REQ-003, REQ-009

## What Was Implemented

### New files

**`api/handlers_tasks.go`** — Full implementation of `TaskHandler.Submit` (POST /api/tasks). Also defines the shared `writeError` helper and the `submitRequest`, `submitResponse`, and `errorResponse` types. The `List`, `Get`, and `Cancel` stubs remain as `panic("not implemented")` per their Cycle 2 deferred status.

**`internal/db/task_repository.go`** — `PgTaskRepository` implementing `db.TaskRepository`. Key design point: `UpdateStatus` runs inside a single database transaction that updates `tasks.status` and writes a row to `task_state_log` atomically, ensuring the audit trail is never partially written even under concurrent failure.

**`internal/db/pipeline_repository.go`** — `PgPipelineRepository` implementing `db.PipelineRepository`. Translates between sqlcdb-generated types and domain model types via JSON-marshal/unmarshal for the JSONB phase config columns.

**`api/handlers_tasks_test.go`** — 12 unit tests covering all 6 acceptance criteria plus additional edge cases (malformed body, missing fields, invalid UUID format, explicit retry config, user ID attachment). All tests use in-memory stubs and the `auth.Middleware` wrapper to inject authenticated sessions without requiring Redis.

### Routing

POST /api/tasks was already registered in the authenticated route group in `api/server.go` (pre-wired by the Scaffolder). No routing changes were required.

### State transition flow

The `Submit` handler performs the following sequence:

1. Validate session (401 if absent)
2. Parse and validate request body (400 on parse error, missing pipelineId, or empty tags)
3. Parse pipelineId as UUID (400 on invalid format)
4. Look up pipeline in PostgreSQL via `PipelineRepository.GetByID` (400 if not found)
5. Apply `DefaultRetryConfig()` when `retryConfig` is absent in the request
6. Insert task with `status="submitted"` and `executionId="{taskId}:0"`
7. Enqueue task to Redis Streams via `producer.Enqueue` (tags drive stream routing per ADR-001)
8. Transition task to `status="queued"` via `UpdateStatus`, which atomically writes to `task_state_log`
9. Return 201 with `{ "taskId": "...", "status": "queued" }`

## Unit Tests

- Tests written: 12
- All passing: yes
- Key behaviors covered:
  - Valid payload returns 201 with a non-nil task ID (AC-1)
  - Task persisted in repository with final status "queued" and a submitted→queued state log entry (AC-2)
  - Task message enqueued to `stubProducer` with correct tags (AC-3)
  - Non-existent pipelineId returns 400 with structured JSON error body (AC-4)
  - Absent `retryConfig` defaults to `{maxRetries: 3, backoff: "exponential"}` (AC-5)
  - No session in context returns 401 (AC-6)
  - Malformed JSON body returns 400
  - Missing `pipelineId` field returns 400
  - Empty `tags` list returns 400
  - Non-UUID pipelineId string returns 400
  - Explicit retry config is preserved (not overridden)
  - Task `UserID` comes from session, not from the request body

## Deviations from Task Description

**No `//lint:ignore U1000` directives were present** in the files I implemented. All types and functions are used.

**`errorResponse` type defined in `handlers_tasks.go`** rather than a shared file. Other handlers currently use `http.Error` (plain text) directly. Moving `errorResponse` and `writeError` to a shared utilities file was considered but rejected as YAGNI — no other handler in scope uses structured JSON errors. The Cycle 2 builder implementing `List`, `Get`, and `Cancel` should refactor this if a consistent JSON error format is desired across all handlers.

**Pipeline ownership is not enforced in `Submit`** — any authenticated user can submit a task against any pipeline (including another user's pipeline). The task-plan and scaffold docstring both note ownership enforcement is at the pipeline query level (callers verify ownership), but the scaffold's `Submit` docstring says "owned by the caller (or caller is Admin)" as a precondition. Pipeline ownership enforcement (checking `pipeline.UserID == session.UserID || session.Role == Admin`) is not listed in the TASK-005 acceptance criteria, and TASK-013 (Pipeline CRUD) is where ownership is definitively specified. This has been left as an open constraint for TASK-013 or a follow-on to clarify.

## Known Limitations

**SSE event not published** — The scaffold docstring on `Submit` notes "SSE event published to events:tasks:{userId} (fire-and-forget)" as a postcondition. The SSE `EventPublisher` interface (`Publish` in `queue.Publish`) is marked as a TASK-015 TODO (panics if called). Publishing the SSE event was intentionally skipped to avoid triggering the panic. The Verifier should note that no SSE publication occurs on task submission until TASK-015 is implemented. The `broker` field on `Server` is available for the TASK-015 Builder to wire in.

**`staticcheck@latest` incompatibility** — The CI workflow pins `staticcheck@latest` which resolves to v0.7.0 (requires Go 1.25+, but the project uses Go 1.23). This causes a panic in staticcheck on any package importing `pgx/v5` due to range-over-func in Go 1.23 stdlib. This is a pre-existing issue from TASK-001, not introduced by TASK-005. staticcheck v0.5.1 passes cleanly on all packages including the new ones.

## Iteration 2 Fixes (2026-03-26)

### Fix 1 — Wire dependencies in `cmd/api/main.go`

Three arguments to `api.NewServer(...)` were `nil`. They are now wired:

- `tasks` → `db.NewPgTaskRepository(pool)` (constructed as `taskRepo`)
- `pipelines` → `db.NewPgPipelineRepository(pool)` (constructed as `pipelineRepo`)
- `producer` → `queue.NewRedisQueue(redisClient)` (constructed as `q`)

The startup sequence comment in the package doc was updated to reflect step 6 (repositories and queue constructor).

### Fix 2 — Auth middleware JSON 401 response

`auth.Middleware` previously used `http.Error(w, "unauthorized", 401)` which writes a plain-text body. The two call sites have been replaced with `writeJSONUnauthorized(w)`, a new private helper that sets `Content-Type: application/json`, writes status 401, and encodes `{"error":"unauthorized"}`. The `Middleware` postcondition docstring was updated to name the JSON body explicitly.

### Verification

- `go build ./...` — passed (no new errors)
- `go vet ./...` — passed (clean)
- `go test ./...` — all 7 testable packages pass; no regressions

## For the Verifier

**Acceptance criteria mapping:**

| AC | How to verify |
|---|---|
| AC-1: 201 with unique task ID | `POST /api/tasks` with valid session cookie and a valid pipeline UUID; assert `201` and `taskId` in body |
| AC-2: task in PostgreSQL as submitted then queued | Query `tasks` table after POST; assert `status='queued'`; query `task_state_log` for the task ID; assert a row with `from_state='submitted', to_state='queued'` |
| AC-3: task in Redis stream | `XLEN queue:etl` (or whichever tag was used) should be 1 after a single submit; `XRANGE queue:etl - +` should show the payload |
| AC-4: 400 for invalid pipeline | POST with a UUID that does not exist in the `pipelines` table; assert `400` and `{"error":"pipeline not found"}` |
| AC-5: default retry config | POST without `retryConfig`; query the task; assert `retry_config = '{"maxRetries":3,"backoff":"exponential"}'` |
| AC-6: 401 for unauthenticated | POST without a session cookie or Authorization header; assert `401` |

**Integration note:** The acceptance tests will need a seeded pipeline record (from TASK-013, but can be inserted directly for TASK-005 verification). The task's `pipelineId` must reference a row in the `pipelines` table or the handler returns 400.

**State log trigger:** The `task_state_log` table has a CHECK constraint and trigger enforcing valid `(from_state, to_state)` transitions. If the DB trigger rejects `submitted -> queued`, the handler returns 500. Ensure the migration is applied and the trigger allows this transition.
