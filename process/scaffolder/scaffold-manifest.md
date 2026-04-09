# Scaffold Manifest — NexusFlow
**Version:** 3 | **Date:** 2026-04-09
**Artifact Weight:** Blueprint
**Profile:** Critical
**Cycle:** 4 (additions — appended to Cycle 3 manifest)

---

## Revision Summary

Version 3 adds Cycle 4 scaffolding on top of the Cycle 3 scaffold (Version 2).
All prior cycle entries remain valid and are not repeated here. This document covers
only the new structure added for Cycle 4.

Cycle 4 tasks: TASK-030 (MinIO connector), TASK-031 (Mock-Postgres connector),
TASK-032 (Sink Inspector GUI), TASK-033 (Snapshot Capturer), TASK-034 (Chaos
Controller GUI), SEC-001 (Password change), TASK-038 (Fitness function CI gate).

Most Cycle 4 backend scaffolding was created during Cycle 3 (connector stubs,
snapshot.go, handlers_chaos.go, SinkInspectorPage, ChaosControllerPage,
useSinkInspector hook). Cycle 4 scaffolding adds: SEC-001 password change
infrastructure (handler, page, middleware changes, DB layer changes), the
useChaosController hook, the TASK-038 fitness function test file, CI workflow
changes, and all acceptance test stubs.

---

## Cycle 4 Scaffolding Status — What Was Already Done vs What Was Added Now

### Already scaffolded before Cycle 4 (created in prior cycles):

| File | Task | Status |
|---|---|---|
| `worker/connector_minio.go` | TASK-030 | Stub with TODO bodies |
| `worker/connector_postgres.go` | TASK-031 | Stub with TODO bodies |
| `worker/snapshot.go` | TASK-033 | Stub with TODO bodies |
| `api/handlers_chaos.go` | TASK-034 | Stub with TODO bodies |
| `web/src/pages/SinkInspectorPage.tsx` | TASK-032 | Stub with TODO bodies |
| `web/src/pages/ChaosControllerPage.tsx` | TASK-034 | Stub with TODO bodies |
| `web/src/hooks/useSinkInspector.ts` | TASK-032 | Stub with TODO bodies |
| `internal/models/models.go` (MustChangePassword) | SEC-001 | Field added |
| `internal/db/000007_must_change_password.up.sql` | SEC-001 | Migration written |
| `internal/db/000007_must_change_password.down.sql` | SEC-001 | Migration written |

### Added in Cycle 4 scaffolding pass:

| File | Task | What Was Done |
|---|---|---|
| `api/handlers_password_change.go` | SEC-001 | NEW — PasswordChangeHandler stub |
| `web/src/pages/ChangePasswordPage.tsx` | SEC-001 | NEW — ChangePasswordPage stub |
| `web/src/hooks/useChaosController.ts` | TASK-034 | NEW — useChaosController hook stub |
| `tests/system/TASK-038-fitness-functions_test.go` | TASK-038 | NEW — fitness function test stubs |
| `tests/acceptance/SEC-001-acceptance.sh` | SEC-001 | NEW — acceptance test stub |
| `tests/acceptance/SEC-001-change-password-page.test.tsx` | SEC-001 | NEW — frontend acceptance test |
| `tests/acceptance/TASK-030-acceptance.sh` | TASK-030 | NEW — acceptance test stub |
| `tests/acceptance/TASK-031-acceptance.sh` | TASK-031 | NEW — acceptance test stub |
| `tests/acceptance/TASK-032-acceptance.test.tsx` | TASK-032 | NEW — acceptance test stub |
| `tests/acceptance/TASK-033-acceptance.sh` | TASK-033 | NEW — acceptance test stub |
| `tests/acceptance/TASK-034-acceptance.test.tsx` | TASK-034 | NEW — acceptance test stub |
| `tests/acceptance/TASK-038-acceptance.sh` | TASK-038 | NEW — acceptance test script |
| `web/src/api/client.ts` | SEC-001, TASK-034 | EXTENDED — changePassword, killWorker, disconnectDatabase, floodQueue |
| `web/src/types/domain.ts` | SEC-001 | EXTENDED — mustChangePassword field on User |
| `web/src/components/ProtectedRoute.tsx` | SEC-001 | EXTENDED — allowMustChangePassword prop; MustChangePassword redirect |
| `web/src/App.tsx` | SEC-001 | EXTENDED — /change-password route added |
| `api/server.go` | SEC-001, TASK-034 | EXTENDED — routes for change-password and chaos endpoints |
| `internal/auth/auth.go` | SEC-001 | EXTENDED — MustChangePassword enforcement in Middleware |
| `internal/models/models.go` | SEC-001 | EXTENDED — MustChangePassword field on Session |
| `internal/db/repository.go` | SEC-001 | EXTENDED — ChangePassword method on UserRepository interface |
| `internal/db/user_repository.go` | SEC-001 | EXTENDED — ChangePassword impl + MustChangePassword in toModelUser |
| `internal/db/queries/users.sql` | SEC-001 | EXTENDED — UpdateUserPassword query |
| `internal/db/sqlc/models.go` | SEC-001 | EXTENDED — MustChangePassword on sqlcdb.User |
| `internal/db/sqlc/users.sql.go` | SEC-001 | EXTENDED — UpdateUserPassword function stub; SELECT column lists updated |
| `.github/workflows/ci.yml` | TASK-038 | EXTENDED — fitness-functions job added |

---

## Cycle 4 Structure Overview

