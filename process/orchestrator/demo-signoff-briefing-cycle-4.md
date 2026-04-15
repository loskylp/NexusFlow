# Demo Sign-off Briefing -- NexusFlow
**Cycle:** 4 | **Date:** 2026-04-15 | **Profile:** Critical

## What Was Built

Cycle 4 delivered the demo infrastructure needed to exercise NexusFlow against realistic, controllable conditions, plus the SEC-001 remediation and the fitness-function instrumentation foundation:

- **MinIO Fake-S3 (TASK-030):** an object-storage sink that behaves like S3 but runs locally in the demo environment, so pipelines can write records to a bucket and an operator can inspect them.
- **Mock-Postgres with seed data (TASK-031):** an isolated PostgreSQL instance pre-seeded with 10K rows of sample data, available to pipelines as a source or sink without touching the application database.
- **Sink Before/After snapshot capture (TASK-033):** every sink write now publishes a compact before/after snapshot over SSE, so the UI can show the effect of each task on the destination.
- **Sink Inspector GUI (TASK-032):** an admin screen that subscribes to the snapshot stream and visualises what each sink wrote, when, and with what row counts and sizes.
- **Chaos Controller GUI (TASK-034):** an admin console that can kill worker containers, disconnect the database for a fixed window, and flood the queue with synthetic tasks -- to demonstrate NexusFlow's monitor, retry, failover, and DLQ behaviour on demand.
- **SEC-001 remediation (SEC-001):** every user now has a mandatory password change on first login, and a `/change-password` endpoint is available from the UI. The default `admin/admin` no longer survives first login.
- **Fitness-function instrumentation (TASK-038):** a formal, CI-executable test suite that continuously verifies the architectural properties declared in the Architecture document (FF-003 fully implemented; 10 additional stubs declared with documented skip reasons).

This was the final demo-readiness cycle. After Cycle 4, Cycle 5 will deliver the production environment (TASK-036) -- which now also carries the SEC-018 and SEC-019 follow-up items from this cycle.

## Requirements Satisfied

| Requirement | Status |
|---|---|
| REQ-007 (extended): Database-class sink with atomicity | Satisfied (MinIO sink continues the sink family) |
| REQ-029: MinIO Fake-S3 demo infrastructure | Satisfied (TASK-030) |
| REQ-030: Mock-Postgres demo source/sink | Satisfied (TASK-031) |
| REQ-031: Sink before/after inspection | Satisfied (TASK-033, TASK-032) |
| REQ-032: Chaos demonstration surface | Satisfied (TASK-034) |
| REQ-020 / SEC-001 remediation: Password self-service and mandatory first-login change | Satisfied (SEC-001) |
| REQ-033: Fitness functions continuously verified | Partially satisfied -- FF-003 live; 10 stubs declared; full enforcement deferred to TASK-039 (Cycle 5 CI gate) |

## Tasks Completed

| Task | Verification |
|---|---|
| TASK-030: MinIO Fake-S3 | PASS (iteration 1; 9 unit + 7 integration + 12 acceptance + 4 system tests) |
| TASK-033: Sink Before/After snapshot capture | PASS (iteration 1; 6/6 ACs, CI run 24457333420) |
| TASK-031: Mock-Postgres with seed data | PASS (iteration 1; 4/4 ACs, CI run 24458872430) |
| TASK-032: Sink Inspector GUI | PASS (iteration 1; 6/6 ACs, CI run 24460040995) |
| TASK-034: Chaos Controller GUI | PASS (iteration 2; 6/6 ACs, CI run 24464513140) |
| SEC-001: Password change + mandatory first-login | PASS (iteration 1; 7 backend + 9 frontend ACs, CI run 24466108551) |
| TASK-038: Fitness-function instrumentation | PASS (iteration 2; 4/4 ACs, CI run 24474903030) |

## Verification Summary

| Layer | Written | Passing | Failing |
|---|---|---|---|
| Unit | 40+ | 40+ | 0 |
| Integration | 25+ | 25+ | 0 |
| System | 8 | 8 | 0 |
| Acceptance | 42 (backend+frontend, across 7 tasks) | 42 | 0 |
| Performance | N/A (deferred to Cycle 5 TASK-037) | -- | -- |

