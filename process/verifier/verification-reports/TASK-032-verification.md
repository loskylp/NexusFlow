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

# Verification Report — TASK-032
**Date:** 2026-04-15 | **Result:** PASS
**Task:** Sink Inspector (GUI) | **Requirement(s):** DEMO-003, ADR-009, UX Spec (Sink Inspector)

## Acceptance Criteria Results

Criterion | Layer | Result | Notes
--- | --- | --- | ---
AC-1: Selecting a task subscribes to SSE channel GET /events/sink/{taskId} | Acceptance | PASS | `AC-4: Selecting a task subscribes to the SSE channel` — confirms EventSource opened for `/events/sink/{taskId}` on task selection; negative case: no EventSource opened before task selected (AC-1 renders without task, no sink instances)
AC-2: Before snapshot displayed when sink phase begins | Acceptance | PASS | `AC-5: Before panel populates on sink:before-snapshot` — confirms `object_count` and `bucket` keys rendered in Before panel after event fires; negative case: keys absent before event fires (AC-3 task-selector test confirms panels start empty)
AC-3: After result displayed when sink phase completes or rolls back | Acceptance | PASS | `AC-6: After panel populates on sink:after-result` — confirms `new_key` rendered in After panel after after-result event; rollback path verified in AC-8
AC-4: Successful completion — delta summary shows new/changed items highlighted | Acceptance | PASS | `AC-7: Delta highlights and atomicity checkmark on successful write` — confirms `aria-label="Atomicity verified"` checkmark rendered and "Write committed successfully" text shown after success event; delta highlight logic (`#F0FDF4` green-50 background for changed/new keys) verified in Builder page unit tests
AC-5: Rollback — After panel matches Before; "ROLLED BACK" badge shown | Acceptance | PASS | `AC-8: Rollback — ROLLED BACK badge shown` — confirms `/rolled back/i` text and "write failed: connection reset" error message rendered after rollback event; After snapshot built with `{ ...before.data }` to confirm data parity
AC-6: Admin-only — User role cannot access this view | Acceptance | PASS | `AC-2: Non-admin user sees access denied` — user-role renders `/access denied/i` and no "Sink Inspector" heading; SSE guard analysis confirms no EventSource opened for non-admin (guard fires before SinkInspectorContent mounts, before useSinkInspector runs)

## Test Summary

Layer | Written | Passing | Failing
--- | --- | --- | ---
Integration | 0 | — | —
System | 0 | — | —
Acceptance | 10 | 10 | 0
Performance | 0 | — | —

**Builder unit tests:** 24 hook tests (useSinkInspector.test.ts) + 19 page tests (SinkInspectorPage.test.tsx) = 43 of 43 passing.

**Full web suite:** 627 tests passed, 0 failed, 27 todo, 2 skipped (consistent with pre-existing baseline).

**Note on integration and system layers:** No integration or system test files were authored. The component has no new seams at the server boundary (no Go backend changes — the SSE endpoint was wired by TASK-033/TASK-015). Integration-layer coverage for the SSE contract is provided by TASK-033's integration tests (TASK-033-IT1 through IT6, all passing). Acceptance tests exercise the full component-to-SSE-event path through the public `SinkInspectorPage` interface, which substitutes for a separate system test layer here.

## AC-6 Admin Guard: Detailed Analysis

**Routing instruction requirement:** Confirm SSE subscription is NOT established for non-admin.

**Mechanism at the UI layer:**

1. `SinkInspectorPage` checks `user?.role !== 'admin'` as its first branch. When true, it returns the access-denied JSX immediately and never renders `SinkInspectorContent`.
2. `useSinkInspector` (which opens the EventSource) is called only inside `SinkInspectorContent`. Because `SinkInspectorContent` is never mounted for non-admin users, no `EventSource` is ever constructed.
3. Acceptance test `AC-2` verifies this: after `mockUseAuth` returns a user-role user, `renderPage()` shows `/access denied/i` but no task selector combobox — confirming `SinkInspectorContent` was not rendered.
4. `SinkInspectorPage.test.tsx` "does not render the task selector for user role" additionally confirms no combobox is present, closing the path to any EventSource construction.

**Mechanism at the backend layer:**

The SSE endpoint `/events/sink/{taskId}` is in the `protected` group but does NOT apply `auth.RequireRole(models.RoleAdmin)`. Instead, `ServeSinkEvents` calls `authoriseTaskAccess`, which returns HTTP 403 for callers who are not the task owner or an admin. This means a user-role caller who happens to own the task can technically subscribe to the sink channel for that task. The in-component guard in `SinkInspectorPage` prevents this path in the web client but the API is not strictly admin-only at the HTTP layer. See OBS-032-1.

**Verdict for AC-6:** PASS — the UI-layer admin guard is correctly implemented. The acceptance test confirms non-admin users see access denied and no SSE subscription is established. The backend access control gap is noted as a non-blocking observation.

## Acceptance Test Run

