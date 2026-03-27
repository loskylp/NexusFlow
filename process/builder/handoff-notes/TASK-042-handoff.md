# Builder Handoff — TASK-042
**Date:** 2026-03-26
**Task:** Demo connectors — demo source, simulated worker, demo sink
**Requirement(s):** Nexus walking skeleton directive (Cycle 1 scope)

---

## What Was Implemented

### Modified files

**`worker/connectors.go`** — replaced stub implementations, added imports, added `RegisterDemoConnectors`

- `DemoDataSource.Fetch`: Produces deterministic sample records. Count controlled by `config["count"]` (float64); defaults to 5. Each record has fields `id` (int), `name` (string `"record-{i}"`), `value` (int `i*10`). Same config always yields identical output (deterministic). No external dependencies.

- `DemoProcessConnector.Transform`: Copies all input fields into new record maps (non-mutating). Adds `"processed": true` to every output record. When `config["uppercase_field"]` is set, uppercases the named field's string value. Empty input slice returns empty output without error.

- `DemoSinkConnector` (struct): Replaced scaffold stub. Now has `mu sync.Mutex` and `store map[string]logEntry` (replaces the unusable `//lint:ignore U1000` stub field). The `//lint:ignore U1000` directive has been removed as the field is now used.

- `DemoSinkConnector.Snapshot`: Returns `{"record_count": N, "execution_count": M}` where N is the total records across all committed executionIDs and M is the number of distinct executionIDs. Never returns an error.

- `DemoSinkConnector.Write`: Checks for duplicate `executionID` (idempotency guard, ADR-003). Deep-copies records before storing so the in-memory store is not affected by later mutations. Logs each committed write to stdout via `log.Printf`.

- `NewDemoSinkConnector()`: New constructor. Required because `DemoSinkConnector` now holds a `store` map that must be initialised. The external test package cannot use a struct literal directly since the `store` field is unexported.

- `RegisterDemoConnectors(reg *DefaultConnectorRegistry)`: Registers all three demo connectors (type `"demo"`) in the given registry. Called at worker startup.

- `logEntry` (unexported): Internal record for each committed write; replaces the exported `LogEntry` scaffold type which is kept for the Sink Inspector (TASK-033).

- Added imports: `fmt`, `log`, `strings`, `sync`, `time`.

**`worker/worker.go`** — OBS-022 fix

- `applyMappingsToSlice` docstring: corrected the misleading phrase "Empty when mappings is empty: each record becomes {}". The actual behaviour is pass-through (the original slice is returned unchanged). New docstring accurately states: "When mappings is empty, the original slice is returned unchanged so that all fields from the preceding phase are passed through to the next phase without any renaming."

**`worker/helpers_test.go`** — new file (compiled only during testing)

- `Worker.ExecuteTaskForTest`: Thin wrapper around the unexported `executeTask` method. Lives in `package worker` (not `package worker_test`) so it accesses the unexported method, but is compiled only during testing (the `_test.go` suffix). Follows the standard Go "export_test.go" pattern.

**`worker/demo_connectors_test.go`** — new file

19 new unit tests (see Unit Tests section below).

**`cmd/worker/main.go`** — wired demo connectors

- `RegisterDemoConnectors(connectorRegistry)` called immediately after `NewDefaultConnectorRegistry()`.
- Comment updated to reflect TASK-042 complete.

---

## Unit Tests

- Tests written: 19 new
- All passing: yes
- Pre-existing tests: 22 (all still pass)
- Total worker package tests: 41

### Key behaviours covered

| Test | Behaviour |
|---|---|
| `TestDemoDataSource_Type_ReturnsDemoString` | Type() = "demo" |
| `TestDemoDataSource_Fetch_ReturnsDefaultRecords` | Non-empty slice on default config |
| `TestDemoDataSource_Fetch_IsDeterministic` | Same config yields identical records on two calls (AC-1) |
| `TestDemoDataSource_Fetch_CountConfig` | `count` config key controls record count |
| `TestDemoDataSource_Fetch_RecordsHaveRequiredFields` | Each record has "id", "name", "value" fields |
| `TestDemoDataSource_Fetch_NameFieldsContainRecordIndex` | Name field encodes record index for test observability |
| `TestDemoProcessConnector_Type_ReturnsDemoString` | Type() = "demo" |
| `TestDemoProcessConnector_Transform_PassesThroughUnknownFields` | Fields without a config directive are preserved |
| `TestDemoProcessConnector_Transform_UppercasesConfiguredField` | `uppercase_field` config key triggers uppercase (AC-2) |
| `TestDemoProcessConnector_Transform_AddsProcessedFlag` | `processed: true` added to every output record |
| `TestDemoProcessConnector_Transform_EmptyInput` | Empty input yields empty output without error |
| `TestDemoProcessConnector_Transform_DoesNotMutateInput` | Input records not mutated |
| `TestDemoSinkConnector_Type_ReturnsDemoString` | Type() = "demo" |
| `TestDemoSinkConnector_Write_StoresRecords` | Write returns nil on success (AC-3) |
| `TestDemoSinkConnector_Write_IdempotentOnDuplicateExecutionID` | Second Write with same executionID returns ErrAlreadyApplied (ADR-003) |
| `TestDemoSinkConnector_Snapshot_ReturnsEmptyBeforeFirstWrite` | Snapshot returns record_count=0 before any write |
| `TestDemoSinkConnector_Snapshot_ReflectsCommittedWrites` | record_count reflects all committed records |
| `TestDemoConnectors_RegisteredInDefaultRegistry` | All three connectors resolved by type "demo" after RegisterDemoConnectors |
| `TestDemoConnectors_EndToEnd_TaskCompletes` | Full pipeline: DataSource -> Process -> Sink -> task status = "completed" (AC-4) |

