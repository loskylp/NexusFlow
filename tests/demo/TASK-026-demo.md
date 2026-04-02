# Demo Script — TASK-026
**Feature:** Schema mapping validation at design time
**Requirement(s):** REQ-007, ADR-008
**Environment:** Staging API — `https://<staging-host>/api` (substitute actual staging URL)

---

## Scenario 1: Valid schema mappings are accepted on pipeline creation
**REQ:** REQ-007

**Given:** You are logged in as an authenticated user with a session token from `POST /api/auth/login`. The pipeline definition declares a `dataSourceConfig.outputSchema`, a `processConfig` whose `inputMappings` reference only fields that appear in that schema, a `processConfig.outputSchema`, and a `sinkConfig` whose `inputMappings` reference only fields from the process output schema.

**When:** Send `POST /api/pipelines` with `Authorization: Bearer <token>` and the following body:
```json
{
  "name": "demo-026-valid",
  "dataSourceConfig": {
    "connectorType": "demo",
    "config": {},
    "outputSchema": ["userId", "amount"]
  },
  "processConfig": {
    "connectorType": "demo",
    "config": {},
    "inputMappings": [
      {"sourceField": "userId", "targetField": "uid"},
      {"sourceField": "amount", "targetField": "value"}
    ],
    "outputSchema": ["uid", "value"]
  },
  "sinkConfig": {
    "connectorType": "demo",
    "config": {},
    "inputMappings": [
      {"sourceField": "uid",   "targetField": "dest_uid"},
      {"sourceField": "value", "targetField": "dest_value"}
    ]
  }
}
```

**Then:** The response is `201 Created`. The body contains the new pipeline object including an `id` (UUID), `name`, `userId`, and all three phase configs with their schema mappings. Record the `id` for use in Scenarios 3 and 4.

**Notes:** This confirms AC-2 — valid schema mappings are not rejected.

---

## Scenario 2: Invalid DataSource→Process mapping is rejected on pipeline creation
**REQ:** REQ-007

**Given:** You are logged in as an authenticated user. You have a pipeline definition where `processConfig.inputMappings` contains a `sourceField` (`"nonexistent_field"`) that does not appear in `dataSourceConfig.outputSchema` (`["userId", "amount"]`).

**When:** Send `POST /api/pipelines` with the following body:
```json
{
  "name": "demo-026-bad-ds-proc",
  "dataSourceConfig": {
    "connectorType": "demo",
    "config": {},
    "outputSchema": ["userId", "amount"]
  },
  "processConfig": {
    "connectorType": "demo",
    "config": {},
    "inputMappings": [
      {"sourceField": "userId",          "targetField": "uid"},
      {"sourceField": "nonexistent_field", "targetField": "x"}
    ],
    "outputSchema": ["uid"]
  },
  "sinkConfig": {
    "connectorType": "demo",
    "config": {},
    "inputMappings": []
  }
}
```

**Then:** The response is `400 Bad Request`. The `error` field in the body reads:
```
process input mapping: source field "nonexistent_field" not found in datasource output schema
```
The string `nonexistent_field` appears in the error, identifying exactly which field failed. The string `process input mapping` appears, identifying which pipeline transition (DataSource→Process) was checked. No pipeline is created — a subsequent `GET /api/pipelines` does not include a pipeline named `demo-026-bad-ds-proc`.

**Notes:** This confirms AC-1 (400 with identified field), AC-3 (DS→Process transition validated), and AC-4 (specific field and mapping named in error).

---

## Scenario 3: Invalid Process→Sink mapping is rejected on pipeline creation
**REQ:** REQ-007

**Given:** You are logged in as an authenticated user. The pipeline definition has valid DS→Process mappings but `sinkConfig.inputMappings` contains a `sourceField` (`"ghost_field"`) that does not appear in `processConfig.outputSchema` (`["processed"]`).

**When:** Send `POST /api/pipelines` with the following body:
```json
{
  "name": "demo-026-bad-proc-sink",
  "dataSourceConfig": {
    "connectorType": "demo",
    "config": {},
    "outputSchema": ["raw"]
  },
  "processConfig": {
    "connectorType": "demo",
    "config": {},
    "inputMappings": [
      {"sourceField": "raw", "targetField": "processed"}
    ],
    "outputSchema": ["processed"]
  },
  "sinkConfig": {
    "connectorType": "demo",
    "config": {},
    "inputMappings": [
      {"sourceField": "processed",  "targetField": "dest"},
      {"sourceField": "ghost_field", "targetField": "x"}
    ]
  }
}
```

**Then:** The response is `400 Bad Request`. The `error` field reads:
```
sink input mapping: source field "ghost_field" not found in process output schema
```
The string `ghost_field` appears in the error. The string `sink input mapping` appears, identifying the Process→Sink transition. No pipeline is created.

**Notes:** This confirms AC-3 (Process→Sink transition is independently validated, not just the DS→Process transition) and AC-4 (field and mapping context both present in the error).

---

## Scenario 4: Invalid mapping is rejected on pipeline update
**REQ:** REQ-007

**Given:** The pipeline created in Scenario 1 exists. You have its `id`. You prepare an update body where `processConfig.inputMappings` references `"missing_on_update"`, which is not in `dataSourceConfig.outputSchema` (`["userId"]`).

**When:** Send `PUT /api/pipelines/<id>` with the following body:
```json
{
  "name": "demo-026-invalid-update",
  "dataSourceConfig": {
    "connectorType": "demo",
    "config": {},
    "outputSchema": ["userId"]
  },
  "processConfig": {
    "connectorType": "demo",
    "config": {},
    "inputMappings": [
      {"sourceField": "missing_on_update", "targetField": "x"}
    ],
    "outputSchema": ["x"]
  },
  "sinkConfig": {
    "connectorType": "demo",
    "config": {},
    "inputMappings": []
  }
}
```

**Then:** The response is `400 Bad Request`. The `error` field contains `missing_on_update`. A subsequent `GET /api/pipelines/<id>` returns the pipeline with its previous valid name (`demo-026-valid`) — the update was not applied.

**Notes:** This confirms AC-1 applies to `PUT` as well as `POST` — validation runs on every save operation, not only on creation.

---

## Scenario 5: Empty mappings are always accepted (no false positives)
**REQ:** REQ-007

**Given:** You are logged in as an authenticated user. The pipeline definition has no `inputMappings` in either `processConfig` or `sinkConfig` (both arrays are empty).

**When:** Send `POST /api/pipelines` with the following body:
```json
{
  "name": "demo-026-empty-mappings",
  "dataSourceConfig": {
    "connectorType": "demo",
    "config": {},
    "outputSchema": ["field1"]
  },
  "processConfig": {
    "connectorType": "demo",
    "config": {},
    "inputMappings": [],
    "outputSchema": ["out1"]
  },
  "sinkConfig": {
    "connectorType": "demo",
    "config": {},
    "inputMappings": []
  }
}
```

**Then:** The response is `201 Created`. An empty mapping list is valid — there are no fields to check, so validation passes unconditionally.

**Notes:** This confirms AC-2 (valid mappings pass) also covers the empty-mapping case. Validators that reject empty mappings would produce false positives here.
