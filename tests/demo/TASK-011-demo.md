---
task: TASK-011
title: Dead letter queue with cascading cancellation
requirement: REQ-012, REQ-014
status: PASS
verified: 2026-03-29
smoke: false
---

# Demo Script — TASK-011: Dead letter queue with cascading cancellation

**Purpose:** Demonstrate that NexusFlow routes tasks that exhaust retries to the `queue:dead-letter` stream with status "failed", and that when a task in a pipeline chain fails, all downstream tasks in that chain are cancelled with the reason "upstream task failed" — while standalone tasks and tasks in other chains are not affected.

**Prerequisites:**
- All services running (`docker compose up -d`)
- Admin credentials available (`admin` / `admin`)
- `docker exec` access to `nexusflow-postgres-1` and `nexusflow-redis-1`
- `curl`, `jq` available in the terminal session

---

## Scenario 1: Task exhausting retries appears in queue:dead-letter

**Acceptance Criterion:** AC-1 — Task exhausting retries appears in `queue:dead-letter` stream.

| Step | Action | Expected Result |
|---|---|---|
| Given | Obtain an auth token: `curl -s -X POST http://localhost:8080/api/auth/login -H 'Content-Type: application/json' -d '{"username":"admin","password":"admin"}' \| jq -r '.token'` | JWT token returned |
| Given | Create a pipeline via `POST /api/pipelines` with a demo source and demo sink phase | HTTP 201; pipeline ID obtained |
| Given | Submit a task for that pipeline with `retryConfig: {maxRetries: 0, backoff: "exponential"}` and `tags: ["demo"]` via `POST /api/tasks` | HTTP 201; task ID obtained |
| When | The worker container is paused: `docker pause nexusflow-worker-1` causing heartbeats to stop for >15 seconds | Monitor detects the worker as down; task has max_retries=0 so retry count is already at limit |
| Then | After ~30 seconds (15s timeout + 10s scan + 5s margin), the task status in PostgreSQL is "failed": `docker exec nexusflow-postgres-1 psql -U nexusflow -d nexusflow -c "SELECT status FROM tasks WHERE id='<task-id>'"` | Returns `failed` |
| Then | The task ID appears in `queue:dead-letter`: `docker exec nexusflow-redis-1 redis-cli XRANGE queue:dead-letter - +` | Stream output contains the task UUID |
| Then | Unpause the worker: `docker unpause nexusflow-worker-1` | Worker resumes |

---

## Scenario 2: Pipeline chain A→B→C — dead-lettering A cascades cancellation to B and C

**Acceptance Criterion:** AC-2 — Pipeline chain A→B→C: when task A enters the dead letter queue, tasks B and C are cancelled with reason "upstream task failed".

| Step | Action | Expected Result |
|---|---|---|
| Given | Create three pipelines (A, B, C) via `POST /api/pipelines` | Three pipeline IDs obtained |
| Given | Create a chain linking A→B→C via `POST /api/chains` with body `{"name":"demo-chain","pipelineIds":["<A>","<B>","<C>"]}` | HTTP 201; chain ID returned |
| Given | Submit task A with `pipelineId: <A>`, `retryConfig: {maxRetries: 0}`, and `tags: ["demo"]` | HTTP 201; task A ID obtained |
| Given | Submit task B with `pipelineId: <B>`, `retryConfig: {maxRetries: 3}`, and `tags: ["demo"]` | HTTP 201; task B ID obtained |
| Given | Submit task C with `pipelineId: <C>`, `retryConfig: {maxRetries: 3}`, and `tags: ["demo"]` | HTTP 201; task C ID obtained |
| Given | Wait for task A to reach `running` status: `GET /api/tasks/<A-id>` returns `status: "running"` | Task A assigned to a worker |
| When | Pause the worker: `docker pause nexusflow-worker-1` | Worker heartbeats stop; task A is pending with exhausted retries |
| When | Wait ~30 seconds for monitor detection and cascade processing | Monitor marks task A failed, dead-letters it, and triggers cascade |
| Then | Task A status is "failed": `SELECT status FROM tasks WHERE id='<A-id>'` | Returns `failed` |
| Then | Task A appears in `queue:dead-letter`: `redis-cli XRANGE queue:dead-letter - +` | Contains task A's UUID |
| Then | Task B status is "cancelled": `SELECT status FROM tasks WHERE id='<B-id>'` | Returns `cancelled` |
| Then | Task C status is "cancelled": `SELECT status FROM tasks WHERE id='<C-id>'` | Returns `cancelled` |
| Then | Cancellation reason for B and C is "upstream task failed": `SELECT reason FROM task_state_log WHERE task_id='<B-id>' AND to_status='cancelled'` | Returns `upstream task failed` |
| Then | Unpause the worker: `docker unpause nexusflow-worker-1` | Worker resumes |

**Notes:** If pipeline B's task has not yet been submitted when pipeline A fails (chain trigger has not fired), there is no downstream task to cancel for B — this is expected and documented as a known limitation. The cascade cancels whatever non-terminal tasks exist at the moment of dead-lettering.

---

## Scenario 3: Standalone task enters DLQ without cascading cancellation

**Acceptance Criterion:** AC-3 — Standalone task (not in a chain) enters the dead letter queue without cascading cancellation.

| Step | Action | Expected Result |
|---|---|---|
| Given | Create a pipeline that is NOT added to any chain via `POST /api/pipelines` | Pipeline ID obtained; no chain membership |
| Given | Create a second "bystander" pipeline (also standalone) and submit a task for it with `retryConfig: {maxRetries: 3}` | Task bystander ID obtained; task in non-terminal state |
| Given | Submit a task for the standalone pipeline with `retryConfig: {maxRetries: 0}` and `tags: ["demo"]` | Standalone task ID obtained |
| Given | Wait for the standalone task to reach `running` status | Task assigned to a worker |
| When | Pause the worker: `docker pause nexusflow-worker-1` | Worker heartbeats stop |
| When | Wait ~30 seconds for monitor detection | Monitor dead-letters the standalone task |
| Then | Standalone task status is "failed": `SELECT status FROM tasks WHERE id='<standalone-id>'` | Returns `failed` |
| Then | Standalone task appears in `queue:dead-letter` | Redis XRANGE output contains standalone task UUID |
| Then | Bystander task is NOT cancelled: `SELECT status FROM tasks WHERE id='<bystander-id>'` | Returns its original non-terminal status (submitted, queued, or running) — NOT cancelled |
| Then | Unpause the worker: `docker unpause nexusflow-worker-1` | Worker resumes |

---

## Scenario 4: Dead letter tasks are visible via the task API with status "failed"

**Acceptance Criterion:** AC-4 — Dead letter tasks are visible via the task API with status "failed".

| Step | Action | Expected Result |
|---|---|---|
| Given | At least one task has been dead-lettered (e.g. from Scenario 1 or 2 above) | Task ID of a dead-lettered task available |
| When | Call `GET /api/tasks?status=failed` with admin auth header | HTTP 200 |
| Then | The dead-lettered task ID appears in the response: `curl -s http://localhost:8080/api/tasks?status=failed -H 'Authorization: Bearer <token>' \| jq '.tasks[].id'` | Task UUID present in the list |
| When | Call `GET /api/tasks/<task-id>` for the dead-lettered task | HTTP 200 |
| Then | Response body contains `"status": "failed"` | `jq '.task.status'` returns `"failed"` |
| Then | Calling `GET /api/tasks?status=completed` does NOT include the dead-lettered task ID | Dead-lettered task is not mis-classified as completed |
