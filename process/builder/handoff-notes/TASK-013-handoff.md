# Builder Handoff — TASK-013
**Date:** 2026-03-26
**Task:** Pipeline CRUD via REST API
**Requirement(s):** REQ-022

## What Was Implemented

### Modified file

**`api/handlers_pipelines.go`** — Full implementation of `PipelineHandler` (Create, List, Get, Update, Delete). Replaced the five `panic("not implemented")` stub bodies with working handlers. Added two private helper functions:

- `parsePipelineID(w, r)` — Extracts and parses the `{id}` chi URL parameter as a UUID; writes 400 and returns false on failure. Extracted from each handler via Extract Function to eliminate repetition.
- `canAccessPipeline(sess, pipeline)` — Returns true when the session user is admin or owns the pipeline. Centralises the ownership check used by Get, Update, and Delete.

Added two new request struct types:
- `createPipelineRequest` — JSON body for POST /api/pipelines (name + three phase configs).
- `updatePipelineRequest` — JSON body for PUT /api/pipelines/{id} (same fields; separate type per Single Responsibility — create and update have distinct validation semantics in Cycle 2).

### New file

**`api/handlers_pipelines_test.go`** — 28 unit tests covering all 7 acceptance criteria plus edge cases. Uses `capturingPipelineRepo`, an in-memory `db.PipelineRepository` stub with error injection and active-task simulation. Tests run entirely in-process with no external dependencies.

### No changes required

**`api/server.go`** — All five pipeline routes were pre-wired in the authenticated group by the Scaffolder. No routing changes needed.

**`cmd/api/main.go`** — `pipelineRepo` was already constructed and wired into `api.NewServer` as argument 6. No dependency changes needed.

**`internal/db/pipeline_repository.go`** — The full `PgPipelineRepository` implementation was already in place (built as part of TASK-005). No repository changes needed.

## Handler behaviour by endpoint

### POST /api/pipelines (Create)
1. Validate session (401 if absent).
2. Decode JSON body (400 on parse error).
3. Validate `name` is non-empty (400 if blank).
4. Assign a new UUID and set `user_id` from the session (not from the request body).
5. Call `PipelineRepository.Create`; return 201 with the created pipeline JSON.

### GET /api/pipelines (List)
1. Validate session (401 if absent).
2. If `session.Role == admin`, call `List()` to return all pipelines.
3. Otherwise, call `ListByUser(session.UserID)` to return only the caller's pipelines.
4. Return 200 with a JSON array; empty array (not null) when no pipelines exist.

### GET /api/pipelines/{id} (Get)
1. Validate session (401 if absent).
2. Parse `{id}` as UUID (400 on invalid format).
3. Call `GetByID`; return 404 if not found.
4. Enforce ownership via `canAccessPipeline`; return 403 for non-owner non-admin.
5. Return 200 with the pipeline JSON.

### PUT /api/pipelines/{id} (Update)
1. Validate session (401 if absent).
2. Parse `{id}` as UUID (400 on invalid format).
3. Decode JSON body (400 on parse error).
4. Fetch existing pipeline; return 404 if not found.
5. Enforce ownership (403 for non-owner non-admin).
6. Call `PipelineRepository.Update` preserving `user_id` from the existing record.
7. Return 200 with the updated pipeline JSON.

### DELETE /api/pipelines/{id} (Delete)
1. Validate session (401 if absent).
2. Parse `{id}` as UUID (400 on invalid format).
3. Fetch existing pipeline; return 404 if not found.
4. Enforce ownership (403 for non-owner non-admin).
5. Call `PipelineRepository.Delete`; map `db.ErrActiveTasks` to 409; return 204 on success.

## Unit Tests

- Tests written: 28
- All passing: yes
- Build: `go build ./...` — clean
- Vet: `go vet ./...` — clean
- Staticcheck: v0.5.1 — clean (note: staticcheck@latest requires Go 1.25+; see TASK-005 handoff)
- Regression: all 48 tests in `api` package pass (20 prior + 28 new)

Key behaviours covered:

