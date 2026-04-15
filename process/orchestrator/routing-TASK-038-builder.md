# Routing Instruction

**To:** @nexus-builder
**Phase:** EXECUTION -- Cycle 4
**Task:** Implement TASK-038 -- Fitness function instrumentation (final Cycle 4 task)
**Iteration:** 1 of 3
**Return to:** Orchestrator when complete

---

## Required documents

| Document | Link | Why needed |
|---|---|---|
| Task spec -- TASK-038 | [task-plan.md#task-038](../planner/task-plan.md) (lines 673-686) | Acceptance criteria and FF scope |
| Fitness functions catalogue | [fitness-functions.md](../architect/fitness-functions.md) | Authoritative FF definitions and thresholds |
| Scaffold manifest -- TASK-038 section | [scaffold-manifest.md](../scaffolder/scaffold-manifest.md) (lines 521-545, 593-625) | Scaffolded test file layout and per-FF status |
| FF test stub file | [tests/system/TASK-038-fitness-functions_test.go](../../tests/system/TASK-038-fitness-functions_test.go) | File to populate |
| FF acceptance script stub | [tests/acceptance/TASK-038-acceptance.sh](../../tests/acceptance/TASK-038-acceptance.sh) | Acceptance stub to complete |
| CI workflow extension | [.github/workflows/ci.yml](../../.github/workflows/ci.yml) | `fitness-functions` job already added by Scaffolder |
| Architecture overview | [architecture.md](../architect/architecture.md) | System context for FF assertions |
| Project state | [project-state.md](./project-state.md) | Current Cycle 4 status, Go-Live dependency |

---

## Skills required

- [`.claude/skills/bash-execution.md`](../../.claude/skills/bash-execution.md)
- [`.claude/skills/commit-discipline.md`](../../.claude/skills/commit-discipline.md)

---

## Acceptance Criteria (from task-plan.md)

- Each fitness function has an automated test or monitoring check
- Tests are runnable in CI (dev checks) and reportable
- Tests cover the critical thresholds defined in the fitness functions index
- CI pipeline includes fitness function tests in the test suite

---

## Context

**This is the final Cycle 4 task.** After TASK-038 passes Verifier, Sentinel will be dispatched for the cycle-level security review, then Demo Sign-off Briefing is prepared for the Nexus.

**Scaffolding already in place:**
- `tests/system/TASK-038-fitness-functions_test.go` contains named test stubs for FF-001, FF-002, FF-004, FF-005, FF-006, FF-007, FF-008, FF-013, FF-015 (passes immediately), FF-017, FF-019, FF-020, FF-022, FF-024 -- populate TODO bodies and keep Docker-dependent tests using `t.Skip` when the environment flag is not set (matches Scaffolder intent).
- `.github/workflows/ci.yml` already has a `fitness-functions` job added by Scaffolder -- verify it invokes the new test package and reports results.
- Acceptance shell `tests/acceptance/TASK-038-acceptance.sh` -- complete per the ACs above (runs the non-Docker FFs in CI mode, reports pass/fail).

**Cycle 4 progress:** 6 of 7 tasks PASS. SEC-001 just verified PASS at commit 6111d75 (CI run 24466108551). SEC-001 regression should not be introduced -- the FF-013 (AuthEnforcement) test should exercise the new mandatory password-change path where relevant.

**Builder auto-chain rule (global memory):** Before handoff, confirm no `nil` wiring remains in `cmd/*/main.go` for any FF dependencies being asserted.

**Handoff expectations:**
1. Local test run summary (which FFs pass, which skip and why, which run only in CI).
2. CI run ID once pushed and the `fitness-functions` job is green.
3. Commit SHA of the final change.
4. Any observations for the Orchestrator (non-blocking) to carry into the Cycle 4 Demo Sign-off.

**Next:** Invoke @nexus-orchestrator -- Builder reports TASK-038 complete with CI run ID and commit SHA.
