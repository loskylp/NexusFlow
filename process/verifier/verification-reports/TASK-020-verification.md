# Verification Report — TASK-020

**Task:** Worker Fleet Dashboard (GUI)
**Requirements:** REQ-016, REQ-004
**Verifier invocation:** Initial (iteration 1)
**Date:** 2026-03-27
**Mode:** Pre-staging (local — unit tests, static analysis, build, typecheck; no running server)
**Verdict:** PASS

---

## Summary

All 8 acceptance criteria pass. The Builder's implementation of `WorkerFleetDashboard.tsx`, `useWorkers.ts`, and `useSSE.ts` is complete, correct, and consistent with the UX specification and domain model.

72 unit tests pass. TypeScript build is clean. Typecheck passes with zero errors. 36 acceptance test checks pass (including 17 verifier-added negative cases).

---

## Test Layers Applied

| Layer | Applied | Rationale |
| Integration | No | This task delivers a pure frontend component tree with no new seams between backend services. The hook-to-component boundary is tested via unit tests with mock hooks. |
| System | No | Pre-staging mode — no running server available. System tests will run at staging deploy (TASK-029). |
| Acceptance | Yes | Shell script with static analysis + vitest suite execution. One positive and at least one negative case per AC. |
| Performance | No | No performance fitness function is defined for this task by the Architect. |

---

## Acceptance Criteria Results

### AC-1 — Dashboard shows all registered workers with correct status indicators (green = online, red = down)

**Verdict: PASS**

**Requirement:** REQ-016
**Traced test:** `WorkerFleetDashboard.test.tsx` — "status indicators (AC-1)" describe block

Given   a worker list containing one online and one down worker
When    WorkerFleetDashboard renders the table
Then    `[data-status="online"]` is present in the DOM for the online worker, and `[data-status="down"]` for the down worker

