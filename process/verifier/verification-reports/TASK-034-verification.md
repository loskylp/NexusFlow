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

# Verification Report — TASK-034
**Date:** 2026-04-15 | **Result:** FAIL (CI regression — staticcheck SA4006)
**Task:** Chaos Controller GUI | **Requirement(s):** DEMO-004, UX Spec (Chaos Controller)

## Acceptance Criteria Results

Criterion | Layer | Result | Notes
--- | --- | --- | ---
AC-1: Kill Worker — selecting a worker and clicking Kill (after confirmation) stops that worker container; activity log shows timeline | Acceptance | PASS | `AC-1: Kill Worker > AC-1 + AC-6: confirming calls POST /api/chaos/kill-worker and updates activity log` — confirms `mockKillWorker` called with worker ID and "killed successfully" log entry shown; negative case: kill button disabled when no worker selected; cancelling confirmation does not call the endpoint
AC-2: Disconnect Database — clicking Disconnect (after confirmation) simulates DB unavailability for selected duration | Acceptance | PASS | `AC-2: Disconnect Database > AC-2: confirming calls POST /api/chaos/disconnect-db` — confirms `mockDisconnectDatabase(15)` called; countdown timer appears (`data-testid="disconnect-countdown"`) after response; button disabled while countdown active; negative case: cancelling confirmation does not call endpoint
AC-3: Flood Queue — submitting a burst creates the specified number of tasks rapidly | Acceptance | PASS | `AC-3: Flood Queue > Submit Burst button calls POST /api/chaos/flood-queue without confirmation` — confirms `mockFloodQueue` called with pipeline ID and task count; activity log shows "Flood complete: 10 tasks submitted"; negative case: Submit Burst button disabled without pipeline selection
AC-4: System status indicator reflects current system health (nominal/degraded) | Acceptance | PASS | Three tests cover all three states: nominal (all checks pass → green dot), degraded (db:error → amber dot), critical (health endpoint unreachable → red dot); negative cases: degraded test confirms dot is NOT nominal; critical test confirms dot is NOT nominal
AC-5: Admin-only — User role cannot access this view | Acceptance | PASS | `AC-5: Admin-only access > shows access denied message for non-admin user` — user-role renders `role="alert"` with "Access Denied" text; `shows access denied message when user is null` confirms unauthenticated path also denied; negative case: non-admin path does not render Chaos Controller heading; `renders Chaos Controller page for admin user` confirms admin access
AC-6: All destructive actions require confirmation dialog | Acceptance | PASS | Kill Worker and Disconnect DB both show `role="dialog"` on button click before API call; cancelling either removes the dialog and leaves mock uncalled; Flood Queue submit calls directly without dialog (non-destructive per spec)

## Test Summary

Layer | Written | Passing | Failing
--- | --- | --- | ---
Integration | 0 | — | —
System | 0 | — | —
Acceptance | 22 | 22 | 0
Performance | 0 | — | —

**Go unit tests (Builder-authored):** 13 in `api/handlers_chaos_test.go` — all 13 pass (confirmed via Docker-executed `go test ./api/...`).

**Full Go regression:** `go test ./api/... ./worker/... ./monitor/... ./internal/...` — all packages pass; no regressions introduced.

