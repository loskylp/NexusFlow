# Builder Handoff — TASK-015
**Date:** 2026-03-27
**Task:** SSE event infrastructure
**Requirement(s):** REQ-016, REQ-017, REQ-018, NFR-003, ADR-007

## What Was Implemented

### `internal/sse/redis_broker.go` — Full RedisBroker implementation

The `RedisBroker` struct implements the `Broker` interface. Scaffold stubs replaced with working code; all `//lint:ignore U1000` directives removed.

**Struct additions beyond scaffold** (required by the implementation):
- `tasks taskOwnerGetter` — narrow interface (only `GetByID`) for ownership checks in `ServeLogEvents` and `ServeSinkEvents`. Set via `WithTaskRepo()` chaining method.
- `logs logReplayer` — narrow interface (only `ListByTask`) for Last-Event-ID replay in `ServeLogEvents`. Set via `WithLogRepo()` chaining method.

**New unexported methods:**
- `Subscribe(channelKey) chan *models.SSEEvent` — registers a buffered channel (capacity 64) in the subscriber registry and returns it.
- `Unsubscribe(channelKey, ch)` — removes the channel from the registry and closes it, signalling the read loop to exit.
- `fanOut(channelKey, evt)` — delivers an event to all subscribers on a channel using a non-blocking send; drops the event for any subscriber whose buffer is full (backpressure strategy per ADR-007).
- `routeMessage(msg)` — decodes a Redis Pub/Sub message JSON payload into `models.SSEEvent` and calls `fanOut`.
- `serveSSEChannel(w, r, channelKey)` — shared delivery loop used by all four public Serve* methods: sets SSE headers, subscribes a client channel, ranges over events until the request context is cancelled.
- `authoriseTaskAccess(w, r, session, taskID)` — checks admin role or task ownership; writes 403 and returns false on denial. Fail-closed: denies access if `b.tasks` is nil or the task UUID is unparseable.
- `replayLogs(w, ctx, taskID, afterID)` — fetches cold log lines from `b.logs.ListByTask` and writes them as `log:line` SSE events before live streaming begins.

**Package-private helpers (testable):**
- `writeSSEEvent(w, evt)` — formats one SSE event per spec (`id:`, `event:`, `data:`, `\n\n`), calls `Flush()`. Returns an error if the writer is not an `http.Flusher` or if the write fails.
- `setSSEHeaders(w)` — sets `Content-Type: text/event-stream`, `Cache-Control: no-cache`, `Connection: keep-alive`, `X-Accel-Buffering: no` (nginx bypass per ADR-005).
- `taskChannelKey(userID, role)` — maps user/admin to the correct Redis channel key.
- `logChannelKey(taskID)` / `sinkChannelKey(taskID)` — Redis channel key constructors.
- `taskEventType(status)` — maps `TaskStatus` to SSE event type string.

**`Start()` implementation:**
Uses `redis.Client.PSubscribe` with four patterns: `events:tasks:*`, `events:logs:*`, `events:workers`, `events:sink:*`. Blocks on the channel from `PubSub.Channel()` until the context is cancelled. On cancellation, `PubSub.Close()` is deferred and the method returns nil.

**Publish methods:**
All four publish methods marshal an `SSEEvent` envelope to JSON and call `redis.Client.Publish`. `PublishTaskEvent` publishes to both `events:tasks:{userId}` and `events:tasks:all` so both user-specific and admin feeds receive the event. `PublishLogLine` sets the event `ID` to `log.ID.String()` for Last-Event-ID replay. All publish methods return a non-nil error if the Redis client is nil (safe for unit test use without a live Redis instance).

### `api/handlers_sse.go` — SSE handler delegation

Four handlers implement the `SSEHandler` struct. Each handler reads the session from context via `auth.SessionFromContext` (populated by the auth middleware before the handler is called) and delegates to the corresponding broker method. `Logs` extracts `chi.URLParam(r, "id")`; `Sink` extracts `chi.URLParam(r, "taskId")`.

### `cmd/api/main.go` — RedisBroker wired at startup

`sse.NewRedisBroker(redisClient).WithTaskRepo(taskRepo)` constructs the broker. A goroutine calls `broker.Start(mainCtx)` immediately after construction; the context is cancelled by `defer mainCancel()` on shutdown, cleanly terminating the Pub/Sub subscription before the process exits. The broker is passed to `api.NewServer` replacing the previous `nil` placeholder.

The `sse` import was added to the import block.

## Unit Tests

**`internal/sse/redis_broker_test.go`** — 28 tests, all pass (RED/GREEN confirmed):

| Test group | What is verified |
|---|---|
| `writeSSEEvent` | SSE wire format (id/event/data lines, double newline), ID omission when empty, JSON payload validity, error on non-Flusher writer |
| Channel key helpers | `taskChannelKey` for user and admin, `logChannelKey`, `sinkChannelKey` |
| Subscriber registry | `Subscribe` returns buffered channel, `Unsubscribe` closes channel and removes from registry, multiple subscribers tracked separately |
| Fan-out | Delivers to all subscribers, non-blocking drop on full-buffer slow consumer |
| Goroutine lifecycle | `ServeWorkerEvents` and `ServeTaskEvents` return on context cancel, subscriber cleaned up on disconnect |
| SSE headers | `Content-Type: text/event-stream` set by `ServeWorkerEvents` and `ServeTaskEvents` |
| Access control | `ServeLogEvents` returns 403 for non-owner user, 403 when task not found, admin bypasses 403; `ServeSinkEvents` returns 403 for non-owner |
| Publish with nil client | All four publish methods return a non-nil error (no panic) when Redis client is nil |
| Concurrent safety | 20 concurrent Subscribe/Unsubscribe goroutines — race detector detects no issues |