| Test | AC |
|---|---|
| `TestCreate_ValidPayloadReturns201WithPipeline` | AC-1 |
| `TestCreate_UserIDFromSessionIsUsed` | AC-1 |
| `TestCreate_UnauthenticatedReturns401` | AC-1 |
| `TestCreate_MalformedBodyReturns400` | AC-1 |
| `TestCreate_MissingNameReturns400` | AC-1 |
| `TestList_UserSeeOwnPipelinesOnly` | AC-2 |
| `TestList_AdminSeesAllPipelines` | AC-2 |
| `TestList_UnauthenticatedReturns401` | AC-2 |
| `TestList_EmptyListReturnsEmptyArray` | AC-2 |
| `TestGet_ExistingOwnedPipelineReturns200` | AC-3 |
| `TestGet_NonExistentPipelineReturns404` | AC-3 |
| `TestGet_InvalidIDFormatReturns400` | AC-3 |
| `TestGet_UnauthenticatedReturns401` | AC-3 |
| `TestGet_NonOwnerNonAdminReturns403` | AC-7 |
| `TestGet_AdminCanAccessAnyPipeline` | AC-7 |
| `TestUpdate_OwnerCanUpdatePipeline` | AC-4 |
| `TestUpdate_NonExistentPipelineReturns404` | AC-4 |
| `TestUpdate_MalformedBodyReturns400` | AC-4 |
| `TestUpdate_UnauthenticatedReturns401` | AC-4 |
| `TestUpdate_NonOwnerNonAdminReturns403` | AC-7 |
| `TestUpdate_AdminCanUpdateAnyPipeline` | AC-7 |
| `TestDelete_OwnerCanDeletePipeline` | AC-5 |
| `TestDelete_NonExistentPipelineReturns404` | AC-5 |
| `TestDelete_UnauthenticatedReturns401` | AC-5 |
| `TestDelete_ActiveTasksReturns409` | AC-6 |
| `TestDelete_NonOwnerNonAdminReturns403` | AC-7 |
| `TestDelete_AdminCanDeleteAnyPipeline` | AC-7 |
| `TestDelete_AdminCanDeleteAnyPipeline` | AC-7 |

## Deviations from Task Description

**No `//lint:ignore U1000` directives to remove.** The handlers_pipelines.go scaffold had no lint suppressions; the repository was already complete from TASK-005.

**No routing changes.** The task description listed wiring the routes as a deliverable, but the Scaffolder had pre-registered all five pipeline routes in `api/server.go` and pre-wired `pipelineRepo` in `cmd/api/main.go`. This is not a deviation from the acceptance criteria; all ACs are satisfied.

**`updatePipelineRequest` is a separate type from `createPipelineRequest`.** They have identical fields today. They are kept separate because Create sets `user_id` from the session and will gain connector-type validation (TASK-026), while Update replaces existing config and must preserve `user_id` from the existing record. Merging them into one type would violate Open/Closed when TASK-026 adds divergent validation logic.

**No design-time schema mapping validation.** The scaffold docstring explicitly defers this to TASK-026 (Cycle 2). The handler accepts any JSON for phase configs without cross-field validation.

## Known Limitations

**No schema mapping validation on Create/Update.** Accepted as a deferred Cycle 2 concern (TASK-026). The handler stores whatever JSON is provided for `dataSourceConfig`, `processConfig`, and `sinkConfig` without validating that `inputMappings` reference fields declared in `outputSchema` of the preceding phase.

**Pipeline ownership not enforced in `TaskHandler.Submit`.** This was noted in the TASK-005 handoff as an open concern. TASK-013 does not change `Submit`; it adds ownership enforcement only on the pipeline CRUD endpoints. The Verifier or Planner should decide whether Submit should also check `pipeline.UserID == session.UserID`.

## For the Verifier

**Acceptance criteria mapping:**

| AC | How to verify |
|---|---|
| AC-1: POST creates pipeline with correct schema | `POST /api/pipelines` with valid session and body; assert 201 and response JSON contains `id`, `name`, `userId` (matching the authenticated user), `dataSourceConfig`, `processConfig`, `sinkConfig`, `createdAt`, `updatedAt`. Query `pipelines` table to confirm the row exists. |
| AC-2: GET returns list (user sees own, admin sees all) | As user A: `GET /api/pipelines`; assert only user A's pipelines returned. As admin: assert all pipelines returned. |
| AC-3: GET /{id} returns pipeline details | `GET /api/pipelines/{id}` with owning user session; assert 200 and correct pipeline JSON. |
| AC-4: PUT updates pipeline | `PUT /api/pipelines/{id}` with a changed `name`; assert 200 and `name` is updated in response and in DB. |
| AC-5: DELETE deletes pipeline | `DELETE /api/pipelines/{id}`; assert 204; assert row absent from `pipelines` table. |
| AC-6: DELETE returns 409 for active tasks | Insert a task referencing the pipeline with `status='running'`; `DELETE /api/pipelines/{id}`; assert 409 and pipeline still in DB. |
| AC-7: Non-owner non-admin gets 403 | As user B: `GET`, `PUT`, `DELETE` on a pipeline owned by user A; assert 403 on all three. As admin: assert 200/204 (no 403). |

**Integration prerequisites:** The `pipelines` table and all associated migrations must be applied. The test user must have a valid session cookie or Bearer token. The active-tasks 409 check requires a task with `status NOT IN ('completed','failed','cancelled')` referencing the target pipeline.