```
npm --prefix web run test -- --reporter=verbose --run tests/acceptance/TASK-032-acceptance.test.tsx

 ✓ AC-1: Sink Inspector page renders for admin > renders the "Sink Inspector" heading for an Admin user
 ✓ AC-1: Sink Inspector page renders for admin > renders the DEMO badge
 ✓ AC-2: Non-admin user sees access denied > shows access denied message and no page content for user role
 ✓ AC-3: Task selector dropdown lists recent tasks > renders a combobox with the two test tasks as options
 ✓ AC-4: Selecting a task subscribes to the SSE channel > opens EventSource for /events/sink/{taskId} on task selection
 ✓ AC-5: Before panel populates on sink:before-snapshot > shows snapshot data keys in the Before panel after event fires
 ✓ AC-6: After panel populates on sink:after-result > shows After panel data after sink:after-result event fires
 ✓ AC-7: Delta highlights and atomicity checkmark on successful write > shows atomicity verified checkmark on successful write
 ✓ AC-8: Rollback — ROLLED BACK badge shown > shows ROLLED BACK badge and error message when rollback fires
 ✓ AC-9: Changing selected task resets snapshot state > clears Before panel data when a different task is selected

 Test Files  1 passed (1)
      Tests  10 passed (10)
   Duration  1.87s
```

## Builder Unit Test Run

```
npm --prefix web run test -- --reporter=verbose --run src/hooks/useSinkInspector.test.ts src/pages/SinkInspectorPage.test.tsx

 Test Files  2 passed (2)
      Tests  43 passed (43)
   Duration  1.89s
```

## CI Run

Run ID: 24458872430 | Branch: main | Commit: 625cf79 (HEAD, includes f3c9a95)

Job | Result
--- | ---
Go Build, Vet, and Test | PASS
Frontend Build and Typecheck | PASS
Fitness Function Tests | PASS
Docker Build Smoke Test | PASS

All four CI jobs green. Note: run 24458872430 is for the HEAD process commit; f3c9a95 (the TASK-032 implementation commit) was already on main at that point and passed the same CI pipeline. No CI run specifically for f3c9a95 exists in isolation because subsequent process commits atop main all passed.

## SSE Event Contract: Cross-check Against ADR-009 and TASK-033

ADR-009 §Execution flow specifies:
- `sink:before-snapshot` published before Sink writes begin (step 4c)
- `sink:after-result` published on success or failure (steps 4e, 4f)
- Both events carry the full `SinkInspectorState` payload including `taskId`, `before`, `after`, `rolledBack`, `writeError`

TASK-033 Verification Report confirms these events are published on `events:sink:{taskId}` with the correct payload shape.

`useSinkInspector` consumes:
- `sink:before-snapshot` → sets `beforeSnapshot = payload.before`, clears `afterSnapshot` — correct
- `sink:after-result` → sets `afterSnapshot = payload.after`, `rolledBack = payload.rolledBack`, `writeError = payload.writeError || null` — correct; empty string from backend normalized to null
- `sink:error` / `access:denied` → surfaces as `accessError` — correct; matches backend 403 path per TASK-033 OBS-3

Event contract is correctly implemented and cross-checks against ADR-009 and TASK-033.

## Handoff Note: AC-to-Test Mapping Discrepancy

The handoff note's table maps Task Plan ACs to acceptance test labels (e.g., "AC-1 Selecting task subscribes to SSE → AC-4 acceptance test"). This label offset is confusing: the acceptance test file uses describe-block labels AC-1 through AC-9, where AC-1/AC-2 correspond to page-render and access-denied checks (not the Task Plan's AC-1). The Task Plan's six ACs are all covered — the mapping is correct in substance, only the label cross-referencing is misleading. Not a defect.

## Observations (non-blocking)

**OBS-032-1: Backend `/events/sink/{taskId}` is not strictly admin-only.** The SSE endpoint uses `authoriseTaskAccess` (admin OR task owner), not `RequireRole(models.RoleAdmin)`. A user-role caller who owns the task can subscribe to the sink channel via direct API call, bypassing the in-component admin guard. The Task Plan AC-6 says "Admin-only: User role cannot access this view" — the UI enforces this correctly, but the API allows task owners regardless of role. If strict admin-only is required at the API layer, the endpoint should be moved inside the `admin.Use(auth.RequireRole(models.RoleAdmin))` group. This is a known pattern in the codebase (TASK-017 user management, TASK-034 chaos endpoints both use the RequireRole group). Current implementation satisfies the AC as a UI concern.

**OBS-032-2: AC-4 delta highlight coverage is implicit.** The acceptance test suite does not directly assert the `background-color: #F0FDF4` highlight on changed/new rows in the After panel (this is a CSS style assertion that JSDOM does not evaluate). The delta highlight logic is covered by Builder page unit tests (SnapshotDataTable renders `#F0FDF4` row background for `isNew || isChanged` keys). The acceptance test suite verifies the checkmark and delta summary text but relies on the unit tests for the visual highlight assertion. This is a known limitation of JSDOM-based testing for inline styles; acceptable for this tier.

**OBS-032-3: Waiting-spinner animation requires a CSS `@keyframes spin` rule.** The `SinkInspectorPage` inlines `<style>{...}</style>` directly inside the component for the spin animation. This works but the keyframe string is re-evaluated on every render. A minor optimization would be to move this to a CSS module or a global stylesheet. Not a defect.

**OBS-032-4: Handoff note AC count mismatch.** The handoff note header says "10 tests covering AC-1 through AC-9" (i.e., 9 numbered AC groups with AC-1 containing 2 tests = 10 total). The task plan has 6 ACs mapped to these 9 test groups. All 6 task ACs are covered; the count reflects additional granularity (page-renders, badge check) beyond the 6 AC lines.

## Recommendation
PASS TO NEXT STAGE
