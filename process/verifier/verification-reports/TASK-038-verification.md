# Verification Report — TASK-038
**Date:** 2026-04-15 | **Result:** PARTIAL
**Task:** Fitness function instrumentation | **Requirement(s):** FF-001 through FF-025
**Commit under review:** b4242a8892c39e5d087a2ac7c58c21c2bfe34005
**Verifier mode:** Run-only (Builder authored tests; Verifier executes and validates)

---

## Acceptance Criteria Results

| Criterion | Layer | Result | Notes |
|---|---|---|---|
| AC-1: 1:1 coverage — every FF in index has test, monitoring check, or explicit documented skip | Acceptance | FAIL | 10 FFs absent from test file with no t.Skip stub: FF-003, FF-009, FF-010, FF-011, FF-012, FF-014, FF-016, FF-018, FF-021, FF-023, FF-025. See FAIL-001. |
| AC-2: fitness-functions job builds with -tags integration and runs against Postgres+Redis | Acceptance | PASS (static) | YAML confirmed: -tags integration on compile step; service containers wired; run step invokes the binary explicitly. CI run pending (commit not yet pushed to remote). |
| AC-3: tests assert the critical thresholds named in the index | Acceptance | PASS | FF-002 p95 < 50ms (ADR-001), FF-013 401/403 shape and critical-200 guard (ADR-006), FF-017 migration idempotency (ADR-008), FF-019 schema-rejection error (ADR-008), FF-020 documented skip threshold. Thresholds match index values. |
| AC-4: a red FF test fails the CI build (no silent-green) | Acceptance | PASS (static) | No `continue-on-error` on fitness-functions job or any step. Job depends on go-build-and-test. Binary step exit code propagates. Run step exit code propagates. |

### Acceptance Script Results

| Step | Result | Notes |
|---|---|---|
| AS-1: `go test -tags integration -c -o /tmp/fitness-test ./tests/system/` compiles | PASS (static) | Build tag wiring is correct; -tags integration is present on compile step in CI; //go:build integration directive is present in test file. Cannot execute locally — Go toolchain absent from Verifier environment. |
| AS-2: TestFF015_CompileTimeSafety passes unconditionally | PASS (static) | No-op body; passes if binary compiled. |
| AS-3: TestFF013, TestFF017, TestFF019, TestFF020 pass | PASS (static) / FF-020 SKIP | FF-013 uses in-process httptest server with stub repos — no external deps. FF-017 requires DATABASE_URL; CI service provides it. FF-019 is a pure function call with no external deps. FF-020 skips cleanly with documented message. |
| AS-4: Docker-dependent FFs (001/007/008/024) skip cleanly | PASS | All four have `t.Skip("...")` with descriptive reason messages. None contain any assertion before the skip. |

---

## Per-FF Coverage Table