All CI runs for Cycle 4 task commits are green. REG-030 (scaffold-era CI regressions) is fully closed.

## Security Summary

Sentinel verdict: **PASS WITH CONDITIONS** (2026-04-15).

- **SEC-001 CLOSED.** Password change + mandatory first-login delivered and verified.
- **2 new HIGH findings (SEC-018 Docker socket mount on API container; SEC-019 no rate limit on change-password):** Nexus disposition 2026-04-15 -- **both ACCEPTED as demo-scope risk**; both to be addressed in Cycle 5 / TASK-036 production hardening (socket mount removed from production; change-password rate limiter reusing SEC-003 infrastructure).
- **2 MEDIUM (SEC-020 SQL Sprintf in postgres connector, SEC-021 default demo creds) + 3 LOW/INFO (SEC-022, SEC-023, SEC-024, SEC-025, SEC-026):** all ACCEPTED as previously categorized (demo-scope).
- **Carried-forward:** SEC-002 accepted risk unchanged; SEC-003 still fixed; SEC-004 through SEC-007 and SEC-014 remain accepted.

**No unresolved Critical or High findings block Demo Sign-off.** Full Sentinel report: `process/sentinel/cycle-4-security-report.md`.

## Demo

**Environment:** staging -- https://nexusflow.staging.nxlabs.cc (behind existing Traefik + basic-auth peer layer).

Follow these scenarios in order. Every step below has been executed under verification; the UI and API behaviour are frozen at the commits referenced in the task table.

### 1. Mandatory first-login password change (SEC-001)

- **Given** I open the staging URL in a clean browser and sign in with the seeded `admin / admin` credentials,
- **When** the session is accepted,
- **Then** I am redirected to `/change-password` and every other route in the app is blocked until I complete the form.
- **When** I submit a new password at least 8 characters long,
- **Then** all my existing sessions are invalidated, I am sent back to the login screen, and the new password (and only the new password) works on the next login. The `admin/admin` credentials no longer grant access.

### 2. MinIO Fake-S3 sink (TASK-030)

- **Given** the demo profile is running (`docker compose --profile demo up`),
- **When** I create a pipeline whose sink is `minio` with a chosen bucket key prefix,
- **Then** task executions upload their record batches to MinIO atomically (multipart upload, abort-on-error).
- **And** browsing `http://<host>:9001` (MinIO console; default `minioadmin/minioadmin` for demo only) shows the written objects under the configured prefix.

### 3. Mock-Postgres source / sink (TASK-031)

- **Given** the `demo-postgres` container is healthy,
- **When** I create a pipeline whose source is `postgres` with `table=sample_data`,
- **Then** the worker reads rows via `pgxpool` and emits them into the pipeline.
- **When** the sink is `postgres` with the same DSN and a target table,
- **Then** the worker inserts rows using parameterised `$N` placeholders for values.
- **Note:** Identifiers are still interpolated via `fmt.Sprintf` (SEC-020, ACCEPTED demo-scope). Do not point the `postgres` connector at non-demo infrastructure during the demo.

### 4. Sink Before/After snapshot capture (TASK-033) + Sink Inspector GUI (TASK-032)

- **Given** I am signed in as admin and navigate to `Sink Inspector`,
- **When** I submit a task whose pipeline writes to a tracked sink,
- **Then** the Sink Inspector shows a live Before snapshot, then an After snapshot, for that task within seconds of completion.
- **And** the panel displays row counts, byte sizes, and the connector type, without exposing row contents.
- **And** a non-admin user navigating to the same route sees an Access Denied panel (UI guard); a non-owner non-admin user is rejected at the SSE handshake (`authoriseTaskAccess`).

### 5. Chaos Controller GUI (TASK-034)

- **Given** I am signed in as admin and navigate to `Chaos`,
- **When** I pick a worker from the dropdown and click `Kill Worker`,
- **Then** the selected worker container is killed on the host, the Worker Fleet Dashboard (TASK-020) marks it Down within the heartbeat window, and the monitor/DLQ pipeline reclaims any in-flight task.
- **When** I click `Disconnect Database` with duration 30 seconds,
- **Then** the primary `nexusflow-postgres-1` container is stopped; the API's health endpoint goes to `unhealthy`; tasks in flight retry per backoff; 30 seconds later the DB restarts and normal operation resumes.
- **When** I click `Flood Queue` with taskCount = 500 against a demo pipeline,
- **Then** 500 tasks are enqueued sequentially; the Task Feed (TASK-021) shows the spike; worker throughput behaves as expected; no task is dropped.
- **And** a non-admin attempting the same routes receives 403 at both UI and API layers.

