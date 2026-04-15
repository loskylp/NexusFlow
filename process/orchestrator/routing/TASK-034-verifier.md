# Routing Instruction

**To:** @nexus-verifier
**Phase:** EXECUTION -- Cycle 4
**Task:** Verify TASK-034 (Chaos Controller GUI -- worker kill/pause/resume + DB disconnect controls with admin enforcement) against all acceptance criteria and author + execute the demo script.
**Iteration:** 1 of 3
**Verifier mode:** Initial verification
**Return to:** Orchestrator when complete

---

## Required documents

| Document | Link | Why needed |
|---|---|---|
| TASK-034 spec + acceptance criteria | [process/planner/task-plan.md -- TASK-034](../planner/task-plan.md#task-034) | Acceptance criteria, demo script path, dependencies |
| Builder handoff | [process/builder/handoff-notes/TASK-034-handoff.md](../builder/handoff-notes/TASK-034-handoff.md) | What was built, deviations, integration check steps |
| DEMO-004 / chaos requirement | [process/analyst/requirements.md](../analyst/requirements.md) | Source requirement for Chaos Controller GUI |
| Architecture -- admin role + RequireRole middleware | [process/architect/architecture.md](../architect/architecture.md) | Existing admin-only middleware pattern to reuse |
| TASK-032 verification (peer GUI + OBS-032-1) | [process/verifier/verification-reports/TASK-032-report.md](../verifier/verification-reports/TASK-032-report.md) | Peer demo-infra GUI precedent; confirm OBS-032-1 pattern resolved here via API-level RequireRole(Admin) |
| Builder commit | commit `bfd22dd` | Code under verification |

---

## Skills required

- [`.claude/skills/bash-execution.md`](../../.claude/skills/bash-execution.md) -- absolute paths; no `cd dir && cmd`
- [`.claude/skills/demo-script-execution.md`](../../.claude/skills/demo-script-execution.md) -- author `tests/demo/TASK-034-demo.md` and execute it
- [`.claude/skills/commit-discipline.md`](../../.claude/skills/commit-discipline.md) -- commit the Verification Report when complete
- [`.claude/skills/traceability-links.md`](../../.claude/skills/traceability-links.md) -- link Verification Report back to ACs and requirements

---

## Context

TASK-034 delivers the Chaos Controller GUI -- admin controls to kill/pause/resume workers and disconnect the database, to drive resilience demonstrations against the live demo stack. It is the last GUI-layer demo-infrastructure piece before SEC-001 and TASK-038.

**Builder-reported state (commit bfd22dd):**
- Chaos Controller page + handlers implemented; wired into app shell
- Admin enforcement via existing `RequireRole(Admin)` middleware at the API layer -- deliberately addresses OBS-032-1 (UI-only admin gate in TASK-032) by enforcing at server boundary, not just UI
- 670 frontend tests pass in Builder env
- Go tests UNVERIFIED in Builder env -- Verifier must confirm via `go test` and CI run

**Builder-flagged deviations (require explicit Verifier disposition in the report):**

1. **Docker socket mount on base `api` service is not profile-gated.** Builder annotated for removal under TASK-036 (production environment). Verifier must record this as an observation (OBS-034-N), confirm the annotation comment is present in `docker-compose.yml`, and confirm non-demo profiles do not expose the socket unintentionally at runtime. Do NOT treat as a FAIL unless an acceptance criterion explicitly forbids it -- this is a known-deferred hardening item.

2. **`DisconnectDatabase` uses `docker stop` / `docker start` instead of `docker pause` / `docker unpause`.** Builder rationale: `pause` keeps the TCP connection alive from the client's perspective, which does not exercise the connection-loss recovery path the demo is meant to show. Verifier must confirm the stop/start behaviour actually produces the intended observable failure mode (dropped connections, reconnect/retry exercised by TASK-010 / TASK-011 paths) and that the recovery ("reconnect") path restores full functionality. Record as an observation regardless of outcome; FAIL only if the chaos action does not produce the effect the AC requires.

**Verification focus:**

1. Run the full frontend test suite against commit `bfd22dd` -- confirm Builder's 670-pass claim reproduces.
2. Run `go test ./...` (or equivalent for the worker/API packages touched) -- Builder did not execute these; catching regressions here is the Verifier's job.
3. CI run: confirm a green CI run at `bfd22dd` (or the next orchestrator-visible commit) covering both Go and frontend.
4. Admin enforcement: exercise each chaos endpoint (kill/pause/resume/disconnect-db) as a non-admin session -- must return 403 at the API layer, not merely a hidden UI control. Then repeat as admin -- must succeed. This explicitly closes OBS-032-1 for the TASK-034 surface.
5. Live demo stack exercise:
   - `docker compose --profile demo up` stack healthy
   - Kill a worker from the GUI -- fleet dashboard reflects "down"; monitor service (TASK-009) detects failure; tasks reassigned via TASK-010/TASK-011 paths
   - Pause / resume a worker -- heartbeat gap observed, then resumes without re-registration issues
   - Disconnect database -- worker logs show connection loss and retry (TASK-010); reconnect restores normal operation; any tasks that were mid-flight handled per cancellation/retry rules
6. Regression check on TASK-020 (Worker Fleet Dashboard) and TASK-021 (Task Feed) -- these are declared dependencies; regression in either is a FAIL.

**Demo script:** `tests/demo/TASK-034-demo.md` does not yet exist. Author it per `demo-script-execution.md` and execute it as part of this pass. Model it on `tests/demo/TASK-032-demo.md` (most recent peer).

**Iteration bounds:** max 3 per Manifest v1. If the implementation cannot converge in 3 iterations, escalate to the Orchestrator -- do not silently extend the loop.

Return a Verification Report at `process/verifier/verification-reports/TASK-034-report.md` with PASS/FAIL per AC, observations (OBS-034-N -- minimum two, covering the two Builder-flagged deviations), disposition on whether OBS-032-1 is closed by the API-level enforcement added here, and the completed demo script output. Commit the report when complete.

---

**Next:** Invoke @nexus-orchestrator -- Verifier complete for TASK-034.
