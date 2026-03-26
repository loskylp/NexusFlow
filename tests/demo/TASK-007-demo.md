---
task: TASK-007
feature: Tag-based task assignment and pipeline execution
requirements: REQ-005, REQ-006, REQ-007, REQ-009
environment: Staging — Docker Compose stack with PostgreSQL, Redis, Worker, API
prerequisites:
  - Docker Compose stack is up (api, worker, postgres, redis)
  - Worker is configured with capability tags ["etl"]
  - TASK-042 (demo connectors) is deployed — required for Scenarios 2–5
  - TASK-013 (pipeline CRUD API) is deployed — required for pipeline creation
  - A valid user session token is available (obtain via POST /api/auth/login)
---

# Demo Script — TASK-007
**Feature:** Tag-based task assignment and pipeline execution
**Requirement(s):** REQ-005, REQ-006, REQ-007, REQ-009
**Environment:** Staging — Docker Compose (`docker compose up -d`). All commands use `curl` against the API container or `docker exec` for direct Redis/PostgreSQL inspection.

---

## Scenario 1: Worker registers and listens on its tag-specific stream
**REQ:** REQ-005

**Given:** The Docker Compose stack is running. The worker service is configured with `WORKER_TAGS=etl`.

**When:** The worker container starts (or has been running for at least 5 seconds).

**Then:** Verify the following two conditions:

1. The worker appears in the `workers` table with `status = 'online'` and `tags = '{etl}'`:
   ```
   docker exec nexusflow-postgres-1 psql -U nexusflow -d nexusflow \
     -c "SELECT id, tags, status FROM workers WHERE 'etl' = ANY(tags);"
   ```
   Expected: at least one row with `status = online`.

2. The `queue:etl` Redis stream exists with a consumer group named `workers`:
   ```
   docker exec nexusflow-redis-1 redis-cli XINFO GROUPS queue:etl
   ```
   Expected: output shows `name: workers` and `consumers: 1` (or more if multiple etl workers are running).

**Notes:** A worker with `WORKER_TAGS=report` would appear under `queue:report`, not `queue:etl`. The stream name follows the convention `queue:{tag}` as specified in ADR-001.

---

## Scenario 2: Task executes through all three pipeline phases and completes
**REQ:** REQ-006

**Given:** A pipeline is defined with a demo DataSource, a pass-through demo Process, and a demo Sink. The worker is running with tags `["etl"]`. (Requires TASK-042 demo connectors and TASK-013 pipeline API.)

**When:**

Step 1 — Create a pipeline:
```
curl -s -X POST http://localhost:8080/api/pipelines \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "demo-etl-pipeline",
    "dataSourceConfig": {
      "connectorType": "demo",
      "config": {},
      "outputSchema": ["id", "name"]
    },
    "processConfig": {
      "connectorType": "demo",
      "config": {},
      "inputMappings": [],
      "outputSchema": ["id", "name"]
    },
    "sinkConfig": {
      "connectorType": "demo",
      "config": {},
      "inputMappings": []
    }
  }'
```
Record the `id` field from the response as `$PIPELINE_ID`.

Step 2 — Submit a task for this pipeline:
```
curl -s -X POST http://localhost:8080/api/tasks \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"pipelineId\": \"$PIPELINE_ID\", \"requiredTags\": [\"etl\"]}"
```
Record the `id` field from the response as `$TASK_ID`.

**Then:** Within 5 seconds, verify in PostgreSQL that the task reached "completed":
```
docker exec nexusflow-postgres-1 psql -U nexusflow -d nexusflow \
  -c "SELECT id, status FROM tasks WHERE id = '$TASK_ID';"
```
Expected: `status = completed`.

**Notes:** The demo DataSource produces a fixed set of sample records. The demo Process passes them through unchanged. The demo Sink stores them in memory. All three phases must run to completion for the task to reach "completed".

---

## Scenario 3: State transitions are recorded with timestamps in task_state_log
**REQ:** REQ-009

