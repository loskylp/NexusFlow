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

# Demo Script — TASK-014
**Feature:** Pipeline chain definition
**Requirement(s):** REQ-014
**Environment:** Staging API server. Admin credentials: username `admin`, password `admin`. Requires `curl` and `docker exec` access to the PostgreSQL and Redis containers.

---

## Scenario 1: Create a linear chain of three pipelines
**REQ:** REQ-014

**Given:** you are logged in as admin; three "demo" pipelines (A, B, C) exist in the system

**When:** you call `POST /api/chains` with body `{"name":"demo-chain","pipelineIds":["<A-id>","<B-id>","<C-id>"]}` using a valid Bearer token

**Then:** the response is `201 Created`; the body contains a JSON object with `id` (UUID), `name` equal to `"demo-chain"`, and `pipelineIds` as an array with all three pipeline IDs in the order supplied

---

## Scenario 2: Branching structure is rejected
**REQ:** REQ-014

**Given:** you are logged in as admin; at least two pipelines (A, B) exist

**When:** you call `POST /api/chains` with a body where pipeline A appears twice in `pipelineIds`, e.g. `{"name":"branching-chain","pipelineIds":["<A-id>","<B-id>","<A-id>"]}`

**Then:** the response is `400 Bad Request`; no chain record appears in the `chains` table in the database

---

## Scenario 3: Retrieve chain definition with pipeline ordering
**REQ:** REQ-014

**Given:** a chain was created in Scenario 1 with ID `<chain-id>` and ordered pipelines [A, B, C]

**When:** you call `GET /api/chains/<chain-id>` with a valid Bearer token

**Then:** the response is `200 OK`; the body contains `id` equal to `<chain-id>`, `name` equal to `"demo-chain"`, and `pipelineIds` as a three-element array in the same order [A, B, C] as submitted; querying `chain_steps` in the database confirms position 0 = A, position 1 = B, position 2 = C

---

## Scenario 4: Chain trigger auto-submits next pipeline task on completion
**REQ:** REQ-014

**Given:** a two-pipeline chain A->B exists; no tasks for pipeline B exist yet

**When:** you submit a task for pipeline A via `POST /api/tasks` with `{"pipelineId":"<A-id>","tags":["demo"],"input":{}}` and wait for the worker to execute and complete it

**Then:** within a few seconds of pipeline A's task reaching `completed` status, a new task row for pipeline B appears in the `tasks` table with status `queued`; the worker will then pick it up and execute it

**Notes:** Verify pipeline A task status via `docker exec nexusflow-postgres-1 psql -U nexusflow -d nexusflow -c "SELECT status FROM tasks WHERE id='<task-A-id>';"` and pipeline B task via `SELECT id, status, pipeline_id FROM tasks WHERE pipeline_id='<B-id>';`

---

## Scenario 5: Idempotency — duplicate completion event creates no extra downstream task
**REQ:** REQ-014

**Given:** the chain trigger fired for pipeline A's task (Scenario 4), creating one task for pipeline B; the Redis key `chain-trigger:<task-A-id>:<B-id>` is set

**When:** you inspect the idempotency key in Redis: `docker exec nexusflow-redis-1 redis-cli EXISTS "chain-trigger:<task-A-id>:<B-id>"` — it returns `1`; then you attempt `SET "chain-trigger:<task-A-id>:<B-id>" "test" NX EX 86400` — it returns `(nil)` meaning the key is already held

**Then:** the SET-NX call returns nil (already held), confirming the idempotency guard is active; the task count for pipeline B remains at 1 — no duplicate task was created

**Notes:** The idempotency key format is `chain-trigger:{taskID}:{nextPipelineID}` with a 24-hour TTL (ADR-003).
