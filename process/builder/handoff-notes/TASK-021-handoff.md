# Handoff Note — TASK-021: Task Feed and Monitor (GUI)

**Task:** TASK-021
**Status:** Complete
**Iteration:** 1
**Date:** 2026-04-07

---

## What Was Built

### `web/src/hooks/useTasks.ts`
Replaces the stub with a full implementation:

- **`mergeTaskEvent(tasks, event)`** — pure function that handles all seven SSE event types:
  - `task:submitted` — adds the task if not already present (deduplicates by ID)
  - `task:queued`, `task:assigned`, `task:running`, `task:completed`, `task:failed`, `task:cancelled` — merges the full SSE payload into the matching task
  - Unknown event types return the original list unchanged (referential equality preserved)
- **`useTasks(filters?)`** — hook combining REST seed and SSE merge:
  - Fetches `GET /api/tasks` on mount with optional `TaskFilters` params
  - Re-fetches on `refresh()` call (via `refreshTick` counter)
  - Serialises `filters` to a stable JSON key for the effect dependency (avoids infinite loops from object identity churn)
  - Subscribes to `GET /events/tasks` via `useSSE`; all state updates go through `mergeTaskEvent`
  - Exposes `{ tasks, isLoading, error, sseStatus, refresh }`

### `web/src/components/TaskCard.tsx`
Replaces the stub with a full implementation:

- **`isCancellable(status)`** — returns `true` for `submitted`, `queued`, `assigned`, `running`; `false` for terminal states
- **`statusBadgeStyle(status)`** — returns CSS properties using exact DESIGN.md hex tokens (violet-500 for submitted, amber-600 for queued, amber-500 for assigned, blue-600 for running, green-600 for completed, red-600 for failed, slate-500 for cancelled)
- **`TaskCard`** — pure presentational component:
  - Status badge (pill with 10% opacity background, color text, text label always present per WCAG)
  - Running status includes pulsing dot animation
  - Failed state: 4px red-600 left border accent + "Task failed — check logs for details" alert
  - `isRecentlyUpdated` prop: 200ms yellow-50 background flash (background-color CSS transition)
  - View Logs always shown; Cancel shown when `(isOwner || isAdmin) && isCancellable(status)`; Retry shown only when `status === 'failed'`
  - Cancel triggers `window.confirm()` dialog before invoking `onCancel`
  - Worker assignment and timestamps displayed in metadata row

### `web/src/pages/TaskFeedPage.tsx`
Replaces the stub with a full implementation:

- **`FilterBar`** — controlled component with status dropdown (7 statuses + "All"), pipeline dropdown, search input, "Submit Task" button
- **`FeedStatusBar`** — role badge ("Viewing: All Tasks" for Admin, "Viewing: My Tasks" for User) + SSE status dot with "Reconnecting..." state
- **`SkeletonTaskCard`** — animated pulse placeholder with representative card shape
- **`TaskFeedPage`** — main page component:
  - Composes `useTasks`, `usePipelines`, `useAuth`, `TaskCard`, `SubmitTaskModal`
  - Loading state: 4 skeleton cards
  - Error state: red alert banner
  - Empty (no tasks, no filters): full card with "Submit Task" CTA
  - Empty (filters active, no results): "No tasks match your filters." with "Clear Filters" link
  - Populated: task cards sorted newest-first by `createdAt` (reverse chronological)
  - `isRecentlyUpdated` flash: tracks `updatedAt` changes per task across renders; sets flag + schedules 200ms timeout to clear
  - Cancel calls `cancelTask` API then `refresh()`; navigates View Logs to `/tasks/logs?taskId=<id>`
  - Route wiring: already present in `App.tsx` at `/tasks` (no change needed)

---

## Tests Written

### `web/src/hooks/useTasks.test.ts` (27 tests)
- `mergeTaskEvent` pure function: all 7 event types, unknown events, deduplication, immutability
- `useTasks` hook: initial REST fetch (loading state, success, failure, filter params), SSE event merging, refresh trigger, SSE status passthrough

### `web/src/components/TaskCard.test.tsx` (32 tests)
- `isCancellable`: all 7 status values
- `statusBadgeStyle`: all 7 statuses, color token presence
- `TaskCard`: rendering (task ID, pipeline name, status badge), Cancel visibility rules (all combinations of isOwner/isAdmin/status), Retry visibility, View Logs always shown, action callbacks (including confirm dialog logic), failed state, isRecentlyUpdated, worker assignment display

### `web/src/pages/TaskFeedPage.test.tsx` (14 tests)
- Loading skeleton, empty state (no tasks), task list rendering, page title, role badge (Admin/User), SSE status bar, filter bar, Submit Task modal trigger, error state

**All 253 tests pass. TypeScript compiles clean (`tsc --noEmit`).**

---

## Acceptance Criteria Traceability

| Criterion | Implementation | Status |
|---|---|---|
| Task Feed shows tasks in reverse chronological order with correct status badges | `sortTasksNewestFirst` + `statusBadgeStyle` in `TaskCard` | DONE |
| Task state changes update in real time via SSE (badge transition with 200ms highlight) | `mergeTaskEvent` + `isRecentlyUpdated` flag with 200ms timeout | DONE |
| "Submit Task" modal allows pipeline selection, parameter input, and retry config; submission creates a task via API | `SubmitTaskModal` (existing, from TASK-023) wired via `isModalOpen` | DONE (minimal; full form in TASK-035) |
| "Cancel" button visible only on cancellable states for task owner or admin | `isCancellable` + `(isOwner || isAdmin)` guard in `TaskCard` | DONE |
| "View Logs" navigates to Log Streamer with task pre-selected | `navigate('/tasks/logs?taskId=...')` in `handleViewLogs` | DONE |
| Admin sees all tasks with "Viewing: All Tasks" badge; User sees own tasks with "Viewing: My Tasks" | `FeedStatusBar` role badge + server-side SSE filtering | DONE |
| Filter by status, pipeline, and search works correctly | `FilterBar` + `useTasks(taskFilters)` | DONE |
| Empty state and loading skeleton shown appropriately | `isLoading` → `SkeletonTaskCard`; empty states per filter/no-filter condition | DONE |

---

## Deviations and Limitations

1. **Retry button scope:** Clicking "Retry" opens the `SubmitTaskModal` (same pipeline) rather than re-submitting with the exact original input/retryConfig. Full retry-with-same-configuration is deferred to TASK-035, consistent with the existing `SubmitTaskModal` minimal implementation note.

2. **Confirm dialog for Cancel:** Uses `window.confirm()` (browser native dialog) rather than the DESIGN.md custom confirmation dialog pattern. The custom dialog component is not yet built. This meets the UX spec requirement for a confirmation step. The Verifier should note this as an observation; a proper modal confirmation can be added in a follow-up task.

3. **No `errorReason` field on Task domain type:** The Task type does not include an `errorReason` field (not in `domain.ts`). The failed state shows a generic "Task failed — check logs for details" message. Upgrading this requires a domain type extension and backend change.

4. **Pre-existing acceptance test error:** `tests/acceptance/TASK-023-acceptance.test.tsx` contains a `waitFor` call without `await` that produces a floating promise error in the test runner. This error existed before TASK-021 (confirmed by reverting and re-running) and is outside this task's scope.

---

## Nil-Wiring Check

This task is frontend-only. No new backend dependencies were introduced. The existing API endpoints (`GET /api/tasks`, `GET /events/tasks`, `POST /api/tasks/{id}/cancel`) are already implemented in prior cycles. Route `/tasks` was already wired in `App.tsx`.
