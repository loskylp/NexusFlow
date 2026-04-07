# Handoff Note — TASK-035: Task Submission via GUI (Complete Flow)

**Task:** TASK-035
**Status:** Complete
**Iteration:** 1
**Date:** 2026-04-07

---

## What Was Built

### `web/src/components/SubmitTaskModal.tsx` — Full implementation

Replaced the minimal TASK-023 stub with the complete task submission form. All form sections and behaviors are implemented.

**Pipeline selector:**
- Dropdown populated from `pipelines` prop (fetched by parent via `usePipelines`)
- Pre-selects `initialPipelineId` when provided (Run button from Pipeline Builder)
- Falls back to first pipeline when no `initialPipelineId` is given
- Shows "No pipelines available" option when list is empty

**Parameter form:**
- Dynamic key-value pair rows: users click "Add Parameter" to append rows
- Each row has a Key input (`placeholder="Key"`) and a Value input (`placeholder="Value"`)
- "Remove parameter" (×) button on each row removes that row from the list
- Inline validation (`validateParams`) runs at submit time:
  - Empty key in any row → error "Parameter key cannot be empty", submission blocked
  - Duplicate keys → error "Duplicate parameter key — each key must be unique", submission blocked
  - Validation error clears when the user edits any parameter field

**Retry configuration:**
- Max Retries number input (min 0, max 10, default 0)
- Backoff Strategy selector: Fixed (default), Linear, Exponential
- `retryConfig` is omitted from the POST payload when `maxRetries === 0` (system default per REQ-001); included when `maxRetries > 0`

**Submission:**
- `handleSubmit` validates params, then calls `submitTask(payload)` from `api/client.ts`
- Payload: `{ pipelineId, input: Record<string, unknown> }` (+ `retryConfig` when non-default)
- `input` is built by `buildInputRecord(params)` — pure function converting rows to `Record<string, unknown>`
- On success: `onSuccess(taskId)` then `onClose()` called
- During submission: button shows spinner + "Submitting..." text, all inputs disabled
- On failure: API error message shown in a `role="alert"` div below the form; submit re-enabled

**State reset:**
- `useEffect` on `isOpen` resets all form state when the modal is re-opened (false → true)
- Parameters, errors, retry config all return to defaults on re-open

**Key pure functions extracted for testability:**
- `buildInitialState(initialPipelineId, pipelines) -> SubmitFormState` — constructs default form state
- `validateParams(rows) -> string | null` — validation logic, returns error message or null
- `buildInputRecord(rows) -> Record<string, unknown>` — converts row array to API payload

**ParameterRow sub-component:**
- Single-responsibility presentational component: renders one key-value row with remove button
- All state managed by parent; onChange/onRemove callbacks prop-threaded

---

## TDD Cycle

**Red:** Wrote `SubmitTaskModal.test.tsx` (44 tests) before implementation. All new tests targeting the parameter form, retry config, validation, and full submission failed against the stub.

**Green:** Implemented `SubmitTaskModal.tsx` in dependency order:
1. Pure helpers (`buildInitialState`, `validateParams`, `buildInputRecord`)
2. `ParameterRow` sub-component
3. `SubmitTaskModal` form state and event handlers

**Refactor:**
- Extracted `buildInitialState` as a named pure function to separate concern from the component
- Extracted `validateParams` as a named pure function with documented precondition/postcondition
- Extracted `buildInputRecord` for single-responsibility payload construction
- Named `LABEL_STYLE` constant for the `IBM Plex Sans` uppercase label style used throughout — avoids style duplication across 5 uses
- `ParameterRow` extracted as a sub-component (not an inline lambda) for clarity

---

## Tests Written

### `web/src/components/SubmitTaskModal.test.tsx` (44 tests)

| Group | Tests |
|---|---|
| Closed state | returns null when isOpen is false |
| Open state | dialog present, heading visible, Cancel/Submit buttons |
| Pipeline selector | options rendered, initialPipelineId pre-selection, fallback to first, empty list |
| Parameter form | section renders, Add Parameter button, row added, row removed, key/value typing |
| Retry configuration | section renders, max retries input defaults to 0, backoff selector, changing values |
| Inline validation | empty key blocks submission + shows error, duplicate key blocks + shows error, error clears on edit |
| Submission | payload without params (no retryConfig), payload with params, payload with retry config, onSuccess called, onClose called, spinner during submit, Submitting... text |
| Error handling | API error displayed, submit re-enabled after error, onSuccess not called on error |
| Cancel | onClose on Cancel click, no submitTask call, onClose on × button |
| State reset | params reset on re-open, error reset on re-open |

**All 509 tests pass (up from 473 before this task). TypeScript compiles clean.**

---

## Acceptance Criteria Traceability

| AC | Criterion | Implementation | Status |
|---|---|---|---|
| AC-1 | User can submit a task via the Task Feed "Submit Task" modal | SubmitTaskModal full form wired in TaskFeedPage.tsx (from TASK-021) | DONE |
| AC-2 | Pipeline selector shows available pipelines from GET /api/pipelines | `pipelines` prop from parent's `usePipelines()` hook; options render pipeline names | DONE |
| AC-3 | Missing required parameters show inline validation errors | `validateParams` fires on submit; empty key → inline error, submission blocked | DONE |
| AC-4 | Submitted task appears in Task Feed with status "submitted" | POST /api/tasks → `onSuccess(taskId)` → parent `refresh()` re-fetches task list | DONE (via parent; no change to TaskFeedPage.tsx needed) |
| AC-5 | Task created via GUI is identical in state and behavior to one created via API | Payload `{ pipelineId, input, retryConfig? }` matches the `submitTask` function in client.ts exactly | DONE |

---

## Deviations

1. **`retryConfig` omitted when `maxRetries === 0`.** The minimal TASK-023 implementation submitted `{ pipelineId, input: {} }` (no `retryConfig`). The Verifier added an integration test (`tests/integration/TASK-023-pipeline-manager-integration.test.tsx`) asserting exactly that payload shape. To preserve this behavior and the passing integration test, `retryConfig` is omitted from the payload when `maxRetries === 0` (the default). This is semantically correct: the API spec (REQ-001) states "unspecified values use safe system defaults". Including `{ maxRetries: 0, backoff: 'fixed' }` explicitly vs. omitting `retryConfig` produces identical backend behavior.

2. **No input schema validation against pipeline definition.** The task description says "dynamic form fields based on selected pipeline's input schema". The `Pipeline` domain type in `domain.ts` does not include an explicit task-level input parameter schema (it has `outputSchema` on `DataSourceConfig` and `ProcessConfig`, but no `inputParameters` definition). The parameter form therefore uses free-form key-value pairs without schema-driven required fields. Inline validation catches structural issues (empty keys, duplicate keys). If the backend requires specific input fields, it will return a 400 that is shown as an API error. A formal input parameter schema on the Pipeline type would be a backend domain model extension.

---

## Nil-Wiring Check

Frontend-only task. No backend changes. No new wiring in `main.go` or `server.go`.

`TaskFeedPage.tsx` already passes `pipelines` from `usePipelines()` to `SubmitTaskModal` and wires `onSuccess` to `refresh()` — this was in place from TASK-021.

`PipelineManagerPage.tsx` already passes `initialPipelineId` when opening the modal via the Run button — this was in place from TASK-023.
