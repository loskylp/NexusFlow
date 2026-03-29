# Builder Handoff — TASK-010
**Date:** 2026-03-29
**Task:** Infrastructure retry with backoff
**Requirement(s):** REQ-011

## What Was Implemented

### New files

- `/Users/pablo/projects/Nexus/NexusTests/NexusFlow/monitor/backoff.go` — pure function `computeBackoffDelay(strategy, retryCount)` implementing exponential (1s×2^n), linear (1s×(n+1)), and fixed (1s) strategies. Falls back to fixed on unknown strategy (fail-safe).
- `/Users/pablo/projects/Nexus/NexusTests/NexusFlow/monitor/backoff_test.go` — 5 unit tests covering all three strategies, unknown-strategy fallback, and the postcondition that delay is always positive.
- `/Users/pablo/projects/Nexus/NexusTests/NexusFlow/internal/db/migrations/000005_retry_after.up.sql` — adds `retry_after TIMESTAMPTZ` and `retry_tags TEXT[]` columns to the tasks table with a partial index on `(retry_after) WHERE status='queued' AND retry_after IS NOT NULL`.
- `/Users/pablo/projects/Nexus/NexusTests/NexusFlow/internal/db/migrations/000005_retry_after.down.sql` — drops the columns and index.

### Modified files

**Domain model** (`internal/models/models.go`):
- Added `RetryAfter *time.Time` and `RetryTags []string` fields to `Task`. `RetryAfter` gates re-enqueue; `RetryTags` records which stream(s) to target when the gate opens.

**Repository interface** (`internal/db/repository.go`):
- Added `SetRetryAfterAndTags(ctx, id, retryAfter, retryTags) error` — sets both columns atomically.
- Added `ListRetryReady(ctx) ([]*models.Task, error)` — returns tasks with `status=queued AND retry_after <= now()`.

**sqlc generated models** (`internal/db/sqlc/models.go`):
- Added `RetryAfter pgtype.Timestamptz` and `RetryTags []string` to the sqlc `Task` struct.

**sqlc generated queries** (`internal/db/sqlc/tasks.sql.go`):
- Updated all SELECT statements to include `retry_after, retry_tags` (CreateTask RETURNING, GetTaskByID, ListAllTasks, ListTasksByUser).
- Updated all scan calls to match.
- Added `SetTaskRetryAfterAndTags` query and `ListRetryReadyTasks` query.

**PgTaskRepository** (`internal/db/task_repository.go`):
- Implemented `SetRetryAfterAndTags` and `ListRetryReady`.
- Updated `toModelTask` to populate `RetryAfter` and `RetryTags` from the DB row.

**Monitor** (`monitor/monitor.go`):
- Updated package comment to describe the three-loop architecture.
- `reclaimTask`: rewrote to implement deferred re-enqueue. Sequence: XCLAIM → GetByID (read pre-increment RetryCount for backoff) → IncrementRetryCount → SetRetryAfterAndTags → UpdateStatus("queued") → AcknowledgePending. No immediate `producer.Enqueue`.
- `processEntry`: added skip guard — if `task.RetryAfter != nil && task.RetryAfter.After(time.Now())`, skip the pending entry (backoff not elapsed).
- Added `scanRetryReady(ctx)` — queries `ListRetryReady`, re-enqueues each task via `producer.Enqueue`, then clears `retry_after` via `SetRetryAfterAndTags(nil)`.
- Added `dispatchRetryReadyTask(ctx, task)` — extracted from `scanRetryReady` (Single Responsibility).
- `Run`: added `scanRetryReady` call on each tick alongside `checkHeartbeats` and `scanPendingEntries`.

**Monitor tests** (`monitor/monitor_test.go`):
- Updated `fakeTaskRepository` struct to add `retryAfters map[uuid.UUID]*time.Time`.
- Implemented `SetRetryAfterAndTags` and `ListRetryReady` on `fakeTaskRepository`.
- Updated `TestReclaimTask_ReEnqueuesForHealthyWorker` → renamed `TestReclaimTask_DeferredEnqueueViaRetryAfter` to reflect the new deferred behavior.

**Stub fakes in other test files** (`api/handlers_tasks_test.go`, `worker/executor_test.go`):
- Added noop `SetRetryAfterAndTags` and `ListRetryReady` methods to satisfy the updated `TaskRepository` interface.

## Unit Tests

- Tests written: 13 new (5 backoff + 8 AC coverage)
- Original tests retained: 13 (1 updated to match new deferred-enqueue behavior)
- All passing: yes — `go test ./... -count=1` green across all packages
- `go vet ./...` clean

