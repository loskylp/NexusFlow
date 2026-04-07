# Demo Script — TASK-021
**Feature:** Task Feed and Monitor (GUI)
**Requirement(s):** REQ-017, REQ-002, REQ-009, REQ-010
**Environment:** Staging — https://nxlabs.cc (authenticated session required)

---

## Scenario 1: Task Feed loads and displays tasks in reverse chronological order
**REQ:** REQ-017

**Given:** You are logged in as a User (non-admin) with at least two previously submitted tasks having different submission times

**When:** You navigate to `/tasks`

**Then:** The Task Feed loads showing a list of task cards; the most recently submitted task appears at the top of the list; each card shows the task ID, pipeline name, a colored status badge, and the submission timestamp

**Notes:** If no tasks exist yet, submit one via the API (`POST /api/tasks`) to create initial data before this scenario. The reverse-chronological order is by `createdAt` timestamp.

---

## Scenario 2: Real-time SSE status update with flash animation
**REQ:** REQ-017, REQ-009

**Given:** You are logged in and on the Task Feed at `/tasks`; a task is visible in the feed in state "queued" or "running"; the SSE status bar at the bottom shows "Connected" with a green dot

**When:** The task transitions to the next lifecycle state (e.g., queued -> assigned -> running -> completed) — either wait for the worker to process it, or trigger a transition via the API

**Then:** The status badge on the task card changes color without a page refresh; the card briefly flashes yellow (200ms background transition) at the moment the update arrives; the final badge color matches the new state (e.g., blue for running, green for completed)

**Notes:** The yellow flash is brief (200ms). Watch the card at the moment the transition arrives. If the feed is not connected, the SSE status bar shows "Reconnecting..." in red — wait for reconnection before observing.

---

## Scenario 3: Submit Task button opens the submission modal
**REQ:** REQ-002

**Given:** You are logged in and on the Task Feed

**When:** You click the "Submit Task" button in the filter bar (top-right of the filter row)

**Then:** A modal dialog opens allowing you to select a pipeline and configure the task; the modal has a close button; dismissing the modal returns you to the feed without submitting

**Notes:** Full form validation and submission is exercised in the TASK-035 demo. This scenario verifies the modal opens and closes correctly.

---

## Scenario 4: Cancel button — owner can cancel their own running task
**REQ:** REQ-010

**Given:** You are logged in as User-A; you have a task in "running" state visible in the feed

**When:** You click the "Cancel" button on that task card

**Then:** A browser confirmation dialog appears asking you to confirm; clicking "OK" sends the cancel request; the task transitions to "cancelled" state (visible via SSE update or page refresh); clicking "Cancel" in the confirmation dialog dismisses it without sending any request

**Notes:** The Cancel button is only visible on cancellable states (submitted, queued, assigned, running) and only when you own the task or are admin. Verify the button is absent on a completed or failed task.

---

## Scenario 5: Cancel button — admin can cancel any user's task
**REQ:** REQ-010

**Given:** You are logged in as Admin; a task submitted by a different user is visible in the feed in a cancellable state

**When:** You click the "Cancel" button on that task card

**Then:** The confirmation dialog appears; confirming cancels the task; the status badge updates to "cancelled"

**Notes:** The Admin can cancel any task regardless of who submitted it. The Cancel button is always shown to admins on cancellable-state cards.

---

## Scenario 6: View Logs navigates to Log Streamer with task pre-selected
**REQ:** REQ-017

**Given:** You are on the Task Feed with at least one task card visible (any status)

**When:** You click the "View Logs" button on any task card

**Then:** You are navigated to the Log Streamer view at `/tasks/logs?taskId=<task-id>`; the task is pre-selected in the task selector

**Notes:** "View Logs" is always visible on every task card regardless of status.

---

## Scenario 7: Role-based visibility — Admin sees all tasks
**REQ:** REQ-017

**Given:** User-A has submitted 3 tasks and User-B has submitted 2 tasks; you are logged in as Admin

**When:** You navigate to `/tasks`

**Then:** The feed shows all 5 tasks (from both users); the status bar badge reads "Viewing: All Tasks"

---

## Scenario 8: Role-based visibility — User sees only their own tasks
**REQ:** REQ-017

**Given:** User-A has submitted 3 tasks and User-B has submitted 2 tasks; you are logged in as User-A (non-admin)

**When:** You navigate to `/tasks`

**Then:** The feed shows only User-A's 3 tasks; User-B's 2 tasks are not visible; the status bar badge reads "Viewing: My Tasks"

**Notes:** Visibility isolation is enforced server-side via the SSE channel. The UI trusts what the server sends.

---

## Scenario 9: Filter by status
**REQ:** REQ-017

**Given:** You are on the Task Feed with tasks in multiple states (e.g., running, completed, failed)

**When:** You select "Running" from the status filter dropdown

**Then:** Only tasks in "running" state are shown; tasks in other states disappear from the feed

**When:** You select "All Statuses" from the dropdown again

**Then:** All tasks are shown again

---

## Scenario 10: Loading skeleton and empty state
**REQ:** REQ-017

**Given:** You navigate to `/tasks` for the first time (or after clearing browser cache)

**When:** The page loads

**Then:** Four skeleton placeholder cards appear while the initial API call is in flight; once the data loads, the skeleton cards are replaced with either real task cards or the empty state message "No tasks found. Submit your first task to get started."

**Notes:** The skeleton is brief on a fast network. To observe it clearly, use browser DevTools to throttle the network to "Slow 3G" before navigating.

---

## Scenario 11: SSE reconnection state
**REQ:** REQ-017

**Given:** You are on the Task Feed with the SSE connection established (green dot in status bar)

**When:** You temporarily disconnect from the network (e.g., airplane mode on laptop, or kill the API server briefly)

**Then:** The SSE status bar dot turns red and the label changes to "Reconnecting..."

**When:** The connection is restored

**Then:** The status bar returns to green "Connected"; the feed re-seeds from the API to catch up on any missed events

---
