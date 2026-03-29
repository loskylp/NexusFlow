# Builder Handoff — TASK-016
**Date:** 2026-03-29
**Task:** Log production and dual storage
**Requirement(s):** REQ-018, ADR-008

## What Was Implemented

### `worker/log_publisher.go` — LogPublisher interface and RedisLogPublisher

**`LogPublisher` interface** — narrow single-method interface accepted by the Worker:
- `Publish(ctx, log) error` — writes a log line to hot storage and fires SSE fan-out.

**`NewLogLine(taskID, level, phase, message, ts) *models.TaskLog`** — constructor for a log line with a fresh UUID, the phase encoded as a bracketed prefix in `Line` (`[datasource] message text`), and all required fields set. Satisfies AC-5: timestamp, level, phase tag, and message are all present.

**`RedisLogPublisher`** — implements `LogPublisher`:
- Writes to Redis Stream `logs:{taskId}` via `XADD` with fields: `id`, `task_id`, `level`, `line`, `timestamp`.
- Publishes to `events:logs:{taskId}` via the `logLineBroker` narrow interface (only `PublishLogLine`).
- Panics at construction if the Redis client is nil (fail-fast).
- SSE broker may be nil; XADD still runs; SSE publish is skipped silently.

**`logLineBroker` interface** — Interface Segregation: the Worker's log publisher needs only `PublishLogLine`, not the full `sse.Broker`. `*sse.RedisBroker` satisfies this interface without any adapter.

### `worker/worker.go` — log production during pipeline execution

**`logPublisher LogPublisher` field** added to `Worker`. Set via `WithLogPublisher(publisher) *Worker` (fluent API, panics on nil).

**`emitLog(ctx, taskID, level, phase, message)`** — internal helper. No-op when `logPublisher == nil` so workers without a log publisher configured continue to function correctly. Errors from `Publish` are logged and discarded (fire-and-forget per ADR-007).

**Log production in `runPipeline`**: `emitLog` called at the start and end of each phase, and on every error path:
- `datasource`: start (INFO), completion with record count (INFO), error (ERROR), cancellation after phase (WARN)
- `process`: start with record count (INFO), completion (INFO), error (ERROR), schema mapping error (ERROR), cancellation after phase (WARN)
- `sink`: start with record count (INFO), commit (INFO), error (ERROR)

Each log line satisfies AC-1 (Redis Stream), AC-2 (SSE pub), and AC-5 (timestamp, level, phase tag, message).

### `internal/db/log_repository.go` — PgTaskLogRepository

Implements `db.TaskLogRepository` backed by sqlc-generated queries:

- **`BatchInsert(ctx, logs)`** — loops over the slice and calls `queries.BatchInsertLogs` per row (sqlc does not support variable-length bulk inserts). Returns on the first error; partial inserts are not rolled back (TASK-028 will address retention cleanup).
- **`ListByTask(ctx, taskID, afterID)`** — passes the zero UUID when `afterID` is empty (SQL `id > zero-uuid` returns all rows), or the parsed UUID for Last-Event-ID replay. Maps `pgtype.Timestamptz` → `time.Time` in the returned `models.TaskLog` slice. Never returns nil (empty slice on no results).

OBS-007 (no explicit PK on partitioned `task_logs`) is by design — no change to the table schema.

### `api/log_sync.go` — background log sync goroutine

**`StartLogSync(ctx, client, taskLogs)`** — launches the sync goroutine. Returns immediately; the goroutine runs every 60 seconds (ADR-008) until the context is cancelled.

**`syncLogs(ctx, client, taskLogs)`** — one cycle: SCAN for `logs:*` keys, call `syncStream` per key, log and continue on per-stream errors.

**`syncStream(ctx, client, taskLogs, streamKey)`** — reads up to 1000 entries via `XRANGE`, calls `parseStreamEntries` to decode them, batch-inserts into PostgreSQL, then trims the processed entries via `XDEL`. `XDEL` failure is logged but non-fatal (entries will be re-processed next cycle; TASK-028 will add XTRIM-based retention).

**`parseStreamEntry(e)`** — decodes one Redis Stream entry into `models.TaskLog`. Returns an error for any missing or malformed field. Entries with errors are skipped by `parseStreamEntries` (logged, not fatal) so one bad entry does not block the whole stream.