**`api/handlers_sse_test.go`** — 4 tests, all pass:

| Test | What is verified |
|---|---|
| `Tasks` | Delegates to `ServeTaskEvents` with the correct session |
| `Workers` | Delegates to `ServeWorkerEvents` |
| `Logs` | Extracts `{id}` from chi URL param and passes to `ServeLogEvents` |
| `Sink` | Extracts `{taskId}` from chi URL param and passes to `ServeSinkEvents` |

## Build and Static Analysis

- `go build ./...` — passes (no errors)
- `go vet ./...` — passes (no issues)
- `staticcheck ./...` (v0.5.1) — passes (no issues)
- All pre-existing unit tests continue to pass (zero regressions)

## Deviations from Task Description

1. **`WithTaskRepo` / `WithLogRepo` chaining methods added** — The task description said to implement `ServeLogEvents` with ownership verification and Last-Event-ID replay, but the original scaffold struct had no fields for those dependencies. Added two narrow-interface fields and two chaining constructor helpers rather than adding full `db.TaskRepository` and `db.TaskLogRepository` to the struct (Interface Segregation — the broker needs only `GetByID` and `ListByTask`, not all repository methods).

2. **`b.logs` (log replayer) not wired in `cmd/api/main.go`** — `TASK-016` (log persistence) has not yet been implemented, so `PgTaskLogRepository` does not exist yet. `WithLogRepo` is defined and ready; wiring it requires only adding `.WithLogRepo(logRepo)` in `main.go` once TASK-016 is complete. Last-Event-ID replay is structurally implemented; the replay path is silently skipped when `b.logs == nil`.

3. **`PublishTaskEvent` publishes to both user and admin channels** — ADR-007 specifies separate channels (`events:tasks:{userId}` and `events:tasks:all`). The task description said to publish to `events:tasks:{userId}` only, but the ADR requires both. Implemented both publishes so admin feeds receive all task events in real time, consistent with the architecture decision.

4. **`Start()` uses `PSubscribe` (pattern subscription) instead of individual `Subscribe` calls** — This subscribes to `events:tasks:*`, `events:logs:*`, `events:workers`, `events:sink:*` in a single Pub/Sub connection rather than four separate connections. The router `routeMessage` uses the concrete channel name from `msg.Channel` to fan out to the correct subscribers. This is more efficient and consistent with how go-redis manages Pub/Sub connections.

## Limitations and Known Constraints

- **Last-Event-ID replay is a no-op** until TASK-016 wires `PgTaskLogRepository` into the broker via `WithLogRepo`.
- **`goroutine` count grows with connected clients** — each SSE connection spawns no additional goroutines (the handler goroutine is the same goroutine created by `http.Server` per request). The `Start()` goroutine is the only broker-managed goroutine. This is the intended design.
- **No retry on Redis Pub/Sub disconnect** — if Redis disconnects, `Start()` returns nil (the Pub/Sub channel closes). The API server will serve subsequent SSE connections without live event delivery until the broker goroutine is restarted. The current `cmd/api/main.go` does not restart the broker. A supervisor pattern (retry loop in `main.go`) is a TASK-016/TASK-009 concern and out of scope for TASK-015.

## Verifier Instructions

1. Run `go test ./... -short` — all 32 new tests plus all pre-existing tests must pass.
2. Run `go vet ./...` and `staticcheck ./...` — both must produce no output.
3. Acceptance criteria map:
   - **AC-1** (task events stream in real time): `TestRedisBroker_FanOut_DeliveresToAllSubscribers` + integration test against live Redis.
   - **AC-2** (user/admin filtering): `TestTaskChannelKey_UserRole`, `TestTaskChannelKey_AdminRole`.
   - **AC-3** (worker events to all authenticated users): `TestRedisBroker_ServeWorkerEvents_SetsSSEContentType`, `TestRedisBroker_ServeWorkerEvents_CleansUpSubscriberOnDisconnect`.
   - **AC-4** (2-second log delivery, NFR-003): Integration test with live Redis required.
   - **AC-5** (Last-Event-ID replay): `TestRedisBroker_ServeLogEvents_AdminBypasses403Check` (structural); end-to-end test requires TASK-016 log repo + live Redis.
   - **AC-6** (403 for unauthorised log/sink access): `TestRedisBroker_ServeLogEvents_Returns403WhenNotOwnerOrAdmin`, `TestRedisBroker_ServeSinkEvents_Returns403WhenNotOwnerOrAdmin`.
   - **AC-7** (goroutine cleanup on disconnect): `TestRedisBroker_ServeWorkerEvents_CleansUpSubscriberOnDisconnect`, `TestRedisBroker_FanOut_DoesNotBlockOnSlowConsumer`.
