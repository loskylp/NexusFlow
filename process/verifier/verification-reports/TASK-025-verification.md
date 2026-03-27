# Verification Report — TASK-025
**Date:** 2026-03-27 | **Result:** PASS
**Task:** Worker fleet status API | **Requirement(s):** REQ-016
**Iteration:** 2 (re-verification after Builder fix)

## Acceptance Criteria Results

| REQ | Criterion | Layer | Result | Notes |
|---|---|---|---|---|
| REQ-016 | GET /api/workers returns all registered workers regardless of caller role | Acceptance | PASS | HTTP 200 with JSON array; 2 live workers returned |
| REQ-016 | Each worker includes: id, status (online/down), tags, currentTaskId (nullable), lastHeartbeat | Acceptance | PASS | All 5 fields present and correctly typed on live data |
| REQ-016 | Unauthenticated request returns 401 | Acceptance | PASS | No-auth, invalid Bearer token, and wrong auth scheme all return 401 |

## Test Summary

| Layer | Written | Passing | Failing |
|---|---|---|---|
| Integration | 0 | 0 | 0 |
| System | 0 | 0 | 0 |
| Acceptance | 14 | 14 | 0 |
| Performance | 0 | 0 | 0 |

**Unit tests (Builder's tests — run for confirmation):** 6 written, 6 passing (unchanged from iteration 1).

**Acceptance test file:** `tests/acceptance/TASK-025-acceptance.sh`

**go build:** PASS — clean build across all packages.
**go vet:** PASS — no issues reported.

## Iteration 2 Fix Confirmed

The fix applied by the Builder is correct and complete:

- `cmd/api/main.go` line 83: `workerRepo := db.NewPgWorkerRepository(pool)` added.
- `cmd/api/main.go` line 114: `workerRepo` passed as the `workers` argument to `api.NewServer` (replacing the prior `nil`).

The nil pointer dereference at `api/handlers_workers.go` no longer occurs. The handler now correctly calls `db.PgWorkerRepository.List()` via the interface.

## Test Run Output (iteration 2)

```
=== TASK-025 Acceptance Tests — Worker fleet status API ===
    API: http://localhost:8080

Setup: login as admin
  admin token obtained (64 chars)

AC-1: GET /api/workers returns 200 with JSON array (admin)
  PASS: AC-1a [REQ-016]: GET /api/workers with admin token returns 200
  PASS: AC-1b [REQ-016]: response body is a JSON array
  PASS: AC-1c [REQ-016]: Content-Type is application/json

AC-1 (Domain Invariant 5): GET /api/workers returns 200 for user role
  SKIP: AC-1d [REQ-016]: user creation endpoint not available (HTTP 404); Domain Invariant 5 verified by unit tests

AC-2: Each worker includes required fields
  (2 worker(s) found — verifying field presence)
  PASS: AC-2a [REQ-016]: every worker has a non-empty 'id' field
  PASS: AC-2b [REQ-016]: every worker 'status' is 'online' or 'down'
  PASS: AC-2c [REQ-016]: every worker 'tags' is an array
  PASS: AC-2d [REQ-016]: every worker has 'currentTaskId' key (null or UUID string)
  PASS: AC-2e [REQ-016]: every worker has a non-null 'lastHeartbeat'

AC-2 (negative) [VERIFIER-ADDED]: Response must not include internal fields
  PASS: AC-2f [REQ-016] [VERIFIER-ADDED]: private field 'passwordHash' absent from response
  PASS: AC-2f [REQ-016] [VERIFIER-ADDED]: private field 'registered_at' absent from response
  PASS: AC-2f [REQ-016] [VERIFIER-ADDED]: private field 'internalState' absent from response

AC-3: Unauthenticated request returns 401
  PASS: AC-3a [REQ-016]: GET /api/workers with no auth header returns 401

AC-3 (negative) [VERIFIER-ADDED]: Invalid token returns 401
  PASS: AC-3b [REQ-016] [VERIFIER-ADDED]: GET /api/workers with invalid Bearer token returns 401
  PASS: AC-3c [REQ-016] [VERIFIER-ADDED]: GET /api/workers with Basic auth scheme returns 401

=== Results ===
  Total: 14  PASS: 14  FAIL: 0

=== TASK-025 PASS ===
```

## Observations (non-blocking, carried forward)

1. **Domain Invariant 5 (user role sees all workers)** remains verified only at the unit test layer. The `POST /api/users` endpoint is not yet available (Cycle 2 task), so the system-level user-role path cannot be exercised via acceptance tests. `TestWorkerList_UserRoleSeesAllWorkers` in the Builder's unit suite confirms the behaviour. This is unchanged from iteration 1 and is not a blocker.

2. **AC-2 live data confirmed.** Two workers (`nexusflow-worker-1` and `nexusflow-worker-2`) appeared in the response with all required fields populated. `currentTaskId` was `null` on both — correctly serialised as a JSON null per the nullable contract.