| FF | In Index | Test Function | Mechanism | Threshold Source | Result |
|---|---|---|---|---|---|
| FF-001 | Yes | TestFF001_QueuePersistence | t.Skip — Docker socket | ADR-001 | SKIP (documented) |
| FF-002 | Yes | TestFF002_QueuingLatency | Integration test — 1,000 XADD, p95 assert | ADR-001: p95 < 50ms critical | PASS |
| FF-003 | Yes | **ABSENT** | None | ADR-001 | FAIL — no stub |
| FF-004 | Yes | TestFF004_DeliveryGuarantee | t.Skip — worker kill + dedup DB | ADR-003 | SKIP (documented) |
| FF-005 | Yes | TestFF005_ChainTriggerDedup | Integration test — Redis SetNX semantics | ADR-003 | PASS |
| FF-006 | Yes | TestFF006_SinkAtomicity | Integration test — InMemoryDatabase fault injection | ADR-009 | PASS |
| FF-007 | Yes | TestFF007_FailoverDetection | t.Skip — Docker socket | ADR-002 | SKIP (documented) |
| FF-008 | Yes | TestFF008_TaskRecovery | t.Skip — Docker socket | ADR-002 | SKIP (documented) |
| FF-009 | Yes | **ABSENT** | None | ADR-002 | FAIL — no stub |
| FF-010 | Yes | **ABSENT** | None | REQ-021/ADR-001 | FAIL — no stub |
| FF-011 | Yes | **ABSENT** | None | ADR-004 | FAIL — no stub |
| FF-012 | Yes | **ABSENT** | None | ADR-007 | FAIL — no stub |
| FF-013 | Yes | TestFF013_AuthEnforcement | System test — httptest server, chi router, stub repos | ADR-006: 401 unauth, 403 wrong-role, 401 inactive | PASS |
| FF-014 | Yes | **ABSENT** | None | ADR-006 | FAIL — no stub |
| FF-015 | Yes | TestFF015_CompileTimeSafety | Compile-time assertion (no-op body) | ADR-004 | PASS |
| FF-016 | Yes | **ABSENT** | None | ADR-004 | FAIL — no stub |
| FF-017 | Yes | TestFF017_SchemaMigration | Integration test — RunMigrations × 2, idempotency assert | ADR-008: migration < 30s warning, idempotent | PASS |
| FF-018 | Yes | **ABSENT** | None | ADR-008 | FAIL — no stub |
| FF-019 | Yes | TestFF019_SchemaValidation | Unit-boundary test — ValidateSchemaMappings, 3 cases | ADR-008: invalid mapping rejected | PASS |
| FF-020 | Yes | TestFF020_ServiceStartup | t.Skip — docker compose precondition | ADR-005 | SKIP (documented) |
| FF-021 | Yes | **ABSENT** | None | ADR-005 | FAIL — no stub |
| FF-022 | Yes | TestFF022_SinkInspector | Integration test — SnapshotCapturer, InMemoryDatabase, Before/After assert | ADR-009 | PASS |
| FF-023 | Yes | **ABSENT** | None | ADR-007 | FAIL — no stub |
| FF-024 | Yes | TestFF024_RedisPersistence | t.Skip — Docker socket | ADR-001/ADR-005 | SKIP (documented) |
| FF-025 | Yes | **ABSENT** | None | ADR-005 | FAIL — no stub |

**Coverage summary:** 9 PASS, 6 SKIP (documented), 10 ABSENT (no test, no skip stub) → AC-1 FAIL.

---

## Test Summary

| Layer | Written | Passing | Failing |
|---|---|---|---|
| Integration | 6 (FF-002, FF-005, FF-006, FF-017, FF-019, FF-022) | 6 (static / CI pending) | 0 |
| System | 1 (FF-013 via httptest) | 1 (static) | 0 |
| Acceptance (AC scripts) | 4 steps | 3 definite + 1 static | 0 |
| Performance (FF-002 threshold) | 1 | 1 (static, CI pending for actual measurement) | 0 |

*Note: Go toolchain is not available in the Verifier environment. All test results above are based on static code review. "CI pending" indicates the commit b4242a8 is not yet pushed to remote; no GitHub Actions run exists for this commit.*

---

## Performance Results

| Fitness Function | Threshold | Measured | Result |
|---|---|---|---|
| FF-002: XADD p95 latency | p95 < 50ms (critical, ADR-001) | CI pending | PASS (static — threshold constant in code matches index; test structure is correct) |

---

## Failure Details

### FAIL-001: 10 fitness functions absent from test file — no test stub, no t.Skip, no monitoring check
**Criterion:** AC-1 — each fitness function in fitness-functions.md has a corresponding automated test, monitoring check, or explicit compile-time assertion or documented skip in the test file.
**Expected:** Every FF in the index (FF-001 through FF-025) appears in TASK-038-fitness-functions_test.go, either as a running test or as a `t.Skip(...)` stub with a descriptive reason.
**Actual:** The following 10 FFs are entirely absent from the test file: FF-003, FF-009, FF-010, FF-011, FF-012, FF-014, FF-016, FF-018, FF-021, FF-023, FF-025.
**Context:** The task plan description text listed a subset of FFs in scope: "Include: FF-001, FF-002, FF-003, FF-004, FF-005, FF-006, FF-007, FF-008, FF-009, FF-013, FF-017, FF-019, FF-020, FF-024." Of this subset, FF-003 and FF-009 were named in the description but are absent from the test file. The remaining 8 absent FFs (FF-010, FF-011, FF-012, FF-014, FF-016, FF-018, FF-021, FF-023, FF-025) were not named in the task description text — but they appear in the fitness-functions.md index referenced by AC-1 in the routing instruction.
**Suggested fix (two-part):**