**Given:** The task from Scenario 2 completed successfully (`$TASK_ID`).

**When:** Query the state log for that task:
```
docker exec nexusflow-postgres-1 psql -U nexusflow -d nexusflow \
  -c "SELECT from_state, to_state, reason, timestamp \
      FROM task_state_log WHERE task_id = '$TASK_ID' ORDER BY timestamp ASC;"
```

**Then:** The output shows at least three rows:

| from_state | to_state  | reason                        |
|------------|-----------|-------------------------------|
| queued     | assigned  | assigned to worker ...        |
| assigned   | running   | pipeline execution started    |
| running    | completed | pipeline completed successfully|

Each row has a non-null `timestamp`. The transitions appear in chronological order.

**Notes:** The `task_state_log` table is append-only. A database trigger (defined in migration 000001) enforces valid `(from_state, to_state)` pairs — invalid transitions are rejected at the database level.

---

## Scenario 4: Schema mapping renames fields between DataSource and Process phases
**REQ:** REQ-007

**Given:** A pipeline where the DataSource outputs `{"customer_id": "123", "amount": 50.0}` and the Process `inputMappings` renames `customer_id` to `id`. (Requires TASK-042 demo connectors.)

**When:**

Create the pipeline with schema mappings:
```
curl -s -X POST http://localhost:8080/api/pipelines \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "schema-mapping-demo",
    "dataSourceConfig": {
      "connectorType": "demo",
      "config": {"fields": ["customer_id", "amount"]},
      "outputSchema": ["customer_id", "amount"]
    },
    "processConfig": {
      "connectorType": "demo",
      "config": {},
      "inputMappings": [
        {"sourceField": "customer_id", "targetField": "id"},
        {"sourceField": "amount", "targetField": "total"}
      ],
      "outputSchema": ["id", "total"]
    },
    "sinkConfig": {
      "connectorType": "demo",
      "config": {},
      "inputMappings": []
    }
  }'
```

Submit a task for this pipeline and wait for it to complete (as in Scenario 2).

**Then:** The task status reaches `completed` — confirming the schema mapping did not produce an error. If the mapping were referencing a field that does not exist, the task would be marked `failed` with a message identifying the missing source field.

**Notes:** The mapping `{"sourceField": "customer_id", "targetField": "id"}` causes the Process connector to receive `{"id": "123", "total": 50.0}` instead of `{"customer_id": "123", "amount": 50.0}`. The original field names are no longer present in the Process input.

---

## Scenario 5: Failed connector sets task status to "failed"
**REQ:** REQ-006

**Given:** A task is submitted with a pipeline that references a connector type that is not registered (`"connectorType": "nonexistent"`). The worker is running. (Requires TASK-013 pipeline API.)

**When:**

Create a pipeline with an unregistered connector:
```
curl -s -X POST http://localhost:8080/api/pipelines \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "failing-connector-demo",
    "dataSourceConfig": {"connectorType": "nonexistent", "config": {}, "outputSchema": []},
    "processConfig": {"connectorType": "demo", "config": {}, "inputMappings": [], "outputSchema": []},
    "sinkConfig": {"connectorType": "demo", "config": {}, "inputMappings": []}
  }'
```

Submit a task for this pipeline. Wait 3 seconds, then check the task status:
```
docker exec nexusflow-postgres-1 psql -U nexusflow -d nexusflow \
  -c "SELECT id, status FROM tasks WHERE id = '$TASK_ID';"
```

**Then:**
- `status = failed`
- The message is XACKed from the Redis stream pending list (it does not appear in the pending list):
  ```
  docker exec nexusflow-redis-1 redis-cli XPENDING queue:etl workers - + 10
  ```
  Expected: the message ID for this task is absent (XACK was called after the domain failure).

**Notes:** The connector registry returns `ErrUnknownConnector` for unregistered types. This is classified as a domain error (not an infrastructure failure), so the worker XACKs the message to remove it from the pending list without triggering Monitor XCLAIM retry. This follows ADR-003 Domain Invariant 2.

---