```
api/
├── handlers_password_change.go   — POST /api/auth/change-password (SEC-001)   [NEW]
web/src/
├── pages/
│   └── ChangePasswordPage.tsx    — forced first-login password change (SEC-001) [NEW]
├── hooks/
│   └── useChaosController.ts     — chaos controller state + API calls (TASK-034) [NEW]
├── components/
│   └── ProtectedRoute.tsx        — extended: allowMustChangePassword (SEC-001)  [EXTENDED]
├── App.tsx                       — /change-password route (SEC-001)             [EXTENDED]
├── api/client.ts                 — changePassword, chaos API calls              [EXTENDED]
└── types/domain.ts               — mustChangePassword on User                   [EXTENDED]
internal/
├── auth/auth.go                  — MustChangePassword enforcement (SEC-001)     [EXTENDED]
├── models/models.go              — MustChangePassword on Session (SEC-001)      [EXTENDED]
├── db/
│   ├── repository.go             — ChangePassword on UserRepository             [EXTENDED]
│   ├── user_repository.go        — ChangePassword impl                          [EXTENDED]
│   ├── queries/users.sql         — UpdateUserPassword query                     [EXTENDED]
│   └── sqlc/
│       ├── models.go             — MustChangePassword on User                   [EXTENDED]
│       └── users.sql.go          — UpdateUserPassword stub + column lists       [EXTENDED]
tests/
├── system/
│   └── TASK-038-fitness-functions_test.go  — FF test stubs (TASK-038)          [NEW]
└── acceptance/
    ├── SEC-001-acceptance.sh               — password change acceptance         [NEW]
    ├── SEC-001-change-password-page.test.tsx — frontend acceptance              [NEW]
    ├── TASK-030-acceptance.sh              — MinIO connector acceptance         [NEW]
    ├── TASK-031-acceptance.sh              — Postgres connector acceptance      [NEW]
    ├── TASK-032-acceptance.test.tsx        — Sink Inspector GUI acceptance      [NEW]
    ├── TASK-033-acceptance.sh              — snapshot capture acceptance        [NEW]
    ├── TASK-034-acceptance.test.tsx        — Chaos Controller GUI acceptance    [NEW]
    └── TASK-038-acceptance.sh             — fitness function CI acceptance      [NEW]
.github/workflows/
└── ci.yml                        — fitness-functions job added (TASK-038)       [EXTENDED]
```

---

## Cycle 4 Components

---

### Cycle 3 (previously scaffolded, now being built)

The following components were scaffolded in a prior pass and have TODO bodies awaiting Builder implementation:

---

---

## Cycle 3 Structure Overview

```
web/src/
├── hooks/
│   ├── useTasks.ts           — live task list (REST seed + SSE merge)      [NEW]
│   ├── useLogs.ts            — log SSE stream with Last-Event-ID            [NEW]
│   └── usePipelines.ts       — pipeline list fetch + refresh                [NEW]
├── components/
│   ├── TaskCard.tsx          — single task card (presentational)            [NEW]
│   ├── SubmitTaskModal.tsx   — task submission modal                        [NEW]
│   ├── PipelineCanvas.tsx    — drag-and-drop pipeline canvas (dnd-kit)      [NEW]
│   └── SchemaMappingEditor.tsx — field mapping modal                        [NEW]
├── pages/
│   ├── TaskFeedPage.tsx      — full implementation (replaces placeholder)  [REPLACED]
│   ├── LogStreamerPage.tsx   — full implementation (replaces placeholder)  [REPLACED]
│   └── PipelineManagerPage.tsx — full implementation (replaces placeholder) [REPLACED]
├── api/
│   └── client.ts             — extended with Cycle 3 API functions         [EXTENDED]
api/
├── handlers_openapi.go       — GET /api/openapi.json handler               [NEW]
└── openapi.yaml              — OpenAPI 3.0 spec source                     [NEW]
internal/
└── retention/
    └── retention.go          — log retention: partition pruning + Redis XTRIM [NEW]
```

---

## Components

### useTasks — `web/src/hooks/useTasks.ts`
**Responsibility:** Maintains a live task list by combining an initial REST fetch with SSE event merging.
**Architectural source:** ADR-007 (SSE channels), TASK-021

#### Exported interfaces

| Element | Kind | Signature summary | Contract |
|---|---|---|---|
| `TaskFilters` | interface | `{ status?, pipelineId?, search? }` | Optional filter params for initial fetch and display |
| `UseTasksReturn` | interface | `{ tasks, isLoading, error, sseStatus, refresh }` | Live task list with loading/error state |
| `mergeTaskEvent(tasks, event)` | function | `(Task[], SSEEvent<Task>) -> Task[]` | Pure merge of one SSE event into the task list; exported for unit tests |
| `useTasks(filters?)` | hook | `(TaskFilters?) -> UseTasksReturn` | Seeds from REST, merges SSE events |

---

### useLogs — `web/src/hooks/useLogs.ts`
**Responsibility:** Streams log lines for a specific task via SSE, tracking Last-Event-ID for reconnect replay.
**Architectural source:** ADR-007 (Last-Event-ID replay), TASK-022

#### Exported interfaces

| Element | Kind | Signature summary | Contract |
|---|---|---|---|
| `UseLogsOptions` | interface | `{ taskId?, enabled? }` | Hook configuration |
| `UseLogsReturn` | interface | `{ lines, isLoading, accessError, sseStatus, lastEventId, clearLines }` | Accumulated log state |
| `useLogs(options)` | hook | `(UseLogsOptions) -> UseLogsReturn` | SSE connection with Last-Event-ID and access error surfacing |

---

### usePipelines — `web/src/hooks/usePipelines.ts`
**Responsibility:** Fetches and caches the pipeline list for the current user.
**Architectural source:** TASK-023, TASK-024, REQ-022

