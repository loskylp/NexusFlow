---
task: TASK-030
title: Demo infrastructure -- MinIO Fake-S3 connector
smoke: false
---

# Demo Script — TASK-030
**Feature:** Demo infrastructure — MinIO Fake-S3 connector (DataSource + Sink)
**Requirement(s):** DEMO-001
**Environment:** Staging — `docker compose --profile demo up` from the project root

## Scenario 1: MinIO starts healthy with demo-profile and seed data is present

**REQ:** DEMO-001 / AC-1

**Given:** You are at the project root with a valid `.env` file (copy from `.env.example`); no prior `minio-data` volume conflict.

**When:** Run `docker compose --profile demo up` and wait for all services to report healthy (watch for `minio-init` to log "MinIO seed complete").

**Then:**
- The `minio` service is listed as `(healthy)` in `docker compose ps`.
- `curl -s -o /dev/null -w "%{http_code}" http://localhost:9000/minio/health/live` returns `200`.
- The MinIO console at `http://localhost:9001` is accessible (login: minioadmin / minioadmin).
- Bucket `demo-input` exists with 3 objects under `data/` (record-001.json, record-002.json, record-003.json).
- Bucket `demo-output` exists and is empty.

**Notes:** `minio-init` is a one-shot service; it exits with code 0 after seeding. If the volume already exists from a prior run, seed objects are overwritten (harmless for demo purposes).

---

## Scenario 2: Worker registers MinIO connectors on startup (demo profile)

**REQ:** DEMO-001 / AC-1

**Given:** The full demo stack is running (`docker compose --profile demo up`). The `worker` service has `MINIO_ENDPOINT=http://minio:9000` in its environment (set by the demo compose override or `.env`).

**When:** Run `docker compose --profile demo logs worker | grep -i minio` to inspect the worker startup log.

**Then:** The log contains exactly:
```
worker: MinIO connectors registered (endpoint=minio:9000 ssl=false)
```
This confirms the worker has loaded both the MinIO DataSource and Sink connectors and they are available for pipeline execution.

**Notes:** If `MINIO_ENDPOINT` is not set (non-demo deployment), the log will instead contain `worker: MINIO_ENDPOINT not set — MinIO connectors not registered` and the worker starts normally without MinIO support.

---

## Scenario 3: MinIO DataSource reads objects from a bucket

**REQ:** DEMO-001 / AC-2

**Given:** The demo stack is running. `demo-input` bucket has 3 JSON records under `data/`.

**When:** Submit a pipeline task via `POST /api/tasks` with the following body:
```json
{
  "pipeline": {
    "datasource": {
      "connector_type": "minio",
      "config": { "bucket": "demo-input", "prefix": "data/" }
    },
    "process": {
      "script": "records"
    },
    "sink": {
      "connector_type": "file",
      "config": { "path": "/tmp/minio-datasource-test.json" }
    }
  },
  "input": {}
}
```
Then observe the task in the Task Feed until it reaches `completed` state.

**Then:**
- The task transitions through `submitted → queued → assigned → running → completed`.
- The Process phase receives 3 records from the DataSource (visible in the task log stream).
- Each record contains `id`, `name`, and `value` fields matching the seeded data.

**Notes:** This scenario uses a file Sink for easy local verification. The record order may vary because object listing order is not guaranteed.

---

## Scenario 4: MinIO Sink writes objects to a bucket

**REQ:** DEMO-001 / AC-3

**Given:** The demo stack is running. `demo-output` bucket exists and is empty (or at least has no `results/output.json` key from a prior run).

**When:** Submit a pipeline task with the following body:
```json
{
  "pipeline": {
    "datasource": {
      "connector_type": "demo",
      "config": {}
    },
    "process": {
      "script": "records"
    },
    "sink": {
      "connector_type": "minio",
      "config": { "bucket": "demo-output", "key": "results/output.json" }
    }
  },
  "input": {}
}
```
Then observe the task until `completed`.

**Then:**
- The task completes successfully.
- In the MinIO console at `http://localhost:9001`, navigate to `demo-output → results/` and confirm `output.json` exists.
- Download `output.json` and verify it is a JSON array containing the demo records.

**Notes:** If the same task is re-submitted with the same executionID, the Sink returns `ErrAlreadyApplied` (idempotency guard) and does not overwrite the object. This is by design (ADR-003).

---

## Scenario 5: Full demo pipeline — MinIO as both DataSource and Sink

**REQ:** DEMO-001 / AC-4

**Given:** The demo stack is running. `demo-input` has 3 seeded records. `demo-output` is accessible.

**When:** Submit a pipeline task using MinIO for both DataSource and Sink:
```json
{
  "pipeline": {
    "datasource": {
      "connector_type": "minio",
      "config": { "bucket": "demo-input", "prefix": "data/" }
    },
    "process": {
      "script": "records"
    },
    "sink": {
      "connector_type": "minio",
      "config": { "bucket": "demo-output", "key": "results/full-pipeline-output.json" }
    }
  },
  "input": {}
}
```
Then observe the task until `completed`.

**Then:**
- The task completes successfully.
- In the MinIO console, `demo-output/results/full-pipeline-output.json` exists.
- Its content is a JSON array of 3 records matching the seeded `demo-input` data.
- The Sink Inspector (TASK-032, once implemented) shows a Before snapshot of `object_count: 0` and an After snapshot of `object_count: 1` for the `results/` prefix.

**Notes:** This scenario is the primary integration demonstration for TASK-030. It exercises the complete DataSource → Process → Sink pipeline with MinIO at both ends. The `results/` prefix object count for the Snapshot is scoped to the key's directory (path.Dir), so only objects under `results/` are counted — not the entire `demo-output` bucket.
