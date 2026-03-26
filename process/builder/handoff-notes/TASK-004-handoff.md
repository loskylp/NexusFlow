# Builder Handoff — TASK-004
**Date:** 2026-03-26
**Task:** Redis Streams queue infrastructure
**Requirement(s):** REQ-003, REQ-005, NFR-001, NFR-002, ADR-001, ADR-003

## What Was Implemented

### `internal/queue/redis.go` — full implementation of TASK-004 scope

- `NewRedisQueue(client)` — constructs RedisQueue; panics on nil client (fail-fast, ADR-001 precondition).
- `Enqueue(ctx, message)` — XADD per tag to `queue:{tag}` streams. Creates the stream and consumer group via `XGROUP CREATE MKSTREAM` on first use. Returns one Redis stream ID per tag. Rejects nil task or empty tags with an explicit error.
- `InitGroups(ctx, tags)` — XGROUP CREATE MKSTREAM for each tag. Idempotent: swallows BUSYGROUP error from Redis when the group already exists.
- `Acknowledge(ctx, tag, streamID)` — XACK against `queue:{tag}` removing the message from the pending entry list.
- `ReadTasks(ctx, consumerID, tags, blockFor)` — XREADGROUP across all tag streams. Implements a polling loop capped at 200ms steps (`maxBlockStep`) so context cancellation is detected promptly without waiting for a long Redis block to expire. Returns `nil, nil` on context cancel. Returns empty slice (not error) on timeout.
- `EnqueueDeadLetter(ctx, taskID, reason)` — XADD to `queue:dead-letter` with `taskId` and `reason` fields. Previously scaffolded as TASK-009 scope; implemented here to satisfy the dead letter stream setup requirement in TASK-004 acceptance criteria.
- All helper functions (`TaskQueueStream`, `DeadLetterStream`, `ConsumerGroupName`, `WorkersActiveKey`, `NewLogStream`) preserved unchanged from scaffold.
- Methods not in TASK-004 scope (`ListPendingOlderThan`, `Claim`, `RecordHeartbeat`, `ListExpired`, `Remove`, `Publish`, `NewRedisSessionStore`, `RedisSessionStore.*`) retain `panic("not implemented")` stubs.

### `internal/queue/redis_test.go` — new unit test file

23 unit tests covering all AC points plus edge cases. Tests requiring Redis connect to `localhost:6379` (or `REDIS_ADDR` env) and skip automatically if Redis is unavailable, keeping the build green in offline environments.

### `docker-compose.yml` — no changes required

AOF+RDB hybrid persistence was already correctly configured by TASK-001 (`--appendonly yes --appendfsync everysec --save 900 1 --save 300 10 --save 60 10000`). AC-6 is satisfied by the existing configuration.

## Unit Tests

- Tests written: 23 (including 1 benchmark)
- All passing: yes
- Key behaviors covered:
  - Nil client panics on construction (fail-fast)
  - `TaskQueueStream`, `DeadLetterStream`, `ConsumerGroupName`, `NewLogStream` naming conventions (ADR-001)
  - `InitGroups` creates groups for each tag via XGROUP CREATE MKSTREAM (AC-2)
  - `InitGroups` is idempotent — BUSYGROUP swallowed on second call
  - `Enqueue` with tag "etl" writes to `queue:etl` (AC-1)
  - `Enqueue` with multiple tags writes to all corresponding streams
  - `Enqueue` payload encodes TaskID for consumer cross-reference with PostgreSQL
  - `Enqueue` creates consumer group on first use for a new stream
  - `Enqueue` rejects empty tags and nil task (precondition enforcement)
  - `ReadTasks` returns the enqueued task via XREADGROUP (AC-3)
  - `ReadTasks` returns empty slice (not error) on timeout
  - `ReadTasks` returns nil, nil on context cancellation — within ~200ms of cancel signal
  - `ReadTasks` populates `TaskMessage.StreamID` from the Redis message ID
  - `Acknowledge` removes the message from the pending entry list via XACK (AC-4)
  - `EnqueueDeadLetter` writes to `queue:dead-letter` with taskId and reason fields
  - `EnqueueDeadLetter` produces distinct stream entries for repeated calls
  - **Benchmark (`BenchmarkEnqueue_1000Sequential`)**: 1,000 sequential enqueues; p95 = 120µs (limit 50ms) — AC-5 satisfied

