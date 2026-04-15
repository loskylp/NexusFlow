# Routing Instruction

**To:** @nexus-verifier
**Phase:** EXECUTION -- Cycle 4 -- TASK-032 (Sink Inspector GUI)
**Task:** Verify TASK-032 against its six acceptance criteria, exercise admin-only guard in CI, confirm the full web test suite is green on GitHub Actions, and produce the Verification Report plus Demo Script.
**Iteration:** 1 of 3 (initial verification)
**Verifier mode:** Initial verification
**Return to:** Orchestrator when complete

---

## Required documents

| Document | Link | Why needed |
|---|---|---|
| Task Plan -- TASK-032 entry | [process/planner/task-plan.md#task-032-sink-inspector-gui](../planner/task-plan.md) (lines 637-652) | Six acceptance criteria, demo script path, dependencies |
| Builder handoff note | [process/builder/handoff-notes/TASK-032-handoff.md](../builder/handoff-notes/TASK-032-handoff.md) | Implementation summary, test inventory, AC-to-test mapping, commit f3c9a95 |
| UX Spec -- Sink Inspector | [process/designer/ux-spec.md](../designer/ux-spec.md) (§ Sink Inspector, line 345+) | Panel states (default, before-only, after-success, after-rollback), delta highlight rules, ROLLED BACK badge |
| ADR-009 -- Sink atomicity and inspector | [process/architect/adr/ADR-009-sink-atomicity-and-inspector.md](../architect/adr/ADR-009-sink-atomicity-and-inspector.md) | SSE channel contract, sink:before-snapshot / sink:after-result event shapes |
| TASK-033 Verification Report | [process/verifier/verification-reports/TASK-033-verification.md](../verifier/verification-reports/TASK-033-verification.md) | Upstream snapshot producer -- inspector consumes these events |
| Dependency -- TASK-019 (auth flow) | [tests/demo/TASK-019-demo.md](../../tests/demo/TASK-019-demo.md) | RequireRole integration pattern (admin-only guard) |

---

## Skills required

- [`.claude/skills/bash-execution.md`](../../.claude/skills/bash-execution.md)
- [`.claude/skills/commit-discipline.md`](../../.claude/skills/commit-discipline.md)
- [`.claude/skills/demo-script-execution.md`](../../.claude/skills/demo-script-execution.md)

---

## Context

Builder reports TASK-032 complete at commit `f3c9a95` on branch `main`:

- `web/src/pages/SinkInspectorPage.tsx` + 24 unit tests
- `web/src/hooks/useSinkInspector.ts` + 19 unit tests
- `tests/acceptance/TASK-032-acceptance.test.tsx` -- 10 tests covering AC-1 through AC-6 (with AC-4 covering delta highlights + checkmark together per handoff note §Acceptance Criteria → Test Mapping)
- Admin-only route guard using `RequireRole` (same pattern as TASK-020 / TASK-021)
- Local test run: 24 + 19 + 10 pass; full `npm run test` suite: 627 passed, 0 failed
- Builder reports no known gaps against the UX spec's four panel states + waiting-spinner state

**Your job:**
1. Produce the Verification Report at `process/verifier/verification-reports/TASK-032-verification.md` with each of the six ACs mapped to executed tests (pass/fail), and confirm the AC-to-test mapping in the Builder handoff is accurate -- note any AC that lacks a covering test.
2. Trigger CI on the current HEAD (commit f3c9a95) and record the GitHub Actions run ID + green status. Verifier PASS requires CI green, not just local green.
3. Exercise the admin-only guard end-to-end: confirm a non-admin user receives the access-denied state, and the SSE subscription is NOT established for a non-admin. AC-6 is a security-adjacent AC; do not accept it on unit-test evidence alone if the acceptance test does not cover the full route-guard path.
4. Verify the SSE event contract: `sink:before-snapshot` populates the Before panel, `sink:after-result` populates the After panel, rollback path renders "ROLLED BACK" badge with After == Before. Cross-check event shapes against TASK-033 Verification Report and ADR-009.
5. Produce the Demo Script at `tests/demo/TASK-032-demo.md` per the Task Plan (path declared in task-plan.md line 651). Demo should walk a reviewer through: login as admin, select a task running a sink, observe Before panel, observe After panel with delta highlights, then a rollback scenario showing ROLLED BACK badge.
6. Record any non-blocking observations as OBS-032-N entries in the report.

**If any AC fails or CI is red:** produce the report with findings and return to Orchestrator for iterate-loop dispatch. Do not commit Builder fixes.

**On PASS:** commit the Verification Report and Demo Script per commit-discipline.md, then return to Orchestrator.
