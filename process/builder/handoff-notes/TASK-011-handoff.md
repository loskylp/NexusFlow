# Builder Handoff — TASK-011
**Date:** 2026-03-29
**Task:** Dead letter queue with cascading cancellation
**Requirement(s):** REQ-012, REQ-014

## What Was Implemented

### New behavior in `monitor/monitor.go`

- `Monitor` struct gains a `chains db.ChainRepository` field.
- `NewMonitor` signature updated to accept `chains db.ChainRepository` as the 4th parameter (after `tasks`, before `heartbeat`). The chains argument may be nil; when nil, `cascadeCancelDownstream` is a no-op (preserves standalone-monitor deployments).
- `deadLetterTask` extended: after marking a task failed and enqueuing to `queue:dead-letter`, it loads the task record (needed for `PipelineID`), then calls `cascadeCancelDownstream`. Cascade errors are non-fatal — they are logged and the dead-letter event is preserved.
- `cascadeCancelDownstream(ctx, task)` — new method. Uses `ChainRepository.FindByPipeline` to determine whether the failing task's pipeline belongs to a chain. If yes, walks `chain.PipelineIDs` from the position after the failing pipeline to the end, calling `cancelNonTerminalTasksForPipeline` for each downstream pipeline.
- `cancelNonTerminalTasksForPipeline(ctx, pipelineID)` — new method. Calls `TaskRepository.ListByPipelineAndStatuses` with the four non-terminal statuses (submitted, queued, assigned, running), then cancels each returned task with reason "upstream task failed" and publishes a task SSE event (fire-and-forget per ADR-007).
- `nonTerminalStatuses` — package-level var listing the four cancellable states.

### New interface method: `internal/db/repository.go`

`TaskRepository` gains `ListByPipelineAndStatuses(ctx, pipelineID, statuses) ([]*models.Task, error)`. Used exclusively by the Monitor for cascade lookup. Returns an empty (non-nil) slice when no matches exist.

### New sqlc query and implementation

- `internal/db/queries/tasks.sql` — `ListTasksByPipelineAndStatuses :many` query added.
- `internal/db/sqlc/tasks.sql.go` — generated code added manually (follows the existing hand-edit pattern for sqlc output in this repo). The query uses `ANY($2::text[])` for the status set.
- `internal/db/task_repository.go` — `PgTaskRepository.ListByPipelineAndStatuses` implemented. Converts `[]models.TaskStatus` to `[]string` before passing to sqlc.

### Wiring: `cmd/monitor/main.go`

`NewPgChainRepository(pool)` constructed and passed as the `chains` argument to `NewMonitor`. All dependencies are now fully wired (no nil placeholders in production).

### Stub updates (interface compliance)

- `api/handlers_tasks_test.go` — `stubTaskRepo.ListByPipelineAndStatuses` added (noop).
- `worker/executor_test.go` — `fakeTaskRepo.ListByPipelineAndStatuses` added (noop).

## Unit Tests

- Tests written: 5 new (TASK-011 acceptance criterion tests)
- Existing tests updated: 21 `NewMonitor` call sites in `monitor/monitor_test.go` updated for the new `chains` parameter (all pass `nil`); `fakeTaskRepository` and `fakeChainRepository` fakes added.
- All passing: yes — `go test ./...` passes across all 10 test packages; `go vet ./...` clean.

Key behaviors covered by new tests:
- Chain A→B→C: dead-lettering task A cascades cancellation to tasks B and C (`TestDeadLetterTask_CascadeCancelsDownstreamTasks`)
- Standalone task (pipeline not in any chain) is dead-lettered without cascade (`TestDeadLetterTask_StandaloneTaskNoCascade`)
- SSE task events are published for each cascade-cancelled task (`TestDeadLetterTask_CascadePublishesSSEEvents`)
- When the failing task is the middle step (B in A→B→C), only downstream task C is cancelled — upstream completed task A is untouched (`TestDeadLetterTask_CascadeOnlyDownstreamNotUpstream`)
- Nil chains repository is a safe no-op (`TestDeadLetterTask_NilChainsIsNoop`)

## Deviations from Task Description

None. All acceptance criteria are addressed as specified.

## Known Limitations

- **Race between cascade and chain trigger:** If the `ChainTrigger` (worker) fires for pipeline A's completion at the same time as the Monitor dead-letters pipeline A's task, it is theoretically possible for a task B to be submitted after cascade cancellation. This is an inherent race between the worker and monitor paths and is out of scope for this task. The chain trigger fires on task *completion*; the dead-letter path fires on *failure after exhausted retries*. These are mutually exclusive task states, so the race window is limited to the brief period between status update and SSE event propagation.
- **Task B not yet submitted:** If pipeline B's task has not been submitted yet when pipeline A fails (i.e. the chain trigger hasn't fired), there is no task to cancel for B. The cascade correctly cancels whatever non-terminal tasks exist at the time of dead-lettering. Future tasks cannot be pre-cancelled.
- The sqlc-generated file was edited manually. If `sqlc generate` is re-run, it will overwrite the manual addition. The query is also present in `internal/db/queries/tasks.sql`, so re-running `sqlc generate` should regenerate the same code.

## For the Verifier

- Acceptance criterion 1 (task appears in `queue:dead-letter`): exercised by `TestDeadLetterTask_ExhaustedRetries` (pre-existing) and the new cascade tests which all verify `producer.deadLettered` is populated.
- Acceptance criterion 2 (A→B→C cascade): `TestDeadLetterTask_CascadeCancelsDownstreamTasks`.
- Acceptance criterion 3 (standalone — no cascade): `TestDeadLetterTask_StandaloneTaskNoCascade`.
- Acceptance criterion 4 (dead letter tasks visible via task API with status "failed"): the `deadLetterTask` method calls `UpdateStatus(... TaskStatusFailed ...)` before enqueuing — tasks are persisted as "failed" in PostgreSQL and are therefore visible via `GET /api/tasks?status=failed`. This was already implemented by TASK-009/010; the new code does not change the status-setting logic.
- The `NewMonitor` signature change is a breaking change for any caller. All call sites in this repo have been updated (monitor main, all monitor tests). No other callers exist.