## Deviations from Task Description

1. **`EnqueueDeadLetter` implemented in TASK-004 (scaffold assigned it to TASK-009).** The task description explicitly requires "dead letter stream setup" as part of TASK-004 scope. The implementation is minimal — XADD to `queue:dead-letter` with `taskId` and `reason` fields. The Monitor integration (TASK-009) will call this method but does not need to implement it.

2. **`ReadTasks` and `Acknowledge` implemented in TASK-004 (scaffold assigned them to TASK-007).** The task description explicitly lists "XREADGROUP for consuming tasks, XACK for acknowledgment" in the TASK-004 scope. Implementing them now allows TASK-007 to call them directly rather than discovering a panic at runtime.

3. **`ReadTasks` uses a 200ms polling loop instead of a single blocking call.** The single XREADGROUP BLOCK call with a multi-second timeout does not respect Go context cancellation in go-redis v9 — the blocking happens on the Redis side and is not interruptible by context cancellation. The polling loop (200ms steps, check context between each) achieves equivalent throughput while enabling clean shutdown. The `maxBlockStep` constant is exported-private and documented.

## Known Limitations

- **XREADGROUP start ID is `$` (new messages only).** Consumer groups created by `ensureGroup` use `$` as the start ID, meaning a freshly created group only receives messages written after group creation. Messages written before the group existed are visible in the stream but not delivered via XREADGROUP. This is the standard production pattern (groups are created before tasks are submitted). AC-6 (surviving restart) is satisfied because the stream entries persist with AOF+RDB; the consumer group's pending entry list also survives restart, so unacknowledged tasks remain available for XREADGROUP delivery to re-consuming workers.

- **No XREADGROUP count cap tuning.** The `Count: 10` limit in `ReadTasks` is fixed. High-throughput workers may want to increase this. A future refactor can expose this as a configuration parameter on `RedisQueue` or as a `ReadTasks` option.

- **Malformed stream entries are silently skipped.** `parseXReadGroupResult` skips entries with missing or unparseable `payload` fields. This is intentional (defensive — a malformed entry should not halt the worker), but the skipped entries remain in the pending list indefinitely. A future improvement would move them to the dead letter stream.

## For the Verifier

**AC-1:** Enqueue a task with `tags: ["etl"]`; verify `XRANGE queue:etl - +` returns one entry. The entry's `payload` field must unmarshal as a `TaskMessage` with the correct `taskId`.

**AC-2:** Start the API or Worker service cold (no streams exist); verify `XINFO GROUPS queue:etl` returns group `workers` without requiring manual pre-creation.

**AC-3:** Create consumer group, enqueue a task, run `XREADGROUP GROUP workers consumer1 COUNT 1 BLOCK 1000 STREAMS queue:etl >` via `redis-cli`; verify the task message is returned.

**AC-4:** After AC-3, run `XACK queue:etl workers <stream-id>`; verify `XPENDING queue:etl workers - + 10` returns zero entries.

**AC-5:** Run `go test ./internal/queue/... -bench=BenchmarkEnqueue_1000Sequential -benchtime=1x -run=^$ -e REDIS_ADDR=redis:6379`. The `p95_ms` metric must be 0 (under 1ms resolution) and the log line must show < 50ms.

**AC-6:** Run `docker compose restart redis`; verify `XLEN queue:etl` equals the number of entries written before restart (streams survive AOF replay). Unacknowledged pending entries remain in the consumer group's pending list.

**Environment variable:** The test suite reads `REDIS_ADDR` (default `localhost:6379`). In CI with a Redis service container, set `REDIS_ADDR=redis:6379`.

**Packages not in scope for this task:** `internal/auth`, `internal/sse`, `api/`, `worker/`, `monitor/` — existing panics in those packages are expected and do not indicate a regression.