#### Exported interfaces

| Element | Kind | Signature summary | Contract |
|---|---|---|---|
| `UsePipelinesReturn` | interface | `{ pipelines, isLoading, error, refresh }` | Pipeline list with refresh trigger |
| `usePipelines()` | hook | `() -> UsePipelinesReturn` | Fetches GET /api/pipelines on mount |

---

### TaskCard — `web/src/components/TaskCard.tsx`
**Responsibility:** Renders a single task as a card in the Task Feed; pure presentational component.
**Architectural source:** TASK-021, UX Spec (Task Feed and Monitor)

#### Exported interfaces

| Element | Kind | Signature summary | Contract |
|---|---|---|---|
| `TaskCardProps` | interface | `{ task, pipelineName, isAdmin, isOwner, onViewLogs, onCancel, onRetry, isRecentlyUpdated? }` | All data via props — no internal state fetch |
| `isCancellable(status)` | function | `TaskStatus -> boolean` | True for submitted/queued/assigned/running; exported for unit tests |
| `statusBadgeStyle(status)` | function | `TaskStatus -> React.CSSProperties` | Returns badge style from DESIGN.md tokens |
| `TaskCard` | React component | `(TaskCardProps) -> ReactElement` | Renders task card with action buttons |

---

### SubmitTaskModal — `web/src/components/SubmitTaskModal.tsx`
**Responsibility:** Modal dialog for submitting a new task; owns form state.
**Architectural source:** TASK-021, TASK-035, REQ-002, UX Spec

#### Exported interfaces

| Element | Kind | Signature summary | Contract |
|---|---|---|---|
| `SubmitTaskModalProps` | interface | `{ isOpen, onClose, onSuccess, pipelines, initialPipelineId? }` | Controlled modal; pipelines injected from parent |
| `SubmitTaskModal` | React component | `(SubmitTaskModalProps) -> ReactElement \| null` | Returns null when isOpen is false |

---

### PipelineCanvas — `web/src/components/PipelineCanvas.tsx`
**Responsibility:** Drag-and-drop canvas for composing a three-phase linear pipeline; controlled component.
**Architectural source:** TASK-023, REQ-015, ADR-008, UX Spec (Pipeline Builder)

#### Exported interfaces

| Element | Kind | Signature summary | Contract |
|---|---|---|---|
| `PipelineCanvasState` | interface | `{ dataSource, process, sink, dataSourceToProcessMappings, processToSinkMappings }` | Complete canvas state; null phases = not yet placed |
| `MappingValidationError` | interface | `{ boundary, sourceField, message }` | Validation error from API or client validator |
| `PipelinePhase` | type | `'DataSource' \| 'Process' \| 'Sink'` | Phase names for dnd-kit identity |
| `PipelineCanvasProps` | interface | `{ value, onChange, validationErrors?, readOnly? }` | Controlled canvas; onChange called on any state mutation |
| `PipelineCanvas` | React component | `(PipelineCanvasProps) -> ReactElement` | dnd-kit drag source and drop target with linearity enforcement |

**Dependency note:** Requires `@dnd-kit/core` and `@dnd-kit/utilities`. Builder must install:
```
npm --prefix web install @dnd-kit/core @dnd-kit/utilities
```

---

### SchemaMappingEditor — `web/src/components/SchemaMappingEditor.tsx`
**Responsibility:** Modal for defining field-to-field schema mappings between adjacent pipeline phases; validates against declared source phase outputSchema.
**Architectural source:** TASK-023, REQ-007, ADR-008

#### Exported interfaces

| Element | Kind | Signature summary | Contract |
|---|---|---|---|
| `SchemaMappingEditorProps` | interface | `{ isOpen, title, sourceFields, mappings, onSave, onClose }` | Controlled editor; local edit state reset on re-open |
| `SchemaMappingEditor` | React component | `(SchemaMappingEditorProps) -> ReactElement \| null` | Returns null when isOpen is false |

---

### TaskFeedPage — `web/src/pages/TaskFeedPage.tsx`
**Responsibility:** Task lifecycle feed with filters, real-time SSE updates, task submission, and cancellation.
**Architectural source:** TASK-021, TASK-035, REQ-017, REQ-002, REQ-009, REQ-010

#### Exported interfaces

| Element | Kind | Signature summary | Contract |
|---|---|---|---|
| `FilterBar` | React component (internal) | `(FilterBarProps) -> ReactElement` | Status/pipeline/search dropdowns + Submit Task button |
| `FeedStatusBar` | React component (internal) | `(FeedStatusBarProps) -> ReactElement` | Role indicator badge + SSE status dot |
| `SkeletonTaskCard` | React component (internal) | `({ index }) -> ReactElement` | Placeholder during initial load |
| `TaskFeedPage` | React component (default export) | `() -> ReactElement` | Root page component |

---

### LogStreamerPage — `web/src/pages/LogStreamerPage.tsx`
**Responsibility:** Real-time log viewer with phase filtering, auto-scroll, download, and Last-Event-ID reconnection.
**Architectural source:** TASK-022, REQ-018, ADR-007

#### Exported interfaces

| Element | Kind | Signature summary | Contract |
|---|---|---|---|
| `PhaseFilter` | type | `'all' \| 'datasource' \| 'process' \| 'sink'` | Client-side phase filter toggle value |
| `filterLogLines(lines, phase)` | function | `(TaskLog[], PhaseFilter) -> TaskLog[]` | Pure filter; exported for unit tests |
| `LogLine` | React component (internal) | `({ line }) -> ReactElement` | Single formatted log line with phase color tag |
| `LogPanel` | React component (internal) | `(LogPanelProps) -> ReactElement` | Scrollable dark terminal panel |
| `LogStatusBar` | React component (internal) | `(LogStatusBarProps) -> ReactElement` | SSE status, line count, Last-Event-ID display |
| `LogStreamerPage` | React component (default export) | `() -> ReactElement` | Root page component; reads ?taskId query param on mount |

