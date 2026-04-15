# Routing Instruction — Builder — TASK-038

**From:** Orchestrator
**To:** @nexus-builder
**Date:** 2026-04-15
**Cycle:** 4 (final task)
**Task:** TASK-038 -- Fitness function instrumentation

---

## Objective

Fill in the bodies of the fitness function test stubs scaffolded at
`tests/system/TASK-038-fitness-functions_test.go` so that each fitness function
defined in `process/architect/fitness-functions.md` is exercised by an automated
check that either runs in CI against the test Postgres/Redis services, runs as a
compile-time safety assertion, or is explicitly skipped with a documented reason
(Docker-socket dependence, compose-only).

The scaffold has already:
- Created the test file with `//go:build integration` tag.
- Added shared helpers (`redisClientOrSkip`, `databaseURLOrSkip`).
- Named one test function per fitness function (see stub table in the scaffold
  manifest).
- Added the `fitness-functions` job to `.github/workflows/ci.yml`.
- Added `tests/acceptance/TASK-038-acceptance.sh` which drives the four-step
  canonical acceptance flow: compile, FF-015, FF-013/017/019/020, and the Docker
  skip annotations.

Your job is to implement the test bodies so the acceptance script passes, ensure
CI runs the integration-tagged suite, and verify the Docker-dependent tests skip
cleanly (not fail) when their preconditions are absent.

## Acceptance Criteria

From `process/planner/task-plan.md` (lines 676-680) and the acceptance script:

- **AC-1:** Each fitness function listed in `fitness-functions.md` has a
  corresponding automated test, monitoring check, or explicit compile-time
  assertion in `tests/system/TASK-038-fitness-functions_test.go`.
- **AC-2:** Tests are runnable in CI -- the `fitness-functions` job in
  `.github/workflows/ci.yml` builds with `-tags integration` and executes the
  suite against the Postgres + Redis service containers.
- **AC-3:** Tests assert the critical thresholds named in the fitness functions
  index (e.g. FF-002 p95 queuing latency threshold, FF-013 403 on unauth,
  FF-017 migration idempotency, FF-019 schema rejection status code).
- **AC-4:** CI pipeline includes the fitness function tests; a red fitness
  function test fails the build.

From `tests/acceptance/TASK-038-acceptance.sh`:

- **AS-1:** `go test -tags integration -c -o /tmp/fitness-test ./tests/system/`
  compiles without error.
- **AS-2:** `TestFF015_CompileTimeSafety` passes unconditionally.
- **AS-3:** `TestFF013`, `TestFF017`, `TestFF019`, `TestFF020` run and pass
  against the live stack (FF-020 may skip if compose preconditions are absent;
  skip must be explicit, not a failure).
- **AS-4:** Docker-socket-dependent tests (FF-001, FF-007, FF-008, FF-024) skip
  cleanly with a descriptive `t.Skip` message referencing the missing
  precondition.

## Required Documents

- Task plan entry: [process/planner/task-plan.md](../../planner/task-plan.md) -- lines 673-686
- Fitness functions index (canonical thresholds): [process/architect/fitness-functions.md](../../architect/fitness-functions.md)
- Scaffold surface (FF test table + CI job notes): [process/scaffolder/scaffold-manifest.md](../../scaffolder/scaffold-manifest.md) -- lines 523-545, 595
- Test file to implement: [tests/system/TASK-038-fitness-functions_test.go](../../../tests/system/TASK-038-fitness-functions_test.go)
- Acceptance script: [tests/acceptance/TASK-038-acceptance.sh](../../../tests/acceptance/TASK-038-acceptance.sh)
- CI workflow (fitness-functions job added by scaffold): [.github/workflows/ci.yml](../../../.github/workflows/ci.yml)
- Precedent for integration-tagged system tests: existing `tests/system/` Go files + the TASK-029 CI job pattern

## Dependencies (all satisfied)