### 6. Fitness-function instrumentation (TASK-038)

- **Given** the CI pipeline is green,
- **When** I open the latest CI run summary,
- **Then** the `fitness-functions` job has executed the FF-003 integration test (live) plus skip-stubs for FF-001, FF-002, FF-004 through FF-012 with documented reasons.
- **And** the project's architectural assertions are now continuously verifiable; TASK-039 (Cycle 5) will gate PRs on failure.

### 7. Self-service password change from the UI (SEC-001)

- **Given** I am signed in,
- **When** I navigate to `/change-password` from the user menu,
- **Then** the form requires the current password; an incorrect current password returns 401; a correct current password plus a new password of at least 8 characters returns 204, invalidates every session for the user, and forces re-login.

## Known Limitations or Deferred Items

- **Production environment not yet deployed.** Go-Live is BLOCKED until TASK-036 delivers a running production instance (Cycle 5). The v1.0.0 artefacts exist on staging only.
- **SEC-018 (Docker socket on API container) -- ACCEPTED as demo-scope; remediated in TASK-036.** Socket mount must be absent from production compose; move under demo-profile override. Optional hardening: container-name allowlist, socket proxy, Docker Go SDK in place of `exec.Command`.
- **SEC-019 (no rate limit on change-password) -- ACCEPTED as demo-scope; remediated in TASK-036.** Per-user rate limiter to reuse SEC-003 rate-limiter infrastructure; WARN-level log on 401s.
- **SEC-020 (SQL composition in postgres connector) -- ACCEPTED; demo-scope only.** Connector registration is gated on `DEMO_POSTGRES_DSN`; non-demo deployments do not load it. Recommend identifier regex validation if the connector is ever generalised.
- **SEC-021 (default minioadmin / demo creds) -- ACCEPTED; demo-scope only.** Same treatment as SEC-002.
- **FF-001, FF-002, FF-004 through FF-012** are t.Skip stubs with documented skip reasons (TASK-038 AC-1 satisfied by 1:1 coverage). TASK-039 (Cycle 5) will convert the CI `-test.run` filter to cover all FFs and gate on failure.
- **TASK-037 (throughput load test)** deferred to Cycle 5 per plan.
- Carried-forward Verifier observations (see project-state.md §"Open Verifier Observations") remain open; none are blockers.

## Technical Observations

New non-blocking observations from Cycle 4 verification:

- **OBS-032-2 (TASK-032):** JSDOM test environment required `scrollIntoView` / `ResizeObserver` polyfills for Sink Inspector page tests. Awareness for future UI test infrastructure.
- **OBS-032-3 (TASK-032):** CSS keyframe animations not exercised under JSDOM; visual smoke only. Awareness for visual regression tooling.
- **OBS-032-1 (TASK-032) -- CLOSED** by TASK-034 (server-side `RequireRole(Admin)` applied; confirmed in Sentinel report).

Carried Verifier observations from Cycles 1-3 remain open and are listed in `process/orchestrator/project-state.md` under "Open Verifier Observations". None block Demo Sign-off. If you wish to act on any of them, raise it as demo feedback and it will route through the Analyst like any other requirement change.

## Recommendation

**APPROVED recommended.** All 7 Cycle 4 tasks verified PASS. SEC-001 closed. Sentinel findings dispositioned by Nexus (SEC-018 + SEC-019 bundled into TASK-036; remainder accepted as demo-scope). No blocking issues.

On approval, the Orchestrator will:
1. Hand control to the Methodologist with the single retrospective question (one change for Cycle 5?).
2. On Methodologist return, route the Planner to open Cycle 5: TASK-036 (production environment, with SEC-018 + SEC-019 remediation bundled), TASK-037 (throughput load test), TASK-039 (fitness-function CI gate).
3. Go-Live for v1.0.0 remains BLOCKED until TASK-036 delivers production.
