# Routing Instruction -- Verifier -- TASK-038 (iteration 1)

**From:** Orchestrator
**To:** @nexus-verifier
**Date:** 2026-04-15
**Cycle:** 4 (final task)
**Task:** TASK-038 -- Fitness function instrumentation
**Iteration:** 1
**Verifier mode:** Run-only (Builder already authored the fitness function tests; your role is to execute and validate, not to author new tests for TASK-038). You MAY add regression tests only if a defect is discovered that is not exercised by an existing test.

---

## Objective

Verify that TASK-038 Builder output satisfies the four acceptance criteria in
`process/planner/task-plan.md` (lines 676-680) and the four acceptance-script
steps in `tests/acceptance/TASK-038-acceptance.sh`. Confirm that each fitness
function named in `process/architect/fitness-functions.md` is exercised by an
automated check, a compile-time assertion, or an explicit documented skip, and
that the `fitness-functions` CI job runs the suite with `-tags integration`
against the service containers and would fail the build on a red test.

## Builder Handoff Summary

- Commit: **b4242a8**
- All 9 fitness-function tests implemented.
- `.github/workflows/ci.yml` `fitness-functions` job expanded to run the full
  non-Docker suite: **FF-002, FF-005, FF-006, FF-013, FF-015, FF-017, FF-019,
  FF-020, FF-022**.
- Docker-socket-dependent tests (FF-001, FF-007, FF-008, FF-024) skip with
  descriptive `t.Skipf` messages per the routing reminder.
- One bug fixed during implementation: **snapshotRowCount key mismatch**
  (investigate the diff and confirm the fix is correct and covered; do not
  just accept the claim).

## What to Verify

### Acceptance Criteria (task-plan.md lines 676-680)

- **AC-1:** Each fitness function in `fitness-functions.md` has a corresponding
  automated test, monitoring check, or explicit compile-time assertion in
  `tests/system/TASK-038-fitness-functions_test.go`. Walk the index; confirm
  coverage is 1:1 with no orphans.
- **AC-2:** The `fitness-functions` CI job builds with `-tags integration` and
  executes the suite against Postgres + Redis service containers. Confirm the
  YAML wiring and inspect the latest CI run.
- **AC-3:** Tests assert the critical thresholds named in the index
  (FF-002 p95 queuing latency, FF-013 401/403 shape, FF-017 migration
  idempotency + HEAD version, FF-019 schema-rejection status code and body,
  FF-020 documented threshold). Thresholds must match the index -- any drift
  is a FAIL.
- **AC-4:** A red fitness function test fails the CI build. Validate by
  inspecting the job's failure propagation (exit code, `continue-on-error`
  absent, no silent green).

### Acceptance Script (`tests/acceptance/TASK-038-acceptance.sh`)

- **AS-1:** `go test -tags integration -c -o /tmp/fitness-test ./tests/system/`
  compiles without error.
- **AS-2:** `TestFF015_CompileTimeSafety` passes unconditionally.
- **AS-3:** `TestFF013`, `TestFF017`, `TestFF019`, `TestFF020` pass (or SKIP
  with documented reason for FF-020 if compose preconditions absent).
- **AS-4:** Docker-dependent FFs (FF-001/007/008/024) SKIP cleanly with
  descriptive messages, they do not fail.

### Additional Verification

- **Default test suite stays green:** `go test ./...` (no integration tag)
  must remain green -- the integration tag must keep heavy tests out of the
  default path. Run it.
- **Local integration run:** `go test -tags integration ./tests/system/...`
  against a local Postgres/Redis -- green.
- **Threshold traceability:** every FF test comment references the FF-NNN id
  and the threshold source (ADR or `fitness-functions.md` line number). Spot-
  check three tests; any missing reference is a non-blocking OBS.
- **snapshotRowCount bug fix:** review the diff that fixed the key mismatch.
  Confirm the fix is correct, the key name is consistent with the producer
  site, and that a test covers the previously-broken path.
- **CI run:** pull the latest `fitness-functions` job run on the commit
  b4242a8 branch. Confirm it is green and the integration-tagged suite
  actually ran (inspect logs -- missing `-tags integration` on the go command
  line would be a silent failure).

## Required Documents

- Task plan: [process/planner/task-plan.md](../../planner/task-plan.md) (lines 673-686)
- Fitness functions index: [process/architect/fitness-functions.md](../../architect/fitness-functions.md)
- Scaffold manifest (FF test table): [process/scaffolder/scaffold-manifest.md](../../scaffolder/scaffold-manifest.md) (lines 523-545, 595)
- Test file under verification: [tests/system/TASK-038-fitness-functions_test.go](../../../tests/system/TASK-038-fitness-functions_test.go)
- Acceptance script: [tests/acceptance/TASK-038-acceptance.sh](../../../tests/acceptance/TASK-038-acceptance.sh)
- CI workflow: [.github/workflows/ci.yml](../../../.github/workflows/ci.yml) -- confirm `fitness-functions` job definition
- Builder routing (for context on constraints): [process/orchestrator/routing/TASK-038-builder.md](./TASK-038-builder.md)

## Demo Script

**N/A** -- TASK-038 produces no user-visible behavior. The Demo Script field
in the Verification Report should note "N/A -- fitness function instrumentation
is a CI concern; demo coverage is the green `fitness-functions` job badge."

## Exit Criteria for Your Report

- Verification Report in `process/verifier/TASK-038-verification.md` with per-
  AC and per-AS PASS/FAIL.
- Explicit PASS/SKIP line per fitness function (FF-001 through FF-024 as
  enumerated in the index), stating the mechanism (test, compile-time, skip-
  with-reason) and the threshold source.
- CI run ID for the green `fitness-functions` job on commit b4242a8 (or the
  latest head if pushed further).
- Output of `go test ./...` (untagged) and `go test -tags integration
  ./tests/system/...` (tagged) pasted or summarised.
- Confirmation or rejection of the snapshotRowCount bug fix quality.
- OBS entries for any non-blocking observations (threshold comment gaps,
  skip-message wording, etc.).
- Overall verdict: **PASS** or **FAIL** with bounded follow-up items if FAIL.

## Context Notes

- Cycle 4 execution converges on this task. On PASS, Cycle 4 becomes
  presentable: 7/7 tasks + SEC-001 remediation verified. The Orchestrator
  will then route to Sentinel for the Cycle 4 security review and assemble
  the Demo Sign-off Briefing.
- REG-030 lessons apply: the scaffold for Cycle 4 tripped four CI regressions.
  A silent-green (e.g. test file not actually compiled into the CI run) is
  the failure mode to watch for. Verify the CI job **executed** the tests,
  not just that it exited 0.

---

**Next:** Invoke @nexus-orchestrator -- on completion, report overall PASS or
FAIL, the CI run ID, the per-FF PASS/SKIP table, and any OBS entries.
