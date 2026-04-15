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

# Demo Script — TASK-032
**Feature:** Sink Inspector (GUI) — Before/After sink state comparison with atomicity verification
**Requirement(s):** DEMO-003, ADR-009, UX Spec (Sink Inspector)
**Environment:** Staging — https://nexusflow.staging (or local: http://localhost:3000 with `docker compose up`)

---

## Scenario 1: Admin login and navigation to Sink Inspector
**REQ:** DEMO-003

**Given:** you are on the `/login` page; you have Admin credentials (default seed: username `admin`, password `admin`)

**When:** you log in as admin and click "Sink Inspector" in the left sidebar under the "DEMO" label

**Then:** the browser navigates to `/demo/sink-inspector`; the page header shows "Sink Inspector" in 20px SemiBold text; a purple "DEMO" badge appears next to the title; the monitoring status indicator shows a grey dot with "No task selected"; a "Task:" label with a dropdown reading "Select a task to inspect..." is visible below the header; two equal-width panels labeled "BEFORE SNAPSHOT" (left) and "AFTER RESULT" (right) are shown side-by-side; both panels display the placeholder text "Select a task to inspect its sink operation"; an "Atomicity Verification" section below the panels reads "Awaiting sink phase completion..."

---

## Scenario 2: Access denied for User-role accounts
**REQ:** DEMO-003, Task Plan AC-6

**Given:** you are logged out (clear the session cookie via DevTools → Application → Cookies); you have a User-role account (not admin)

**When:** you log in as the user-role account and navigate directly to `/demo/sink-inspector` (or click any link that reaches it)

**Then:** the page shows "Access denied" in red text and "The Sink Inspector is only available to Admin users." below it; the "Sink Inspector" page title, DEMO badge, task selector dropdown, and data panels do NOT appear; the "Sink Inspector" link is also absent from the sidebar (the DEMO section is hidden for User-role sessions per TASK-019)

**Notes:** To verify the sidebar is also filtered: after logging in as user, inspect the left sidebar — the "DEMO" section heading and both "Sink Inspector" and "Chaos Controller" links must be absent.

---

## Scenario 3: Selecting a task subscribes to the SSE channel
**REQ:** DEMO-003, Task Plan AC-1

**Given:** you are logged in as admin and on `/demo/sink-inspector`; at least one task has been executed (the dropdown is populated from recent tasks)

**When:** you open the "Select a task to inspect..." dropdown and select any completed task

**Then:** the monitoring status indicator in the header transitions from grey "No task selected" to an amber/indigo dot "Connecting..."; once connected, the dot turns green and the label reads "Monitoring"; the Before panel changes from the placeholder text to "Waiting for sink phase to begin..." with a spinning indigo/grey progress indicator; the After panel continues to show the placeholder (no after data yet)

**Notes:** If no tasks are visible in the dropdown, submit a pipeline task first via the Task Feed page, wait for it to complete, then return to the Sink Inspector and refresh.

---

## Scenario 4: Before snapshot populates when sink phase begins
**REQ:** DEMO-003, Task Plan AC-2, ADR-009

**Given:** you have selected a task in Scenario 3 and the Before panel shows the "Waiting for sink phase" spinner

**When:** the worker executing the selected task reaches the Sink phase and publishes the `sink:before-snapshot` event (this happens automatically during normal pipeline execution — no manual action required)

**Then:** the "BEFORE SNAPSHOT" panel populates with a key-value table showing the pre-write destination state (e.g., `object_count: 3`, `bucket: "demo-bucket"` for an S3 sink); a timestamp (HH:MM:SS) appears in the panel header showing when the snapshot was captured; the waiting spinner disappears; the monitoring status dot remains green; the After panel still shows "Awaiting sink:after-result event..."

---

## Scenario 5: After snapshot with delta highlights and atomicity checkmark
**REQ:** DEMO-003, Task Plan AC-3, AC-4, ADR-009

**Given:** the Before panel is populated from Scenario 4

**When:** the Sink write commits successfully and the `sink:after-result` event arrives

**Then:** the "AFTER RESULT" panel populates with a key-value table showing the post-write destination state; keys that are new (not present in Before) are shown in green text with a "NEW" label beside the key name and a `#F0FDF4` (green-50) row background; keys whose values changed compared to Before are shown in green text with the same green-50 row background; keys unchanged from Before are shown in the default text color with no highlight; the "Atomicity Verification" section below the panels shows a green circle checkmark (aria-label "Atomicity verified"), the text "Write committed successfully", and a delta summary line such as "Delta: 1 changed, 1 new"

**Notes:** For an S3 sink, `object_count` will have increased (e.g., 3 → 5) — that row will be highlighted as changed. A `new_key` column present only in After will be highlighted as new.

---

## Scenario 6: Rollback — ROLLED BACK badge and Before/After parity
**REQ:** DEMO-003, Task Plan AC-5, ADR-009

**Given:** you are on `/demo/sink-inspector`; select a task that will fail during its Sink phase (either a task configured to inject a Sink error via the Chaos Controller, or a task targeting an unavailable destination)

**When:** the Sink write fails, the worker rolls back, and the `sink:after-result` event arrives with `rolledBack: true`

**Then:** the "AFTER RESULT" panel populates with a key-value table whose data matches the Before panel exactly (the destination was restored to its pre-write state); no green-50 highlights appear in the After panel (no net changes — rollback succeeded); the "Atomicity Verification" section shows a red "ROLLED BACK" badge (white uppercase text on red background), the message "Write rolled back — destination restored to Before state", and the error text (e.g., "Error: write failed: connection reset") in monospace below

**Notes:** To trigger a rollback scenario during a live demo, use the Chaos Controller (TASK-034) to disconnect the database while a task is in the Sink phase. The worker will detect the write failure, abort, restore the destination, and publish the rollback after-event. The Sink Inspector will show the ROLLED BACK badge automatically.

---

## Scenario 7: Switching tasks resets all snapshot state
**REQ:** DEMO-003, Task Plan (reset behavior)

**Given:** you have completed Scenario 5 — both panels are populated and the atomicity checkmark is shown

**When:** you open the task dropdown and select a different task (or select "Select a task to inspect..." to clear)

**Then:** both panels immediately clear and return to their default placeholder state; if a new task was selected, the Before panel transitions to the "Waiting for sink phase" spinner; the monitoring status indicator resets to "Connecting..."; the Atomicity Verification section resets to "Awaiting sink phase completion..."; no data from the previous task remains visible

---
