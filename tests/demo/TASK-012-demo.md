# Demo Script — TASK-012
**Feature:** Task cancellation — POST /api/tasks/{id}/cancel
**Requirement(s):** REQ-010
**Environment:** Staging — API at https://staging.nexusflow.io (or `http://localhost:8080` for local)

## Scenario 1: Task owner cancels their own queued task
**REQ:** REQ-010

**Given:** You are logged in as a regular user who owns at least one task in "queued" status. Note the task ID from the task list.

**When:** Send `POST /api/tasks/{taskId}/cancel` with your Bearer token in the Authorization header.

**Then:** The response is `204 No Content`. Fetching `GET /api/tasks/{taskId}` shows `"status": "cancelled"`.

---

## Scenario 2: Admin cancels another user's task
**REQ:** REQ-010

**Given:** You are logged in as an admin. A task owned by a different user exists in "queued" or "running" status. Note the task ID.

**When:** Send `POST /api/tasks/{taskId}/cancel` with the admin Bearer token.

**Then:** The response is `204 No Content`. The task status in the system is now "cancelled", regardless of ownership.

---

## Scenario 3: Non-owner non-admin cancel attempt is rejected
**REQ:** REQ-010

**Given:** You are logged in as a regular user. A task owned by a different user exists in "queued" status.

**When:** Send `POST /api/tasks/{taskId}/cancel` using your Bearer token (you are not the owner, not an admin).

**Then:** The response is `403 Forbidden` with a structured JSON error body. The task status remains unchanged ("queued").

---

## Scenario 4: Cancelling a completed task returns 409
**REQ:** REQ-010

**Given:** A task exists in "completed" status. You are authenticated as admin or owner.

**When:** Send `POST /api/tasks/{taskId}/cancel`.

**Then:** The response is `409 Conflict`. The task status remains "completed". Repeat for "failed" and "cancelled" tasks — all return 409.

---

## Scenario 5: Cancelling a running task sets the Redis cancel flag
**REQ:** REQ-010

**Given:** A task is in "running" status (actively being processed by the worker).

**When:** Send `POST /api/tasks/{taskId}/cancel` as the owner or admin.

**Then:** The response is `204 No Content`. The task status transitions to "cancelled". The worker stops processing at the next cancellation checkpoint and does not write output to the Sink.

**Notes:** In staging, you can observe the worker's log stream for a log line indicating cancellation was detected. The Redis cancel flag (`cancel:{taskId}`) is set transiently and expires within 60 seconds after the worker acknowledges it.

---

## Scenario 6: Cancellation is recorded in the state log
**REQ:** REQ-010

**Given:** A task in "queued" status.

**When:** The owner sends `POST /api/tasks/{taskId}/cancel` and receives 204.

**Then:** A state log entry exists for this task recording the transition from the previous state to "cancelled". This confirms the cancellation is durable and auditable. The entry includes the reason string "cancelled by user".

**Notes:** The state log is accessible via the internal DB or future audit endpoints. This scenario confirms that the Cancel path uses the same transactional log-write as all other status transitions.
