# Demo Script — TASK-015
**Feature:** SSE event infrastructure — real-time task, worker, and log streaming
**Requirement(s):** REQ-016, REQ-017, REQ-018, NFR-003, ADR-007
**Environment:** Staging API — `https://nexusflow.staging.nxlabs.cc` (substitute actual staging URL)

---

## Scenario 1: Task state change events stream in real-time to admin
**AC:** AC-1 — SSE endpoint streams task state change events in real-time

**Given:** You are logged in as admin. Obtain a session token from `POST /api/auth/login` with `{"username":"admin","password":"admin"}`. Note the `token` value as `<admin-token>`.

Open a terminal and start an SSE stream to the task feed:
```
curl -N -H "Authorization: Bearer <admin-token>" \
     https://nexusflow.staging.nxlabs.cc/events/tasks
```
Leave this terminal open. The connection is established but no events arrive yet.

**When:** In a second terminal, submit a task via `POST /api/tasks` with a valid pipeline ID and `"tags": ["demo"]`:
```json
{
  "pipelineId": "<pipeline-id>",
  "input": {},
  "tags": ["demo"]
}
```

**Then:** Within 2 seconds, the first terminal receives an SSE event of the form:
```
event: task:created
data: {"task": {...}, "reason": "submitted"}
```
As the worker picks up and processes the task, further events appear: `task:state-changed` (status `assigned`, then `running`) and eventually `task:completed` or `task:failed`. Each event arrives within 2 seconds of the state change (NFR-003).

The response headers on the stream include `Content-Type: text/event-stream`, `Cache-Control: no-cache`, and `X-Accel-Buffering: no`.

**Notes:** Opening the stream without an `Authorization` header returns `401 Unauthorized` immediately (no stream opened).

---

## Scenario 2: Events are filtered by user role
**AC:** AC-2 — Events filtered by user (users see own tasks, admins see all)

**Given:** You are logged in as admin (`<admin-token>`) and as a non-admin user (`<user-token>`). Open two terminals, each with an SSE connection:
- Terminal A (admin): `curl -N -H "Authorization: Bearer <admin-token>" .../events/tasks`
- Terminal B (user): `curl -N -H "Authorization: Bearer <user-token>" .../events/tasks`

**When:** Submit a task as the admin. Submit a separate task as the non-admin user.

**Then:** Terminal A (admin) receives events for both tasks. Terminal B (non-admin user) receives only the event for their own task. The admin's task event does not appear in Terminal B.

**Notes:** This behavior is enforced by channel routing: admins subscribe to `events:tasks:all`; users subscribe to `events:tasks:{userId}`. No per-event filtering is required in the handler — the channel key determines scope at the Redis Pub/Sub level.

---

## Scenario 3: Worker fleet events stream to all authenticated users
**AC:** AC-3 — Worker fleet events stream to all authenticated users

**Given:** You are logged in as any authenticated user (admin or regular user). Open an SSE connection to the worker feed:
```
curl -N -H "Authorization: Bearer <token>" \
     https://nexusflow.staging.nxlabs.cc/events/workers
```

**When:** A worker connects to the system (starts up), sends a heartbeat, or goes offline.

**Then:** The SSE stream delivers a `worker:heartbeat` or `worker:down` event within 2 seconds. Both admin and non-admin users receive the same worker events. The event payload includes the worker ID and status.

**Notes:** Opening `/events/workers` without authentication returns `401 Unauthorized`.

---

## Scenario 4: Log streaming delivers log lines within 2 seconds (NFR-003)
**AC:** AC-4 — Log streaming endpoint delivers logs within 2 seconds of production

**Given:** A task is running (status `running`) with task ID `<task-id>`. You are logged in as the task owner or as admin. Open an SSE log stream:
```
curl -N -H "Authorization: Bearer <token>" \
     https://nexusflow.staging.nxlabs.cc/events/tasks/<task-id>/logs
```

**When:** The worker produces a log line (processing output, debug info).

**Then:** The SSE stream delivers an event of the form:
```
id: <uuid>
event: log:line
data: {"taskId": "<task-id>", "line": "...", "level": "info", "timestamp": "..."}
```
The event arrives within 2 seconds of the worker publishing it (NFR-003). Each `log:line` event includes an `id:` field (the log entry's UUID) for Last-Event-ID reconnection.

**Notes:** A non-owner non-admin user requesting this stream receives `403 Forbidden` immediately.

---

## Scenario 5: Unauthorized access to log and sink streams returns 403
**AC:** AC-6 — Unauthorized log/sink access returns 403

**Given:** User A owns task `<task-id-a>`. User B (non-admin) is logged in with their token.

**When:** User B requests `GET /events/tasks/<task-id-a>/logs` (User A's task):
```
curl -N -H "Authorization: Bearer <user-b-token>" \
     https://nexusflow.staging.nxlabs.cc/events/tasks/<task-id-a>/logs
```

**Then:** The response is `403 Forbidden`. No SSE stream is opened. The same behavior applies to `GET /events/sink/<task-id-a>` — User B receives 403.

**When:** User A requests `GET /events/tasks/<task-id-a>/logs` (their own task):

**Then:** The response opens the SSE stream (`200 text/event-stream`). Admin can access any task's log or sink stream regardless of ownership.

**Notes:** The fail-closed design means a non-parseable task ID also returns 403 (task existence must not leak to unauthorised callers).

---

## Scenario 6: Last-Event-ID reconnection (structural — requires TASK-016)
**AC:** AC-5 — Last-Event-ID reconnection replays missed log events

**Given:** A client was previously connected to `GET /events/tasks/<task-id>/logs` and received events with IDs. The client disconnects and reconnects, sending the last received event ID in the `Last-Event-ID` header:
```
curl -N -H "Authorization: Bearer <token>" \
     -H "Last-Event-ID: <last-received-uuid>" \
     https://nexusflow.staging.nxlabs.cc/events/tasks/<task-id>/logs
```

**When:** The server receives the reconnect with `Last-Event-ID`.

**Then:** The server replays any log lines produced after `<last-received-uuid>` from the database before switching to the live stream. The client receives all missed lines followed by new live events. The stream does not return 500 on reconnect — it opens cleanly.

**Notes:** This scenario requires TASK-016 (log persistence) to be completed. Until then, the replay section is a no-op (no lines are replayed) but the live stream opens normally. The structural plumbing (header extraction, `replayLogs` call, `b.logs` guard) is in place and verified.
