# Builder Handoff — TASK-006
**Date:** 2026-03-26
**Task:** Worker self-registration and heartbeat
**Requirement(s):** REQ-004, ADR-002, ADR-008

## What Was Implemented

### `internal/queue/redis.go` — RecordHeartbeat

- `RecordHeartbeat(ctx, workerID)` — executes `ZADD workers:active <now_unix> <workerID>`.
  Uses the current Unix timestamp as the score so the Monitor can find expired workers
  via `ZRANGEBYSCORE workers:active -inf <cutoff>` (ADR-002). Updates the score when the
  member already exists (idempotent ZADD).
- Removed the `//lint:ignore U1000` directive from this method (it is now used).

### `internal/db/worker_repository.go` — new file

New file implementing `WorkerRepository` backed by sqlc-generated queries:

- `NewPgWorkerRepository(pool)` — constructs repository; panics on nil pool (fail-fast).
- `Register(ctx, worker)` — upserts the worker record via the `RegisterWorker` sqlc query
  (INSERT ... ON CONFLICT DO UPDATE). Idempotent: a restarting worker updates its tags,
  status, and last_heartbeat without creating a duplicate row.
- `GetByID(ctx, id)` — returns nil, nil when the worker does not exist (maps to 404 at the service layer).
- `List(ctx)` — returns all workers with `CurrentTaskID` populated from the tasks subquery.
- `UpdateStatus(ctx, id, status)` — sets status and refreshes `last_heartbeat` via the SQL query.

Refactoring applied:
- **Extract Function** (Fowler): `toTimestamptz` and `fromTimestamptz` extracted from repeated
  inline `pgtype.Timestamptz` construction. Both `toModelWorker` and `toModelWorkerFromListRow`
  now delegate to these helpers, eliminating duplication and making the conversion intent explicit.

### `worker/worker.go` — full implementation of TASK-006 scope

- `NewWorker(cfg, tasks, workers, consumer, heartbeat, broker, connectors)` — constructs the Worker
  struct. No longer panics. Removed all `//lint:ignore U1000` directives from fields that are now used.
- `Register(ctx)` — builds a `models.Worker` with `Status="online"`, `Tags=cfg.WorkerTags`,
  `RegisteredAt=now`, calls `WorkerRepository.Register`, then calls `HeartbeatStore.RecordHeartbeat`
  so the worker appears in `workers:active` immediately. Both dependencies are nil-guarded (unit
  tests inject nil for dependencies outside TASK-006 scope).
- `emitHeartbeats(ctx)` — ticker at `cfg.HeartbeatInterval` (default 5s per ADR-002). On each
  tick calls `HeartbeatStore.RecordHeartbeat`. Stops on `ctx.Done()`. Falls back to 5s when
  `HeartbeatInterval` is zero (defensive guard).
- `Run(ctx)` — orchestrates startup: Register → InitGroups → emitHeartbeats goroutine →
  runConsumptionLoop (blocks until ctx cancelled) → markOffline (graceful shutdown).
- `markOffline(ctx)` — extracted private method (Extract Function) that calls
  `WorkerRepository.UpdateStatus(ctx, id, "down")`. Uses a 5-second shutdown timeout context
  so the DB write completes even after the parent context is cancelled.
- `runConsumptionLoop(ctx)` — retains TASK-007 scaffold: blocks on `ctx.Done()` only.
- `executeTask` and `applySchemaMapping` — retain TASK-007 scaffold with `//lint:ignore U1000`
  directives added (not yet called, pending TASK-007).

### `cmd/worker/main.go` — full wiring

- Replaced the TASK-001 placeholder with the full wiring:
  - Config load → `WORKER_ID` generation (`hostname-uuid[:8]` format, fallback pure UUID)
  - Default tags to `["demo"]` when `WORKER_TAGS` is unset
  - `db.New` (PostgreSQL + migrations)
  - Redis client (verified with PING; fatal on failure)
  - `db.NewPgWorkerRepository` and `queue.NewRedisQueue`
  - `worker.NewWorker` with `TaskRepository=nil`, `Broker=nil`, `ConnectorRegistry=nil`
    (wired in TASK-007, TASK-015, TASK-042 respectively)
  - Signal handler: SIGTERM/SIGINT → `runCancel()` → waits for `Run` to return
- `generateWorkerID()` — extracted private function: combines hostname with a UUID suffix
  for human-readable uniqueness. Falls back to a pure UUID when hostname is unavailable.

## Unit Tests

### `internal/queue/heartbeat_test.go` (new file)

Tests skipped automatically when Redis is unavailable (same pattern as `redis_test.go`):

| Test | Assertion |
|------|-----------|
| `TestRecordHeartbeat_AddsToSortedSet` | ZADD creates entry with current Unix timestamp score |
| `TestRecordHeartbeat_UpdatesExistingEntry` | Second call increases the score (not a duplicate) |
| `TestRecordHeartbeat_MultipleWorkers` | Three distinct workers each get their own entry |
| `TestWorkersActiveKey_Constant` | `WorkersActiveKey == "workers:active"` |