- TASK-001 (CI pipeline, dev environment) -- COMPLETE Cycle 1
- TASK-004 (Redis Streams queue) -- COMPLETE Cycle 1; needed for FF-002
- TASK-007 (Tag-based assignment / pipeline execution) -- COMPLETE Cycle 1; needed for FF-005, FF-006
- TASK-009 (Monitor service / failover) -- COMPLETE Cycle 2; needed for FF-007, FF-008
- TASK-018 (Sink atomicity with idempotency) -- COMPLETE Cycle 2; needed for FF-006
- Scaffold v3 (2026-04-09 commit 66c4bf0) + REG-030 cleanup -- in place

## Reminders

- **Integration tag discipline:** the file is already `//go:build integration`.
  Do **not** remove the tag -- default `go test ./...` must stay green without
  the heavy services. The `fitness-functions` CI job invokes with
  `-tags integration` explicitly.
- **Skip, don't fail, on missing preconditions:** FF-001, FF-007, FF-008,
  FF-024 require Docker socket access; FF-020 requires docker-compose. Use
  `t.Skipf` with the exact reason (`Docker socket unavailable`,
  `docker-compose not present`). A CI run without Docker must not fail because
  of these tests.
- **Thresholds come from the index, not your taste:** FF-002 p95 latency, FF-017
  migration count, FF-019 rejection codes, FF-013 403 body shape -- all are
  stated in `fitness-functions.md`. Do not invent numbers. If a threshold is
  ambiguous, stop and escalate rather than choosing one.
- **FF-013 already has a precedent:** SEC-001 acceptance test exercises the
  `{"error": "password_change_required"}` 403 shape. FF-013 validates the
  broader auth enforcement surface -- unauth requests to protected endpoints
  return 401/403 with the standard error envelope.
- **FF-017 migration test:** assert the migration runner is idempotent (second
  run is a no-op) and that the version reached is the current HEAD. Note
  OBS-028-3 (existing test hardcodes version=1); fix or work around, do not
  propagate that staleness.
- **FF-019 schema validation:** exercise the pipeline schema validation that
  TASK-026 delivered -- POST a pipeline with a type-mismatched mapping and
  assert the documented rejection code / body shape.
- **FF-015 compile-time safety:** this is the "binary exists" tautology -- the
  test function body can be a no-op `// FF-015 asserted at compile time.` as
  long as the test runs. Do not over-engineer.
- **CI job wiring:** verify `.github/workflows/ci.yml` runs the integration-
  tagged suite AND that the job fails the build when a fitness function test
  fails. Add a deliberate failing assertion locally, run the job definition
  through a dry-run (or inspect the YAML), and revert before pushing.
- **REG-030 lessons:** Cycle 4 scaffolding tripped four regressions in CI.
  Before pushing, run: `go build ./...`, `go vet ./...`, `staticcheck ./...`,
  `go test ./...` (default tag), and `go test -tags integration ./tests/system/...`
  against a local Postgres/Redis. Push only when all five are clean.
- **No secrets in tests:** use `DATABASE_URL` / `REDIS_URL` environment
  variables; do not hardcode credentials. The CI service containers expose
  predictable URLs -- mirror the TASK-029 job env block.
- **Demo script is N/A:** this task produces no user-visible behavior. The
  Verifier will exercise the CI run output and the local acceptance script; no
  Playwright capture is needed.

## Exit Criteria for Your Handoff

- `tests/acceptance/TASK-038-acceptance.sh` exits 0 against a local stack.
- `go test -tags integration ./tests/system/...` is green locally and in CI.
- `go test ./...` (no tags) remains green -- the integration tag must keep the
  heavy tests out of the default path.
- The `fitness-functions` job runs green on the pushed branch (share the CI
  run ID).
- Docker-dependent tests (FF-001, FF-007, FF-008, FF-024) emit a `SKIP` line
  with a descriptive reason when Docker is unavailable; they do not fail.
- Each FF test comment references the FF-NNN id and the threshold source
  (ADR or fitness-functions.md line).
- Final commit SHA reported.

---

**Next:** Invoke @nexus-orchestrator -- on completion, report commit SHA, CI run
ID, acceptance script exit status, and a one-line PASS/SKIP summary per FF
function so the Verifier can be dispatched.