**Positive evidence:**
- `StatusDot` sub-component sets `data-status={status}` on its outer span, allowing both programmatic selection and accessibility targeting.
- Online state uses `var(--color-success)` (#16A34A per DESIGN.md); down state uses `var(--color-error)` (#DC2626).
- Both states render the text labels "Online" and "Down" alongside the colored dot.
- `aria-label="Worker status: Online"` / `aria-label="Worker status: Down"` is present on every StatusDot.
- `role="status"` is present on the StatusDot span.

**Negative evidence (WCAG compliance):**
- [VERIFIER-ADDED] Text labels "Online" and "Down" must accompany the color dot — color alone is never the sole indicator. Verified present in source.
- [VERIFIER-ADDED] `aria-label` is present so screen readers can announce worker status without visual rendering. Verified present.

---

### AC-2 — Summary cards show accurate counts (Total, Online, Down)

**Verdict: PASS**

**Requirement:** REQ-016
**Traced test:** `WorkerFleetDashboard.test.tsx` — "summary cards (AC-2)" describe block; `useWorkers.test.ts` — "summary counts" describe block

Given   a worker list with 3 workers: 2 online and 1 down
When    the summary is computed
Then    the Total card shows 3, Online shows 2, Down shows 1

**Positive evidence:**
- `SummaryCard` components receive `summary.total`, `summary.online`, and `summary.down` from `useWorkers()`.
- `computeSummary` in `useWorkers.ts` derives counts by filtering the live workers array: `workers.filter(w => w.status === 'online').length` and `workers.filter(w => w.status === 'down').length`.
- Summary is recomputed on every render, keeping it consistent with the live workers array after SSE updates.
- Down card uses `var(--color-error)` for its value when `summary.down > 0`, consistent with DESIGN.md semantic colors.

**Negative evidence:**
- [VERIFIER-ADDED] `computeSummary` derives counts by filtering — it does not hard-code values. Verified by source inspection.

**Deviation note:** The UX spec lists an "Avg Load" fourth summary card. This card was not implemented because the `Worker` domain type in `domain.ts` carries no CPU% or memory% fields. The three implemented cards satisfy all 8 acceptance criteria. The fourth card requires a backend change (adding fields to `Worker`) before it can be added. This is an observation, not a blocking failure.

---

### AC-3 — Worker going down updates in real time without page refresh

**Verdict: PASS**

**Requirement:** REQ-016, REQ-004
**Traced test:** `useWorkers.test.ts` — "SSE: worker:down" describe block (2 tests)

Given   useWorkers is subscribed to SSE `/events/workers` and the worker list has an online worker
When    a `worker:down` SSE event arrives with the matching worker ID
Then    the worker's status in React state becomes 'down', and summary.down increments

**Positive evidence:**
- `mergeWorkerEvent` handles `case 'worker:down'`: maps workers, finds the worker by `w.id === event.payload.id`, and returns `{ ...w, status: 'down' }`.
- `setWorkers(current => mergeWorkerEvent(current, event))` applies the update via functional state update, which React batches safely.
- No page refresh is needed — the update flows through SSE → `handleWorkerEvent` → `setWorkers` → component re-render.
- `WorkerFleetDashboard` re-sorts the `sortedWorkers` memo on each state change, so a newly downed worker immediately moves to the top of the table.

**Negative evidence:**
- [VERIFIER-ADDED] The handler matches by ID — it does not set all workers to down. Verified `w.id === event.payload.id` predicate is present.

---

### AC-4 — Worker coming online updates in real time

**Verdict: PASS**

**Requirement:** REQ-016, REQ-004
**Traced test:** `useWorkers.test.ts` — "SSE: worker:registered" describe block (2 tests); "SSE: worker:heartbeat" describe block (2 tests)

Given   useWorkers is subscribed to SSE `/events/workers`
When    a `worker:registered` SSE event arrives for a new worker
Then    the worker is added to the workers array and appears in the table without a page refresh

**Positive evidence:**
- `mergeWorkerEvent` handles `case 'worker:registered'`: adds `event.payload` to the list if not already present.
- `mergeWorkerEvent` handles `case 'worker:heartbeat'`: merges full payload into the matching worker, keeping `lastHeartbeat` and `status` current.
- No page refresh required — same reactive path as AC-3.

**Negative evidence:**
- [VERIFIER-ADDED] Deduplication guard `workers.some(w => w.id === event.payload.id)` prevents double-adding if a `worker:registered` event arrives for an already-known worker. Verified present.

---

### AC-5 — Table columns are sortable by click

**Verdict: PASS**

**Requirement:** REQ-016
**Traced test:** `WorkerFleetDashboard.test.tsx` — "sortable columns (AC-5)" describe block (2 tests)

Given   the data table is rendered with multiple workers
When    the Admin clicks the "Worker ID" column header
Then    the table sorts ascending by Worker ID, and ▲ appears on the header

When    the Admin clicks "Worker ID" again
Then    the sort reverses to descending, and ▼ appears

**Positive evidence:**
- All five columns use `SortableHeader` sub-component: Status, Worker ID, Tags, Current Task, Last Heartbeat.
- `SortableHeader` renders a `<th>` with `onClick={() => onSort(column)}`, `aria-sort` attribute, and a directional caret.
- `handleSort` toggles direction: `prev.direction === 'asc' ? 'desc' : 'asc'` when the same column is clicked again.
- `sortWorkers` is wrapped in `useMemo` keyed on `[workers, sortState]`, ensuring efficient re-sorting on each click.

**Negative evidence:**
- [VERIFIER-ADDED] `onClick` handler is present on the `<th>` element — inert headers would not trigger any sort. Verified.
- [VERIFIER-ADDED] Toggle logic is present in `handleSort` — a handler that only ever sets 'asc' would fail on the second click. Verified.

---

### AC-6 — Down workers sorted to top by default

**Verdict: PASS**

**Requirement:** REQ-016
**Traced test:** `WorkerFleetDashboard.test.tsx` — "default sort (AC-6)" describe block (1 test)

Given   the Worker Fleet Dashboard renders with a mix of online and down workers
When    the page first loads (no column click)
Then    down workers appear above online workers — the first data row is a down worker

**Positive evidence:**
- `DEFAULT_SORT` is `{ column: 'status', direction: 'asc' }`.
- `statusSortWeight` maps `'down'` to 0 and all other statuses to 1.
- Ascending sort (lowest weight first) places down workers (0) before online workers (1).
- `useState<SortState>(DEFAULT_SORT)` applies this sort from the first render.

**Negative evidence:**
- [VERIFIER-ADDED] `statusSortWeight` assigns `down=0` (lower) and `online=1` (higher). A reversed assignment (`down=1`) would produce the wrong default order. Verified.

---

### AC-7 — SSE disconnection shows "Reconnecting..." in status bar

**Verdict: PASS**

**Requirement:** REQ-016
**Traced test:** `WorkerFleetDashboard.test.tsx` — "SSE status bar (AC-7)" describe block (3 tests); `useSSE.test.ts` — "reconnecting state" describe block (2 tests)

Given   the Worker Fleet Dashboard is open and the SSE connection drops
When    `useSSE` fires its `onerror` handler
Then    `sseStatus` transitions to `'reconnecting'`
        and `StatusBar` renders a red dot with the text "Reconnecting..."

**Positive evidence:**
- `useSSE` transitions to `'reconnecting'` on the `onerror` callback: closes the dead `EventSource`, calls `setStatus('reconnecting')`, and schedules a new `connect()` call after exponential backoff (initial 1 s, max 30 s).
- `StatusBar` reads `sseStatus` from `useWorkers().sseStatus` and renders "Reconnecting..." conditionally when `sseStatus === 'reconnecting'`.
- The status bar carries `role="status"` and `aria-live="polite"` so screen readers announce the state change.
- "Connected" is shown (green dot) when `sseStatus === 'connected'`. "Reconnecting..." is absent in this state.

**Negative evidence:**
- [VERIFIER-ADDED] "Reconnecting..." is conditionally rendered only when `sseStatus === 'reconnecting'` — not always shown. Verified by source inspection.
- [VERIFIER-ADDED] `useSSE` has a `'reconnecting'` state in its `SSEConnectionStatus` union type. Verified.

---

### AC-8 — Empty state message shown when no workers registered

**Verdict: PASS**

**Requirement:** REQ-016
**Traced test:** `WorkerFleetDashboard.test.tsx` — "empty state (AC-8)" describe block (2 tests)

Given   `GET /api/workers` returns an empty array (or fails and defaults to empty)
When    the Worker Fleet Dashboard renders after `isLoading` transitions to `false`
Then    the centered message "No workers registered. Workers self-register when they start." appears
        and the data table (`role="table"`) is not in the DOM

**Positive evidence:**
- The ternary `workers.length === 0 ? <empty state> : <data table>` provides mutually exclusive rendering.
- The empty state message matches the UX spec text exactly.
- The data table is not rendered — `queryByRole('table')` returns null in the unit test.
- Summary cards still show 0 / 0 / 0 counts, giving the Admin a complete picture.

**Negative evidence:**
- [VERIFIER-ADDED] The empty state branch renders a `<p>` element (not a table). The ternary structure prevents both from appearing simultaneously. Verified by grep inspection of source and unit test.

---

## Builder Deviations — Evaluation

The Builder documented three deviations from the task description and UX spec. None constitute an AC failure:

| Deviation | Reason | AC Impact |
| Avg Load summary card absent | `Worker` domain type in `domain.ts` carries no CPU% or memory% fields. Including a card with no data source would be misleading. | None — Avg Load is not referenced in any of the 8 ACs. |
| Hostname column absent | `Worker` domain type has no `hostname` field. An empty-only column would produce a misleading table. | None — Hostname is not referenced in any of the 8 ACs. |
| CPU% and Memory% columns absent | Same reason as Hostname — fields absent from domain type. | None — CPU%/Memory% not referenced in any of the 8 ACs. |

All three deviations require a backend task (adding fields to the `Worker` model and `GET /api/workers` response) before the corresponding frontend columns can be implemented. This is correctly deferred per the Builder's analysis.

---

## Test Execution Results

### Unit tests (72 total)

```
Test Files  8 passed (8)
     Tests  72 passed (72)
  Duration  ~5s
```

All 72 tests pass. The 41 new tests for TASK-020 (18 `WorkerFleetDashboard`, 12 `useWorkers`, 11 `useSSE`) each trace to a specific AC. 31 pre-existing tests (TASK-019) continue to pass.

### Acceptance tests (36 checks)

```
Passed: 36
Failed: 0
VERDICT: PASS
```

Acceptance test file: `tests/acceptance/TASK-020-acceptance.sh`

### Build

```
vite v5.4.21 building for production...
✓ 49 modules transformed.
✓ built in 1.45s
```

### Typecheck

```
tsc --noEmit
(zero errors — clean exit)
```

---

## Observations

**SSE reconnection does not re-fetch REST state.** The UX spec says "Fetch current state via `GET /api/workers` on reconnect." `useWorkers` currently only fetches on mount. On SSE reconnect, the worker list remains as-is and SSE events bring it up to date incrementally. This is a known limitation noted by the Builder. It is not an AC failure — AC-7 only requires "Reconnecting..." to appear in the status bar, which it does. A future task should add a re-fetch on reconnect to close the gap with the UX spec.

**`formatHeartbeat` renders relative time ("2s ago") rather than a raw ISO timestamp.** The UX spec wireframe shows the raw value. Relative time is more useful operationally and is consistent with dashboard convention. This is not an AC failure. The UX spec notes this column should show "Last Heartbeat" without prescribing an exact format in the acceptance criteria.

**"Last updated: X ago" stale-data indicator absent.** The UX spec mentions showing how long ago data was last updated when SSE is disconnected. The status bar shows "Reconnecting..." (satisfying AC-7) but does not compute a staleness timestamp. This requires `setInterval` polling and is correctly out of scope for this task's ACs.

---

## Demo Script

Produced at: `tests/demo/TASK-020-demo.md`

Nine scenarios covering all 8 acceptance criteria, plus a skeleton loading scenario not in the ACs but specified by the UX spec. Written for Admin role execution against a staging environment with at least one online worker.

---

## Commit and CI

Per commit-discipline protocol: all acceptance tests pass, build is clean, typecheck is clean. Proceeding to commit.
