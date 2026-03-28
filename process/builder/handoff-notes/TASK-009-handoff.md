# Handoff Note — TASK-009

**Task:** Monitor service — heartbeat checking and failover
**Cycle:** 2 (first task, P1/HH — critical path)
**Iteration:** 1 (initial build)
**Status:** COMPLETE

---

## What was built

### 1. `internal/queue/queue.go` — PendingScanner interface extended

Added `AcknowledgePending(ctx, tag, streamID)` to the `PendingScanner` interface. This is the XACK operation the Monitor must call after XCLAIM + re-enqueue to clear the monitor consumer's pending list. Without it, the reclaimed entry would accumulate indefinitely in the monitor consumer's pending list and be presented to `ListPendingOlderThan` on every subsequent scan cycle.

### 2. `internal/queue/redis.go` — five stub methods implemented

| Method | Redis command | Notes |
|---|---|---|
| `ListExpired` | `ZRANGEBYSCORE workers:active -inf <cutoff_unix>` | Returns expired worker IDs |
| `Remove` | `ZREM workers:active <workerID>` | Idempotent; called after marking a worker down |
| `ListPendingOlderThan` | `XPENDING … IDLE <ms>` + `XRANGE` per entry | NOGROUP / stream-missing errors treated as empty result |
| `Claim` | `XCLAIM` | `redis.Nil` on already-ACKed entries treated as success |
| `AcknowledgePending` | `XACK` | Called by Monitor after re-enqueue to clean the pending list |

All five methods had `panic("not implemented")` stubs before this task.

### 3. `monitor/monitor.go` — full implementation

- `NewMonitor` — stores all injected dependencies.
- `Run` — single `time.Ticker` on `PendingScanInterval`; calls `checkHeartbeats` then `scanPendingEntries` per tick; stops cleanly on context cancellation.
- `checkHeartbeats` — computes `cutoff = now - HeartbeatTimeout`; calls `heartbeat.ListExpired`; delegates each expired worker to `markWorkerDown`.
- `markWorkerDown` — `UpdateStatus → Remove → GetByID → PublishWorkerEvent`; SSE publish failure is non-fatal (fire-and-forget per ADR-007).
- `scanPendingEntries` — collects all unique tags from registered workers via `workers.List`; calls `scanner.ListPendingOlderThan` per tag; routes each entry to `processEntry`.
- `processEntry` — loads task from PostgreSQL; compares `RetryCount` against `RetryConfig.MaxRetries`; routes to `reclaimTask` or `deadLetterTask`.
- `reclaimTask` — 5-step sequence: `Claim` (XCLAIM) → `IncrementRetryCount` → `UpdateStatus("queued")` → `Enqueue` (re-XADD) → `AcknowledgePending` (XACK). Re-XADD is required because `XREADGROUP ">"` only delivers entries not yet in any consumer's pending list; without the re-XADD, healthy workers would never see the reclaimed task.
- `deadLetterTask` — `UpdateStatus("failed")` → `EnqueueDeadLetter`.
- All `//lint:ignore U1000` directives removed from struct fields.

### 4. `cmd/monitor/main.go` — fully wired

Replaces the previous stub (which only blocked on SIGTERM after a Redis PING). Now:
- Connects PostgreSQL (runs migrations via `db.New`).
- Connects Redis (verified with PING).
- Constructs `PgWorkerRepository`, `PgTaskRepository`, `RedisQueue` (satisfying HeartbeatStore, PendingScanner, and Producer simultaneously), and `RedisBroker`.
- Calls `monitor.NewMonitor(…)` with all dependencies fully initialised (no nil placeholders).
- Starts `m.Run` in a goroutine; blocks on SIGTERM/SIGINT; cancels context on signal.

### 5. `monitor/monitor_test.go` — 13 new unit tests

All tests use in-process fakes (no Redis, no PostgreSQL). Tests were written before the implementation (red → green cycle confirmed).