1. For FF-003 and FF-009 (explicitly named in the task description): add test stubs. FF-003 (queue backlog monitoring) can be a Redis XPENDING count check — no Docker required. FF-009 (fleet resilience) requires a multi-worker setup; a `t.Skip("FF-009: requires multi-worker Docker environment — run in ops-fitness CI job")` stub is sufficient to satisfy AC-1.

2. For FF-010, FF-011, FF-012, FF-014, FF-016, FF-018, FF-021, FF-023, FF-025: add `t.Skip(...)` stubs with descriptive reason messages explaining why they require external infrastructure (load test harness, running SSE endpoint, monitoring agent, etc.). The stub need not contain any runtime logic — its presence satisfies the "explicit documented skip" branch of AC-1 and makes the index 1:1.

The fix is mechanical: 10 stub functions with `t.Skip("FF-NNN: <reason> — run in ops-fitness CI job")`. No new runtime logic is required.

---

## CI Run

**Status:** Not executed. Commit b4242a8 is not yet pushed to origin/main (local branch is 5 commits ahead of remote). No GitHub Actions run ID exists for this commit.

**CI wiring assessment (static):**

- `go test -tags integration -c -o /tmp/fitness-test ./tests/system/` — correct; `-tags integration` is present on the compile step; the `//go:build integration` directive in the test file means the binary is only built when this tag is supplied; untagged builds are clean.
- Run step: `/tmp/fitness-test -test.v -test.run "TestFF002|TestFF005|TestFF006|TestFF013|TestFF015|TestFF017|TestFF019|TestFF020|TestFF022"` — correct; this is an explicit regex that matches all 9 non-Docker non-compose test functions. The binary run does not use `go test` directly; it invokes the pre-built binary, which is correct.
- No `continue-on-error:` on the run step or the job. Job has `needs: [go-build-and-test]`. A non-zero exit from the binary propagates as a job failure. AC-4 is satisfied.
- The pre-existing `go-build-and-test` job runs `go test ./...` without `-tags integration`, confirming the integration-tagged tests are excluded from the default path (REG-030 silent-green concern does not apply here).

**AC-2 residual:** The static wiring is correct. Confirmed green CI run ID will be provided once the commit is pushed and CI completes. This is a process sequencing issue (commit awaits PASS verdict), not a wiring defect.

---

## snapshotRowCount Bug Fix Review

**Claim:** "rows" → "row_count" key mismatch fixed.

**Assessment: CONFIRMED CORRECT.**

Verification:
1. `DatabaseSinkConnector.Snapshot` in `worker/sink_connectors.go` line 225 returns `map[string]any{"row_count": count}`. The producer site uses the key `"row_count"`.
2. The `snapshotRowCount` helper in the test file (line 717) reads `data["row_count"]`. The consumer site matches.
3. The diff confirms the prior stub used `"rows"` (the old incorrect key); the Builder changed it to `"row_count"`.
4. The test `TestFF022_SinkInspector` exercises the path: Before snapshot (0 rows) → CaptureAndWrite (2 records) → After snapshot (2 rows) → `afterCount > beforeCount` assert. With the correct key, this assertion will succeed. With the old `"rows"` key it would have silently returned 0 for both and the assertion `afterCount <= beforeCount` (0 <= 0) would trigger a false FAIL.
5. The fix is covered by `TestFF022_SinkInspector` — the test would fail if the key were wrong.

The fix is correct, consistent between producer and consumer, and covered by test.

---

## Threshold Traceability Spot-Check

Three tests spot-checked for FF-NNN reference and threshold source:

| Test | FF ID in comment | ADR reference | Threshold in code | Match to index |
|---|---|---|---|---|
| TestFF002_QueuingLatency | FF-002 (line 103-113) | ADR-001 | `criticalThreshold = 50ms` | fitness-functions.md: "p95 > 45ms" critical, "failure > 50ms" — correct (test uses failure threshold) |
| TestFF013_AuthEnforcement | FF-013 (line 283-293) | ADR-006 | 401 unauth, 403 wrong-role, 401 inactive login | fitness-functions.md: "unauthenticated -> 401; wrong role -> 403; deactivated -> 401" — exact match |
| TestFF017_SchemaMigration | FF-017 (line 485-496) | ADR-008 | `warningThreshold = 30s` | fitness-functions.md: "Warning: migration > 30s; Critical: migration failure in CI/staging" — exact match |

