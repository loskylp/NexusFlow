# Builder Handoff — TASK-025
**Date:** 2026-03-27
**Task:** Worker fleet status API
**Requirement(s):** REQ-016

## What Was Implemented

`api/handlers_workers.go` — replaced the scaffold stub's `panic("not implemented")` with a complete `WorkerHandler.List` implementation.

The handler:
- Reads the session from context via `auth.SessionFromContext`; returns 401 when absent
- Calls `h.server.workers.List(ctx)` which returns all workers with `CurrentTaskID` populated from a subquery on the tasks table (implemented in TASK-006 via `internal/db/worker_repository.go` and `internal/db/sqlc/workers.sql.go`)
- Projects each `*models.Worker` to a `workerListItem` response struct that serialises to `{ id, status, tags, currentTaskId, lastHeartbeat }`
- Initialises the response slice as `make([]workerListItem, 0, ...)` so an empty registry returns `[]` not `null`
- Returns 500 on repository errors with the error logged server-side and a generic message sent to the client

A new `workerListItem` type was introduced in the handler file; it is the API-facing projection of `models.Worker` with only the fields required by AC-2.

**No changes were required to:**
- `api/server.go` — the route `GET /api/workers -> WorkerHandler.List` was already wired in the authenticated group
- `cmd/api/main.go` — `workers` was already wired as a dependency
- `internal/db/worker_repository.go` — `List` with `CurrentTaskID` populated was already complete
- `internal/db/sqlc/workers.sql.go` — `ListWorkers` query with the `current_task_id` subquery was already generated

`api/handlers_workers_test.go` — new test file with 6 unit tests.

## Unit Tests

- Tests written: 6
- All passing: yes
- Key behaviors covered:
  - Authenticated request returns 200 with all workers in the database
  - Each worker response includes id, status (string), tags (array), currentTaskId (UUID pointer, nullable), lastHeartbeat (timestamp)
  - Worker with an active task has a non-null `currentTaskId`; idle worker has `currentTaskId: null`
  - Empty registry returns `[]` (not `null`)
  - Request with no session in context returns 401
  - Repository error returns 500 without leaking the error message
  - User role (non-admin) sees all workers (Domain Invariant 5)
  - Response carries `Content-Type: application/json`

## Deviations from Task Description

None. The task required implementing `WorkerHandler.List`; all three acceptance criteria are satisfied by the implementation as described above.

## Known Limitations

None. The `CurrentTaskID` is populated from the existing `ListWorkers` SQL query which selects the most recent `assigned` or `running` task for each worker. If a worker has multiple active tasks (which cannot happen given Domain Invariant 3 — one task per worker at a time), the query returns only the most recent one. This is correct behaviour.

## For the Verifier

- AC-1: `GET /api/workers` with a valid session token must return 200 with a JSON array. An integration test should seed the workers table and verify the response length and content.
- AC-2: Each element must include `id` (string), `status` ("online" or "down"), `tags` (array of strings), `currentTaskId` (UUID string or `null`), `lastHeartbeat` (ISO-8601 timestamp).
- AC-3: `GET /api/workers` without a session token (no `Authorization` header, or invalid token) must return 401.
- The `currentTaskId` field is populated by the existing `ListWorkers` SQL subquery; an integration test that seeds a task with status `assigned` or `running` on a worker should verify the field is non-null for that worker.
- All existing unit tests continue to pass (`go test ./...` green).
