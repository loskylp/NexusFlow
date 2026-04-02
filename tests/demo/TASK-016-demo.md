# Demo Script — TASK-016
**Feature:** Log production and dual storage
**Requirement(s):** REQ-018, ADR-008
**Environment:** Staging — requires API, worker, PostgreSQL, and Redis running via Docker Compose

## Scenario 1: Log lines written to Redis Stream with phase tags during task execution
**REQ:** REQ-018

**Given:** You are logged in as any authenticated user and have a demo pipeline created (or use an existing one). No logs exist for the task you are about to submit.

**When:** Submit a new task via `POST /api/tasks` with a valid `pipelineId` and `"tags":["etl"]`. Then, before 60 seconds elapse, run `docker exec nexusflow-redis-1 redis-cli XRANGE logs:{taskId} - +` where `{taskId}` is the ID returned by the submission.

**Then:** The XRANGE output contains 6 entries (2 per phase: start and completion). Each entry has fields: `id`, `task_id`, `level`, `line`, and `timestamp`. The `line` field values are prefixed with `[datasource]`, `[process]`, or `[sink]`. All entries have `level` = `INFO` for a successful pipeline run.

**Notes:** The background sync runs every 60 seconds and deletes entries from the stream via XDEL after inserting them into PostgreSQL. Check the stream within the first 60 seconds to see live entries. After sync, use `XINFO STREAM logs:{taskId}` to confirm `entries-added: 6` even if the stream is now empty.

---

## Scenario 2: Real-time log delivery via SSE stream
**REQ:** REQ-018

**Given:** You are logged in as the task owner and have a task ID.

**When:** Open the SSE endpoint: `GET /events/tasks/{taskId}/logs` with an `Authorization: Bearer {token}` header and `Accept: text/event-stream`. You can use `curl --no-buffer -H "Authorization: Bearer {token}" http://staging/events/tasks/{taskId}/logs` from a terminal.

**Then:** The connection is established with `Content-Type: text/event-stream` and HTTP 200. If the task is still running, log events arrive as SSE `data:` lines. If the task has already completed, the connection opens (the SSE handler uses Last-Event-ID replay to deliver cold logs) and then remains open for future events.

**Notes:** For a new task that completes quickly, open the SSE stream before submitting the task to observe log events as they stream in real time. The worker publishes each `emitLog` call to `events:logs:{taskId}` via the RedisBroker.

---

## Scenario 3: Background sync copies logs to PostgreSQL cold store
**REQ:** REQ-018, ADR-008

**Given:** A task has completed and its log entries are in the Redis stream `logs:{taskId}`.

**When:** Wait 60 seconds for the background sync goroutine to fire. Then query PostgreSQL: `docker exec nexusflow-postgres-1 psql -U nexusflow -d nexusflow -c "SELECT id, level, line, timestamp FROM task_logs WHERE task_id = '{taskId}' ORDER BY timestamp;"`.

**Then:** 6 rows appear in the `task_logs` table. Each row has a non-null `id` (UUID), `level` (INFO), `line` (phase-prefixed), and `timestamp`. The Redis stream `logs:{taskId}` is now empty (`XLEN` = 0) because XDEL trimmed the processed entries.

**Notes:** The sync runs every 60 seconds regardless of task completion. At staging, the sync fires within 60 seconds of the task finishing. If you check PostgreSQL before 60 seconds have elapsed, you may see 0 rows — wait and retry.

---

## Scenario 4: GET /api/tasks/{id}/logs returns historical log lines
**REQ:** REQ-018

**Given:** A task has completed and its logs have been synced to PostgreSQL (at least 60 seconds since task completion, or rows are visible via the Scenario 3 query).

**When:** Call `GET /api/tasks/{taskId}/logs` with a valid owner session token.

**Then:** Response is `200 OK` with a JSON array of log line objects. Each object contains:
- `id`: UUID string
- `taskId`: matches the task UUID
- `level`: `"INFO"` (or `"WARN"`/`"ERROR"` for error phases)
- `line`: phase-prefixed message, e.g. `"[datasource] starting DataSource(demo)"`
- `timestamp`: RFC3339 timestamp, e.g. `"2026-03-29T08:57:14.217044Z"`

The array has 6 entries for a successful demo pipeline run covering all three phases.

---

## Scenario 5: Access control — only owner and admin can retrieve logs
**REQ:** REQ-018

**Given:** A task owned by user A exists with synced logs. User B (a different regular user) is logged in separately.

**When — Owner access:** Call `GET /api/tasks/{taskId}/logs` with user A's token. Observe 200 with the log array.

**When — Admin access:** Call `GET /api/tasks/{taskId}/logs` with the admin token. Observe 200 with the log array.

**When — Non-owner access:** Call `GET /api/tasks/{taskId}/logs` with user B's token (who did not submit the task). Observe 403 Forbidden. Confirm the response body contains no log data (only an error message).

**When — Unauthenticated access:** Call `GET /api/tasks/{taskId}/logs` with no Authorization header. Observe 401 Unauthorized.

**Then:** The access control matrix is enforced: owner and admin get logs; all others get a 4xx response with no data disclosure.

**Notes:** At staging, create a second regular user account to test the non-owner path. The admin account (username: `admin`) can retrieve logs for any task.