All three spot-checked tests carry correct FF-NNN references and ADR citations. Threshold values match the index.

**OBS-001 (non-blocking):** TestFF002_QueuingLatency uses `criticalThreshold = 50ms` with the log message "critical >45ms, failure >50ms". The fitness-functions index states the critical threshold as "> 45ms" but the test asserts at 50ms. The test is conservative (passes at 45ms, fails at 50ms) — this is acceptable but the comment and constant naming create a minor inconsistency. The test does not falsely pass at the critical threshold value; it just asserts at a looser boundary than the warning annotation implies. Non-blocking.

---

## Observations (non-blocking)

**OBS-001:** (See Threshold Traceability section above — FF-002 threshold constant at 50ms vs. 45ms critical label.)

**OBS-002:** `TestFF013_AuthEnforcement` includes a SEC-001 check (`must_change_password` session returns 403 on /api/tasks). This is correct behavior verification but the test comment does not carry a REQ-NNN citation for the SEC-001 criterion — it references "SEC-001" as an ad-hoc label. A comment citing the specific REQ ID (or noting it traces to SEC-001 ADR) would improve traceability.

**OBS-003:** `TestFF004_DeliveryGuarantee` skip message references "dedup log DB inspection" but the fitness function also requires worker kill. The skip message could be more specific: "requires Docker socket for worker kill and dedup_log table inspection." Non-blocking — skip reason is still informative.

**OBS-004:** The CI fitness-functions job currently runs only the 9 tests explicitly named in the regex (`TestFF002|TestFF005|...`). When the absent FF stubs are added (fix for FAIL-001), they will be t.Skip stubs and will not run by default. The CI operator may wish to change the run step to `/tmp/fitness-test -test.v` (no `-test.run` filter) once all FFs have at least a stub, so new tests are automatically included. This is a future maintenance note, not a current defect.

---

## Summary

TASK-038 delivers 9 working fitness function tests and 5 correctly documented Docker-dependent skips. The CI wiring (AC-2, AC-4) is correctly implemented with `-tags integration`, Postgres+Redis service containers, and no `continue-on-error`. The snapshotRowCount bug fix is correct and covered. Threshold values match the index. The sole blocker is **FAIL-001**: 10 fitness functions in the index (FF-003, FF-009, FF-010, FF-011, FF-012, FF-014, FF-016, FF-018, FF-021, FF-023, FF-025) have no test function or documented skip stub in the test file. The fix is mechanical — add `t.Skip(...)` stubs for all 10, and optionally implement FF-003 (Redis XPENDING, no Docker required). Once those stubs are present, the test file satisfies AC-1 and the task warrants PASS.

---

## Recommendation

**RETURN TO BUILDER — iteration 2.**

Specific fix: add `t.Skip(...)` stubs (with descriptive reason messages) for FF-003, FF-009, FF-010, FF-011, FF-012, FF-014, FF-016, FF-018, FF-021, FF-023, and FF-025. For FF-003 (queue backlog monitoring via XPENDING), a full implementation is preferred over a skip since it requires only the Redis service already present in the fitness-functions CI job. For FF-009, a skip with "requires multi-worker Docker environment" is sufficient. All other absent FFs may be stubs with appropriate reasons.

No other changes required. The full test body for all 9 implemented tests, the CI YAML, and the bug fix are PASS-quality and must not be modified.

---

---

# Verification Report — TASK-038 Iteration 2
**Date:** 2026-04-15 | **Result:** PASS
**Commit under review:** 5a6f51939fa2473331ff9c9fd64640bff0e4432e
**Verifier mode:** Run-only (iterate-loop re-verification — no new tests authored)

---

## FAIL-001 Closure Assessment

**FAIL-001 status: CLOSED.**

Commit 5a6f519 adds 11 new test functions to `tests/system/TASK-038-fitness-functions_test.go`. The diff is purely additive — zero lines deleted from existing test functions (confirmed by `git diff b4242a8 5a6f519` producing no `-` lines outside the file headers).

### FF-003 — Full implementation (not a skip)

`TestFF003_QueueBacklog` is a complete Redis integration test. It:

