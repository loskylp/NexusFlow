<!-- Copyright 2026 Pablo Ochendrowitsch — Apache License 2.0 -->

# Verification Report — TASK-030
**Date:** 2026-04-15 | **Result:** PASS
**Task:** Demo infrastructure — MinIO Fake-S3 connector (DataSource + Sink) | **Requirement(s):** DEMO-001

## Acceptance Criteria Results

| # | Criterion | Layer | Result | Notes |
|---|---|---|---|---|
| AC-1 | MinIO starts via `docker compose --profile demo up` | System | PASS | Health endpoint 200 verified; seed script verified in source; log messages confirmed in cmd/worker/main.go lines 189 and 216; Docker Compose healthcheck and minio-init service wired correctly |
| AC-2 | S3 DataSource can read objects from MinIO buckets | Acceptance + Integration | PASS | In-memory positive + 2 negative cases; live adapter test against running MinIO (3 seeded records fetched and decoded correctly) |
| AC-3 | S3 Sink can write objects to MinIO buckets | Acceptance + Integration | PASS | In-memory positive + idempotency + atomicity abort negative cases; live adapter round-trip confirmed (object written and read back as valid JSON array) |
| AC-4 | A demo pipeline can be defined using MinIO as DataSource and Sink | Acceptance | PASS | Registry resolution, type string, unregistered negative, and full end-to-end DataSource→Sink pipeline all pass |

## Test Summary

| Layer | Written | Passing | Failing |
|---|---|---|---|
| Unit (Builder — run for confirmation) | 9 | 9 | 0 |
| Integration | 7 | 7 | 0 |
| System | 4 | 4 (2 live, 2 source-verified) | 0 |
| Acceptance | 12 | 12 | 0 |
| Performance | 0 | n/a | n/a |

**Unit tests (Builder-authored):** `go test ./worker/ -run TestMinIO -v` — 9/9 PASS.
All 4 DataSource tests, 4 Sink tests, and 1 registration test pass.

**Integration tests (Verifier-authored):** `tests/integration/TASK-030-minio-integration_test.go` — 7/7 PASS against live MinIO (localhost:19000).
Covers: ListKeys prefix filter, GetObject JSON decode, multipart upload round-trip, abort leaves no object, ListObjectCount scoped prefix, DataSourceConnector.Fetch live, SinkConnector.Write live with read-back.

**Acceptance tests (Verifier-authored):** `tests/acceptance/TASK-030-acceptance_test.go` — 12/12 PASS.
In-memory and live variants for AC-2 and AC-3; pure in-memory for AC-4.
Each criterion has at least one positive and one negative test case.

**System tests (Verifier-authored):** `tests/system/TASK-030-system_test.go` — 4 tests covering AC-1 scenarios.
Guard-skipped on `SYSTEM_TEST_DOCKER_COMPOSE=true`. AC-1 source verification confirmed directly:
- `/minio/health/live` returns HTTP 200 on running test container.
- `demo-input/data/` contains exactly 3 seeded objects (confirmed via mc ls against test container).
- Log line `worker: MINIO_ENDPOINT not set — MinIO connectors not registered` confirmed in source at line 189.
- Log line `worker: MinIO connectors registered (endpoint=minio:9000 ssl=false)` confirmed in source at line 216.

## Regression Check

Full worker unit suite (`go test ./worker/... -v`) — all passing. No regressions introduced.

## Observations (non-blocking)

**OBS-030-1:** `RegisterMinIOConnectors` uses `NewInMemoryDedupStore` for the Sink dedup store. This is consistent with the pattern used by `RegisterAtomicSinkConnectors`. For production use, a persistent DedupStore backed by the `sink_dedup_log` table should be injected. The docstring documents this. No action required for TASK-030 (demo infrastructure only).

**OBS-030-2:** `MinioClientAdapter.ListObjectCount` iterates over a channel returned by `ListObjects`; any per-object errors in the listing stream are silently ignored (the range over a channel does not expose object.Err). The `ListKeys` method does check `obj.Err`. The discrepancy is acceptable for demo use (Snapshot is informational) but may produce a silently incorrect count if listing fails mid-stream in production. Flagged for awareness — not a blocker for TASK-030.