### `worker/worker_test.go` (new file)

All tests use in-memory fakes (`fakeWorkerRepo`, `fakeHeartbeatStore`) — no live infrastructure required:

| Test | Assertion |
|------|-----------|
| `TestNewWorker_ReturnsNonNil` | Constructor returns non-nil `*Worker` |
| `TestRegister_InsertsWorkerWithOnlineStatus` | Worker in repo has Status="online" |
| `TestRegister_RecordsInitialHeartbeat` | `RecordHeartbeat` called at least once during Register |
| `TestRegister_SetsCorrectTags` | Tags in repo match config |
| `TestRegister_SetsRegisteredAt` | RegisteredAt is non-zero and within test window |
| `TestEmitHeartbeats_CallsRecordHeartbeatPeriodically` | At 40ms interval, at least 4 calls in 250ms |
| `TestRun_MarksWorkerDownOnShutdown` | Status="down" after ctx cancel |
| `TestRun_MultipleWorkersDifferentIDs` | Two concurrent workers each register independently |

## Acceptance Criteria Traceability

| AC | Met | Evidence |
|----|-----|---------|
| AC-1: Worker starts and appears in `workers` table with status "online" and correct tags | Yes | `TestRegister_InsertsWorkerWithOnlineStatus`, `TestRegister_SetsCorrectTags` |
| AC-2: Heartbeat updates `workers:active` every 5 seconds | Yes | `TestRecordHeartbeat_AddsToSortedSet`, `TestEmitHeartbeats_CallsRecordHeartbeatPeriodically` |
| AC-3: Multiple workers register simultaneously with different tags | Yes | `TestRun_MultipleWorkersDifferentIDs`, `TestRecordHeartbeat_MultipleWorkers` |
| AC-4: Worker record includes registration timestamp and tags | Yes | `TestRegister_SetsRegisteredAt`, `TestRegister_SetsCorrectTags` |

## Deviations

1. **`cmd/worker/main.go` rewritten, not extended.** The TASK-001 placeholder blocked entirely on signal with no DB, repo, or worker.Run wiring. Rewriting was necessary to satisfy the acceptance criteria; no behaviour from the placeholder (Redis ping logging) was lost — that logging remains in the new implementation.

2. **nil-guarded dependencies in Worker.** `Register`, `emitHeartbeats`, and `markOffline` nil-check `w.workers` and `w.heartbeat` before calling them. This allows unit tests to pass nil for dependencies outside TASK-006 scope without a panic. The production path (`cmd/worker/main.go`) always passes non-nil instances.

3. **`runConsumptionLoop` not implemented** — this is TASK-007 scope. It currently blocks on `ctx.Done()`. This means `Worker.Run` does not consume tasks until TASK-007 is implemented, but it does register, heartbeat, and shut down cleanly.

## Verifier Instructions

1. Start infrastructure:
   ```
   docker compose -f /Users/pablo/projects/Nexus/NexusTests/NexusFlow/docker-compose.yml up postgres redis -d
   ```

2. Run all unit tests:
   ```
   docker run --rm --network nexusflow_internal \
     -v /Users/pablo/projects/Nexus/NexusTests/NexusFlow:/app -w /app \
     -e REDIS_ADDR=redis:6379 \
     golang:1.23-alpine go test ./...
   ```
   Expected: all packages PASS.

3. Run vet and staticcheck:
   ```
   docker run --rm -v /Users/pablo/projects/Nexus/NexusTests/NexusFlow:/app -w /app \
     golang:1.23-alpine go vet ./...
   docker run --rm -v /Users/pablo/projects/Nexus/NexusTests/NexusFlow:/app -w /app \
     golang:1.23-alpine sh -c "apk add -q git && GOPROXY=direct go install honnef.co/go/tools/cmd/staticcheck@2024.1.1 && staticcheck ./..."
   ```
   Expected: no output from either command.

4. For acceptance criteria requiring a live worker (AC-1, AC-2), set the env variables and run the worker binary:
   ```
   DATABASE_URL=postgresql://nexus:nexus@localhost:5432/nexusflow \
   REDIS_URL=redis://localhost:6379 \
   WORKER_TAGS=demo \
   go run ./cmd/worker
   ```
   Verify in PostgreSQL: `SELECT id, tags, status, registered_at FROM workers;`
   Verify in Redis: `ZRANGE workers:active 0 -1 WITHSCORES`

## Known Limitations

- `runConsumptionLoop` is a stub (TASK-007). The worker registers and heartbeats correctly but does not pull tasks from the queue.
- `Broker` and `ConnectorRegistry` are wired as nil in `cmd/worker/main.go`. SSE events and pipeline execution require TASK-015 and TASK-042 respectively.
- `TaskRepository` is nil — task status transitions during execution require TASK-007.