---

### PipelineManagerPage — `web/src/pages/PipelineManagerPage.tsx`
**Responsibility:** Pipeline Builder (drag-and-drop canvas with schema mapping) and pipeline management (list/edit/delete).
**Architectural source:** TASK-023, TASK-024, REQ-015, REQ-007, REQ-023, ADR-008

#### Exported interfaces

| Element | Kind | Signature summary | Contract |
|---|---|---|---|
| `PipelineEditorState` | interface (internal) | `{ pipelineId, name, canvas, hasUnsavedChanges, isSaving, validationErrors }` | Editor session state |
| `ComponentPalette` | React component (internal) | `(ComponentPaletteProps) -> ReactElement` | Left panel with drag sources and saved pipeline list |
| `CanvasToolbar` | React component (internal) | `(CanvasToolbarProps) -> ReactElement` | Name field, Save/Run/Clear buttons |
| `PipelineManagerPage` | React component (default export) | `() -> ReactElement` | Root page; handles save flow, edit flow, and navigation guard |

---

### API Client Extensions — `web/src/api/client.ts`
**Responsibility:** Additional typed API calls added for Cycle 3.
**Architectural source:** ADR-004, TASK-013, TASK-008, TASK-012, TASK-016, TASK-017

#### New exported functions

| Element | Kind | Signature summary | Contract |
|---|---|---|---|
| `listTasksWithFilters(params?)` | function | `({ status?, pipelineId?, search? }?) -> Promise<Task[]>` | GET /api/tasks with query string; replaces listTasks for Cycle 3 |
| `getTask(taskId)` | function | `string -> Promise<Task>` | GET /api/tasks/{id}; throws 403/404 |
| `cancelTask(taskId)` | function | `string -> Promise<void>` | POST /api/tasks/{id}/cancel; throws 403/409 |
| `downloadTaskLogs(taskId)` | function | `string -> Promise<string>` | GET /api/tasks/{id}/logs as raw text; throws 403/404 |
| `getPipeline(pipelineId)` | function | `string -> Promise<Pipeline>` | GET /api/pipelines/{id}; throws 403/404 |
| `updatePipeline(id, updates)` | function | `(string, Omit<Pipeline,...>) -> Promise<Pipeline>` | PUT /api/pipelines/{id}; throws 400/403 |
| `deletePipeline(pipelineId)` | function | `string -> Promise<void>` | DELETE /api/pipelines/{id}; throws 403/409 |
| `listUsers()` | function | `() -> Promise<User[]>` | GET /api/users (admin only); throws 403 |

---

### OpenAPI Handler — `api/handlers_openapi.go`
**Responsibility:** Serves the embedded OpenAPI 3.0 specification as JSON.
**Architectural source:** ADR-004, TASK-027, FF-011

#### Exported interfaces

| Element | Kind | Signature summary | Contract |
|---|---|---|---|
| `OpenAPIHandler` | struct | `{ specJSON []byte }` | Serves pre-embedded spec bytes |
| `NewOpenAPIHandler(specJSON)` | function | `[]byte -> *OpenAPIHandler` | Constructs handler with embedded spec |
| `OpenAPIHandler.ServeSpec` | method | `http.HandlerFunc` | GET /api/openapi.json; unauthenticated; Cache-Control: public max-age=3600 |

---

### OpenAPI Spec — `api/openapi.yaml`
**Responsibility:** Source-of-truth OpenAPI 3.0 specification for all REST endpoints.
**Architectural source:** ADR-004, TASK-027

All endpoints listed in the file header comment. Builder must populate the full spec.

---

### Log Retention — `internal/retention/retention.go`
**Responsibility:** Background jobs for PostgreSQL partition pruning and Redis log stream trimming.
**Architectural source:** ADR-008, TASK-028, FF-018

#### Exported interfaces

| Element | Kind | Signature summary | Contract |
|---|---|---|---|
| `StartRetentionJobs(ctx, pool, client)` | function | `(Context, *db.Pool, *redis.Client)` | Launches partition pruner (weekly) and Redis log trimmer (hourly); runs until ctx cancelled |
| `DropOldPartitions(ctx, pool)` | function | `(Context, *db.Pool) -> (int, error)` | Drops task_logs_YYYY_WW partitions with end boundary < now - 30 days; returns drop count |
| `TrimHotLogs(ctx, client)` | function | `(Context, *redis.Client) -> (int, error)` | XTRIM all logs:{taskId} streams to 72-hour window; returns trimmed key count |

---

## Dependencies between Cycle 3 components

| Component | Depends on | Nature of dependency |
|---|---|---|
| `useTasks` | `api/client.ts` (`listTasksWithFilters`), `useSSE`, `types/domain` | calls API, consumes SSE |
| `useLogs` | `api/client.ts` (`downloadTaskLogs`), `useSSE`, `types/domain` | SSE with Last-Event-ID |
| `usePipelines` | `api/client.ts` (`listPipelines`), `types/domain` | REST fetch only |
| `TaskFeedPage` | `useTasks`, `usePipelines`, `TaskCard`, `SubmitTaskModal` | composes hooks and sub-components |
| `LogStreamerPage` | `useLogs`, `useTasks` (task selector options), `api/client.ts` (`downloadTaskLogs`) | stream + download |
| `PipelineManagerPage` | `usePipelines`, `PipelineCanvas`, `SchemaMappingEditor`, `SubmitTaskModal`, `api/client.ts` | full builder flow |
| `PipelineCanvas` | `SchemaMappingEditor`, `@dnd-kit/core`, `@dnd-kit/utilities`, `types/domain` | drag-and-drop engine |
| `SubmitTaskModal` | `api/client.ts` (`submitTask`), `types/domain` | form submission |
| `internal/retention` | `internal/db` (`db.Pool`), `go-redis` | DB partition drop + Redis XTRIM |
| `api/handlers_openapi.go` | embedded `api/openapi.yaml` via `go:embed` | serves spec bytes |