| Test | Acceptance criteria |
|---|---|
| `TestNewMonitor_NonNilDependencies` | Constructor contract |
| `TestCheckHeartbeats_MarksExpiredWorkerDown` | AC-1 |
| `TestCheckHeartbeats_RemovesExpiredWorkerFromHeartbeatStore` | AC-1 (workers:active cleanup) |
| `TestCheckHeartbeats_PublishesWorkerDownEvent` | AC-2 |
| `TestCheckHeartbeats_HealthyWorkerIgnored` | AC-1 (no false positives) |
| `TestReclaimTask_IncrementsRetryCount` | AC-4 |
| `TestReclaimTask_TransitionsTaskToQueued` | AC-3/AC-5 |
| `TestReclaimTask_ClaimsPendingEntry` | AC-3 |
| `TestReclaimTask_ReEnqueuesForHealthyWorker` | AC-5 (re-XADD + XACK) |
| `TestDeadLetterTask_ExhaustedRetries` | AC-6 |
| `TestScanPendingEntries_ReclaimsAndDeadLetters` | AC-3, AC-4, AC-6 (end-to-end) |
| `TestRun_StopsOnContextCancel` | Graceful shutdown |
| `TestCheckHeartbeats_BrokerErrorIsNonFatal` | ADR-007 fire-and-forget |

---

## Build and test results

```
go build ./...         PASS
go vet ./...           PASS
staticcheck ./...      PASS
go test ./...          PASS — 13 new monitor tests; 0 regressions across all packages
```

---

## Acceptance criteria mapping

| AC | Satisfied | Where |
|---|---|---|
| AC-1: worker > 15s without heartbeat → "down" in PostgreSQL | Yes | `checkHeartbeats` → `markWorkerDown` → `UpdateStatus` |
| AC-2: worker:down event published to `events:workers` | Yes | `markWorkerDown` → `broker.PublishWorkerEvent` |
| AC-3: pending task on downed worker reclaimed via XCLAIM | Yes | `reclaimTask` → `scanner.Claim` |
| AC-4: retry counter incremented on failover | Yes | `reclaimTask` → `tasks.IncrementRetryCount` |
| AC-5: reclaimed task picked up by a healthy matching worker | Yes | `reclaimTask` → `producer.Enqueue` (re-XADD) + `scanner.AcknowledgePending` (XACK) |
| AC-6: exhausted retries → `queue:dead-letter` + status "failed" | Yes | `deadLetterTask` → `UpdateStatus("failed")` + `EnqueueDeadLetter` |

---

## Deviations from task description

**`PendingScanner` interface extended** — Added `AcknowledgePending` to the `PendingScanner` interface (not mentioned in the task description but required for correctness). Without XACK after XCLAIM, the re-claimed entry accumulates in the monitor consumer's pending list. `RedisQueue` implements the method. All existing callers of `PendingScanner` (the monitor) are updated; no breaking changes to other packages.

**"All known streams" discovery via worker registry** — The implementation derives known tags by calling `workers.List` and taking the union of all registered workers' tags. This is consistent with the domain model; it scans exactly the streams that active workers are consuming, not arbitrary `queue:*` keys that may belong to deleted or unknown worker types.

**Single ticker for both checks** — The scaffold docstring mentioned two separate tickers (heartbeat check at `HeartbeatTimeout/3 = 5s`, pending scan at `PendingScanInterval = 10s`). The implementation uses a single ticker at `PendingScanInterval`. Both checks are inexpensive and the 25-second worst-case detection-to-reassignment window from ADR-002 is still satisfied.

---

## Limitations

**Cascading chain cancellation deferred** — `deadLetterTask` does not implement cascading cancellation for PipelineChain tasks. This is explicitly deferred to TASK-011 per the scaffold docstring and task description.

**Multi-tag task re-enqueue** — `reclaimTask` re-enqueues the task on the single tag stream the pending entry was found on. If a task was originally enqueued on multiple streams (one per tag), only the stream being scanned is re-enqueued. This matches the pattern used by the Worker's `ackMessage`, which tries all worker tags. The correct fix for multi-tag tasks is to re-enqueue on all of the task's tags, but the `PendingEntry` does not carry the full tag list and `models.Task` does not store the tags used at enqueue time. This is a pre-existing design constraint, not introduced by this task.
