<!--
Copyright 2026 Pablo Ochendrowitsch

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
-->

# Demo Script — TASK-006
**Feature:** Worker self-registration and heartbeat
**Requirement(s):** REQ-004, ADR-002
**Environment:** Staging — Docker Compose stack with PostgreSQL and Redis running

---

## Scenario 1: Worker registers with capability tags on startup
**REQ:** REQ-004

**Given:** The PostgreSQL and Redis services are running. No workers are registered (workers table is empty or contains only previously demo'd entries).

**When:** A worker process starts with `WORKER_TAGS=etl,http` and a generated or explicit `WORKER_ID`. The worker connects to PostgreSQL and Redis, then calls Register on startup.

**Then:** Within 3 seconds of the worker process starting, run:

```sql
SELECT id, status, tags, registered_at FROM workers ORDER BY registered_at DESC LIMIT 5;
```

The most recent row shows:
- `status` = `online`
- `tags` = `{etl,http}`
- `registered_at` is within the last few seconds (not null, not epoch)

**Notes:** The worker does not expose an HTTP API. Verification is done by querying PostgreSQL directly. If running via Docker Compose, use `docker exec nexusflow-postgres-1 psql -U nexusflow -d nexusflow -c "SELECT ..."`.

---

## Scenario 2: Heartbeat updates Redis sorted set every 5 seconds
**REQ:** REQ-004, ADR-002

**Given:** The worker from Scenario 1 is still running. The `workers:active` sorted set in Redis contains the worker's entry.

**When:** Note the current Redis score for this worker's entry, then wait 6 seconds:

```
redis-cli ZSCORE workers:active <worker-id>
# wait 6 seconds
redis-cli ZSCORE workers:active <worker-id>
```

**Then:** The second score is strictly greater than the first. The delta is approximately 5 (Unix timestamp increments by ~5 seconds per heartbeat tick). This confirms the heartbeat goroutine is running at the configured 5-second interval (ADR-002).

**Notes:** The score is a Unix timestamp (seconds since epoch), not a TTL. The Monitor uses `ZRANGEBYSCORE workers:active -inf <now-15>` to find workers that have not heartbeated in 15 seconds.

---

## Scenario 3: Multiple workers register simultaneously with different tags
**REQ:** REQ-004

**Given:** PostgreSQL and Redis are running. No workers registered from the current demo session.

**When:** Start two worker processes concurrently, each with a different `WORKER_ID` and `WORKER_TAGS`:
- Worker A: `WORKER_ID=demo-worker-a`, `WORKER_TAGS=report,batch`
- Worker B: `WORKER_ID=demo-worker-b`, `WORKER_TAGS=ml,gpu`

After 4 seconds, run:

```sql
SELECT id, status, tags FROM workers WHERE id IN ('demo-worker-a', 'demo-worker-b') ORDER BY id;
```

And in Redis:

```
redis-cli ZRANGE workers:active 0 -1 WITHSCORES
```

**Then:** The SQL query returns two rows — one for each worker — each with `status=online` and their respective distinct tags. Both workers appear in the Redis sorted set with current timestamp scores.

**Notes:** Via Docker Compose, start two workers: `docker compose up --scale worker=2 -d`. Each worker generates its own ID from hostname+UUID suffix, ensuring uniqueness even on the same host.

---

## Scenario 4: Graceful shutdown transitions worker status to "down"
**REQ:** REQ-004, ADR-002

**Given:** Worker A from Scenario 3 is running with `status=online` in the database.

**When:** Send SIGTERM to Worker A (via `docker stop <container>` or `kill <pid>`). Wait 3 seconds for the graceful shutdown to complete.

Then run:

```sql
SELECT id, status FROM workers WHERE id = 'demo-worker-a';
```

**Then:** The row shows `status=down`. This confirms that `markOffline` ran during graceful shutdown, allowing the Monitor to distinguish an intentional stop from a crashed worker (ADR-002). Worker B continues to show `status=online` — the shutdown of A does not affect B.

**Notes:** The worker logs "received signal terminated — shutting down" followed by "stopped cleanly" when SIGTERM is handled correctly. The 5-second shutdown timeout context ensures the PostgreSQL write completes even under slow network conditions.

---
