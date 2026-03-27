---
task: TASK-008
title: Task lifecycle state tracking and query API
result: PASS
date: 2026-03-27
requirements: REQ-009, REQ-017
environment: staging API at https://api.nexusflow.staging (or http://localhost:8080 for local)
---

# Demo Script — TASK-008
**Feature:** Task lifecycle state tracking and query API
**Requirement(s):** REQ-009, REQ-017
**Environment:** Staging API server. Admin credentials: username `admin`, password `admin`. Requires `curl` and `docker exec` access.

---

## Setup: obtain admin session token

```bash
curl -s -X POST http://localhost:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin"}'
# Response: {"token":"<64-char-hex>","expiresAt":"..."}
# Copy the token value for use in all subsequent requests as ADMIN_TOKEN.
```

Then submit a task so there is data to query:

```bash
# Insert a pipeline (or reuse one from TASK-005 / TASK-013)
PIPELINE_ID=<uuid-from-pipelines-table>

curl -s -X POST http://localhost:8080/api/tasks \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d "{\"pipelineId\":\"$PIPELINE_ID\",\"tags\":[\"etl\"],\"input\":{\"key\":\"demo\"}}"
# Response: {"taskId":"<uuid>","status":"queued"}
# Copy the taskId as TASK_ID.
```

---

## Scenario 1: Admin sees all tasks via GET /api/tasks
**REQ:** REQ-017

**Given:** the admin is authenticated and at least one task has been submitted

**When:**
```bash
curl -s http://localhost:8080/api/tasks \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

**Then:** the response status is 200 OK; the body is a JSON array (never null); the array contains an object with `"id"` matching the submitted task ID; each object includes `"status"`, `"pipelineId"`, `"userId"`, and other task fields

**Notes:** As admin, tasks from all users are returned. If multiple users have submitted tasks, all are present in the array.

---

## Scenario 2: Regular user sees only own tasks (visibility isolation)
**REQ:** REQ-017

**Given:** a regular user (non-admin) is authenticated; that user has submitted zero tasks; admin has submitted at least one task

**When:**
```bash
curl -s http://localhost:8080/api/tasks \
  -H "Authorization: Bearer $USER_TOKEN"
```

**Then:** the response status is 200 OK; the body is a JSON array; the array does NOT contain the admin's task; if the user has no tasks, the array is `[]` (empty array, not 403 or null)

**Notes:** Visibility isolation (Domain Invariant 5) — users can only see tasks they submitted. The list endpoint never returns 403 for a user with no matching tasks.

---

## Scenario 3: GET /api/tasks/{id} returns full task detail including current status
**REQ:** REQ-009

**Given:** the admin is authenticated and a task ID is known (from setup)

**When:**
```bash
curl -s http://localhost:8080/api/tasks/$TASK_ID \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

**Then:** the response status is 200 OK; the body is a JSON object with two top-level keys: `"task"` (the full task record) and `"stateHistory"` (an array); `"task"` contains `"id"` matching `$TASK_ID`, a `"status"` field (e.g. `"queued"`, `"running"`, or `"completed"` depending on worker state), and other task fields such as `"pipelineId"`, `"userId"`, `"retryConfig"`

---

## Scenario 4: GET /api/tasks/{id} includes state transition history from task_state_log
**REQ:** REQ-009

**Given:** a task that has been submitted (and therefore has at least the submitted→queued transition in `task_state_log`)

**When:** (same request as Scenario 3)
```bash
curl -s http://localhost:8080/api/tasks/$TASK_ID \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

**Then:** the `"stateHistory"` array in the response is non-empty; each entry includes `"fromState"`, `"toState"`, `"reason"`, and `"timestamp"` fields; the first entry shows `"fromState":"submitted"` and `"toState":"queued"`; if the worker has picked up and executed the task, additional transitions (queued→assigned→running→completed) are also present

**Notes:** To cross-check: `SELECT from_state, to_state, reason, timestamp FROM task_state_log WHERE task_id = '$TASK_ID' ORDER BY timestamp;`

---

## Scenario 5: Unauthenticated requests are rejected with 401
**REQ:** REQ-017

**Given:** no Authorization header is provided

**When:**
```bash
# List endpoint
curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/api/tasks

# Detail endpoint
curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/api/tasks/$TASK_ID
```

**Then:** both commands print `401`; the response body for each is `{"error":"unauthorized"}`

---

## Scenario 6: Non-owner non-admin gets 403 on GET /api/tasks/{id}
**REQ:** REQ-017

**Given:** a regular user (USER_TOKEN) who did NOT submit the task; the task was submitted by admin

**When:**
```bash
curl -s -o /tmp/demo008-forbidden.json -w "%{http_code}" \
  http://localhost:8080/api/tasks/$TASK_ID \
  -H "Authorization: Bearer $USER_TOKEN"
# Print response: cat /tmp/demo008-forbidden.json
```

**Then:** the status is 403 Forbidden; the body is `{"error":"forbidden"}`; the response body contains NO task data — the task ID, pipeline ID, and status are not disclosed

**Notes:** The admin can read the same task and receive 200 (verify with `$ADMIN_TOKEN` for positive contrast).
