---
task: TASK-010
title: Infrastructure retry with backoff
requirement: REQ-011
status: PASS
verified: 2026-03-29
smoke: false
---

# Demo Script — TASK-010: Infrastructure retry with backoff

**Purpose:** Demonstrate that NexusFlow retries tasks that fail due to infrastructure failures (downed workers) up to the configured `max_retries` limit, applies exponential backoff between retries, and routes exhausted-retry tasks to the dead letter queue — while never retrying tasks that failed due to process/script errors.

**Prerequisites:**
- All services running (`docker compose up -d`)
- Admin credentials available (`admin` / `admin`)
- Migration 000005 applied (retry_after and retry_tags columns on tasks table)
- `docker exec` access to `nexusflow-postgres-1` and `nexusflow-redis-1`

---

## Scenario 1: Task is retried up to max_retries=3 on infrastructure failure

**Acceptance Criterion:** AC-1 — Task with {max_retries: 3, backoff: "exponential"} is retried up to 3 times on infrastructure failure.

| Step | Action | Expected Result |
|---|---|---|
| Given | A task is submitted with `retryConfig: {maxRetries: 3, backoff: "exponential"}` and `tags: ["demo"]` via `POST /api/tasks` | HTTP 201 returned; task ID obtained |
| Given | The task is picked up by a worker (status transitions to "running") | `GET /api/tasks/{id}` returns `status: "running"` |
| When | The worker container is paused (`docker pause nexusflow-worker-1`) causing heartbeats to stop for >15 seconds | Monitor detects the worker as down |
| Then | After ~25 seconds (15s timeout + 10s scan interval), the task `retry_count` increments to 1 | `SELECT retry_count FROM tasks WHERE id='...'` returns `1` |
| Then | The task status transitions to "queued" (deferred re-enqueue via backoff gate) | `GET /api/tasks/{id}` returns `status: "queued"` |
| Then | After the backoff delay elapses, the task is re-enqueued and picked up by a healthy worker | Task eventually reaches "completed" or "assigned/running" |
| Then | The task does NOT appear in `queue:dead-letter` (retries not exhausted) | `redis-cli XRANGE queue:dead-letter - +` does not contain the task ID |

---

## Scenario 2: Exponential backoff delay is applied between retries (1s, 2s, 4s)

**Acceptance Criterion:** AC-2 — Backoff delay is applied between retries (exponential: 1s, 2s, 4s).

| Step | Action | Expected Result |
|---|---|---|
| Given | Task A is inserted with `retry_count=0` and `backoff="exponential"` in "running" status; placed in the pending list | Task A in `queue:demo` pending list |
| Given | Task B is inserted with `retry_count=1` and `backoff="exponential"` in "running" status; placed in the pending list | Task B in `queue:demo` pending list |
| Given | The worker is paused before both tasks are enqueued | Heartbeats stop; neither task is picked up by a live worker |
| When | The monitor detects the worker down and reclaims both tasks | `running→queued` transitions recorded in `task_state_log` for both tasks |
| Then | Task A (retry_count=0) is re-dispatched at least 1 second after the reclaim transition | `task_state_log` gap between `running→queued` and next `queued→assigned` >= 1s |
| Then | Task B (retry_count=1) is re-dispatched at least 2 seconds after the reclaim transition | `task_state_log` gap >= 2s |
| Then | Task B is re-dispatched at the same time or after Task A | Task B `queued→assigned` timestamp >= Task A `queued→assigned` timestamp (exponential growth confirmed) |

---

## Scenario 3: Process script error does NOT trigger retry

**Acceptance Criterion:** AC-3 — Task failing due to Process script error is NOT retried and transitions to "failed" immediately.

| Step | Action | Expected Result |
|---|---|---|
| Given | A task is in "failed" status with `retry_count=0` (simulating a domain error: worker XACKed the message after marking failed) | Task in PostgreSQL: `status="failed"`, `retry_count=0`; no entry in `queue:demo` pending list |
| When | One full monitor scan cycle (10 seconds) elapses | Monitor scans `queue:demo` pending list |
| Then | The task status remains "failed" — Monitor did not reclaim it | `SELECT status FROM tasks WHERE id='...'` returns `"failed"` |
| Then | The task `retry_count` remains 0 | `SELECT retry_count FROM tasks WHERE id='...'` returns `0` |
| Then | The task is NOT present in `queue:dead-letter` | Domain errors do not enter the DLQ; only infrastructure-exhausted retries do |
| Then | No pending entry exists in Redis for this task | `redis-cli XPENDING queue:demo workers - + 100` does not contain the task ID |

---

## Scenario 4: Task exhausting retries transitions to "failed" and enters dead letter queue

**Acceptance Criterion:** AC-4 — Task that exhausts retries transitions to "failed" and is placed in dead letter queue.

| Step | Action | Expected Result |
|---|---|---|
| Given | Task A is inserted with `retry_count=3` (= `max_retries=3`) in "running" status; placed in the pending list of a worker | Task A at maximum retry count |
| Given | Task B is inserted with `retry_count=2` (< `max_retries=3`) in "running" status; placed in the same pending list | Task B still has one retry remaining |
| When | The worker is paused and the monitor detects it down and scans both tasks | Monitor processes both pending entries |
| Then | Task A (exhausted retries) transitions to status "failed" | `SELECT status FROM tasks WHERE id='...'` returns `"failed"` |
| Then | Task A appears in `queue:dead-letter` stream | `redis-cli XRANGE queue:dead-letter - +` contains task A's ID |
| Then | Task B (retries remaining) is NOT dead-lettered — it is reclaimed for its third attempt | Task B NOT in `queue:dead-letter`; Task B status = "queued" |

---

## Scenario 5: Retry count is visible in GET /api/tasks/{id}

**Acceptance Criterion:** AC-5 — Retry count is visible in task state.

| Step | Action | Expected Result |
|---|---|---|
| Given | A task is submitted via `POST /api/tasks` with `retryConfig: {maxRetries: 3, backoff: "exponential"}` | HTTP 201; task ID obtained |
| When | `GET /api/tasks/{id}` is called immediately after submission | Response body contains `"retryCount": 0` inside the `task` object |
| Then | The `retryCount` field is present in the API response | Response JSON includes `"retryCount"` key |
| When | The task's `retry_count` is updated to 2 (simulating two infrastructure failures) | `UPDATE tasks SET retry_count=2 WHERE id='...'` |
| When | `GET /api/tasks/{id}` is called again | Response body includes `"retryCount": 2` |
| Then | `retryCount` in the API response reflects the current retry count from PostgreSQL | The field increments with each retry; it is not hardcoded or cached |
