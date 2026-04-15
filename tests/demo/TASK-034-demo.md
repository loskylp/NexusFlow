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

# Demo Script — TASK-034
**Feature:** Chaos Controller GUI — worker kill/pause/resume + DB disconnect controls with admin enforcement
**Requirement(s):** DEMO-004, UX Spec (Chaos Controller)
**Environment:** Staging — https://nexusflow.staging (or local: http://localhost:3000 with `docker compose --profile demo up`)

---

## Scenario 1: Admin login and navigation to Chaos Controller
**REQ:** DEMO-004

**Given:** you are on the `/login` page; you have Admin credentials (default seed: username `admin`, password `admin`); the demo stack is running with at least one worker and one pipeline

**When:** you log in as admin and click "Chaos Controller" in the left sidebar under the "DEMO" label

**Then:** the browser navigates to `/demo/chaos`; the page header shows "Chaos Controller" in 24px bold text; a purple "DEMO" badge and a red "DESTRUCTIVE" badge appear next to the title; a coloured dot with label "System status: Nominal" (green dot) is visible below the title line; three action cards are stacked below: "Kill Worker", "Disconnect Database", and "Flood Queue"; each card shows a description, its controls, and an empty activity log reading "No activity yet."

---

## Scenario 2: Access denied for User-role accounts
**REQ:** DEMO-004, Task Plan AC-5

**Given:** you are logged out (clear the session cookie via DevTools → Application → Cookies); you have a User-role account (not admin)

**When:** you log in as the user-role account and navigate directly to `/demo/chaos`

**Then:** the page shows "Access Denied" in heading text and "The Chaos Controller is only available to admin users." below it; none of the three action cards (Kill Worker, Disconnect Database, Flood Queue), the DEMO badge, the DESTRUCTIVE badge, or the system status dot are visible; the "Chaos Controller" link is also absent from the sidebar (the DEMO section is hidden for User-role sessions)

**Notes:** To verify the sidebar is also filtered: after logging in as user, inspect the left sidebar — the "DEMO" section heading and both "Sink Inspector" and "Chaos Controller" links must be absent.

---

## Scenario 3: System status indicator reflects health
**REQ:** DEMO-004, Task Plan AC-4

**Given:** you are logged in as admin and on `/demo/chaos`; the demo stack is running normally

**When:** you observe the system status indicator below the page title

**Then:** the dot is green and the label reads "System status: Nominal" when all health checks (db, redis) pass; if you stop the postgres container manually (`docker stop nexusflow-postgres-1`) and wait for the next health poll, the dot changes to amber and reads "System status: Degraded"; restoring the container returns the dot to green

**Notes:** Health is polled from GET /api/health. The health endpoint returns `{ status: "ok" | "degraded", checks: { db, redis } }`. The Chaos Controller itself refreshes health after each action completes, so the status updates automatically after a kill or disconnect scenario.

---

## Scenario 4: Kill Worker — confirmation dialog and worker termination
**REQ:** DEMO-004, Task Plan AC-1, AC-6

**Given:** you are logged in as admin and on `/demo/chaos`; at least one worker is registered and visible in the "Select a worker..." dropdown; a pipeline with tasks is running

**When:** you open the "Select a worker" dropdown and select the worker you want to kill; click "Kill Worker"

**Then:** a confirmation dialog appears with the message "This will send SIGKILL to worker container..." and two buttons — "Cancel" and "Confirm"; if you click "Cancel", the dialog closes and the worker is NOT killed; if you click "Confirm", the dialog closes, the "Kill Worker" button shows "Killing..." briefly, then the activity log below the card populates with timestamped entries: first "Sending SIGKILL to worker container…", then "Container … killed successfully", then "Monitor will detect heartbeat absence and reclaim in-flight tasks (ADR-002)"; the system status dot refreshes after the action

**Notes:** To observe the downstream effect: switch to the Worker Fleet Dashboard (`/workers`) in a second browser tab before performing the kill. After confirming, the killed worker's row should transition to "down" within the Monitor's heartbeat window (~30s). Any tasks that were in-flight for that worker should be reassigned (visible in the Task Feed).

---

## Scenario 5: Kill Worker — API-level admin enforcement (negative case)
**REQ:** DEMO-004, Task Plan AC-5

**Given:** you have a non-admin session token (or no session at all); the demo stack is running

**When:** you call `curl -X POST https://nexusflow.staging/api/chaos/kill-worker -H "Content-Type: application/json" -d '{"workerId":"any-worker"}' -b "session=<user-role-cookie>"` (substituting a valid user-role session cookie)

**Then:** the response is HTTP 403 Forbidden with a JSON error body; no container is affected; this confirms the admin gate is enforced at the server boundary, not merely in the UI

**Notes:** This scenario does not require browser interaction. The API-level enforcement was added specifically to address OBS-032-1 (identified during TASK-032 verification). All three chaos endpoints (`/api/chaos/kill-worker`, `/api/chaos/disconnect-db`, `/api/chaos/flood-queue`) are in the `RequireRole(Admin)` sub-group in `api/server.go`.

---

## Scenario 6: Disconnect Database — confirmation, countdown timer, and recovery
**REQ:** DEMO-004, Task Plan AC-2, AC-6

**Given:** you are logged in as admin and on `/demo/chaos`; the demo stack is running with workers processing tasks; the duration selector in "Disconnect Database" is set to "15 seconds"

**When:** you click "Disconnect DB"; a confirmation dialog appears — you click "Confirm"

**Then:** the dialog closes; the "Disconnect DB" button shows "Disconnecting..." briefly; once the API responds, a red banner appears below the button reading "DB DISCONNECTED" and a countdown element (`data-testid="disconnect-countdown"`) shows "15s remaining", then "14s remaining", etc.; the "Disconnect DB" button is disabled for the duration of the countdown; worker logs in a terminal (`docker logs nexusflow-worker-1 -f`) show `pgx` connection error lines; after 15 seconds the countdown disappears, the disconnect button re-enables, and worker logs show successful reconnection; the activity log on the card contains timestamped entries documenting the stop and scheduled restart

**Notes:** The disconnect uses `docker stop` on the `nexusflow-postgres-1` container rather than `docker pause`, so TCP connections are actively torn down and `pgx` observes real connection errors. This exercises the TASK-010/TASK-011 retry paths. Do not click Disconnect again while the countdown is active — the button is disabled and a second API call would return 409 Conflict.

---

## Scenario 7: Flood Queue — burst submission without confirmation
**REQ:** DEMO-004, Task Plan AC-3

**Given:** you are logged in as admin and on `/demo/chaos`; at least one pipeline is registered and visible in the "Select a pipeline for flood" dropdown

**When:** you set the task count input to `50`; select a pipeline from the dropdown; click "Submit Burst"

**Then:** no confirmation dialog appears (Flood Queue is non-destructive); the "Submit Burst" button shows "Flooding..." while the request is in flight; the activity log below the card shows "Flooding queue: submitting 50 tasks for pipeline …" and then "Flood complete: 50 tasks submitted to queue"; in the Task Feed (`/tasks`), a burst of 50 tasks appears in "submitted" or "queued" state; workers begin picking them up; the system status dot refreshes after completion

**Notes:** The task count is clamped to [1, 1000] by the UI (the input has `min=1`, `max=1000`) and validated at the API layer. To observe backpressure, submit 500 tasks; the Task Feed should show the queue depth growing before workers drain it.

---