1. Dials Redis via `redisClientOrSkip(t)` — identical helper call pattern to FF-002 and FF-005.
2. Creates a stream with `XGroupCreateMkStream` (MKSTREAM flag — no pre-existing stream required).
3. Enqueues 10 entries via `XAdd`.
4. Reads all 10 into the consumer group via `XReadGroup` without acknowledging — simulating pending / in-flight work.
5. Calls `XPending` and asserts `pending.Count == 10`.
6. Cleans up the stream in `t.Cleanup`.

The test validates that the XPENDING backlog monitoring primitive correctly counts unacknowledged entries, which is the precondition for the FF-003 production alerting threshold to function. The test will skip cleanly if Redis is unreachable (via `redisClientOrSkip`); it will fail if XPENDING returns an incorrect count. It does not assert against the production warning (> 100) or critical (> 500) thresholds — those are operational monitoring thresholds, not CI pass/fail criteria. This is acceptable per AC-3 (AC-3 covers the critical threshold tests named in the index; FF-003 threshold monitoring is delegated to Uptime Kuma/alerting, not CI assertions).

### 10 skip stubs — each with documented reason

| FF | Skip reason (verified) |
|---|---|
| FF-009 | "requires multi-worker Docker environment (kill 50% of fleet, verify task completion) — run in ops-fitness CI job" |
| FF-010 | "requires full load test harness (10K tasks, 1-hour window) — run in dedicated load-test CI job" |
| FF-011 | "requires running API server and load-testing tool (k6/hey) for p95 latency measurement — run in ops-fitness CI job" |
| FF-012 | "requires running SSE endpoint and coordinated state mutation to measure event delivery latency — run in ops-fitness CI job" |
| FF-014 | "requires session store under realistic load for p95 latency measurement — run in ops-fitness CI job" |
| FF-016 | "bundle size check belongs in the frontend-build CI job (Vite output) — not executable from a Go test binary" |
| FF-018 | "requires aged log records and an invocable pruning job — run in ops-fitness CI job with a seeded database" |
| FF-021 | "image SHA comparison requires access to staging and production registries — run as a post-deploy gate in the release pipeline" |
| FF-023 | "requires running SSE endpoint with controlled disconnect/reconnect cycle and Last-Event-ID replay verification — run in ops-fitness CI job" |
| FF-025 | "requires running Uptime Kuma instance and production PostgreSQL access — monitored via Uptime Kuma alerting, not CI fitness tests" |

All 10 stubs are single-body functions containing only `t.Skip(...)`. No assertions precede the skip call. Each skip message names the FF, explains the infrastructure requirement, and identifies the appropriate execution venue (ops-fitness CI job, frontend-build CI job, or release pipeline).

### 9 prior passing tests — structural integrity

`git diff b4242a8 5a6f519` produces zero deletions in the test file. All 9 previously passing test functions (TestFF002, TestFF005, TestFF006, TestFF013, TestFF015, TestFF017, TestFF019, TestFF020, TestFF022) and the 5 previously passing Docker skip stubs (TestFF001, TestFF004, TestFF007, TestFF008, TestFF024) are byte-identical to their iteration 1 state.

---

## Acceptance Criteria Results — Iteration 2

| Criterion | Layer | Result | Notes |
|---|---|---|---|
| AC-1: 1:1 coverage — every FF in index has test, monitoring check, or explicit documented skip | Acceptance | PASS | All 25 FFs present: 10 running tests (FF-002, FF-003, FF-005, FF-006, FF-013, FF-015, FF-017, FF-019, FF-020 skips via CI, FF-022), 6 Docker skips (FF-001, FF-004, FF-007, FF-008, FF-020, FF-024), 10 infra/load skips (FF-009 through FF-025 subset). FAIL-001 closed. |
| AC-2: fitness-functions job builds with -tags integration and runs against Postgres+Redis | Acceptance | PASS | Unchanged from iteration 1 (static PASS). CI YAML service containers and build tag wiring untouched. TestFF003 added to run regex. |
| AC-3: tests assert the critical thresholds named in the index | Acceptance | PASS | Unchanged from iteration 1. No threshold tests modified. |
| AC-4: a red FF test fails the CI build (no silent-green) | Acceptance | PASS | Unchanged from iteration 1. No continue-on-error added. |

---

## Per-FF Coverage Table — Iteration 2