---

## Builder task surface — Cycle 3 unimplemented elements

| Element | Location | Complexity signal |
|---|---|---|
| `mergeTaskEvent` | `web/src/hooks/useTasks.ts` | Low — same pattern as mergeWorkerEvent in useWorkers.ts |
| `useTasks` | `web/src/hooks/useTasks.ts` | Medium — filter params, REST seed, SSE merge, error handling |
| `useLogs` | `web/src/hooks/useLogs.ts` | **High** — Last-Event-ID tracking, access error surfacing, clearLines preserving lastEventId, task change resets buffer |
| `usePipelines` | `web/src/hooks/usePipelines.ts` | Low — simple REST fetch with refresh |
| `listTasksWithFilters` | `web/src/api/client.ts` | Low — query string construction + apiFetch |
| `getTask` | `web/src/api/client.ts` | Low — apiFetch wrapper |
| `cancelTask` | `web/src/api/client.ts` | Low — apiFetch wrapper |
| `downloadTaskLogs` | `web/src/api/client.ts` | Low — apiFetch variant returning raw text |
| `getPipeline` | `web/src/api/client.ts` | Low — apiFetch wrapper |
| `updatePipeline` | `web/src/api/client.ts` | Low — apiFetch wrapper |
| `deletePipeline` | `web/src/api/client.ts` | Low — apiFetch wrapper |
| `listUsers` | `web/src/api/client.ts` | Low — apiFetch wrapper |
| `isCancellable` | `web/src/components/TaskCard.tsx` | Low — set membership check on TaskStatus |
| `statusBadgeStyle` | `web/src/components/TaskCard.tsx` | Low — switch on status, return CSS token values |
| `TaskCard` | `web/src/components/TaskCard.tsx` | Medium — status badge, action button visibility, recent-update flash animation |
| `SubmitTaskModal` | `web/src/components/SubmitTaskModal.tsx` | Medium — form state management, validation, API call, spinner state |
| `PipelineCanvas` | `web/src/components/PipelineCanvas.tsx` | **High** — dnd-kit integration, linearity enforcement with tooltip rejection, connector line rendering, phase node rendering |
| `SchemaMappingEditor` | `web/src/components/SchemaMappingEditor.tsx` | Medium — row-level editing, client-side sourceField validation, save/cancel lifecycle |
| `FilterBar` | `web/src/pages/TaskFeedPage.tsx` | Low — controlled dropdowns and search input |
| `FeedStatusBar` | `web/src/pages/TaskFeedPage.tsx` | Low — role badge + SSE dot (mirrors Worker Fleet pattern) |
| `SkeletonTaskCard` | `web/src/pages/TaskFeedPage.tsx` | Low — placeholder card shape |
| `TaskFeedPage` | `web/src/pages/TaskFeedPage.tsx` | **High** — integrates useTasks, usePipelines, TaskCard, SubmitTaskModal, filter-driven re-query, recently-updated flash coordination, cancel confirmation dialog |
| `filterLogLines` | `web/src/pages/LogStreamerPage.tsx` | Low — array filter on line.level |
| `LogLine` | `web/src/pages/LogStreamerPage.tsx` | Low — format timestamp + phase tag + level + message with color |
| `LogPanel` | `web/src/pages/LogStreamerPage.tsx` | Medium — auto-scroll logic with useEffect + panelRef, filter application |
| `LogStatusBar` | `web/src/pages/LogStreamerPage.tsx` | Low — mirrors Worker Fleet StatusBar pattern |
| `LogStreamerPage` | `web/src/pages/LogStreamerPage.tsx` | **High** — URL query param taskId, task selector, phase toggles, auto-scroll toggle, download trigger, SSE + Last-Event-ID wiring via useLogs |
| `ComponentPalette` | `web/src/pages/PipelineManagerPage.tsx` | Medium — drag sources (dnd-kit draggable), saved pipeline list with delete confirmation |
| `CanvasToolbar` | `web/src/pages/PipelineManagerPage.tsx` | Low — name input + button states |
| `PipelineManagerPage` | `web/src/pages/PipelineManagerPage.tsx` | **High** — editor state machine, save/edit/load flow, navigation guard (useBlocker), API calls, validation error relay to PipelineCanvas |
| `NewOpenAPIHandler` | `api/handlers_openapi.go` | Low — struct construction |
| `OpenAPIHandler.ServeSpec` | `api/handlers_openapi.go` | Low — write bytes with Content-Type + Cache-Control |
| `openapi.yaml` (populated) | `api/openapi.yaml` | Medium — all endpoints documented with request/response schemas |
| `StartRetentionJobs` | `internal/retention/retention.go` | Low — two goroutines with tickers |
| `DropOldPartitions` | `internal/retention/retention.go` | Medium — query pg_inherits or information_schema.tables for partition names, parse dates, DROP TABLE for old ones |
| `TrimHotLogs` | `internal/retention/retention.go` | Medium — SCAN logs:* in batches, XTRIM MINID per key |

---

## Component dependency order for Builder sequencing

