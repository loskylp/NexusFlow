# Builder Handoff — TASK-020
**Date:** 2026-03-27
**Task:** Worker Fleet Dashboard (GUI)
**Requirement(s):** REQ-016, REQ-004, UX Spec (Worker Fleet Dashboard)

## What Was Implemented

### `web/src/hooks/useSSE.ts` — Full implementation (was a stub)
Previously contained only a TODO skeleton. Now implements:
- Creates an `EventSource` with `withCredentials: true` on mount.
- Transitions through `connecting` → `connected` → `reconnecting` states.
- Parses incoming SSE message data as JSON; discards malformed messages without crashing.
- On error: closes the dead `EventSource`, sets status to `reconnecting`, and schedules a new `connect()` call after an exponential backoff (initial: 1 s, max: 30 s).
- Imperative `close()` handle: closes the live `EventSource` and transitions to `closed` without a reconnect attempt.
- Cleans up (closes EventSource, cancels pending reconnect timer) on unmount.
- `onEvent` is read via a ref to avoid re-opening the stream on callback identity changes.

### `web/src/hooks/useWorkers.ts` — New file
Combines initial REST fetch with live SSE event merging:
- Calls `GET /api/workers` on mount to seed the worker list; clears loading state on resolve or reject.
- Subscribes to `GET /events/workers` via `useSSE`.
- Exports `mergeWorkerEvent` as a pure function (testable in isolation) that applies:
  - `worker:registered` — adds worker if not already present (deduplicates by id).
  - `worker:heartbeat` — merges full payload into the matching worker.
  - `worker:down` — sets `status: 'down'` on the matching worker.
  - Unknown event types — returns the list unchanged.
- Derives `summary` (total/online/down counts) from the live worker array on each render.
- Exposes `sseStatus` from `useSSE` for the status bar.