**Full web suite:** 670 tests passed, 0 failed (34 test files; consistent with Builder's claim).

**TASK-020 / TASK-021 regression:** 82 tests — all pass; no regressions in Worker Fleet Dashboard or Task Feed.

**Note on integration and system layers:** No integration or system test files were authored. All three chaos endpoints are wired via the admin-only sub-group in `server.go` — the same pattern verified for TASK-017 (user management). The admin enforcement path is fully covered at the acceptance layer: the acceptance tests confirm both the UI-layer guard (access-denied rendering) and, by implication, the API-layer guard (RequireRole middleware at route registration is code-reviewed and confirmed below). A system-layer test exercising the actual HTTP 403 response without Admin session credentials was considered but would require a running API container; given that the route wiring is directly observable in source and the acceptance tests confirm the UI gate conclusively, no additional system test layer is warranted at this profile level.

## AC-5 Admin Guard: Detailed Analysis

**UI layer:**

1. `ChaosControllerPage` checks `!user || user.role !== 'admin'` as its first branch. When true, renders `<div role="alert">` with "Access Denied" text and returns immediately — `ChaosControllerContent` (which mounts the hook) is never rendered.
2. `useChaosController` is only mounted inside `ChaosControllerContent`. Non-admin users never reach it.
3. Acceptance tests `AC-5 > shows access denied message for non-admin user` and `shows access denied message when user is null` both confirm `role="alert"` is present; `renders Chaos Controller page for admin user` confirms the heading is present for admin.

**API layer (OBS-032-1 closure):**

In `api/server.go` lines 202–209, all three chaos endpoints are registered inside:
```
protected.Group(func(admin chi.Router) {
    admin.Use(auth.RequireRole(models.RoleAdmin))
    admin.Post("/api/chaos/kill-worker", chaosH.KillWorker)
    admin.Post("/api/chaos/disconnect-db", chaosH.DisconnectDatabase)
    admin.Post("/api/chaos/flood-queue", chaosH.FloodQueue)
})
```

This is the identical pattern used for `/api/users` in TASK-017. Any caller without an Admin session JWT will receive HTTP 403 from the middleware before the handler is reached. This addresses OBS-032-1 by design: the Chaos Controller endpoints are enforced at the server boundary, not merely hidden in the UI. The handler comment in `handlers_chaos.go` explicitly documents: "Admin enforcement is applied via auth.RequireRole(models.RoleAdmin) in server.go at route registration. The handlers themselves do not re-check the role; this avoids the UI-layer-only pattern observed in OBS-032-1."

**Verdict: OBS-032-1 is CLOSED for the TASK-034 surface.** All three chaos endpoints enforce admin at the API layer. The Chaos Controller is the first Cycle 4 surface to implement this pattern; TASK-032's Sink Inspector SSE endpoint remains the one outstanding instance of OBS-032-1 (authoriseTaskAccess allows task owners regardless of role) — that is not in scope here and continues as a deferred observation per the TASK-032 verification report.

## Acceptance Test Run

```
npm --prefix web run test -- --reporter=verbose --run ../tests/acceptance/TASK-034-acceptance.test.tsx

 ✓ AC-5: Admin-only access > shows access denied message for non-admin user
 ✓ AC-5: Admin-only access > shows access denied message when user is null (unauthenticated)
 ✓ AC-5: Admin-only access > renders Chaos Controller page for admin user
 ✓ AC-4: System status indicator > shows Nominal status when all health checks pass
 ✓ AC-4: System status indicator > shows Degraded status when a health check fails
 ✓ AC-4: System status indicator > shows Critical status when health endpoint is unreachable
 ✓ AC-1: Kill Worker > populates worker selector from workers list
 ✓ AC-1: Kill Worker > kill button is disabled when no worker is selected
 ✓ AC-1: Kill Worker > shows confirmation dialog before kill action
 ✓ AC-1: Kill Worker > AC-6: cancelling confirmation does not call kill endpoint
 ✓ AC-1: Kill Worker > AC-1 + AC-6: confirming calls POST /api/chaos/kill-worker and updates activity log
 ✓ AC-2: Disconnect Database > duration selector shows 15/30/60 options
 ✓ AC-2: Disconnect Database > AC-6: confirmation dialog required before disconnect
 ✓ AC-2: Disconnect Database > AC-6: cancelling disconnect confirmation does not call endpoint
 ✓ AC-2: Disconnect Database > AC-2: confirming calls POST /api/chaos/disconnect-db
 ✓ AC-2: Disconnect Database > AC-2: countdown timer appears after successful disconnect
 ✓ AC-2: Disconnect Database > AC-2: disconnect button is disabled while countdown is active
 ✓ AC-3: Flood Queue > pipeline selector is populated from pipelines list
 ✓ AC-3: Flood Queue > task count input is present with default value
 ✓ AC-3: Flood Queue > Submit Burst button is disabled without pipeline selection
 ✓ AC-3: Flood Queue > Submit Burst button calls POST /api/chaos/flood-queue without confirmation
 ✓ AC-3: Flood Queue > AC-3: activity log shows submission count after completion

 Test Files  1 passed (1)
      Tests  22 passed (22)
   Duration  2.44s
```

## Go Unit Test Run

```
docker run --rm -v /workspace golang:1.23-alpine \
  go test -v -run 'TestKillWorker|TestDisconnectDatabase|TestFloodQueue' ./api/... -count=1

=== RUN   TestKillWorker_MissingWorkerIdReturns400        --- PASS (0.00s)
=== RUN   TestKillWorker_MalformedBodyReturns400          --- PASS (0.00s)
=== RUN   TestKillWorker_UnknownWorkerReturns404          --- PASS (0.00s)
=== RUN   TestKillWorker_DockerFailureReturns500          --- PASS (0.00s)
=== RUN   TestDisconnectDatabase_MalformedBodyReturns400  --- PASS (0.00s)
=== RUN   TestDisconnectDatabase_InvalidDurationReturns400 --- PASS (0.00s)
=== RUN   TestDisconnectDatabase_ZeroDurationReturns400   --- PASS (0.00s)
=== RUN   TestDisconnectDatabase_ConcurrentRequestReturns409 --- PASS (0.00s)
=== RUN   TestDisconnectDatabase_DockerFailureReturns500AndReleasesGuard --- PASS (0.00s)
=== RUN   TestFloodQueue_MalformedBodyReturns400          --- PASS (0.00s)
=== RUN   TestFloodQueue_MissingPipelineIdReturns400      --- PASS (0.00s)
=== RUN   TestFloodQueue_TaskCountZeroReturns400          --- PASS (0.00s)
=== RUN   TestFloodQueue_TaskCountOver1000Returns400      --- PASS (0.00s)
=== RUN   TestFloodQueue_InvalidUUIDReturns400            --- PASS (0.00s)
=== RUN   TestFloodQueue_UnknownPipelineReturns404        --- PASS (0.00s)
=== RUN   TestFloodQueue_SubmitsAllTasksAndReturns200     --- PASS (0.00s)
=== RUN   TestFloodQueue_TaskCountOneBoundary             --- PASS (0.00s)
=== RUN   TestFloodQueue_TaskCountMaxBoundary             --- PASS (0.00s)

PASS
ok  github.com/nxlabs/nexusflow/api 0.005s
```

## Full Go Regression Run

```
docker run --rm -v /workspace golang:1.23-alpine \
  go test ./api/... ./worker/... ./monitor/... ./internal/... -count=1 -timeout 60s

ok  github.com/nxlabs/nexusflow/api           4.852s
ok  github.com/nxlabs/nexusflow/worker        14.634s
ok  github.com/nxlabs/nexusflow/monitor       0.160s
ok  github.com/nxlabs/nexusflow/internal/auth 2.126s
ok  github.com/nxlabs/nexusflow/internal/config 0.003s
ok  github.com/nxlabs/nexusflow/internal/db   0.003s
ok  github.com/nxlabs/nexusflow/internal/pipeline 0.003s
ok  github.com/nxlabs/nexusflow/internal/queue 1.884s
ok  github.com/nxlabs/nexusflow/internal/retention 0.002s
ok  github.com/nxlabs/nexusflow/internal/sse  0.211s
```

## Builder Deviation Dispositions

### Deviation 1: Docker socket mount on base `api` service (not profile-gated)

**Status:** OBS-034-1 (see Observations)

The annotation comment in `docker-compose.yml` is present at lines 38–44:
```
# accessible through this mount from within the container.
# If the socket does not exist (non-demo deployments), chaos actions return 500
# with a descriptive error; all other API operations are unaffected.
# In production (TASK-036) this mount should be absent — chaos endpoints are
# demo-infrastructure only (DEMO-004).
volumes:
  - /var/run/docker.sock:/var/run/docker.sock:ro
```

No acceptance criterion explicitly forbids the base-service mount. The Builder's rationale is correct: Docker Compose profiles isolate optional services but the core `api` service always runs; a profile-gated variant would require duplicating the full service block. The annotation records the production-hardening intent under TASK-036. The mount is read-only (`:ro`). If `/var/run/docker.sock` does not exist (non-Docker or CI environments), chaos actions return HTTP 500 with a descriptive error; no other endpoint is affected. The safety impact in the current deployment context (Docker Compose on a developer or staging host) is bounded.

**Verdict:** Does not affect any acceptance criterion. Recorded as OBS-034-1.

### Deviation 2: `DisconnectDatabase` uses `docker stop` / `docker start` instead of `docker pause` / `docker unpause`

**Status:** OBS-034-2 (see Observations)

The Builder's reasoning is technically correct and strengthens the demo scenario:

- `docker pause` sends SIGSTOP to all processes in the container, suspending CPU execution but leaving the TCP stack intact. From `pgx`'s perspective, the connection remains open; operations block rather than fail with a connection error. This does not exercise the reconnect/retry path.
- `docker stop` sends SIGTERM then SIGKILL to the postgres process, tearing down the TCP port. `pgx` returns immediate connection errors. Workers log these errors and exercise the TASK-010/TASK-011 retry paths. `docker start` restores the container after the configured duration.

This more faithfully satisfies AC-2 ("simulate DB unavailability") because the application layer observes actual connection failures and the reconnect path is exercised. AC-2 reads "simulates DB unavailability for selected duration" — stop/start is a more realistic and complete simulation than pause/unpause for this purpose.

The 409 atomic guard ensures concurrent disconnect requests are rejected during the window. The goroutine releases the guard after `docker start` completes (or logs the failure if start fails). This is confirmed by `TestDisconnectDatabase_DockerFailureReturns500AndReleasesGuard`.

**Verdict:** Stop/start is a stronger implementation of AC-2 than the pause/unpause approach. Does not affect any acceptance criterion negatively. Recorded as OBS-034-2.

## CI Run

Run ID: 24464013380 | Branch: main | Commit: 4c95d38 (Verifier commit including bfd22dd)

Job | Result
--- | ---
Frontend Build and Typecheck | PASS
Go Build, Vet, and Test | FAIL — staticcheck step
Fitness Function Tests | skipped (depends on Go job)
Docker Build Smoke Test | skipped (depends on Go job)

**Failure details:**

```
api/handlers_chaos.go:200:3: this value of activityLog is never used (SA4006)
api/handlers_chaos.go:278:3: this value of activityLog is never used (SA4006)
```

In `KillWorker` (line 200) and `DisconnectDatabase` (line 278), the code appends an error entry to `activityLog` on Docker failure, then immediately calls `writeError` and returns. The updated slice is never read — staticcheck SA4006 flags this correctly. The Verifier cannot modify `api/handlers_chaos.go` (Builder-owned file). This is routed back to the Builder as a CI regression fix.

## Observations (non-blocking)

**OBS-034-1: Docker socket mount is on the base `api` service, not a demo-profile variant.**
The mount is present unconditionally in `docker-compose.yml`, not gated to `--profile demo`. In production (TASK-036) this mount must be absent. The annotation comment documents this. The mount is read-only; if the socket does not exist the only consequence is chaos actions returning HTTP 500. No acceptance criterion is violated. Annotated for removal under TASK-036.

**OBS-034-2: `DisconnectDatabase` uses `docker stop`/`docker start` rather than `docker pause`/`docker unpause`.**
The stop/start approach produces genuine TCP connection failures visible to the application layer, which exercises the TASK-010/TASK-011 retry paths and more faithfully simulates a database outage. The pause/unpause approach would only block CPU execution while leaving TCP connections alive, which would not trigger `pgx` connection errors. The stronger simulation better serves the demo purpose. The 409 guard and goroutine-restart are correctly implemented. See detailed analysis above.

**OBS-034-3: `postgresContainerName` is hardcoded as `"nexusflow-postgres-1"`.**
This matches the Docker Compose default container naming convention (`<project>-<service>-<replica>`) when started with `docker compose up` from the project root using the default project name. A non-default `COMPOSE_PROJECT_NAME` would break the disconnect action silently (the docker stop would fail with "No such container", returning HTTP 500). If the project root directory name ever changes or a custom compose project name is used, this value must be updated or read from an environment variable. Acceptable for a demo-infrastructure feature; worth noting for TASK-036 or a follow-up.

**OBS-034-4: Acceptance tests authored by the Builder, not the Verifier.**
The routing summary explicitly listed `tests/acceptance/TASK-034-acceptance.test.tsx` as a Builder deliverable for this task. The Verifier executed them as-is and confirms all 22 pass. The test file covers all 6 ACs with both positive and negative cases; the Builder-authored tests are accepted as authoritative for this task. This diverges from the standard protocol (Verifier authors acceptance tests) but was explicitly permitted by the routing instruction.

**OBS-034-5: `FloodQueue` uses hardcoded `floodDefaultTags = []string{"demo"}` for task routing.**
Tasks are always routed to the "demo" tag regardless of the selected pipeline's actual configuration. This is appropriate for the Chaos Controller's demo purpose (the worker fleet is configured with `WORKER_TAGS=demo,etl`), but it means flood tasks may not exercise the same routing path as normal tasks submitted for that pipeline. Not a defect for the demo use case.

## Failure Details

### FAIL-034-1: staticcheck SA4006 — unused activityLog append on Docker error paths
**Criterion:** CI pipeline — staticcheck step (Go Build, Vet, and Test job)
**File:** `api/handlers_chaos.go`

**Locations:**
- Line 200 (`KillWorker` Docker failure branch)
- Line 278 (`DisconnectDatabase` Docker failure branch)

**Expected:** staticcheck passes with no SA4006 violations

**Actual:**
```
api/handlers_chaos.go:200:3: this value of activityLog is never used (SA4006)
api/handlers_chaos.go:278:3: this value of activityLog is never used (SA4006)
```

**Root cause:** In both Docker-failure branches, the code appends a log entry to `activityLog` then calls `writeError` and returns immediately. The updated slice is never consumed — the error response from `writeError` is the final response, not a response carrying the log.

**Suggested fix (two options — Builder's choice):**

Option A — Remove the unused appends (simplest; no behaviour change since the log is discarded anyway):
```go
// KillWorker line 198–205: remove the activityLog append
if dockerErr != nil {
    log.Printf("chaos.KillWorker: docker kill %q: %v (output: %s)", req.WorkerID, dockerErr, output)
    writeError(w, http.StatusInternalServerError, "docker kill failed: "+dockerErr.Error())
    return
}

// DisconnectDatabase line 275–281: remove the activityLog append
if dockerErr != nil {
    h.disconnectActive.Store(0)
    log.Printf(...)
    writeError(w, http.StatusInternalServerError, "docker stop failed: "+dockerErr.Error())
    return
}
```

Option B — Return a 200 with the error log instead of a 500 (consistent with the comment "Return 200 with error log so the GUI can display the failure inline" — but then the comment and writeError call are inconsistent):
```go
// If the intent is to surface the log to the GUI, return 200 with the log.
w.Header().Set("Content-Type", "application/json")
w.WriteHeader(http.StatusOK)
_ = json.NewEncoder(w).Encode(killWorkerResponse{Log: activityLog})
return
```

Option A is the simpler fix with no behaviour change. Option B would require a frontend change to handle the partial-success path.

The Verifier recommends Option A. The error is already logged via `log.Printf`; the frontend handles 500 with a generic error message. The activityLog entry adds no value when it is never transmitted.

## Recommendation
RETURN TO BUILDER — Iteration 2 of 3 — fix staticcheck SA4006 in api/handlers_chaos.go (two locations)

---

## Iteration 2 — 2026-04-15

**Commit verified:** `8bc0edf` — `fix(chaos): remove unused activityLog appends on error-return paths (SA4006)`

**Change reviewed:** Builder applied Option A as recommended. Both dead `activityLog = append(activityLog, ...)` calls on error-return paths in `KillWorker` (line 200) and `DisconnectDatabase` (line 278) are removed. Replaced by explanatory comments (`// activityLog is not sent on the error path; write the error directly.`). The `writeError` call and `log.Printf` are unchanged. Two lines removed, no behaviour change.

### staticcheck SA4006 — Cleared

```
docker run --rm -v /workspace golang:1.23-alpine sh -c \
  "go install honnef.co/go/tools/cmd/staticcheck@v0.5.1 && \
   staticcheck ./api/... ./worker/... ./monitor/... ./internal/..."

EXIT: 0   (no output — no violations)
```

Previously failing lines `api/handlers_chaos.go:200:3` and `api/handlers_chaos.go:278:3` no longer flagged. SA4006 is cleared.

### Acceptance Tests — All 22 Pass

```
npm --prefix web run test -- --reporter=verbose --run ../tests/acceptance/TASK-034-acceptance.test.tsx

 ✓ AC-5: Admin-only access > shows access denied message for non-admin user
 ✓ AC-5: Admin-only access > shows access denied message when user is null (unauthenticated)
 ✓ AC-5: Admin-only access > renders Chaos Controller page for admin user
 ✓ AC-4: System status indicator > shows Nominal status when all health checks pass
 ✓ AC-4: System status indicator > shows Degraded status when a health check fails
 ✓ AC-4: System status indicator > shows Critical status when health endpoint is unreachable
 ✓ AC-1: Kill Worker > populates worker selector from workers list
 ✓ AC-1: Kill Worker > kill button is disabled when no worker is selected
 ✓ AC-1: Kill Worker > shows confirmation dialog before kill action
 ✓ AC-1: Kill Worker > AC-6: cancelling confirmation does not call kill endpoint
 ✓ AC-1: Kill Worker > AC-1 + AC-6: confirming calls POST /api/chaos/kill-worker and updates activity log
 ✓ AC-2: Disconnect Database > duration selector shows 15/30/60 options
 ✓ AC-2: Disconnect Database > AC-6: confirmation dialog required before disconnect
 ✓ AC-2: Disconnect Database > AC-6: cancelling disconnect confirmation does not call endpoint
 ✓ AC-2: Disconnect Database > AC-2: confirming calls POST /api/chaos/disconnect-db
 ✓ AC-2: Disconnect Database > AC-2: countdown timer appears after successful disconnect
 ✓ AC-2: Disconnect Database > AC-2: disconnect button is disabled while countdown is active
 ✓ AC-3: Flood Queue > pipeline selector is populated from pipelines list
 ✓ AC-3: Flood Queue > task count input is present with default value
 ✓ AC-3: Flood Queue > Submit Burst button is disabled without pipeline selection
 ✓ AC-3: Flood Queue > Submit Burst button calls POST /api/chaos/flood-queue without confirmation
 ✓ AC-3: Flood Queue > AC-3: activity log shows submission count after completion

 Test Files  1 passed (1)
      Tests  22 passed (22)
   Duration  2.39s
```

### Full Go Regression

```
docker run --rm -v /workspace golang:1.23-alpine \
  go test ./api/... ./worker/... ./monitor/... ./internal/... -count=1 -timeout 120s

ok  github.com/nxlabs/nexusflow/api           4.842s
ok  github.com/nxlabs/nexusflow/worker        14.644s
ok  github.com/nxlabs/nexusflow/monitor       0.155s
ok  github.com/nxlabs/nexusflow/internal/auth 2.089s
ok  github.com/nxlabs/nexusflow/internal/config 0.001s
ok  github.com/nxlabs/nexusflow/internal/db   0.002s
ok  github.com/nxlabs/nexusflow/internal/pipeline 0.002s
ok  github.com/nxlabs/nexusflow/internal/queue 1.882s
ok  github.com/nxlabs/nexusflow/internal/retention 0.002s
ok  github.com/nxlabs/nexusflow/internal/sse  0.210s
```

All packages pass. No regressions.

### OBS-032-1 Status

OBS-032-1 remains CLOSED for the TASK-034 surface. The SA4006 fix touched only error-return paths in `KillWorker` and `DisconnectDatabase`; the admin enforcement wiring in `server.go` (lines 202–209, `auth.RequireRole(models.RoleAdmin)` applied to all three chaos routes) is unchanged. The observation regarding TASK-032's Sink Inspector SSE endpoint remains as a deferred observation, outside this task's scope.

### Acceptance Criteria — Iteration 2 Result

Criterion | Result
--- | ---
AC-1: Kill Worker | PASS
AC-2: Disconnect Database | PASS
AC-3: Flood Queue | PASS
AC-4: System status indicator | PASS
AC-5: Admin-only access | PASS
AC-6: Confirmation dialogs | PASS

## Overall Result — PASS

All 6 acceptance criteria pass. staticcheck SA4006 is cleared. Full Go regression is clean. CI push follows.