Builder tasks must be sequenced as follows for Cycle 3:

1. **API client extensions** (`web/src/api/client.ts`) — Low complexity; all GUI hooks depend on these. Complete first.

2. **useTasks + usePipelines + useLogs** — Medium/High complexity hooks. useTasks and usePipelines can run in parallel (no shared dependency). useLogs is independent. All three must complete before their consuming pages.

3. **TaskCard + SubmitTaskModal** — Low/Medium sub-components. No dependencies on hooks (props-driven). Can run in parallel with hooks.

4. **SchemaMappingEditor** — Medium; depends only on SchemaMapping types. Can run in parallel with hooks.

5. **PipelineCanvas** — High; depends on SchemaMappingEditor and dnd-kit install. dnd-kit must be installed before Builder begins.

6. **TASK-021 (TaskFeedPage)** — High; depends on useTasks, usePipelines, TaskCard, SubmitTaskModal. All dependencies must be implemented first.

7. **TASK-022 (LogStreamerPage)** — High; depends on useLogs, useTasks (for selector). Can run in parallel with TASK-021.

8. **TASK-023 (PipelineManagerPage — Pipeline Builder)** — High; depends on PipelineCanvas, SchemaMappingEditor, usePipelines, SubmitTaskModal. Highest risk in Cycle 3.

9. **TASK-024 (Pipeline management in PipelineManagerPage)** — Low; extends TASK-023's page. Complete after TASK-023.

10. **TASK-035 (Task submission flow end-to-end)** — Low; validates the complete flow from TASK-021. Complete after TASK-021 is verified.

11. **TASK-027 (OpenAPI + handlers_openapi.go)** — Low/Medium; no frontend dependencies. Can run in parallel with GUI tasks.

12. **TASK-028 (Log retention)** — Medium; no frontend dependencies. Can run in parallel with GUI tasks. Wire StartRetentionJobs into cmd/api/main.go.

---

## New dependency wiring required in existing files

The Builder must add the following wiring points (existing files not modified by Scaffold):

- **`cmd/api/main.go`**: Call `retention.StartRetentionJobs(ctx, pool, redisClient)` on startup (TASK-028).
- **`api/server.go`** (Handler method): Register `GET /api/openapi.json` → `OpenAPIHandler.ServeSpec` outside the auth middleware (TASK-027).
- **`web/package.json`**: Add `@dnd-kit/core` and `@dnd-kit/utilities` dependencies (TASK-023). Builder installs via `npm --prefix web install @dnd-kit/core @dnd-kit/utilities`.

---

## Boundary ambiguity note

One boundary ambiguity was discovered during Cycle 3 scaffolding:

**Log Streamer route (`/tasks/logs` vs `/tasks/:id/logs`):**
The UX spec lists the Log Streamer at `/tasks/{id}/logs`, but the App.tsx route (scaffolded in Cycle 1) uses `/tasks/logs` with a query param approach. The Scaffold aligns with the existing App.tsx route (`/tasks/logs?taskId=<uuid>`) to avoid modifying the working route table. The Builder for TASK-022 should confirm with the Architect whether the URL structure matters for deep-linking (e.g., bookmarkability of specific task logs). If the UX spec route `/tasks/:id/logs` is preferred, App.tsx must be updated to add a new route parameter.

---

---

## Cycle 4 Components

---

### PasswordChangeHandler — `api/handlers_password_change.go`
**Responsibility:** POST /api/auth/change-password — verifies current password, hashes new password, updates user record, clears MustChangePassword, invalidates all sessions.
**Architectural source:** SEC-001, SEC-007, ADR-006

#### Exported interfaces

| Element | Kind | Signature summary | Contract |
|---|---|---|---|
| `PasswordChangeHandler` | struct | `{ server *Server }` | Admin and user accessible; requires active session |
| `PasswordChangeHandler.ChangePassword` | method | `http.HandlerFunc` | POST /api/auth/change-password; 204 on success; 400/401/403 on error |

---

### ChangePasswordPage — `web/src/pages/ChangePasswordPage.tsx`
**Responsibility:** Forced first-login password change gate. Blocks all navigation until password is changed. No sidebar.
**Architectural source:** SEC-001, SEC-007, UX Spec (Login)

#### Exported interfaces

| Element | Kind | Signature summary | Contract |
|---|---|---|---|
| `ChangePasswordForm` | React component (internal) | `(ChangePasswordFormProps) -> ReactElement` | Three-field form with client-side validation; spinner on submit |
| `ChangePasswordPage` | React component (default export) | `() -> ReactElement` | Redirects to /login on success; redirects away if mustChangePassword=false |

---

### useChaosController — `web/src/hooks/useChaosController.ts`
**Responsibility:** Manages all state and API calls for the Chaos Controller demo view: worker/pipeline lists, system health, kill/disconnect/flood actions, countdown timer, per-card activity logs.
**Architectural source:** DEMO-004, TASK-034

#### Exported interfaces

| Element | Kind | Signature summary | Contract |
|---|---|---|---|
| `UseChaosControllerReturn` | interface | Full state surface (see file) | Workers, pipelines, systemStatus, per-action state + callbacks, per-card logs |
| `useChaosController()` | hook | `() -> UseChaosControllerReturn` | Fetches workers+pipelines on mount; refreshes health after chaos actions; all errors surfaced via logs |

---

### Fitness Function Tests — `tests/system/TASK-038-fitness-functions_test.go`
**Responsibility:** Named Go test functions for each fitness function in the fitness function index. Non-Docker tests run in CI; Docker-dependent tests skip with `t.Skip`.
**Architectural source:** TASK-038, process/architect/fitness-functions.md

#### Test functions