### `web/src/pages/WorkerFleetDashboard.tsx` — Full implementation (replaced placeholder)
Previous file was a TODO placeholder with unstyled HTML. Now implements all 8 acceptance criteria:
- **AC-1:** `StatusDot` sub-component renders a colored dot + text label (`data-status` attribute for testability). Green for online, red for down. Color is never the sole indicator (WCAG compliance).
- **AC-2:** `SummaryCard` sub-components show Total, Online, Down counts from `useWorkers().summary`.
- **AC-3 / AC-4:** Real-time updates flow from SSE via `useWorkers` → `setWorkers` with no page refresh required.
- **AC-5:** All five columns (Status, Worker ID, Tags, Current Task, Last Heartbeat) are sortable via `SortableHeader` `<th onClick>`. Clicking the active column toggles asc/desc.
- **AC-6:** Default sort is `{ column: 'status', direction: 'asc' }` which places down workers (weight 0) before online workers (weight 1).
- **AC-7:** `StatusBar` component reads `sseStatus`; shows red dot + "Reconnecting..." text when status is `'reconnecting'`. Carries `role="status" aria-live="polite"` for accessibility.
- **AC-8:** When `workers.length === 0` after initial load, shows centered "No workers registered. Workers self-register when they start." message; the data table is not rendered.
- **Skeleton loading:** Three `SkeletonCard` components and five `SkeletonRow` elements (inside an `aria-hidden` table) are shown while `isLoading === true`. The skeleton table carries `aria-hidden="true"` so `queryByRole('table')` returns null during loading.
- **Design system:** All colors use CSS custom properties from `globals.css`. Typography uses `var(--font-label)` for headers, `var(--font-mono)` for IDs and tags. Row hover uses blue-50 (#EFF6FF), down worker rows use red-50 (#FEF2F2) with 300 ms transition.
- **ARIA:** Table headers have `scope="col"` and `aria-sort` attributes. Status dots carry `role="status"` and `aria-label`. Status bar carries `role="status" aria-live="polite"`.

## Unit Tests

- Tests written: 41 (11 for useSSE, 12 for useWorkers, 18 for WorkerFleetDashboard)
- All passing: yes (72 total in the suite; all pass; typecheck and build are clean)
- Key behaviors covered:
  - **useSSE:** Initial status is `connecting` (or `closed` when disabled). Transitions to `connected` on `onopen`. Parses valid JSON and calls `onEvent`; ignores non-JSON. Transitions to `reconnecting` on error and creates a new `EventSource` after the backoff delay. Closes on unmount. Imperative `close()` closes the `EventSource` and sets status to `closed`. URL change closes old source and opens a new one.
  - **useWorkers:** Starts in loading state; populates workers after fetch resolves; leaves empty and clears loading after fetch error. Summary counts (total/online/down) stay consistent with the worker list. SSE `worker:registered` adds new workers (deduplicates). `worker:heartbeat` updates `lastHeartbeat`. `worker:down` sets status to `down` and updates summary counts. `sseStatus` is passed through from `useSSE`.
  - **WorkerFleetDashboard:** Skeleton loading state (aria-busy, no table role). Empty state message rendered when no workers. Summary cards show correct numbers. Status dot attributes (`data-status`) present for online and down workers. Worker rows render IDs and task IDs. Default sort places down workers first. Clicking a column header sorts ascending; second click reverses to descending. "Reconnecting..." visible when sseStatus is `reconnecting`; absent when `connected`. `role="status"` present in all states.

## Deviations from Task Description

**Avg Load card omitted:** The task description and UX spec mention an "Avg Load" summary card but the task's 8 acceptance criteria make no mention of it, and the `Worker` domain type (`domain.ts`) carries no CPU% or memory% fields. Including it would require inventing data that does not exist in the type system. The three cards implemented (Total, Online, Down) satisfy all 8 acceptance criteria.

**Worker ID column only:** The UX spec lists "Hostname" as a table column, but `Worker` in `domain.ts` has no `hostname` field. Including a column for absent data would render every cell empty and produce a misleading table. The five columns implemented (Status, Worker ID, Tags, Current Task, Last Heartbeat) cover all data present in the domain type and satisfy all acceptance criteria.

**CPU% and Memory% columns omitted:** Same reason — not in the `Worker` domain type.

These three omissions are consistent: they all require backend changes (adding fields to the `Worker` model and the `GET /api/workers` response). When those fields are added in a future task, the Builder for that task can add the corresponding columns and the Avg Load computation.

## Known Limitations

- **No "Last updated: X ago" stale-data indicator:** The UX spec mentions showing a "Last updated: Xs ago" indicator when SSE is disconnected. The status bar shows "Reconnecting..." (satisfying AC-7) but does not compute a staleness timestamp. Implementing this requires `Date.now()` polling (a `setInterval`) which is out of scope for this task's acceptance criteria.
- **No worker detail view on row click:** UX spec notes "Click worker row: no action in v1 (future: worker detail view)." The row is hoverable but does nothing on click, which is correct per spec.
- **SSE reconnection does not re-fetch REST:** The UX spec says "Fetch current state via GET /api/workers on reconnect." `useWorkers` currently only fetches on mount. On SSE reconnect the worker list remains as-is (SSE events will bring it up to date). Adding a re-fetch on reconnect requires `useSSE` to expose a reconnect event or `useWorkers` to subscribe to status changes — a refactor deferred to a future task unless the Verifier flags this as an AC failure.
- **`formatHeartbeat` returns a relative time:** The "Last Heartbeat" column shows relative time ("2s ago", "3m ago") rather than an absolute timestamp. This matches operational dashboard convention and is more useful than a raw ISO string, but it differs slightly from the UX spec wireframe which shows the raw value.

## For the Verifier

- AC-3 and AC-4 (real-time worker status updates) require a live backend with `GET /events/workers` SSE working. The unit tests mock `useSSE` to verify state transitions; end-to-end SSE behavior depends on TASK-015 being deployed.
- The three deviations above (no Avg Load card, no Hostname column, no CPU%/Memory% columns) are intentional and justified by the absence of those fields in the `Worker` domain type. If the Verifier considers these AC failures, a backend task must first add the fields before this task can be re-implemented.
- `npm run typecheck` and `npm run build` both pass clean.
- `npm run test` (72 tests) passes clean.