Key behaviors covered:
- Exponential backoff: retryCount=0 → 1s, retryCount=1 → 2s, retryCount=2 → 4s (AC-2)
- Linear backoff: retryCount=0 → 1s, retryCount=1 → 2s, retryCount=2 → 3s
- Fixed backoff: always 1s regardless of retryCount
- Unknown strategy falls back to 1s (fail-safe, never zero)
- `reclaimTask` sets `retry_after` and `retry_tags`, does NOT immediately re-enqueue (AC-2)
- `processEntry` skips tasks with future `retry_after` (AC-2)
- `scanRetryReady` re-enqueues tasks with elapsed `retry_after` (AC-2)
- `processEntry` routes to reclaim when `RetryCount < MaxRetries=3` (AC-1)
- `processEntry` dead-letters when `RetryCount >= MaxRetries=3` (AC-4)
- `deadLetterTask` sets status "failed" and calls `EnqueueDeadLetter` (AC-4)
- `reclaimTask` increments `RetryCount`; visible in task state via `GetByID` (AC-5)

## Deviations from Task Description

**AC-3 (Process errors do not retry):** The task description says to verify process errors do not enter the retry path. This is already correctly implemented in `worker/worker.go` via `isDomainError()` and `domainErrorWrapper`: domain errors (connector failures, schema errors) are XACK'd by the worker directly — the message leaves the pending list and the Monitor never sees it. No Monitor-side code change was needed for AC-3. A unit test for this AC was not added to the monitor package because the Monitor's role is only infrastructure retry; the worker test suite already covers the domain-error XACK behavior. This is noted as a Verifier observation.

**`TestReclaimTask_ReEnqueuesForHealthyWorker` renamed:** The original test asserted immediate `producer.Enqueue` — correct for TASK-009 behavior. TASK-010 changes this to deferred enqueue. The test was updated to verify the new contract and renamed to `TestReclaimTask_DeferredEnqueueViaRetryAfter`. The behavioral guarantee (task is eventually picked up by a healthy worker) is now split across two tests: deferred scheduling in `reclaimTask` and actual dispatch in `TestScanRetryReady_ReEnqueuesTasksWhoseRetryAfterHasElapsed`.

**`staticcheck`:** Not runnable — `staticcheck@v0.4.7` panics on Go 1.23 range-over-func syntax used elsewhere in the codebase; `staticcheck@latest` requires Go 1.25+. `go vet` is clean.

## Known Limitations

- **Migration 000005 must be run before deployment.** The new `retry_after` and `retry_tags` columns are required by the runtime. Existing rows will have `retry_after = NULL` and `retry_tags = '{}'` (both safe defaults — no backoff gate, no retry tags).
- **sqlc re-generation not run.** The sqlc-generated files were updated manually to add `retry_after` and `retry_tags` to the SELECT column lists and scan calls, and to add the two new queries. The next `sqlc generate` run will regenerate these files from the query sources and must include the new columns in `internal/db/queries/tasks.sql` (the `.sql` query source files). This is tracked as a follow-up for the DevOps agent.
- **`retry_tags` stores only one tag per task** in the current implementation (the single tag from the XCLAIM pending entry). Tasks submitted to multiple tag streams (multi-tag pipelines) will only be re-enqueued to the tag stream they were claimed from. This is correct for NexusFlow's current single-tag-per-task submission model but should be revisited if multi-tag dispatch is added.
- **AC-3 unit test:** No Monitor-level unit test for "process error does not retry" was added because the Monitor never sees XACK'd entries. The Verifier should confirm via integration test that a worker XACK'ing a domain-error message removes it from the pending list before the Monitor's scan interval fires.

## For the Verifier

1. Run migration 000005 against the test DB before running integration/acceptance tests.
2. Confirm AC-1: submit a task with `{maxRetries: 3, backoff: "exponential"}`, kill the worker three times — task should be retried three times then dead-lettered.
3. Confirm AC-2: verify timestamp gaps between retry attempts match 1s, 2s, 4s (within test tolerance).
4. Confirm AC-3: submit a task whose connector always fails (domain error) — task should appear in "failed" status immediately without any retry, and no Monitor reclaim should fire for it.
5. Confirm AC-4: after the third failure, task status = "failed" and appears in `queue:dead-letter`.
6. Confirm AC-5: `GET /api/tasks/{id}` response includes `retryCount` field incrementing with each retry attempt.
