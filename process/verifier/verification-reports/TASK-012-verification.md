# Verification Report — TASK-012
**Date:** 2026-03-29 | **Result:** PASS
**Task:** Task cancellation — POST /api/tasks/{id}/cancel | **Requirement(s):** REQ-010
**Iteration:** 2 (re-verification after Builder fix)

## Acceptance Criteria Results

| REQ | Criterion | Layer | Result | Notes |
|---|---|---|---|---|
| REQ-010 | AC-1: Owner cancel returns 204 and sets status to "cancelled" | Acceptance | PASS | Verified via API + DB query |
| REQ-010 | AC-2: Admin can cancel any task | Acceptance | PASS | Cross-ownership cancel confirmed |
| REQ-010 | AC-3: Non-owner non-admin gets 403 | Acceptance | PASS | Task status unchanged after rejection |
| REQ-010 | AC-4: Terminal state cancel returns 409 | Acceptance | PASS | Tested completed, failed, cancelled |
| REQ-010 | AC-5: Running task: Redis cancel flag set, worker halts | Acceptance | PASS | Flag exists with value "1" and 60s TTL |
| REQ-010 | AC-6: Cancellation creates a task_state_log entry | Acceptance | PASS | Log row present: from_state=queued, to_state=cancelled, reason="cancelled by user" |

## Test Summary

| Layer | Written | Passing | Failing |
|---|---|---|---|
| Integration | 0 | 0 | 0 |
| System | 0 | 0 | 0 |
| Acceptance | 22 | 22 | 0 |
| Performance | 0 | 0 | 0 |

**Unit tests (Builder-authored):** 14 handler tests + 3 worker cancellation tests — all passing.
**Full suite:** `go test ./...` — all packages pass. `go vet ./...` — clean. `go build ./...` — clean.

## Fix Verified — FAIL-001 Resolved

### FAIL-001 (resolved): PgTaskRepository.Cancel now delegates to UpdateStatus — task_state_log written

**Fix applied:** `PgTaskRepository.Cancel` (`internal/db/task_repository.go:214`) now delegates to
`r.UpdateStatus(ctx, id, models.TaskStatusCancelled, reason, nil)` instead of calling
`r.queries.CancelTask(ctx, id)` directly. `UpdateStatus` opens a transaction, fetches the current
task status, calls `UpdateTaskStatus`, and calls `InsertTaskStateLog` with `from_state`, `to_state`,
and `reason`.

**Verification:** AC-6 test sequence queued a task, cancelled it via `POST /api/tasks/{id}/cancel`,
then queried:

```sql
SELECT COUNT(*) FROM task_state_log WHERE task_id = '<id>' AND to_state = 'cancelled';
-- Result: 1

SELECT from_state, to_state, reason FROM task_state_log WHERE task_id = '<id>' AND to_state = 'cancelled';
-- Result: queued | cancelled | cancelled by user
```

Both checks passed. No regressions in AC-1 through AC-5.

## Observations (carried forward, non-blocking)

**OBS-1: Race condition between API cancel and worker completion (documented deviation)**
The Builder's handoff note correctly identifies that for a "running" task, the API sets the DB status
to "cancelled" before the worker checks the Redis flag. If the worker is between the last cancellation
check and the end of the Sink phase, it may attempt to set the status to "failed" after the API has
already written "cancelled". The DB trigger forbids `cancelled → failed` (valid transitions from
`cancelled` are none — it is a terminal state). This means the worker's `UpdateStatus` call will be
rejected by the trigger, and the task will correctly remain "cancelled". The worker will log an error.
The handoff note documents this accurately. Not a blocking concern for MVP.

**OBS-2: task_state_log trigger and the Cancel race (related to OBS-1)**
If the race described in OBS-1 resolves as expected (trigger rejects `cancelled → failed`), no stale
`task_state_log` entry will be created for the worker's rejected transition. The log correctly contains
only the API-initiated cancellation entry.

**OBS-3: Stale Docker image at verification start (iteration 1)**
Resolved in iteration 2. API and worker images are rebuilt as part of the handoff protocol for tasks
that change API behaviour.
