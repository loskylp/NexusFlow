---
task: TASK-020
title: Worker Fleet Dashboard (GUI)
requirements: REQ-016, REQ-004
date: 2026-03-27
smoke: false
---

# Demo Script — TASK-020: Worker Fleet Dashboard

**Environment:** Staging at nexusflow.staging.nxlabs.cc
**Precondition:** At least one worker process is running and registered. Admin credentials available.
**Role:** Admin (the Worker Fleet Dashboard is the Admin landing page after login)

---

## Scenario 1 — Initial load: skeleton loaders then populated table

**Requirement:** REQ-016

Given   the Admin navigates to `/workers` immediately after page load
When    the browser makes the initial `GET /api/workers` request
Then    skeleton loaders appear in the summary card row and table area while the request is in flight, and disappear once the response resolves

Expected result: Three placeholder card shapes pulse in the summary row; five placeholder rows appear in the table area. After the REST response resolves, the skeleton is replaced by real data.

---

## Scenario 2 — Status indicators: green dot for online, red dot for down

**Requirement:** REQ-016, REQ-004 (AC-1)

Given   the Worker Fleet Dashboard is loaded with at least one online worker and one down worker
When    the Admin views the worker table
Then    online workers show a green dot with the text label "Online" in the Status column
        and down workers show a red dot with the text label "Down" in the Status column

Expected result: The Status column for an online worker shows a green filled circle and the word "Online" side-by-side. The Status column for a down worker shows a red filled circle and the word "Down" side-by-side. Color is never the sole differentiator.

---

## Scenario 3 — Summary cards show accurate live counts

**Requirement:** REQ-016 (AC-2)

Given   the Worker Fleet Dashboard is loaded with, for example, 3 workers: 2 online and 1 down
When    the Admin views the summary card row
Then    the Total card shows "3", the Online card shows "2", and the Down card shows "1"

Expected result: Three summary cards appear at the top of the page. Card values match the actual counts of workers in the table. The Down card value is displayed in red when greater than zero.

---

## Scenario 4 — Real-time update: worker goes down

**Requirement:** REQ-016, REQ-004 (AC-3)

Given   the Admin has the Worker Fleet Dashboard open with two online workers
When    one of the workers stops sending heartbeats and the Monitor marks it down
        (the backend emits a `worker:down` SSE event on `/events/workers`)
Then    within one heartbeat-timeout interval (approximately 15 seconds, no page refresh required):
        - the worker's row background transitions to red-50
        - the Status column changes from green "Online" to red "Down"
        - the Down summary card increments by 1
        - the Online summary card decrements by 1
        - the row moves to the top of the table (default sort: down workers first)

Expected result: The dashboard reflects the failure automatically. No page refresh is needed.

---

## Scenario 5 — Real-time update: worker comes online

**Requirement:** REQ-016, REQ-004 (AC-4)

Given   the Admin has the Worker Fleet Dashboard open
When    a new worker process starts and self-registers
        (the backend emits a `worker:registered` SSE event on `/events/workers`)
Then    a new row appears in the table with:
        - green "Online" status dot
        - the worker's ID in the Worker ID column
        - its capability tags in the Tags column
        - "—" in the Current Task column (no task assigned yet)
        and the Total and Online summary cards increment by 1

Expected result: The new worker appears in the table without a page refresh. Summary counts update immediately.

---

## Scenario 6 — Sortable columns

**Requirement:** REQ-016 (AC-5)

Given   the Worker Fleet Dashboard is loaded with multiple workers
When    the Admin clicks the "Worker ID" column header
Then    the table sorts alphabetically ascending by Worker ID
        and a ▲ indicator appears next to the "Worker ID" header

When    the Admin clicks the "Worker ID" column header a second time
Then    the sort reverses to descending
        and the ▲ indicator changes to ▼

When    the Admin clicks any other column header (Tags, Current Task, Last Heartbeat, Status)
Then    the table sorts by that column ascending
        and the sort indicator moves to the newly active column

Expected result: All five column headers are interactive. Single click sorts ascending; second click reverses. The active sort column is visually distinguished.

---

## Scenario 7 — Default sort: down workers at the top

**Requirement:** REQ-016 (AC-6)

Given   the Worker Fleet Dashboard loads with a mix of online and down workers
When    the page first renders (before any column header click)
Then    all down workers appear above all online workers in the table
        and the Status column header shows a ▲ indicator (ascending — down first)

Expected result: Failures surface at the top without any admin interaction. Down workers are always the first rows visible.

---

## Scenario 8 — SSE disconnection: "Reconnecting..." status bar

**Requirement:** REQ-016 (AC-7)

Given   the Admin has the Worker Fleet Dashboard open
When    the SSE connection to `/events/workers` drops (network interruption or server restart)
Then    the status bar at the bottom of the page shows a red dot and the text "Reconnecting..."
        within one second of the disconnection

When    the SSE connection re-establishes
Then    the status bar transitions back to a green dot and the text "Connected"

Expected result: The status bar is always visible regardless of data state. Connection loss is surfaced immediately with a red indicator. The Admin knows the data may be stale while "Reconnecting..." is shown.

---

## Scenario 9 — Empty state: no workers registered

**Requirement:** REQ-016 (AC-8)

Given   no worker processes have ever registered with the system
When    the Admin navigates to `/workers`
Then    the data table is not rendered
        and a centered message reads: "No workers registered. Workers self-register when they start."
        and the summary cards show Total: 0, Online: 0, Down: 0

Expected result: The empty state is unambiguous and actionable. The Admin understands the workers are absent and how they are added (self-registration on start).

---

## Cleanup

No persistent state is created by viewing the Worker Fleet Dashboard. No cleanup required.
