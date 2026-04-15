---
task: TASK-031
title: Demo infrastructure -- Mock-Postgres with seed data
smoke: false
---

# Demo Script — TASK-031
**Feature:** Demo infrastructure — PostgreSQL DataSource and Sink connector (Mock-Postgres with seed data)
**Requirement(s):** DEMO-002
**Environment:** Staging — `docker compose --profile demo up` from the project root

## Scenario 1: demo-postgres starts healthy with seed data present

**REQ:** DEMO-002 / AC-1

**Given:** You are at the project root with a valid `.env` file (copy from `.env.example`); no prior `demo-pg-data` volume conflict.

**When:** Run `docker compose --profile demo up` and wait for all services to report healthy (the demo-postgres container prints "database system is ready to accept connections" in its logs).

**Then:**

Criterion | Command | Expected result
--- | --- | ---
demo-postgres is healthy | `docker compose --profile demo ps demo-postgres` | `(healthy)` appears in the Status column
pg_isready returns success | `docker exec nexusflow-demo-postgres-1 pg_isready -U demo -d demo` | `/var/run/postgresql:5432 - accepting connections`
sample_data has 10K rows | `docker exec nexusflow-demo-postgres-1 psql -U demo -d demo -t -c "SELECT COUNT(*) FROM sample_data;"` | `10000`
demo_output table exists | `docker exec nexusflow-demo-postgres-1 psql -U demo -d demo -c "\dt"` | Both `sample_data` and `demo_output` listed

**Notes:** The seed script at `deploy/demo-postgres/01-seed.sql` runs exactly once on first container startup. If the `demo-pg-data` volume already exists from a prior run, the seed data is already present and the `SELECT COUNT(*)` will still return 10000.

---

## Scenario 2: Worker registers PostgreSQL connectors on startup (demo profile)

**REQ:** DEMO-002 / AC-1

**Given:** The full demo stack is running (`docker compose --profile demo up`). The `worker` service has `DEMO_POSTGRES_DSN=postgres://demo:demo@demo-postgres:5432/demo` in its environment (set via `.env` or the compose override).

**When:** Run `docker compose --profile demo logs worker | grep -i postgres` to inspect the worker startup log.

**Then:** The log contains:
```
worker: PostgreSQL connectors registered (dsn=postgres://demo:demo@demo-postgres:5432/demo)
```
This confirms the worker has loaded both the PostgreSQL DataSource and Sink connectors and they are available for pipeline execution.

**Notes:** If `DEMO_POSTGRES_DSN` is not set (non-demo deployment), the log will instead contain `worker: DEMO_POSTGRES_DSN not set — PostgreSQL connectors not registered` and the worker starts normally without PostgreSQL connector support.

---

## Scenario 3: PostgreSQL DataSource queries data from demo-postgres

**REQ:** DEMO-002 / AC-2

**Given:** The demo stack is running. `sample_data` contains 10000 rows seeded by the init script.

**When:** Submit a pipeline task via `POST /api/tasks` with the following body:
```json
{
  "pipeline": {
    "datasource": {
      "connector_type": "postgres",
      "config": { "table": "sample_data", "limit": 100 }
    },
    "process": {
      "script": "records"
    },
    "sink": {
      "connector_type": "file",
      "config": { "path": "/tmp/postgres-datasource-test.json" }
    }
  },
  "input": {}
}
```
Then observe the task in the Task Feed until it reaches `completed` state.

**Then:**
- The task transitions through `submitted → queued → assigned → running → completed`.
- The Process phase receives 100 records from the DataSource (visible in the task log stream).
- Each record contains `id`, `name`, `category`, `value`, and `score` fields matching the seeded data (e.g., `{"id":1,"name":"record-1","category":"beta","value":2,"score":"0.14"}`).

**Notes:** The `limit` cap is applied after the database query. The DataSource also accepts a raw `query` config key for arbitrary SQL (e.g., `"query": "SELECT * FROM sample_data WHERE category='alpha' LIMIT 50"`).

---

## Scenario 4: PostgreSQL Sink writes data to demo-postgres

**REQ:** DEMO-002 / AC-3

**Given:** The demo stack is running. `demo_output` table exists and is empty (or note the current row count to verify the delta).

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
      "connector_type": "postgres",
      "config": { "table": "demo_output" }
    }
  },
  "input": {}
}
```
Then observe the task until `completed`.

**Then:**
- The task completes successfully.
- `docker exec nexusflow-demo-postgres-1 psql -U demo -d demo -t -c "SELECT COUNT(*) FROM demo_output;"` shows a row count higher than before the task ran (the demo DataSource emits 3 records by default).
- If the same task is re-submitted with the same `executionID`, the Sink returns `ErrAlreadyApplied` (idempotency guard, ADR-003) and does not insert duplicate rows.

**Notes:** The PostgreSQL Sink wraps all inserts in a single `BEGIN/COMMIT` transaction (ADR-009). If any insert fails, the transaction is rolled back and zero rows are committed. The `demo_output` table schema has a `data JSONB` column — records must include a `"data"` key with a JSON string value when using the real connector against this table.

---

## Scenario 5: Full demo pipeline — demo-postgres as both DataSource and Sink

**REQ:** DEMO-002 / AC-4

**Given:** The demo stack is running. `sample_data` has 10000 rows. `demo_output` is accessible. Note the current row count in `demo_output`:
```
docker exec nexusflow-demo-postgres-1 psql -U demo -d demo -t -c "SELECT COUNT(*) FROM demo_output;"
```

**When:** Submit a pipeline task using postgres for both DataSource and Sink:
```json
{
  "pipeline": {
    "datasource": {
      "connector_type": "postgres",
      "config": { "table": "sample_data", "limit": 10 }
    },
    "process": {
      "script": "records"
    },
    "sink": {
      "connector_type": "postgres",
      "config": { "table": "demo_output" }
    }
  },
  "input": {}
}
```
Then observe the task until `completed`.

**Then:**
- The task completes successfully.
- `SELECT COUNT(*) FROM demo_output` returns a value 10 higher than the value noted before the task ran.
- The Sink Inspector (TASK-032, once implemented) shows a Before snapshot of `row_count: N` and an After snapshot of `row_count: N+10` for the `demo_output` table.

**Notes:** This scenario is the primary integration demonstration for TASK-031. It exercises the complete DataSource → Process → Sink pipeline with PostgreSQL at both ends. The `limit: 10` cap keeps the demo fast; for a full-scale demonstration, increase or remove the limit to stream all 10000 rows from `sample_data` into `demo_output`.