### `api/handlers_logs.go` — GET /api/tasks/{id}/logs

**`LogHandler.GetLogs`** — authenticated endpoint:
1. Parses `{id}` URL parameter as UUID; 400 on parse failure.
2. Loads the task via `TaskRepository.GetByID`; 404 if not found.
3. Enforces AC-6 ownership: `sess.Role != Admin && task.UserID != sess.UserID` → 403.
4. Queries `TaskLogRepository.ListByTask` with empty `afterID` (returns all cold logs).
5. Returns a JSON array (never `null`).

### `api/server.go` — Server struct, NewServer signature, route registration

- `taskLogs db.TaskLogRepository` added to `Server` struct.
- `NewServer` gains a `taskLogs` parameter between `tasks` and `pipelines`.
- Route `GET /api/tasks/{id}/logs` registered in the protected group.

### `cmd/api/main.go` — wiring

- `db.NewPgTaskLogRepository(pool)` constructed and stored in `taskLogRepo`.
- Broker wired with `.WithLogRepo(taskLogRepo)` — Last-Event-ID replay now operational.
- `api.StartLogSync(mainCtx, redisClient, taskLogRepo)` called after broker startup.
- `taskLogRepo` passed to `api.NewServer`.

### `cmd/worker/main.go` — wiring

- `sse.NewRedisBroker(redisClient)` constructed with a dedicated goroutine and cancel context (`runCtxForBroker`). The broker is passed as the `TaskEventBroker` (previously `nil`).
- `worker.NewRedisLogPublisher(redisClient, sseBroker)` constructed and wired via `.WithLogPublisher(logPublisher)`.
- `runCancelForBroker()` called on both shutdown paths to stop the broker goroutine cleanly.

## Unit Tests

**`internal/db/log_repository_test.go`** — 3 tests in package `db_test`:

| Test | What is verified |
|---|---|
| `TestBatchInsert_StoresLogLines` | BatchInsert stores lines; ListByTask returns them for the correct task |
| `TestListByTask_IsolatesPerTask` | Rows for task B do not appear in ListByTask for task A |
| `TestLogLineFieldsIncludeRequiredFields` | All AC-5 fields (id, taskId, level, line, timestamp) are populated |

**`worker/log_publisher_test.go`** — 3 tests in package `worker_test`:

| Test | What is verified |
|---|---|
| `TestNewLogLine_FieldsArePopulated` | Non-zero UUID, correct taskID, level, timestamp, phase bracket in line |
| `TestNewLogLine_WarnAndErrorLevels` | WARN and ERROR level values are preserved verbatim |
| `TestNewLogLine_PhaseTagsAreEncoded` | All three phases produce bracketed prefixes in Line |

**`api/handlers_logs_test.go`** — 6 tests in package `api`:

| Test | What is verified |
|---|---|
| `TestGetLogs_OwnerReceivesLogs` | Owner gets 200 with correct log line fields |
| `TestGetLogs_AdminReceivesAnyTaskLogs` | Admin gets 200 for any task regardless of ownership |
| `TestGetLogs_NonOwnerReceivesForbidden` | Non-admin non-owner gets 403 |
| `TestGetLogs_NotFoundTask` | Absent task returns 404 |
| `TestGetLogs_InvalidUUID` | Non-UUID path param returns 400 |
| `TestGetLogs_EmptyListReturnsArray` | Empty cold storage returns `[]` not `null` |

**`api/log_sync_test.go`** — 4 tests in package `api`:

| Test | What is verified |
|---|---|
| `TestParseStreamEntry_ValidEntry` | All fields decoded correctly from a well-formed entry |
| `TestParseStreamEntry_MissingField` | Each required field individually missing returns an error |
| `TestParseStreamEntry_MalformedUUID` | Non-UUID `id` value returns an error |
| `TestParseStreamEntries_SkipsMalformedEntries` | Valid entries returned; bad entries skipped without propagating error |

## Build and Static Analysis

- `go build ./...` — passes (no errors)
- `go vet ./...` — passes (no issues)
- `staticcheck ./... (v0.5.1 / 2024.1.1)` — passes (no issues)
- `go test ./...` — all tests pass (zero regressions)