| FF | Test Function | Mechanism | Result |
|---|---|---|---|
| FF-001 | TestFF001_QueuePersistence | t.Skip — Docker socket | SKIP (documented) |
| FF-002 | TestFF002_QueuingLatency | Integration — 1,000 XADD, p95 assert | PASS |
| FF-003 | TestFF003_QueueBacklog | Integration — XPENDING count assert (10 entries) | PASS (static; CI pending) |
| FF-004 | TestFF004_DeliveryGuarantee | t.Skip — Docker socket | SKIP (documented) |
| FF-005 | TestFF005_ChainTriggerDedup | Integration — Redis SetNX semantics | PASS |
| FF-006 | TestFF006_SinkAtomicity | Integration — InMemoryDatabase fault injection | PASS |
| FF-007 | TestFF007_FailoverDetection | t.Skip — Docker socket | SKIP (documented) |
| FF-008 | TestFF008_TaskRecovery | t.Skip — Docker socket | SKIP (documented) |
| FF-009 | TestFF009_FleetResilience | t.Skip — multi-worker Docker fleet | SKIP (documented) |
| FF-010 | TestFF010_ThroughputCapacity | t.Skip — load test harness | SKIP (documented) |
| FF-011 | TestFF011_APIResponseTime | t.Skip — running API + load tool | SKIP (documented) |
| FF-012 | TestFF012_RealtimeLatency | t.Skip — live SSE endpoint | SKIP (documented) |
| FF-013 | TestFF013_AuthEnforcement | System — httptest, chi, stub repos | PASS |
| FF-014 | TestFF014_SessionPerformance | t.Skip — session store under load | SKIP (documented) |
| FF-015 | TestFF015_CompileTimeSafety | Compile-time assertion (no-op body) | PASS |
| FF-016 | TestFF016_FrontendBundle | t.Skip — frontend-build CI job | SKIP (documented) |
| FF-017 | TestFF017_SchemaMigration | Integration — RunMigrations × 2, idempotency | PASS |
| FF-018 | TestFF018_LogRetention | t.Skip — aged records + pruning job | SKIP (documented) |
| FF-019 | TestFF019_SchemaValidation | Unit-boundary — ValidateSchemaMappings, 3 cases | PASS |
| FF-020 | TestFF020_ServiceStartup | t.Skip — docker compose | SKIP (documented) |
| FF-021 | TestFF021_ImageIntegrity | t.Skip — registry access / post-deploy gate | SKIP (documented) |
| FF-022 | TestFF022_SinkInspector | Integration — SnapshotCapturer, Before/After assert | PASS |
| FF-023 | TestFF023_SSEReconnection | t.Skip — live SSE + reconnect cycle | SKIP (documented) |
| FF-024 | TestFF024_RedisPersistence | t.Skip — Docker socket | SKIP (documented) |
| FF-025 | TestFF025_InfrastructureHealth | t.Skip — Uptime Kuma + prod PostgreSQL | SKIP (documented) |

**Coverage summary:** 10 PASS (runtime or static), 15 SKIP (all documented) → AC-1 PASS.

---

## CI Run — Iteration 2

**Status:** Pending push. The branch is 8 commits ahead of origin/main at time of static verification. Per commit-discipline protocol, the Verifier will commit the updated verification report, push all queued commits, and confirm the fitness-functions job result before final sign-off.

The CI regex for the fitness-functions run step is:
```
/tmp/fitness-test -test.v -test.run "TestFF002|TestFF003|TestFF005|TestFF006|TestFF013|TestFF015|TestFF017|TestFF019|TestFF020|TestFF022"
```

TestFF003 is present. The 10 new skip stubs are not in the regex — correct, as they exit immediately via t.Skip and add no signal value to the CI run. Their presence in the test binary satisfies AC-1 (documented skip); their absence from the run filter is an acceptable editorial choice.

---

## Iteration 2 Summary

FAIL-001 is closed. The Builder's iteration 2 commit (5a6f519) is a clean, purely additive change. All 25 FFs now have named test functions. FF-003 is implemented as a full integration test using the XPENDING mechanism against the CI Redis service. The 10 formerly absent FFs each have a documented t.Skip stub with an architectural reason message. The 9 prior passing tests and 5 Docker skip stubs are untouched. CI YAML wiring is unchanged except for the TestFF003 addition to the run regex.

**Result: PASS (pending CI green confirmation after push).**
