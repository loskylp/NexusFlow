# Verification Report — TASK-005
**Date:** 2026-03-26 | **Result:** PASS — Iteration 2
**Task:** Task submission via REST API | **Requirement(s):** REQ-001, REQ-003, REQ-009, REQ-019

## Acceptance Criteria Results

| REQ | Criterion | Layer | Result | Notes |
|---|---|---|---|---|
| REQ-001 | POST /api/tasks with valid payload returns 201 with unique task ID | Acceptance | PASS | 201 returned; taskId is a valid UUID; two submissions produce different IDs |
| REQ-003, REQ-009 | Task record in PostgreSQL with status "queued"; submitted→queued transition in task_state_log | Acceptance | PASS | tasks.status = 'queued'; task_state_log has exactly one submitted→queued row with non-empty reason |
| REQ-003, ADR-001 | Task message in the appropriate Redis stream (queue:{tag}) | Acceptance | PASS | Task ID present in queue:etl; report-tagged task correctly routed to queue:report |
| REQ-001 | POST /api/tasks with non-existent pipeline UUID returns 400 with structured error | Acceptance | PASS | Returns 400 {"error":"pipeline not found"}; non-UUID format and malformed JSON also return 400 |
| REQ-001 | POST /api/tasks without retryConfig creates task with default retry settings (max_retries: 3, backoff: exponential) | Acceptance | PASS | DB confirms retry_config={"backoff":"exponential","maxRetries":3}; explicit retryConfig is preserved |
| REQ-019 | Unauthenticated request returns 401 with structured JSON error | Acceptance | PASS | 401 returned; body is {"error":"unauthorized"}; non-existent and revoked tokens also return 401 |

## Test Summary

| Layer | Written | Passing | Failing |
|---|---|---|---|
| Integration | 0 | 0 | 0 |
| System | 0 | 0 | 0 |
| Acceptance | 23 | 23 | 0 |
| Performance | 0 | 0 | 0 |

Pre-verification checks (Iteration 2):

- Unit tests (`go test ./api/... ./internal/auth/...` via Docker golang:1.23-alpine): 35 tests — ALL PASS
- `go build ./...`: PASS
- `go vet ./...`: PASS
- staticcheck v0.5.1 `./...`: PASS

## What changed between Iteration 1 and Iteration 2

### Fix 1: `cmd/api/main.go` — dependency wiring

Three previously nil arguments to `api.NewServer` are now wired:

```go
taskRepo    := db.NewPgTaskRepository(pool)
pipelineRepo := db.NewPgPipelineRepository(pool)
q           := queue.NewRedisQueue(redisClient)

srv := api.NewServer(cfg, pool, redisClient, userRepo, taskRepo, pipelineRepo, nil, q, sessionStore, nil)
```

This eliminated the nil-pointer dereference panic that was producing HTTP 500 on all task submission paths (AC-1 through AC-5 in Iteration 1).

### Fix 2: `internal/auth/auth.go` — JSON 401 body

`auth.Middleware` now calls `writeJSONUnauthorized(w)` (a package-private helper added in this iteration) instead of `http.Error(w, "unauthorized", http.StatusUnauthorized)`. The helper writes:

```
Content-Type: application/json
HTTP 401
{"error":"unauthorized"}
```

This resolved FAIL-002 from Iteration 1, where the 401 body was plain text.

## Acceptance Test Detail — 23 cases, all passing

### AC-1 — Valid payload returns 201 with unique task ID (4 cases)

| Test case | Trace | Result |
|---|---|---|
| AC-1a: POST /api/tasks returns 201 | REQ-001 | PASS |
| AC-1b: response body contains a valid UUID taskId | REQ-001 | PASS |
| AC-1c: response body status is 'queued' | REQ-001 | PASS |
| AC-1d [VERIFIER-ADDED]: second submission produces a different task ID | REQ-001 | PASS |

### AC-2 — Task in PostgreSQL with status 'queued'; submitted→queued transition logged (3 cases)

| Test case | Trace | Result |
|---|---|---|
| AC-2a: tasks table status = 'queued' | REQ-003 | PASS |
| AC-2b: task_state_log has exactly one submitted→queued row | REQ-009 | PASS |
| AC-2c [VERIFIER-ADDED]: submitted→queued entry has non-empty reason ('enqueued to Redis stream') | REQ-009 | PASS |

### AC-3 — Task message in Redis stream queue:{tag} (4 cases)

| Test case | Trace | Result |
|---|---|---|
| AC-3a: queue:etl stream has at least one entry | REQ-003 | PASS |
| AC-3b: queue:etl contains an entry with task ID | REQ-003 | PASS |
| AC-3c [VERIFIER-ADDED]: tag='report' task routed to queue:report | REQ-003 | PASS |
| AC-3d [VERIFIER-ADDED]: tag='report' task correctly absent from queue:etl | REQ-003 | PASS |

### AC-4 — Invalid pipeline reference returns 400 with structured error (5 cases)

| Test case | Trace | Result |
|---|---|---|
| AC-4a: non-existent pipeline UUID returns 400 | REQ-001 | PASS |
| AC-4b: error body is structured JSON with error='pipeline not found' | REQ-001 | PASS |
| AC-4c [VERIFIER-ADDED]: non-UUID pipelineId format returns 400 | REQ-001 | PASS |
| AC-4d [VERIFIER-ADDED]: malformed JSON body returns 400 | REQ-001 | PASS |
| AC-4e [VERIFIER-ADDED]: missing tags field returns 400 | REQ-001 | PASS |

### AC-5 — No retryConfig defaults to maxRetries=3, backoff=exponential (3 cases)

| Test case | Trace | Result |
|---|---|---|
| AC-5a: default maxRetries = 3 | REQ-001 | PASS |
| AC-5b: default backoff = 'exponential' | REQ-001 | PASS |
| AC-5c [VERIFIER-ADDED]: explicit retryConfig preserved (maxRetries=5, backoff=linear) | REQ-001 | PASS |

### AC-6 — Unauthenticated request returns 401 (4 cases)

| Test case | Trace | Result |
|---|---|---|
| AC-6a: POST /api/tasks without auth returns 401 | REQ-019 | PASS |
| AC-6b: 401 response body is structured JSON with an error field | REQ-019 | PASS |
| AC-6c [VERIFIER-ADDED]: non-existent token returns 401 | REQ-019 | PASS |
| AC-6d [VERIFIER-ADDED]: revoked token returns 401 | REQ-019 | PASS |

## Observations (non-blocking)

**OBS-001: workers dependency still nil** — `workers db.WorkerRepository` is still passed as `nil` to `NewServer`. This is correct per the task scope (TASK-006), but the `WorkerHandler.List` route is registered and will panic if called. The chi `Recoverer` middleware will catch it (500 instead of panic crash). No action required for TASK-005.

**OBS-002: `RequireRole` still uses plain-text http.Error for 403** — `internal/auth/auth.go:149` uses `http.Error(w, "forbidden", http.StatusForbidden)`. The TASK-005 acceptance criteria do not cover 403 responses, so this is out of scope. The inconsistency between 401 (now JSON) and 403 (still plain text) should be unified when the first role-gated endpoint is tested end-to-end. Flag for the cycle that introduces TASK-006 or TASK-010.

**OBS-003: staticcheck@latest incompatibility is pre-existing** — staticcheck v0.7.0 panics on this Go 1.23 project. staticcheck v0.5.1 passes clean. This is a CI pipeline concern in TASK-001 scope.

## Verdict

**PASS** — All 6 acceptance criteria satisfied. All 23 test cases pass. Unit tests (35), go build, go vet, and staticcheck all clean.
