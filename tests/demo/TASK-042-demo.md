# Demo Script — TASK-042
**Feature:** Demo connectors — walking skeleton end-to-end pipeline execution
**Requirement(s):** Nexus walking skeleton directive (Cycle 1 scope)
**Environment:** Staging API — `https://nexusflow.staging.nxlabs.cc/api` (substitute actual staging URL)

---

## Scenario 1: Create a demo pipeline with all three connector types configured via JSON
**AC:** AC-5 — All connectors configurable via pipeline definition JSON

**Given:** You are logged in as admin (or any authenticated user). Obtain a session token from `POST /api/auth/login` with `{"username":"admin","password":"admin"}`. Note the `token` value from the response.

**When:** Send `POST /api/pipelines` with `Authorization: Bearer <token>` and the following JSON body:
```json
{
  "name": "demo-walking-skeleton",
  "dataSourceConfig": {
    "connectorType": "demo",
    "config": { "count": 5 },
    "outputSchema": ["id", "name", "value"]
  },
  "processConfig": {
    "connectorType": "demo",
    "config": { "uppercase_field": "name" },
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

**Then:** The response is `201 Created`. The body contains an `id` (UUID) and `name: "demo-walking-skeleton"`. All three `connectorType` values are `"demo"`. The `dataSourceConfig.config.count` is `5` and `processConfig.config.uppercase_field` is `"name"`. Record the returned `id` as `<pipeline-id>` for use in Scenario 2.

**Notes:** Submitting without an `Authorization` header returns `401 Unauthorized`. Submitting with a missing `name` field returns `400 Bad Request`.

---

## Scenario 2: Submit a task and observe the walking skeleton execute end-to-end
**AC:** AC-4 — End-to-end: create pipeline → submit task → worker processes → task completes

**Given:** The pipeline created in Scenario 1 exists (you have `<pipeline-id>`). The worker service is running with the `demo` tag in its `WORKER_TAGS` configuration (confirm via `GET /api/workers` — look for a worker with `"tags": ["demo"]`).

**When:** Send `POST /api/tasks` with `Authorization: Bearer <token>` and the following JSON body:
```json
{
  "pipelineId": "<pipeline-id>",
  "input": {},
  "tags": ["demo"],
  "retryConfig": {
    "maxRetries": 3,
    "backoff": "exponential"
  }
}
```

**Then:** The response is `201 Created`. The body contains `taskId` (UUID) and `status: "queued"`. Record the `taskId` as `<task-id>`.

Within approximately 5 seconds, the worker processes the task. Observable evidence:

1. The worker container stdout log shows:
   ```
   demo-sink: committed 5 record(s) for executionID="<task-id>:0"
   ```
2. The task's state transitions in the database log: `submitted → queued → assigned → running → completed`

Note: `GET /api/tasks/<task-id>` is not yet implemented (TASK-008, Cycle 2). To verify task status during the demo, query the database directly or observe the worker log.

---

## Scenario 3: Verify DemoDataSource produces deterministic data
**AC:** AC-1 — Demo DataSource produces deterministic sample data

**Given:** The pipeline from Scenario 1 exists. You have already submitted one task that completed (Scenario 2).

**When:** Submit a second task to the same pipeline using the same `POST /api/tasks` payload as Scenario 2 (using the same `<pipeline-id>` and `tags: ["demo"]`). Record the new `taskId` as `<task-id-2>`.

**Then:** The worker processes the second task and the stdout log shows another commit line:
```
demo-sink: committed 5 record(s) for executionID="<task-id-2>:0"
```
Both tasks committed exactly 5 records. This confirms that the same pipeline configuration always produces identical output — the DemoDataSource is deterministic.

---

## Scenario 4: Verify DemoProcessConnector transforms data
**AC:** AC-2 — Demo Process transforms data

**Given:** A task has completed as shown in Scenario 2. The pipeline specifies `uppercase_field: "name"`.

**When:** Inspect the worker container logs for the completed task execution. The DemoDataSource produces records with `name` values `"record-0"`, `"record-1"`, ..., `"record-4"`.

**Then:** The DemoProcessConnector uppercases the `name` field so the records flowing to the sink have `name` values `"RECORD-0"`, `"RECORD-1"`, ..., `"RECORD-4"`. Each record also has a `"processed": true` field added by the process phase. The committed record count (5) confirms all records passed through the process phase without error.

**Notes:** The process phase is a non-mutating transform — the DataSource's original records are not modified; new record maps are produced for each output record.

---

## Scenario 5: Verify DemoSinkConnector records output and enforces idempotency
**AC:** AC-3 — Demo Sink records/stores output

**Given:** Two tasks have been processed (Scenarios 2 and 3). Both completed successfully.

**When:** Examine the worker container logs for both `demo-sink: committed` lines. Note that each line references a distinct `executionID` (one per task).

**Then:** Each task's records are committed under a unique `executionID` of the form `"<task-id>:0"`. The DemoSinkConnector's in-memory store holds the records for each completed execution separately. If the same `executionID` were submitted twice (e.g., a worker crash and redelivery), the second `Write` call returns `ErrAlreadyApplied` — the records are not duplicated. This satisfies the idempotency requirement (ADR-003).
