# Demo Script — TASK-009
**Feature:** Monitor service — heartbeat checking and failover
**Requirement(s):** REQ-004, REQ-013, REQ-011, ADR-002
**Environment:** staging — nexusflow.staging.nxlabs.cc (or local Docker Compose stack)

## Scenario 1: Worker heartbeat expiry detection
**REQ:** REQ-013, REQ-004

**Given:** The system is running with one healthy worker. The worker is registered and emitting heartbeats every 5 seconds. Confirm via PostgreSQL: `SELECT id, status FROM workers WHERE status='online';` returns one row.

**When:** Stop the worker process (pause the worker container or terminate the worker process) so that heartbeats stop reaching Redis. Wait 30 seconds (the Monitor's detection window: 15s heartbeat timeout + 10s scan interval per ADR-002).

**Then:** Query PostgreSQL: `SELECT id, status FROM workers WHERE id='<worker-id>';` The status must be `down`. The worker that was "online" before is now "down" — confirming AC-1.

**Notes:** The Monitor runs the heartbeat check on every scan cycle (every 10 seconds). Detection latency is at most 25 seconds from the last missed heartbeat. In this demo, wait 30 seconds to provide a comfortable margin.

---

## Scenario 2: Worker-down event published to Redis Pub/Sub
**REQ:** REQ-013

**Given:** Before stopping the worker, open a terminal and subscribe to the `events:workers` Redis Pub/Sub channel: `redis-cli SUBSCRIBE events:workers`

**When:** Stop the worker (as in Scenario 1) and wait 30 seconds for the Monitor to detect the expiry.

**Then:** The subscribed terminal receives a JSON message on `events:workers` containing the downed worker's ID and `"status":"down"`. This confirms AC-2: the Monitor publishes worker-down events for downstream consumers (SSE clients, dashboards).

**Notes:** The message payload is a JSON-encoded `models.Worker` struct. The `status` field will be `"down"`. The `id` field will match the worker ID from Scenario 1.

---

## Scenario 3: Pending task reclaimed via XCLAIM and re-queued
**REQ:** REQ-013, REQ-011

**Given:** A task is in the downed worker's XREADGROUP pending list with `retry_count=0`. The task's status in PostgreSQL is `running`. Confirm: `SELECT id, status, retry_count FROM tasks WHERE id='<task-id>';`

**When:** The Monitor runs its pending entry scanner (every 10 seconds). The entry has been idle for at least 15 seconds (the HeartbeatTimeout). The Monitor calls XCLAIM to transfer ownership, increments the retry counter, transitions the task to `queued`, re-enqueues via XADD, and XACKs the monitor's claimed entry.

**Then:** Query PostgreSQL: `SELECT status, retry_count FROM tasks WHERE id='<task-id>';` The status must be `queued` and `retry_count` must be `1`. This confirms AC-3 (reclaim) and AC-4 (retry counter increment).

**Notes:** The Monitor scans every 10 seconds. After the worker is detected as down, the next scan picks up the pending entry and routes it to `reclaimTask`. The XCLAIM + XADD sequence ensures the task is visible to healthy workers via `XREADGROUP ">"`.

---

## Scenario 4: Reclaimed task picked up by a healthy matching worker
**REQ:** REQ-013

**Given:** The task from Scenario 3 is now in `queued` state and the `queue:demo` stream has a new entry (the re-XADD from the Monitor). A healthy worker with the matching tag (`demo`) is running.

**When:** The healthy worker's `ReadTasks` loop calls `XREADGROUP ">"` and receives the re-queued task. The worker executes the pipeline and acknowledges the message.

**Then:** Query PostgreSQL: `SELECT status FROM tasks WHERE id='<task-id>';` The status must be `completed` (or `assigned`/`running` if the pipeline is still in progress). This confirms AC-5: the reclaimed task reached a healthy worker and was processed.

**Notes:** If running locally, restart the worker container after the pause to get a fresh worker process that has status `online` in PostgreSQL. The re-queued task is in the stream and will be consumed immediately on the next `XREADGROUP` poll.

---

## Scenario 5: Exhausted retries routed to dead-letter queue
**REQ:** REQ-013, REQ-011

**Given:** A task with `retry_count=3` and `max_retries=3` is in a worker's XREADGROUP pending list. The task's retries are fully exhausted. The task's status in PostgreSQL is `running`. Confirm: `SELECT status, retry_count, (retry_config->>'maxRetries')::int as max_retries FROM tasks WHERE id='<task-id>';`

**When:** The Monitor detects the worker is down and scans the pending entry. `processEntry` compares `retry_count (3) >= max_retries (3)` and routes to `deadLetterTask` instead of `reclaimTask`.

**Then:**
1. Query PostgreSQL: `SELECT status FROM tasks WHERE id='<task-id>';` The status must be `failed`.
2. Query Redis: `XRANGE queue:dead-letter - +` The output must contain an entry with `taskId` equal to the task ID and a `reason` field explaining the dead-letter routing.

This confirms AC-6: exhausted-retry tasks are moved to `queue:dead-letter` and marked `failed` — not re-queued.

**Notes:** The `queue:dead-letter` stream is a standard Redis Stream (XADD). Downstream consumers can read it via XREADGROUP for dead-letter inspection or alerting. The Sink Inspector and TASK-011 (dead-letter + cascading cancellation) build on this foundation.

---