---

## Deviations from Task Description

### 1. `NewDemoSinkConnector()` constructor (not in scaffold)

The scaffold declared `DemoSinkConnector` as a zero-value-usable struct. The implementation requires an initialised `store` map, so a constructor is necessary. Using an uninitialised map would panic on first Write. The constructor is the standard Go pattern for this.

**Impact:** Any code constructing `&DemoSinkConnector{}` directly will compile but will panic on first Write. The only production call site (`RegisterDemoConnectors`) uses `NewDemoSinkConnector()`. The external test package uses the same constructor via the exported `NewDemoSinkConnector`.

### 2. `RegisterDemoConnectors` accepts `*DefaultConnectorRegistry` not `ConnectorRegistry`

The `Register` method on the `ConnectorRegistry` interface accepts `any` and does a runtime type assertion. This is correct but accepting the concrete type in `RegisterDemoConnectors` makes the function signature self-documenting (callers know they need the concrete registry, not any interface value). This also avoids the need to type-assert inside the function.

**Impact:** None. `cmd/worker/main.go` holds a `*DefaultConnectorRegistry` — the only caller.

### 3. OBS-022 docstring fix applied

The misleading `applyMappingsToSlice` docstring was corrected as part of this task (living documentation requirement). This is a documentation-only change; the behaviour was already correct.

---

## Known Limitations

- `DemoSinkConnector.store` is not bounded in size. In a long-running worker processing many tasks, the store grows unboundedly. This is acceptable for the demo/walking skeleton context; production sinks would write to durable storage.
- `DemoSinkConnector.Snapshot` is not scoped by `taskID`. The snapshot returns the global state of the store, not the state relevant to one task. The `taskID` parameter is accepted for interface compliance but not used to filter the snapshot. This is consistent with the demo context (no partitioned storage).
- The `DemoDataSource` does not use the `input` parameter. Task input parameters are accepted for interface compliance but do not influence the generated records. A real DataSource would use `input` to parameterise the query.

---

## For the Verifier

### Unit tests

```bash
go test ./worker/...
```

Expected: 41 tests pass (22 pre-existing + 19 new).

### Full suite

```bash
go build ./...
go vet ./...
staticcheck ./...   # v0.5.1
go test ./...
```

### Acceptance criteria mapping

| AC | Verification |
|---|---|
| AC-1: Demo DataSource produces deterministic sample data | `TestDemoDataSource_Fetch_IsDeterministic` |
| AC-2: Demo Process transforms data (uppercase field) | `TestDemoProcessConnector_Transform_UppercasesConfiguredField` |
| AC-3: Demo Sink records/stores output | `TestDemoSinkConnector_Write_StoresRecords`, `TestDemoSinkConnector_Snapshot_ReflectsCommittedWrites` |
| AC-4: End-to-end: task completes through all three phases | `TestDemoConnectors_EndToEnd_TaskCompletes` |
| AC-5: All connectors configurable via pipeline definition JSON | `TestDemoDataSource_Fetch_CountConfig` (count), `TestDemoProcessConnector_Transform_UppercasesConfiguredField` (uppercase_field) |

### Sample pipeline JSON (POST /api/pipelines)

Requires a valid JWT in the `Authorization: Bearer <token>` header (obtained via POST /api/login).

```json
{
  "name": "demo-walking-skeleton",
  "dataSourceConfig": {
    "connectorType": "demo",
    "config": {
      "count": 5
    },
    "outputSchema": ["id", "name", "value"]
  },
  "processConfig": {
    "connectorType": "demo",
    "config": {
      "uppercase_field": "name"
    },
    "inputMappings": [],
    "outputSchema": ["id", "name", "value", "processed"]
  },
  "sinkConfig": {
    "connectorType": "demo",
    "config": {},
    "inputMappings": []
  }
}
```

The response body includes the pipeline `id`. Use it in the task submission below.

### Sample task submission payload (POST /api/tasks)

```json
{
  "pipelineId": "<pipeline-id-from-above>",
  "input": {},
  "retryConfig": {
    "maxRetries": 3,
    "backoff": "exponential"
  }
}
```

Expected observable outcome:

1. Task status transitions: `queued` → `assigned` → `running` → `completed`
2. Worker stdout shows: `demo-sink: committed 5 record(s) for executionID="<taskID>:1"`
3. `GET /api/tasks/<task-id>` returns `"status": "completed"`

### OBS-022 resolution

The `applyMappingsToSlice` docstring in `worker/worker.go` (lines 622–636) now correctly states that empty mappings result in pass-through, not empty records. The static text "Empty when mappings is empty: each record becomes {}" has been removed. This closes OBS-022.