| Function | FF ID | Status |
|---|---|---|
| `TestFF001_QueuePersistence` | FF-001 | Skip (Docker) |
| `TestFF002_QueuingLatency` | FF-002 | TODO |
| `TestFF004_DeliveryGuarantee` | FF-004 | Skip (Docker) |
| `TestFF005_ChainTriggerDedup` | FF-005 | TODO |
| `TestFF006_SinkAtomicity` | FF-006 | TODO |
| `TestFF007_FailoverDetection` | FF-007 | Skip (Docker) |
| `TestFF008_TaskRecovery` | FF-008 | Skip (Docker) |
| `TestFF013_AuthEnforcement` | FF-013 | TODO |
| `TestFF015_CompileTimeSafety` | FF-015 | Passes immediately |
| `TestFF017_SchemaMigration` | FF-017 | TODO |
| `TestFF019_SchemaValidation` | FF-019 | TODO |
| `TestFF020_ServiceStartup` | FF-020 | Skip (requires compose) |
| `TestFF024_RedisPersistence` | FF-024 | Skip (Docker) |
| `TestFF022_SinkInspector` | FF-022 | TODO |

---

### ProtectedRoute (extended) — `web/src/components/ProtectedRoute.tsx`
**Responsibility:** Extended with `allowMustChangePassword` prop (SEC-001). When the user's `mustChangePassword` flag is true, all routes redirect to /change-password unless the route opts out with `allowMustChangePassword={true}`.
**Architectural source:** SEC-001, TASK-019

---

### SEC-001 DB Layer

| File | Change | Contract |
|---|---|---|
| `internal/models/models.go` | `MustChangePassword bool` added to `Session` | Session carries the flag to avoid per-request DB lookup |
| `internal/auth/auth.go` | Middleware enforces 403 for MustChangePassword sessions | Only POST /api/auth/change-password is exempt |
| `internal/db/repository.go` | `ChangePassword(ctx, id, passwordHash)` added to `UserRepository` | Updates hash + clears flag atomically |
| `internal/db/user_repository.go` | `ChangePassword` implemented; `toModelUser` includes `MustChangePassword` | Delegates to `UpdateUserPassword` sqlc query |
| `internal/db/queries/users.sql` | `UpdateUserPassword :exec` query added | `UPDATE users SET password_hash = $2, must_change_password = FALSE WHERE id = $1` |
| `internal/db/sqlc/models.go` | `MustChangePassword bool` added to `User` | Hand-edited; Builder must run `sqlc generate` to regenerate |
| `internal/db/sqlc/users.sql.go` | `UpdateUserPassword` stub + all SELECT column lists updated | Hand-edited; Builder must run `sqlc generate` |

---

## Cycle 4 Builder task surface — unimplemented elements

| Element | Location | Complexity signal |
|---|---|---|
| `NewMinIODataSourceConnector` | `worker/connector_minio.go` | Low — assign struct field; no logic |
| `MinIODataSourceConnector.Fetch` | `worker/connector_minio.go` | Medium — ListKeys loop, GetObject, JSON decode per object |
| `NewMinIOSinkConnector` | `worker/connector_minio.go` | Low — assign struct fields |
| `MinIOSinkConnector.Snapshot` | `worker/connector_minio.go` | Low — ListObjectCount call, return map |
| `MinIOSinkConnector.Write` | `worker/connector_minio.go` | **High** — multipart upload flow: CreateMultipartUpload, UploadPart, abort on error, CompleteMultipartUpload, DedupStore.Record; ErrAlreadyApplied guard |
| `RegisterMinIOConnectors` | `worker/connector_minio.go` | Low — two reg.Register calls |
| `NewPostgreSQLDataSourceConnector` | `worker/connector_postgres.go` | Low — assign struct field |
| `PostgreSQLDataSourceConnector.Fetch` | `worker/connector_postgres.go` | Medium — build SELECT from config["table"] or config["query"] + limit; call QueryRows |
| `NewPostgreSQLSinkConnector` | `worker/connector_postgres.go` | Low — assign struct fields |
| `PostgreSQLSinkConnector.Snapshot` | `worker/connector_postgres.go` | Low — RowCount call, return map |
| `PostgreSQLSinkConnector.Write` | `worker/connector_postgres.go` | **High** — BeginTx, InsertRow loop, Rollback on first error, Commit on success, DedupStore.Record; ErrAlreadyApplied guard |
| `RegisterPostgreSQLConnectors` | `worker/connector_postgres.go` | Low — two reg.Register calls |
| `NewSnapshotCapturer` | `worker/snapshot.go` | Low — assign struct fields |
| `SnapshotCapturer.CaptureAndWrite` | `worker/snapshot.go` | **High** — before snapshot, publish, write, after snapshot, publish; write error propagation; snapshot failures logged not propagated |
| `ChaosHandler.KillWorker` | `api/handlers_chaos.go` | **High** — Docker daemon socket access; SIGKILL container; activity log generation |
| `ChaosHandler.DisconnectDatabase` | `api/handlers_chaos.go` | **High** — iptables/network manipulation; background goroutine for restore; 409 guard for concurrent disconnect |
| `ChaosHandler.FloodQueue` | `api/handlers_chaos.go` | Medium — loop N task submissions; sequential; partial success tracking |
| `SinkInspectorPage` sub-components | `web/src/pages/SinkInspectorPage.tsx` | Medium/High — see file for per-component complexity |
| `ChaosControllerPage` sub-components | `web/src/pages/ChaosControllerPage.tsx` | Medium — delegating to useChaosController |
| `useSinkInspector` | `web/src/hooks/useSinkInspector.ts` | Medium — SSE event routing, state reset on task change, accessError surfacing |
| `useChaosController` | `web/src/hooks/useChaosController.ts` | **High** — multiple API calls, countdown timer, activity log accumulation, health refresh |
| `ChangePasswordPage` | `web/src/pages/ChangePasswordPage.tsx` | Medium — form state, 401/400 error mapping, redirect on success |
| `PasswordChangeHandler.ChangePassword` | `api/handlers_password_change.go` | Medium — verify current password, hash new, DB update, session invalidation |
| Fitness function test bodies | `tests/system/TASK-038-fitness-functions_test.go` | Low-Medium per test — HTTP client calls + threshold assertions |
| `sqlc generate` (regeneration) | `internal/db/sqlc/` | Low — run command; verify output matches scaffolded intent |
| `UpdateUserPassword` (after sqlc gen) | `internal/db/sqlc/users.sql.go` | Low — remove panic; generated implementation |

