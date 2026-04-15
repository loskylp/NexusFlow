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

# Demo Script — TASK-038
**Feature:** Fitness function instrumentation
**Requirement(s):** FF-001 through FF-025
**Environment:** Staging CI — fitness-functions job in GitHub Actions

## Scenario 1: Fitness function test binary compiles with integration build tag
**REQ:** FF-015 (compile-time safety), AC-2

**Given:** The repository is checked out on the main branch at the verified commit
**When:** The fitness-functions CI job runs its compile step: `go test -tags integration -c -o /tmp/fitness-test ./tests/system/`
**Then:** The binary `/tmp/fitness-test` is produced without error; the job does not fail at the compile step

---

## Scenario 2: Queue backlog monitoring mechanism verified (FF-003)
**REQ:** FF-003 (queue backlog — XPENDING count per stream), ADR-001

**Given:** The fitness-functions CI job has a Redis service container running and reachable
**When:** TestFF003_QueueBacklog runs — it creates a stream, enqueues 10 entries, reads them into a consumer group without acknowledging, then calls XPENDING
**Then:** XPENDING reports exactly 10 pending (unacknowledged) entries; the test logs "FF-003 PASS: XPENDING correctly reports 10 pending (unacknowledged) entries" and exits with PASS; the stream is deleted during cleanup

---

## Scenario 3: Queuing latency within critical threshold (FF-002)
**REQ:** FF-002 (queuing latency p95 < 50ms), ADR-001

**Given:** The fitness-functions CI job has a Redis service container running and reachable
**When:** TestFF002_QueuingLatency runs — it enqueues 1,000 tasks via XADD and measures p95 latency
**Then:** p95 XADD latency is below 50ms; the test passes; the job log shows "--- PASS: TestFF002_QueuingLatency"

---

## Scenario 4: Auth enforcement — unauthenticated, wrong-role, and inactive-user cases (FF-013)
**REQ:** FF-013 (auth enforcement), ADR-006

**Given:** The fitness-functions CI job is running; TestFF013_AuthEnforcement uses an in-process httptest server — no external service required
**When:** The test issues requests: (a) no session token to a protected endpoint, (b) user-role token to an admin-only endpoint, (c) deactivated user session to any endpoint
**Then:** Case (a) returns 401; case (b) returns 403; case (c) returns 401; the test logs all four AC sub-checks as PASS and exits PASS

---

## Scenario 5: Schema migration idempotency (FF-017)
**REQ:** FF-017 (schema migration idempotency), ADR-008

**Given:** The fitness-functions CI job has a Postgres service container running; DATABASE_URL is set to the test database
**When:** TestFF017_SchemaMigration calls RunMigrations twice on the same database
**Then:** Both migration runs complete without error; the test logs "FF-017 PASS: schema migrations applied and are idempotent" and exits PASS

---

## Scenario 6: Sink before/after snapshot captures row count changes (FF-022)
**REQ:** FF-022 (sink inspector / before-after snapshot), ADR-009

**Given:** The fitness-functions CI job is running; TestFF022_SinkInspector uses an in-memory database — no external service required
**When:** The test captures a Before snapshot (0 rows), writes 2 records via CaptureAndWrite, then captures an After snapshot
**Then:** Before snapshot reports 0 rows; After snapshot reports 2 rows; afterCount > beforeCount; the test logs "FF-022 PASS: Before snapshot has 0 rows, After snapshot has 2 rows" and exits PASS

---

## Scenario 7: Docker-dependent and infra-gated FFs skip cleanly without failing the build (FF-001, FF-009, FF-025)
**REQ:** AC-1 (documented skip satisfies 1:1 coverage), AC-4 (no silent-green)

**Given:** The fitness-functions CI job is running; Docker socket and production infrastructure are not available in the CI runner
**When:** Any of the 15 documented-skip test functions are invoked (e.g. by running the binary without a -test.run filter): TestFF001_QueuePersistence (Docker socket), TestFF009_FleetResilience (multi-worker Docker fleet), TestFF025_InfrastructureHealth (Uptime Kuma + prod PostgreSQL)
**Then:** Each test exits with SKIP status and a descriptive reason message; no test exits with FAIL; the binary exits with code 0; the job remains green

**Notes:** In the standard CI run the skip stubs for FF-009 through FF-025 are not in the -test.run regex and will not execute. To verify skip behavior manually, invoke the binary without `-test.run` or with a regex matching a specific stub (e.g. `-test.run TestFF009`).

---
