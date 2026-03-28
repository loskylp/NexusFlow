# Builder Handoff — TASK-008
**Date:** 2026-03-27
**Task:** Task lifecycle state tracking and query API
**Requirement(s):** REQ-009, REQ-017

## What Was Implemented

`api/handlers_tasks.go` — two previously-stubbed handlers replaced with full implementations:

- `TaskHandler.List` — handles `GET /api/tasks`. Reads the session role: User role calls `TaskRepository.ListByUser(session.UserID)`; Admin role calls `TaskRepository.List()`. An optional `?status=<value>` query parameter filters the result set to tasks with that exact status. Always returns a JSON array (never null). Added `filterByStatus` as an extracted private function (Single Responsibility — keep branching logic flat).

- `TaskHandler.Get` — handles `GET /api/tasks/{id}`. Parses the `{id}` URL parameter as a UUID (400 on invalid format). Fetches the task via `TaskRepository.GetByID` (404 if not found). Enforces ownership: non-Admin callers receive 403 if `task.UserID != session.UserID`. On 200: returns `taskDetailResponse{task, stateHistory}` where `stateHistory` is populated from `TaskRepository.GetStateLog` in chronological order.

- `taskDetailResponse` — new response type wrapping `*models.Task` and `[]*models.TaskStateLog`.

No changes were required to `api/server.go` (routes already registered), `cmd/api/main.go` (TaskRepository already wired), `internal/db/task_repository.go` (all needed methods already implemented — `GetByID`, `ListByUser`, `List`, `GetStateLog`), or `internal/db/sqlc/` (all SQL queries already generated).

`api/handlers_tasks_test.go` — test file updated:

- Added `chi` import (needed for `chi.NewRouteContext()` in test helpers).
- Upgraded `stubTaskRepo.ListByUser` and `stubTaskRepo.List` stubs from returning `nil` to returning filtered/full slices from the in-memory map (previously unused stubs that had to become functional for the new tests).
- Added `taskRequest` helper for building authenticated `GET` requests.
- Added `taskGetRequest` helper for building authenticated `GET /api/tasks/{id}` requests with chi URL parameters wired.
- Added 10 new unit tests (see Unit Tests section).

## Unit Tests

- Tests written: 10
- All passing: yes
- Key behaviors covered:
  - Unauthenticated `GET /api/tasks` returns 401
  - User role receives only own tasks (visibility isolation, Domain Invariant 5)
  - Admin role receives all tasks across all users
  - `?status=running` filter returns only matching tasks; non-matching tasks are excluded
  - Unauthenticated `GET /api/tasks/{id}` returns 401
  - Non-existent task ID returns 404
  - Task owner receives 200 with task details and non-empty `stateHistory`
  - Non-owner non-admin caller receives 403 (no task data disclosed)
  - Admin caller receives 200 regardless of task ownership
  - Non-UUID path segment returns 400 before any DB lookup

## Deviations from Task Description

The task description mentions:

> GET /api/tasks?status=running filters to running tasks only

The filter is implemented as a general `?status=<value>` parameter (not hard-coded to "running"). The filter applies any valid TaskStatus string. This is strictly more capable than the stated criterion and does not change the behaviour expected by any acceptance criterion — a `?status=running` request continues to return only running tasks.

The task description also listed:

> Non-owner non-admin gets 403 on GET /api/tasks/{id}

This is implemented. For `GET /api/tasks` (list), the task plan states "User-A cannot see User-B's tasks (returns empty list, not 403)" — that criterion is satisfied by the `ListByUser` path returning only the caller's tasks, which naturally excludes tasks from other users.

## Known Limitations

- The `?status=` filter is applied in application memory after fetching from the database. For large task sets a dedicated SQL query with a `WHERE status = ?` clause would be more efficient. This is a performance concern only; correctness is not affected. A future optimisation task can add the filtered query to the sqlc layer.
- No pagination is implemented on `GET /api/tasks`. This is out of scope for TASK-008 and consistent with the acceptance criteria.

## For the Verifier

- Route registration was already in place from TASK-005 — the Verifier should confirm the routes resolve correctly via the chi router at integration test time.
- `cmd/api/main.go` already wires `taskRepo` to `NewServer` — no nil dependency risk.
- The `stateHistory` array in the `GET /api/tasks/{id}` response is always a JSON array (never `null`), even when there are no entries.
- Acceptance criterion "User-A cannot see User-B's tasks (returns empty list, not 403)" is satisfied by the List handler returning only the caller's own tasks via `ListByUser`. The list endpoint does not return 403 — it returns an empty array when the user has no tasks.
