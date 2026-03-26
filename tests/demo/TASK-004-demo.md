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

# Demo Script — TASK-004
**Feature:** Redis Streams queue infrastructure
**Requirement(s):** REQ-003, REQ-005, NFR-001, NFR-002
**Environment:** Local Docker Compose (staging environment with Redis accessible)

---

## Scenario 1: Task enqueued with tag "etl" appears in queue:etl stream
**REQ:** REQ-003, REQ-005

**Given:** Redis is running and empty (no streams or consumer groups exist)

**When:** Run `docker exec nexusflow-redis-1 redis-cli XGROUP CREATE queue:etl workers 0 MKSTREAM` to create the group, then run `docker exec nexusflow-redis-1 redis-cli XADD queue:etl "*" payload '{"taskId":"demo-task-001"}'`

**Then:** Run `docker exec nexusflow-redis-1 redis-cli XRANGE queue:etl - +` — one entry is returned containing the `payload` field with the task JSON; the stream key is exactly `queue:etl` (not `queue:demo-task-001` or any other key)

**Notes:** The `queue:{tag}` naming convention is defined in ADR-001. Run `docker exec nexusflow-redis-1 redis-cli XRANGE queue:report - +` to confirm that tag isolation works — that stream should have zero entries.

---

## Scenario 2: Consumer group "workers" is created automatically on first use
**REQ:** REQ-003, REQ-005

**Given:** Redis is running; `queue:etl` does not yet exist as a stream

**When:** Run the acceptance test for AC-2: `bash tests/acceptance/TASK-004-acceptance.sh` (or invoke `InitGroups` via any service startup)

**Then:** Run `docker exec nexusflow-redis-1 redis-cli XINFO GROUPS queue:etl` — the output lists one group named `workers` with `consumers 0` and `pending 0`; no manual `XGROUP CREATE` command was required

**Notes:** This verifies the service startup contract. If the group already existed, a second call to `InitGroups` returns nil (BUSYGROUP error is swallowed). Verify idempotency by running the startup call twice and confirming no error is returned.

---

## Scenario 3: XREADGROUP delivers an enqueued task to the consuming worker
**REQ:** REQ-003, REQ-005

**Given:** `queue:etl` has a `workers` consumer group; one task has been added via XADD

**When:** Run `docker exec nexusflow-redis-1 redis-cli XREADGROUP GROUP workers consumer1 COUNT 1 BLOCK 1000 STREAMS queue:etl ">"`

**Then:** The command returns one entry with the `payload` field; the entry ID matches the XADD return value; running `docker exec nexusflow-redis-1 redis-cli XPENDING queue:etl workers - + 10` shows exactly 1 pending entry assigned to `consumer1`

**Notes:** The `">"` argument means "deliver only new messages not yet delivered to this group". The consumer group tracks which messages each worker has received but not yet acknowledged.

---

## Scenario 4: XACK removes the task from the pending entry list
**REQ:** REQ-003

**Given:** A task has been delivered to `consumer1` via XREADGROUP and is in the pending entry list (from Scenario 3)

**When:** Copy the stream ID from the XREADGROUP output (format: `<timestamp>-<sequence>`) and run `docker exec nexusflow-redis-1 redis-cli XACK queue:etl workers <stream-id>`

**Then:** Run `docker exec nexusflow-redis-1 redis-cli XPENDING queue:etl workers - + 10` — the output shows 0 pending entries; the acknowledged task is no longer tracked as in-flight

**Notes:** The task entry itself remains in the stream (XRANGE still shows it); only the pending-entry-list record is removed. This is the at-least-once delivery contract: the message is preserved in the stream for replay if needed.

---

## Scenario 5: 1,000 sequential enqueues complete with p95 latency under 50ms
**REQ:** NFR-001

**Given:** Redis is running; the Go benchmark test is available in `internal/queue/redis_test.go`

**When:** Run `docker run --rm -v "$(pwd)":/app -w /app --network nexusflow_internal -e REDIS_ADDR=redis:6379 golang:1.24 go test ./internal/queue/... -bench=BenchmarkEnqueue_1000Sequential -benchtime=1x -run='^$' -v`

**Then:** The benchmark output includes a line like `redis_test.go:NNN: p95 latency = NNN µs (limit: 50ms)` where the measured value is well under 50ms; the `p95_ms` metric in the benchmark summary is 0 (sub-millisecond); the test exits with PASS (no `b.Errorf` fires)

**Notes:** The benchmark measures wall-clock time per `Enqueue` call (including consumer group creation on first use). The measured p95 of ~118µs is approximately 420x under the 50ms threshold, confirming the Redis Streams + go-redis implementation comfortably satisfies NFR-001 under local Docker conditions.

---

## Scenario 6: Enqueued tasks survive a Redis container restart
**REQ:** NFR-002

**Given:** Redis is configured with AOF+RDB hybrid persistence (`--appendonly yes --appendfsync everysec`) as confirmed in `docker-compose.yml`; 10 tasks are enqueued to `queue:etl`; 5 of those tasks have been read by a consumer (pending, not ACKed)

**When:** Note the current `XLEN queue:etl` count (should be 10), then run `docker compose -f docker-compose.yml restart redis` and wait for the container to return to healthy (`docker exec nexusflow-redis-1 redis-cli ping` returns `PONG`)

**Then:** Run `docker exec nexusflow-redis-1 redis-cli XLEN queue:etl` — the count is still 10; run `docker exec nexusflow-redis-1 redis-cli XRANGE queue:etl - +` — all 10 stream IDs are present and unchanged; run `docker exec nexusflow-redis-1 redis-cli XPENDING queue:etl workers - + 10` — the 5 unacknowledged pending entries are still listed under `consumer1`

**Notes:** The AOF journal replays on startup, recovering both the stream entries and the consumer group's pending-entry-list. This satisfies the "zero task loss" fitness function defined in ADR-001. The max loss window is 1 second (everysec flush policy) — tasks enqueued in the last second before a crash may need resubmission, but tasks that completed a round-trip XADD are guaranteed to survive.