## Acceptance Criteria Mapping

| AC | Status | Where verified |
|---|---|---|
| AC-1: Log lines in Redis Stream `logs:{taskId}` with phase tags | Implemented | `RedisLogPublisher.Publish` → XADD; phase tag in `NewLogLine`; worker calls `emitLog` at each phase |
| AC-2: Published to `events:logs:{taskId}` for SSE | Implemented | `RedisLogPublisher.Publish` → `broker.PublishLogLine` |
| AC-3: Background sync Redis → PostgreSQL | Implemented | `StartLogSync` → 60s ticker → `syncLogs` → `BatchInsert` |
| AC-4: GET /api/tasks/{id}/logs returns historical logs | Implemented | `LogHandler.GetLogs` → `ListByTask` → JSON array |
| AC-5: Timestamp, level (INFO/WARN/ERROR), phase, message | Implemented | `NewLogLine` fields; `models.TaskLog.Level`, `.Timestamp`, `.Line`; worker uses all three levels |
| AC-6: Owner or admin access; 403 otherwise | Implemented | `LogHandler.GetLogs` ownership check; `TestGetLogs_NonOwnerReceivesForbidden` |

## Deviations from Task Description

1. **Phase encoded in `Line` field, not a separate `Phase` column** — The task description considered adding a `phase` column via migration 000006, or encoding phase in the line prefix. The `models.TaskLog` struct has no `phase` field and the sqlc-generated code has no phase column. Rather than add a migration (which requires OBS-007 awareness for partitioned tables), phase is encoded as a bracketed prefix in the `Line` field: `[datasource] message`. The `Line` field is already the canonical human-readable representation. TASK-022 (Log Streamer GUI) uses phase-tag color coding which parses the `[{phase}]` prefix client-side.

2. **`cmd/worker/main.go` now constructs an `sse.RedisBroker`** — Previously the broker was `nil` (commented as "wired in TASK-015"). TASK-015 wired the broker in `cmd/api/main.go` but not in `cmd/worker/main.go`. TASK-016 requires the worker to call `broker.PublishLogLine`; this required constructing the broker in the worker. The broker goroutine runs for the lifetime of the worker process and is cancelled on shutdown.

## Limitations and Known Constraints

- **`XDEL` after sync may be skipped on Redis error** — if `XDEL` fails after a successful `BatchInsert`, the same entries will be re-processed in the next 60-second cycle. Because `task_logs` has no unique constraint (OBS-007 — partitioned table), duplicate entries will be inserted. TASK-028 addresses this with XTRIM-based retention and should also add deduplication logic or a unique index on `(task_id, id)`.

- **Log sync does not use a consumer group** — the sync reads from `XRANGE` (position 0) and relies on `XDEL` for position tracking. If multiple API server instances run simultaneously, all instances will read and insert the same log entries. For a single API server this is fine. Multi-instance deployments should use `XREADGROUP` with a single consumer group. This is out of scope for TASK-016.

- **`BatchInsert` is not truly atomic** — if the process dies mid-loop, some lines will be in PostgreSQL and some won't. The stream entries for committed lines will be re-inserted on the next cycle (see first point above). TASK-028 should address this.

## Verifier Instructions

1. Run `go test ./... -count=1` — all tests must pass, zero regressions.
2. Run `go vet ./...` — must produce no output.
3. Run `staticcheck ./...` — must produce no output.
4. Acceptance criteria verification:
   - **AC-1, AC-2**: Integration test — submit a task, observe `logs:{taskId}` Redis Stream entries with `XRANGE logs:{taskId} - +`, and `events:logs:{taskId}` SSE events.
   - **AC-3**: Integration test — wait 60s after task execution, query `SELECT * FROM task_logs WHERE task_id = $1`.
   - **AC-4**: Integration test — `GET /api/tasks/{id}/logs` with valid session returns log entries.
   - **AC-5**: Verify `level`, `timestamp`, `line` (with phase bracket), and `id` fields are populated in responses.
   - **AC-6**: `TestGetLogs_NonOwnerReceivesForbidden` (unit); integration: call with a non-owner Bearer token.
