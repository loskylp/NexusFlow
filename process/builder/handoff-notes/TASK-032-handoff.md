# Builder Handoff Note — TASK-032

**Task:** TASK-032 — Sink Inspector (GUI)
**Date:** 2026-04-15
**Commit SHA:** f3c9a95
**Status:** Complete

---

## What Was Built

### 1. `web/src/hooks/useSinkInspector.ts`
Implements the `useSinkInspector` hook. Accepts `{ taskId: string | null }` and manages:
- SSE subscription to `GET /events/sink/{taskId}` via the existing `useSSE` hook
- State machine: idle (no task) → connecting → waiting-for-sink-phase → before-received → after-received
- `sink:before-snapshot` event: populates `beforeSnapshot`, clears `afterSnapshot`, sets `isWaitingForSinkPhase = false`
- `sink:after-result` event: populates `afterSnapshot`, sets `rolledBack` and `writeError`
- Task change: resets all snapshot state before re-subscribing
- Access error: `sink:error` and `access:denied` event types surface as `accessError`
- Normalises empty `writeError` string from backend to `null`
- Before snapshot recovered from After event when Before event was missed

### 2. `web/src/pages/SinkInspectorPage.tsx`
Implements the full Sink Inspector page. Sub-components:
- `SinkInspectorHeader` — title, DEMO badge, SSE monitoring status dot
- `TaskSelector` — dropdown populated from `useTasks`, task change propagated up
- `SnapshotPanel` (exported) — handles all four UX states: default/empty, waiting spinner (`role="progressbar"`), before-snapshot, after-result; delta diff with green-50 (`#F0FDF4`) highlight for new/changed keys
- `AtomicityVerification` (exported) — neutral placeholder, green checkmark with delta summary on success, ROLLED BACK badge + error on rollback
- `SinkInspectorContent` — extracted to satisfy Rules of Hooks (hooks unconditionally called)
- Admin-only guard: non-admin users see an "Access denied" message; `useSinkInspector` 403 access error also surfaces here

### 3. Unit Tests
- `web/src/hooks/useSinkInspector.test.ts` — 24 tests covering all hook states, task-change reset, recovered before snapshot, cleanup on unmount
- `web/src/pages/SinkInspectorPage.test.tsx` — 19 tests covering admin access, non-admin access, all panel states, AtomicityVerification, SnapshotPanel sub-components

### 4. Acceptance Tests
- `tests/acceptance/TASK-032-acceptance.test.tsx` — 10 tests, one per acceptance criterion (AC-1 through AC-9, AC-7 covers both delta highlights and checkmark together)

---

## Acceptance Criteria → Test Mapping

| AC | Description | Test Location |
|----|-------------|---------------|
| AC-1 | Selecting a task subscribes to SSE channel | `AC-4` acceptance test |
| AC-2 | Before snapshot displayed when sink phase begins | `AC-5` acceptance test |
| AC-3 | After result displayed when sink phase completes or rolls back | `AC-6` acceptance test |
| AC-4 | Success: delta summary with new/changed items highlighted | `AC-7` acceptance test + page unit tests |
| AC-5 | Rollback: After panel matches Before; ROLLED BACK badge | `AC-8` acceptance test |
| AC-6 | Admin-only: User role cannot access | `AC-2` acceptance test + page unit tests |

---

## Admin-Only Guard Verification

The admin guard is enforced at two levels:
1. **In-component check** (`SinkInspectorPage`): if `user?.role !== 'admin'`, renders "Access denied" message before any hooks fire for the inspector content.
2. **SSE endpoint check** (server-side): the backend `GET /events/sink/{taskId}` returns 403 for non-admin callers; the hook surfaces this as `accessError` in the return value.

The sidebar already hides Demo section links from non-admin users (TASK-019 Sidebar implementation).

Unit test "shows access denied message for user role" and acceptance test "AC-2: Non-admin user sees access denied" both verify the guard explicitly.

---

## No Backend Changes

The `GET /events/sink/{taskId}` SSE endpoint was already wired and implemented by TASK-033/TASK-015:
- Route: `api/server.go` line 194
- Handler: `api/handlers_sse.go` — `SSEHandler.Sink`
- Broker: `ServeSinkEvents` already existed

No Go files were modified.

---

## Deviations

None. All six acceptance criteria are implemented as specified. The UX spec states for all four panel states from the UX spec: default, before-only, after-success, after-rollback — plus the "waiting for sink phase" spinner state — are all implemented.

---

## Test Run Summary

```
web/src/hooks/useSinkInspector.test.ts   24/24 passed
web/src/pages/SinkInspectorPage.test.tsx  19/19 passed
tests/acceptance/TASK-032-acceptance.test.tsx  10/10 passed
Full web suite: 627 passed, 0 failed (pre-existing: 27 todo, 2 skipped)
```
