---
task: TASK-005
title: Task submission via REST API
result: PASS
date: 2026-03-26
requirements: REQ-001, REQ-003, REQ-009, REQ-019
environment: staging API at https://api.nexusflow.staging (or http://localhost:8080 for local)
---

# Demo Script — TASK-005
**Feature:** Task submission via REST API
**Requirement(s):** REQ-001, REQ-003, REQ-009, REQ-019
**Environment:** Staging API server. Admin credentials: username `admin`, password `admin`. Requires `curl` and access to `docker exec` for database/Redis inspection.

---

## Scenario 1: Authenticated task submission returns 201 with a unique task ID
**REQ:** REQ-001

**Given:** the API server is running and you have logged in as admin to obtain a session token (see setup below); a test pipeline exists in the system (inserted directly into PostgreSQL if the Pipeline CRUD feature is not yet deployed)

**When:** POST /api/tasks with `Authorization: Bearer <token>` and body:
```json
{"pipelineId": "<valid-pipeline-uuid>", "tags": ["etl"], "input": {"key": "value"}}
```

**Then:** the response status is 201 Created; the body contains a `taskId` field that is a valid UUID (e.g. `e91595ef-dfe9-44d2-a7b0-d5e0a5b46e1d`) and a `status` field equal to `"queued"`; submitting the same payload a second time produces a different `taskId`

**Notes:** The pipeline UUID must exist in the `pipelines` table. If Pipeline CRUD is not deployed, insert a row directly: `INSERT INTO pipelines (name, user_id, data_source_config, process_config, sink_config) VALUES ('demo', '<admin-user-id>', '{"connectorType":"demo","config":{},"outputSchema":["f"]}', '{"connectorType":"demo","config":{},"inputMappings":[],"outputSchema":["f"]}', '{"connectorType":"demo","config":{},"inputMappings":[]}') RETURNING id;`

---

## Scenario 2: Task record persisted in PostgreSQL with status "queued" and audit log entry
**REQ:** REQ-003, REQ-009

**Given:** a task was just submitted and its `taskId` is known (from Scenario 1)

**When:** query the `tasks` table — `SELECT status FROM tasks WHERE id = '<taskId>';` — and query `task_state_log` — `SELECT from_state, to_state, reason FROM task_state_log WHERE task_id = '<taskId>';`

**Then:** `tasks.status` is `queued`; `task_state_log` contains exactly one row with `from_state = 'submitted'`, `to_state = 'queued'`, and a non-empty `reason` (expected: `"enqueued to Redis stream"`)

---

## Scenario 3: Task message appears in the correct Redis stream for its tag
**REQ:** REQ-003

**Given:** a task was submitted with `"tags": ["etl"]` and a second task with `"tags": ["report"]`

**When:** run `XRANGE queue:etl - +` and `XRANGE queue:report - +` in redis-cli (via `docker exec nexusflow-redis-1 redis-cli XRANGE queue:etl - +`)

**Then:** the first task's ID appears in the `queue:etl` stream entries; the second task's ID appears in `queue:report`; the second task's ID does NOT appear in `queue:etl`

**Notes:** Stream key format is `queue:{tag}`. Each stream entry is a Redis hash with the task payload including the task ID.

---

## Scenario 4: Invalid pipeline reference is rejected with a structured error
**REQ:** REQ-001

**Given:** an authenticated admin session

**When:** POST /api/tasks with a well-formed UUID that does not exist in the `pipelines` table:
```json
{"pipelineId": "00000000-dead-beef-cafe-000000000000", "tags": ["etl"]}
```

**Then:** the response status is 400 Bad Request; the body is `{"error":"pipeline not found"}`

**Notes:** Also verify that a non-UUID string as `pipelineId` (e.g. `"not-a-uuid"`) returns 400, and that a completely malformed JSON body returns 400.

---

## Scenario 5: Task submitted without retry configuration receives safe defaults
**REQ:** REQ-001

**Given:** an authenticated admin session and a valid pipeline

**When:** POST /api/tasks with no `retryConfig` field:
```json
{"pipelineId": "<valid-pipeline-uuid>", "tags": ["etl"], "input": {}}
```

**Then:** the response is 201; querying the `tasks` table for `retry_config` on the new task ID shows `{"backoff":"exponential","maxRetries":3}`

**Notes:** To verify: `SELECT retry_config FROM tasks WHERE id = '<taskId>';`

---

## Scenario 6: Unauthenticated request is rejected with JSON 401
**REQ:** REQ-019

**Given:** no Authorization header or session cookie is included in the request

**When:** POST /api/tasks with a valid body but without any `Authorization` header

**Then:** the response status is 401 Unauthorized; the body is `{"error":"unauthorized"}` (structured JSON, not plain text); `Content-Type` is `application/json`

**Notes:** Also verify that a syntactically valid but non-existent token (e.g. 64 hex chars that were never issued) also returns 401 rather than 400 or 500.

---

## Setup: obtain admin session token

```bash
curl -s -X POST http://localhost:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin"}'
# Response: {"token":"<64-char-hex>","expiresAt":"..."}
# Copy the token value for use in all subsequent requests.
```
