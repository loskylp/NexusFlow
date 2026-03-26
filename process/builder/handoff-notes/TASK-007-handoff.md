# Builder Handoff — TASK-007
**Date:** 2026-03-26
**Task:** Tag-based task assignment and pipeline execution
**Requirement(s):** REQ-005, REQ-006, REQ-007, REQ-009

---

## What Was Implemented

### New files

**`worker/events.go`**
Defines `TaskEventBroker`, a narrow interface (Interface Segregation principle) containing only the single method the Worker calls: `PublishTaskEvent`. The full `sse.Broker` interface (implemented in TASK-015) is a superset of `TaskEventBroker`, so no adapter is required in production. The narrow interface avoids importing `net/http` in the worker package and makes the Worker straightforwardly testable without a real SSE broker.

**`worker/executor_test.go`**
Unit tests covering all nine acceptance criteria. Tests use in-memory fakes for all external dependencies (no live Redis or PostgreSQL). Written Red-first; all tests failed before the implementation was written. 14 new test cases added.

### Modified files

**`worker/worker.go`** — replaced stub implementations

- `runConsumptionLoop`: Blocking `ReadTasks` loop. Iterates until `ctx` is cancelled. Calls `executeTask` for each message. Pauses 500 ms on `ReadTasks` error before retrying to avoid spin-loop on persistent infrastructure failures.

- `executeTask`: Full three-phase pipeline execution.
  - Transition 1: queued → assigned (before loading the pipeline)
  - Calls `runPipeline` which performs all three phases
  - Transition: running → completed on success, running → failed on error
  - XACK on success or domain error; no XACK on infrastructure error (leaves message pending for Monitor XCLAIM)
  - Publishes SSE event via `broker.PublishTaskEvent` after each state transition

- `runPipeline`: Loads pipeline, runs DataSource → (schema mapping) → Process → (schema mapping) → Sink. Returns `*domainErrorWrapper` for connector/schema/pipeline-not-found failures; returns raw error for infrastructure failures (DB, Redis). The distinction drives the XACK/no-XACK decision in `executeTask`.

- `ApplySchemaMapping` (exported, was `applySchemaMapping`): Renames fields per `[]models.SchemaMapping`. Returns error on missing source field. Does not mutate the input map.

- `applyMappingsToSlice`: Applies `ApplySchemaMapping` to every record in a slice. Pass-through (returns slice unchanged) when `mappings` is empty, preserving all fields for phases with no mapping defined.

- `NewWorkerWithPipelines`: New constructor that accepts `db.PipelineRepository` in addition to all existing dependencies. The original `NewWorker` is unchanged for backward compatibility.

- `domainErrorWrapper`: Internal type wrapping domain errors (connector failure, schema error, missing pipeline). Used by `isDomainError` to decide XACK policy.

**`worker/connectors.go`** — added `DefaultConnectorRegistry`

Concrete implementation of `ConnectorRegistry`. Uses three maps keyed by connector type string. Panics on duplicate registration (fail-fast: duplicate connectors are startup misconfiguration). Returns `ErrUnknownConnector` (sentinel) for unregistered types. The `//lint:ignore U1000` directive on `DemoSinkConnector.store` was left in place as that field is still a scaffold stub for TASK-042.

**`cmd/worker/main.go`** — wired TASK-007 dependencies

- `db.NewPgTaskRepository(pool)` — task repository (was nil)
- `db.NewPgPipelineRepository(pool)` — pipeline repository (new)
- `workerPkg.NewDefaultConnectorRegistry()` — empty registry (demo connectors wired in TASK-042)
- Changed from `NewWorker` to `NewWorkerWithPipelines`
- broker remains nil until TASK-015

---

## Unit Tests

| Test | Covers |
|---|---|
| `TestApplySchemaMapping_RenamesFields` | AC-7: field renaming |
| `TestApplySchemaMapping_ErrorOnMissingSourceField` | AC-7: error on missing source field |
| `TestApplySchemaMapping_EmptyMappings` | Empty mapping pass-through |
| `TestApplySchemaMapping_DoesNotMutateInput` | Input immutability |
| `TestDefaultConnectorRegistry_RegisterAndResolve` | Registry resolution |
| `TestDefaultConnectorRegistry_UnknownType` | ErrUnknownConnector sentinel |
| `TestExecuteTask_SuccessfulPipeline_CompletesTask` | AC-1, AC-2, AC-3, AC-4, AC-5, AC-6 |
| `TestExecuteTask_ProcessError_SetsFailedStatus` | AC-8, ADR-003 Domain Invariant 2 |
| `TestExecuteTask_DataSourceError_SetsFailedStatus` | AC-8 (DataSource phase) |
| `TestExecuteTask_SchemaMapping_Applied` | AC-7 (end-to-end schema mapping through phases) |
| `TestExecuteTask_IdempotentSink_ErrAlreadyApplied_CompletesSuccessfully` | ADR-003 idempotent redelivery |
| `TestExecuteTask_MissingPipeline_SetsFailedStatus` | AC-8 (pipeline deleted after submission) |
| `TestExecuteTask_SSEEventsEmitted` | AC-9 |
| `TestRunConsumptionLoop_TagFiltering` | AC-1 (tag-based stream selection) |