---

## Cycle 4 dependency order for Builder sequencing

1. **DB layer (SEC-001)** — Run `sqlc generate` first to produce the real `UpdateUserPassword` function and regenerate `must_change_password` column support. This unblocks all SEC-001 work.

2. **`PasswordChangeHandler.ChangePassword`** — Medium; depends on `UserRepository.ChangePassword`, session store `DeleteAllForUser`. Must be complete before the frontend can be tested end-to-end.

3. **SEC-001 connector implementations (TASK-030, TASK-031)** — Independent of SEC-001. MinIO and Postgres connector bodies can be implemented in parallel.
   - MinIOSinkConnector.Write is the highest-risk item (multipart upload abort pattern).
   - PostgreSQLSinkConnector.Write follows the established DatabaseSinkConnector pattern in sink_connectors.go.

4. **SnapshotCapturer.CaptureAndWrite (TASK-033)** — Depends on MinIO or Postgres connector being testable. Must be complete before Sink Inspector GUI can receive real events.

5. **ChaosHandler (TASK-034)** — Independent of connectors. Highest risk: Docker daemon access. Builder should spike Docker socket access first (verify the API container has /var/run/docker.sock mounted in demo compose profile).

6. **useSinkInspector** — Depends on SSE infrastructure (already implemented). Low external dependencies.

7. **SinkInspectorPage** — Depends on useSinkInspector. Complete after hook.

8. **useChaosController** — Depends on chaos API functions in client.ts (already scaffolded). Complete before ChaosControllerPage.

9. **ChaosControllerPage** — Depends on useChaosController. Complete after hook.

10. **ChangePasswordPage** — Depends on changePassword client.ts call (already scaffolded). Can run in parallel with ChaosControllerPage.

11. **Fitness function test bodies (TASK-038)** — Depends on SEC-001 and connector implementations being testable. Most are straightforward HTTP client assertions against the running API.

---

## Cycle 4 new dependency wiring required in existing files

| File | Required wiring | Task |
|---|---|---|
| `cmd/worker/main.go` | Register MinIO connectors: `worker.RegisterMinIOConnectors(reg, minioBackend)` when MINIO_ENDPOINT env var is set | TASK-030 |
| `cmd/worker/main.go` | Register Postgres connectors: `worker.RegisterPostgreSQLConnectors(reg, pgBackend)` when DEMO_POSTGRES_DSN env var is set | TASK-031 |
| `cmd/worker/main.go` | Wrap the SinkConnector in SnapshotCapturer: `worker.NewSnapshotCapturer(sink, redisClient)` for demo connectors | TASK-033 |
| `api/handlers_auth.go` (`Login` handler) | Set `MustChangePassword: user.MustChangePassword` when creating the Session | SEC-001 |
| `api/handlers_users.go` (`CreateUser` handler) | Set `MustChangePassword: true` on newly created users so they must change on first login | SEC-001 |
| `docker-compose.yml` | Mount `/var/run/docker.sock` into the `api` service under `demo` profile | TASK-034 |

---

## Cycle 4 boundary ambiguities discovered during scaffolding

**1. sqlc regeneration required before SEC-001 can compile cleanly.**
The scaffolded `internal/db/sqlc/models.go` and `users.sql.go` are hand-edited to add `must_change_password` support. These files carry a prominent note that they will be overwritten by `sqlc generate`. The Builder must run `sqlc generate` against a database with migration 000007 applied before any SEC-001 Go code is compiled in CI. The hand-edits exist only to make the package compile before the sqlc tool is run. The CI `go-build-and-test` job will fail on the `UpdateUserPassword` panic if the sqlc regeneration has not been run — this is intentional (fail-fast).

**2. Chaos Controller DB disconnect implementation approach.**
The ChaosHandler.DisconnectDatabase implementation approach (iptables) requires the API container to run with NET_ADMIN capability in the demo Docker Compose profile. This is an Architect-level decision not yet made explicit in the ADRs. The Scaffolder has documented the iptables approach in the handler docstring (matching the task plan description), but the Builder should confirm this approach is viable in the deployment environment before implementing. If iptables is not available, an alternative (e.g., stopping the PostgreSQL container) may be needed.

**3. Session invalidation pattern for password change.**
The `PasswordChangeHandler.ChangePassword` postcondition specifies that all sessions for the user are invalidated after a successful change. The existing `SessionStore` interface (in `internal/queue/`) may or may not have a `DeleteAllForUser(ctx, userID)` method. The Builder must check and, if needed, add this method to the `SessionStore` interface before implementing `ChangePassword`. If `DeleteAllForUser` does not exist, the simplest approach is to delete all session:{token} keys for the user by scanning Redis (consistent with how `DeactivateUser` works in `handlers_users.go`).