**OBS-030-3:** The `minio-init` service does not have a `restart: "no"` or `restart_policy: condition: none` directive. Docker Compose will not restart it by default (no `restart:` key on one-shot services means it runs once), but explicit `restart: "no"` would make the intent unambiguous. Minor documentation clarity issue.

## Builder-declared deviations — Verifier assessment

1. **PutObject instead of low-level multipart API** — Confirmed acceptable. The connector uploads exactly one part per write; the buffer+PutObject mapping is semantically equivalent. The integration tests exercise the full Create/Upload/Complete path and confirm correct behavior. No action required.

2. **v7.0.91 (not latest)** — Confirmed necessary. v7.0.100 requires Go 1.25; project is Go 1.23. v7.0.91 is correct. No action required.

## CI Regression Report

**CI run:** 24439994606 | **Result:** FAILURE (2 jobs)

TASK-030's own implementation (`worker/` package) passes all CI checks. The failures are pre-existing regressions introduced by the Cycle 4 scaffold (`66c4bf0`) — they are present on every commit since that scaffold was pushed, including commits with no TASK-030 changes.

### REG-030-1: Go vet failure — `*stubUserRepo` missing `ChangePassword` method

**File:** `api/handlers_auth_test.go:116`
**Error:** `cannot use users (variable of type *stubUserRepo) as db.UserRepository value: *stubUserRepo does not implement db.UserRepository (missing method ChangePassword)`
**Root cause:** Cycle 4 scaffold added `ChangePassword(ctx, userID, newHash)` to the `db.UserRepository` interface (`internal/db/user_repository.go`) as part of the SEC-001 / `ChangePassword` API stub. The existing `stubUserRepo` in `api/handlers_auth_test.go` was not updated.
**Suggested fix:** Add a no-op `ChangePassword` method stub to `stubUserRepo` in `api/handlers_auth_test.go`.

### REG-030-2: TypeScript typecheck failure — `mustChangePassword` missing from User mock fixtures

**Files:** `src/components/ProtectedRoute.test.tsx`, `src/components/Sidebar.test.tsx`, `src/context/AuthContext.test.tsx`, `src/pages/LogStreamerPage.test.tsx`, `src/pages/LoginPage.test.tsx`, `src/pages/PipelineManagerPage.test.tsx`, `src/pages/TaskFeedPage.test.tsx`
**Error:** `Property 'mustChangePassword' is missing in type '...' but required in type 'User'`
**Root cause:** Cycle 4 scaffold added `mustChangePassword: boolean` to `web/src/types/domain.ts`'s `User` type (for the ChangePassword page stub). Existing test fixtures in all pre-existing test files use inline `User` objects that do not include this field.
**Suggested fix:** Add `mustChangePassword: false` to all User mock objects in the affected test files.

### REG-030-3: TypeScript typecheck failure — unused variables in scaffold stub pages

**Files:** `src/hooks/useSinkInspector.ts`, `src/pages/ChangePasswordPage.tsx`, `src/pages/ChaosControllerPage.tsx`, `src/pages/SinkInspectorPage.tsx`
**Error:** Multiple `TS6133`/`TS6196`/`TS6198` — declared but never used.
**Root cause:** Cycle 4 scaffold created placeholder pages that destructure components/state but the component is not yet rendered in the JSX (pending TASK-032, TASK-034, SEC-001 implementation). The TypeScript compiler treats these as errors under `noUnusedLocals`.
**Suggested fix:** Either remove the unused destructuring from the stubs, or use the `_` prefix convention to suppress unused variable warnings (`const [_SinkInspectorHeader, ...] = ...`). The cleanest fix is to remove the unused destructured names until they are actually wired into the JSX.

**Scope:** REG-030-1, REG-030-2, REG-030-3 are all outside TASK-030 scope. The Builder for TASK-030 does not own `api/handlers_auth_test.go` or the web test fixtures. These regressions should be fixed by the respective task owners (SEC-001 / TASK-032 / TASK-034 scaffold fix) or by a dedicated regression-fix dispatch from the Orchestrator.

## Recommendation

TASK-030 PASS — all 4 acceptance criteria verified. Escalate pre-existing CI regressions (REG-030-1, REG-030-2, REG-030-3) to Orchestrator for Builder dispatch.
