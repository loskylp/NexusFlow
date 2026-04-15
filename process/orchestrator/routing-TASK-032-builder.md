# Routing Instruction — Builder — TASK-032

**From:** Orchestrator
**To:** @nexus-builder
**Date:** 2026-04-15
**Cycle:** 4
**Task:** TASK-032 -- Sink Inspector (GUI)

---

## Objective

Implement the Sink Inspector GUI view against the Cycle 4 scaffold. The view subscribes to
an SSE channel for a selected task and renders Before/After snapshots emitted by the
Snapshot Capturer (TASK-033, COMPLETE) using the demo sinks (TASK-030 MinIO and TASK-031
PostgreSQL, both COMPLETE). Admin-only access.

Concretely:

1. Fill in `web/src/pages/SinkInspectorPage.tsx` (scaffold stub exists).
2. Fill in `web/src/hooks/useSinkInspector.ts` (scaffold stub exists) — SSE subscription
   to `GET /events/sink/{taskId}`, state machine for the four panel states.
3. Wire the page into the router and sidebar navigation (Admin-only, under the Demo
   section — same role-guard pattern as the existing Chaos Controller placeholder).
4. If a backend SSE endpoint for `/events/sink/{taskId}` is not yet exposed, add it —
   bridging the `sink:before-snapshot` and `sink:after-result` events produced by
   TASK-033. Check current backend state before adding to avoid duplication.
5. Produce the acceptance test body in `tests/acceptance/TASK-032-acceptance.test.tsx`.

## Acceptance Criteria (from Task Plan)

- Selecting a task subscribes to SSE channel for sink events
- Before snapshot displayed when sink phase begins
- After result displayed when sink phase completes (or rolls back)
- Successful completion: delta summary shows new/changed items highlighted
- Rollback: After panel matches Before panel; "ROLLED BACK" badge shown
- Admin-only: User role cannot access this view

## Required Documents

- Task definition: [process/planner/task-plan.md](../planner/task-plan.md) — lines 637–652 (TASK-032)
- UX specification: [process/designer/ux-spec.md](../designer/ux-spec.md) — lines 345–366 (Sink Inspector panel zones, states, interactions)
- Designer screenshot: [process/designer/screenshots/06-sink-inspector.png](../designer/screenshots/06-sink-inspector.png)
- Scaffold manifest: [process/scaffolder/scaffold-manifest.md](../scaffolder/scaffold-manifest.md) — Cycle 4 files-added table (lines ~30–60, ~100–113)
- Page stub to implement: [web/src/pages/SinkInspectorPage.tsx](../../web/src/pages/SinkInspectorPage.tsx)
- Hook stub to implement: [web/src/hooks/useSinkInspector.ts](../../web/src/hooks/useSinkInspector.ts)
- Acceptance test scaffold: [tests/acceptance/TASK-032-acceptance.test.tsx](../../tests/acceptance/TASK-032-acceptance.test.tsx)
- Snapshot event producer (Before/After payload shape): [worker/snapshot.go](../../worker/snapshot.go) and related TASK-033 commit `fb4b3d8`
- SSE reference pattern: existing SSE hook in use for Task Feed / Log Streamer — follow the same EventSource + `Last-Event-ID` pattern

## Dependencies (all satisfied)

- TASK-019 -- React app shell with sidebar + auth flow (COMPLETE Cycle 1)
- TASK-015 -- SSE event infrastructure (COMPLETE Cycle 1)
- TASK-033 -- Sink Before/After snapshot capture (COMPLETE 2026-04-15, commit fb4b3d8)
- TASK-030 -- MinIO Fake-S3 (COMPLETE 2026-04-15)
- TASK-031 -- Mock-Postgres with seed data (COMPLETE 2026-04-15, commit e4d5d87)

## Reminders

- **Admin-only guard:** Non-admin users must not see the view in the sidebar and must
  not be able to reach the route. Reuse the role-guard pattern already established for
  the other Admin-only demo views — do not reinvent.
- **SSE lifecycle:** The EventSource must be torn down on task re-selection and on
  unmount. Leaked EventSource connections were flagged in prior GUI reviews — avoid.
- **Rollback rendering:** When the After panel matches Before, render the "ROLLED BACK"
  badge; do not silently treat rollback as success. The atomicity verification section
  is the load-bearing element for the demo narrative.
- **Delta highlighting:** New/changed items should be highlighted with `green-50` per
  the UX spec; do not invent a different accent colour.
- **Nil/empty-state behaviour:** All four states from the UX spec must render —
  default, before-only, after-success, after-rollback — plus the "waiting for sink
  phase" spinner state.
- **Scaffold hygiene:** Cycle 4 scaffold historically carried four regressions
  (REG-030); before claiming completion, run the full web test suite and ensure CI is
  green. Do not leave TODO stubs in shipped files.
- Commit working increments. Report final commit SHA, acceptance pass summary, and
  explicit confirmation that the Admin-only guard is wired on completion.

## Exit Criteria for Your Handoff

- All 6 acceptance criteria implementable against the running demo stack.
- `tests/acceptance/TASK-032-acceptance.test.tsx` passes locally.
- Web + Go CI green.
- Admin-only guard explicitly verified (User role cannot navigate to the route).
- Final commit SHA reported.

---

**Next:** Invoke @nexus-orchestrator — on completion, report commit SHA, acceptance
summary, and Admin-only guard verification so Verifier can be dispatched.