All 14 new tests pass. All 8 pre-existing worker tests pass. Full suite: `go test ./...` green.

Build: `go build ./...` clean.
Vet: `go vet ./...` clean.
Staticcheck: `staticcheck ./...` clean (v0.5.1).

---

## Deviations from Specification

### 1. `TaskEventBroker` interface (ISP deviation from scaffold)

The scaffold specified `broker sse.Broker` in the Worker struct. This has been changed to `broker TaskEventBroker` (defined in `worker/events.go`).

**Reason:** `sse.Broker` imports `net/http` (for `http.ResponseWriter` and `*http.Request` parameters on the Serve* methods). Keeping `sse.Broker` as the broker type in the Worker package imports `net/http` into every file that imports `worker`, and makes unit testing impractical without a full HTTP context. Interface Segregation: the Worker calls exactly one broker method (`PublishTaskEvent`); it should not depend on the full interface.

**Impact on callers:** `cmd/worker/main.go` passes `nil` for broker (unchanged). When TASK-015 wires a real `RedisBroker`, the value will satisfy `TaskEventBroker` automatically since `sse.Broker` is a superset.

### 2. `ApplySchemaMapping` exported (was `applySchemaMapping`)

The scaffold declared the method unexported. Tests in `package worker_test` (external test package) cannot call unexported methods.

**Reason:** The test suite is in `package worker_test` following the existing convention established in `worker_test.go`. Exporting `ApplySchemaMapping` is consistent with it being a well-defined, documented operation with a clear contract — not an internal implementation detail.

### 3. `NewWorkerWithPipelines` added alongside `NewWorker`

**Reason:** `NewWorker` is called by eight existing unit tests that do not need a pipeline repository. Adding a separate constructor preserves those tests without modification. TASK-006 and existing tests remain unaffected.

### 4. XACK multi-tag loop

`ackMessage` tries XACK against each of the worker's tags in order and returns on the first success. This is necessary because `queue.TaskMessage` does not carry the name of the stream it was read from (by design in the TASK-004 scaffold). The XACK call is idempotent and a no-op if the message ID does not belong to that stream.

**Limitation:** In the highly unlikely case where a worker has two tags with stream messages sharing the same ID (Redis stream IDs are monotonic per-stream and never collide across streams), the wrong stream could be ACKed first. This is theoretically impossible in practice but worth noting. The clean fix is to add a `StreamTag` field to `TaskMessage` (TASK-004 scope change — deferred).

---

## Verifier Instructions

### Unit tests (all in `worker/`)

```bash
go test ./worker/...
```

All 22 tests should pass (8 pre-existing + 14 new).

### Full suite

```bash
go build ./...
go vet ./...
staticcheck ./...  # v0.5.1
go test ./...
```

### Acceptance criteria mapping

| AC | Test / Verification |
|---|---|
| AC-1: Worker with tag "etl" picks up from queue:etl | `TestRunConsumptionLoop_TagFiltering` |
| AC-2: State transitions queued → assigned → running → completed | `TestExecuteTask_SuccessfulPipeline_CompletesTask` (checks ≥3 transitions) |
| AC-3: Each transition logged in task_state_log | `TestExecuteTask_SuccessfulPipeline_CompletesTask` (fakeTaskRepo.statusLog) |
| AC-4: DataSource phase extracts data | `TestExecuteTask_SuccessfulPipeline_CompletesTask` (sink receives records) |
| AC-5: Process phase transforms data | `TestExecuteTask_SuccessfulPipeline_CompletesTask` |
| AC-6: Sink phase writes data | `TestExecuteTask_SuccessfulPipeline_CompletesTask` (sink.recordCount > 0) |
| AC-7: Schema mapping renames fields | `TestExecuteTask_SchemaMapping_Applied`, `TestApplySchemaMapping_*` |
| AC-8: Failed execution sets "failed" | `TestExecuteTask_ProcessError_SetsFailedStatus`, `TestExecuteTask_DataSourceError_SetsFailedStatus`, `TestExecuteTask_MissingPipeline_SetsFailedStatus` |
| AC-9: State change events to Redis Pub/Sub | `TestExecuteTask_SSEEventsEmitted` (≥3 events, last = "completed") |

### Note on AC-9 (Redis Pub/Sub)

AC-9 requires events emitted to `events:tasks:{userId}`. In TASK-007, the Worker calls `broker.PublishTaskEvent` which the SSE Broker (implemented in TASK-015) routes to Redis Pub/Sub. The unit tests verify the broker is called with the correct task on each transition. End-to-end Pub/Sub delivery requires TASK-015 to be implemented and is an integration-level concern.

### Demo connectors

The connector registry is wired but empty at startup. Tasks referencing connector type "demo" will be marked "failed" with "unknown DataSource connector" until TASK-042 registers the demo connectors. The walking skeleton demonstration requires TASK-042.
