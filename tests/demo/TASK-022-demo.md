# Demo Script — TASK-022
**Feature:** Log Streamer (GUI)
**Requirement(s):** REQ-018
**Environment:** Staging — https://nexusflow-staging.example.com (authenticated as User or Admin)

## Scenario 1: Stream logs for a running task in real time
**REQ:** REQ-018

**Given:** You are logged in as User-A. Task-1 belongs to User-A and is currently in "running" state. At least one log line has been produced by the worker.

**When:** Navigate to `/tasks` (Task Feed). Locate Task-1 in the feed. Click the "View Logs" button on Task-1's card.

**Then:** The Log Streamer page opens with Task-1 pre-selected in the task dropdown. The log panel (dark background, monospace font) shows existing log lines. New log lines appear in the panel within 2 seconds of the worker producing them. The status bar at the bottom shows a green dot and "Connected".

**Notes:** If Task-1 completes while you are watching, the status bar changes from "Connected" to "Complete — N lines". The SSE connection closes gracefully.

---

## Scenario 2: Filter log lines by pipeline phase
**REQ:** REQ-018

**Given:** You are on the Log Streamer page with a task selected that has produced log lines across multiple phases (DataSource, Process, and Sink lines are all visible).

**When:** Click the "DataSource" phase filter toggle button in the toolbar.

**Then:** Only log lines tagged `[datasource]` are visible in the panel. The DataSource button appears highlighted (colored border). Process and Sink lines are hidden.

**When:** Click "All" to restore the full view.

**Then:** All log lines are visible again across all three phases.

**Notes:** Phase filtering is client-side — no network request is made. Lines are not discarded; they are hidden. Try "Process" and "Sink" toggles separately to confirm each filters independently.

---

## Scenario 3: Phase tag color-coding matches design system
**REQ:** REQ-018

**Given:** You are on the Log Streamer page with a task that has produced log lines from all three phases.

**When:** Observe the log lines in the panel.

**Then:** Each log line with a phase tag shows a small colored badge:
- `[datasource]` lines show a blue badge (#2563EB)
- `[process]` lines show a purple badge (#8B5CF6)
- `[sink]` lines show a green badge (#16A34A)
Log lines without a phase tag show no badge. The message text following the badge does not contain the raw `[phase]` text.

---

## Scenario 4: Auto-scroll follows new lines; disabling allows scrolling back
**REQ:** REQ-018

**Given:** You are on the Log Streamer page with a task producing frequent log lines. The panel is auto-scrolling to the bottom as new lines arrive.

**When:** Uncheck the "Auto-scroll" checkbox in the toolbar.

**Then:** The panel stops scrolling to the bottom. New lines continue to arrive (streaming continues) but the view remains at the position you left it. You can scroll freely through log history.

**When:** Re-check the "Auto-scroll" checkbox.

**Then:** The panel immediately scrolls to the bottom and resumes following new lines.

---

## Scenario 5: Download Logs triggers a browser file download
**REQ:** REQ-018

**Given:** You are on the Log Streamer page with a task selected (any state — running or completed).

**When:** Click the "Download Logs" button in the toolbar.

**Then:** The browser downloads a plain-text file named `task-<taskId>-logs.txt`. The file contains the full log history for the task (all lines, not just those visible after phase filtering). The button briefly shows "Downloading..." and returns to "Download Logs" on completion.

**Notes:** The Download Logs button is disabled (greyed out) when no task is selected. The download uses the REST endpoint `GET /api/tasks/{id}/logs`, not the SSE stream.

---

## Scenario 6: SSE reconnection with Last-Event-ID replay
**REQ:** REQ-018

**Given:** You are on the Log Streamer page with a task streaming logs. The status bar shows "Connected" and a Last-Event-ID value.

**When:** Simulate a brief network interruption (e.g., disable and re-enable the network interface, or wait for a natural SSE timeout). Alternatively, observe the reconnection status if the server drops the connection.

**Then:** The status bar briefly shows a red dot and "Reconnecting...". After reconnection, the status returns to green "Connected". Log lines that were produced during the disconnection window appear in the panel (server replays from the Last-Event-ID). No log gap is visible.

**Notes:** The Last-Event-ID displayed in the status bar shows the ID of the last received event. This value is sent as the `Last-Event-ID` header by the browser's native EventSource on reconnection, enabling the server to replay missed lines.

---

## Scenario 7: Access denied (403) shown in log panel, no redirect
**REQ:** REQ-018

**Given:** You are logged in as User-B. Task-1 belongs to User-A (a different user). User-B is not an Admin.

**When:** Navigate directly to `/tasks/logs?taskId=<Task-1-ID>`.

**Then:** The Log Streamer page remains visible (no redirect to login or another page). The log panel shows the error message "Access denied: you do not have permission to view logs for this task." No log lines are shown.

**Notes:** The page title "Log Streamer" remains visible, confirming no page navigation occurred. The task selector dropdown is still functional — User-B can select one of their own tasks to stream logs.

---

## Scenario 8: Log line format — timestamp, phase tag, and message
**REQ:** REQ-018

**Given:** You are on the Log Streamer page with a task that has produced log lines.

**When:** Observe individual log lines in the panel.

**Then:** Each log line displays:
- A timestamp in HH:MM:SS format (secondary gray color, fixed-width)
- A colored phase badge (blue/purple/green) if the line has a phase tag
- The message text following the badge

Lines with level ERROR are rendered in red text. Lines with INFO, WARN, or DEBUG levels use the default gray text.

---
