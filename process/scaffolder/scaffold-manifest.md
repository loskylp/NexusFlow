# Scaffold Manifest — NexusFlow
**Version:** 2 | **Date:** 2026-04-07
**Artifact Weight:** Blueprint
**Profile:** Critical
**Cycle:** 3 (additions — appended to Cycle 1 manifest)

---

## Revision Summary

Version 2 adds Cycle 3 scaffolding on top of the Cycle 1 scaffold (Version 1).
All Cycle 1 entries remain valid and are not repeated here. This document covers
only the new structure added for Cycle 3.

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
