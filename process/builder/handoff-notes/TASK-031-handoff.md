# Builder Handoff Note — TASK-031

**Task:** TASK-031 — Demo infrastructure: Mock-Postgres with seed data
**Date:** 2026-04-15
**Profile:** Commercial

---

## What Was Built

### 1. `worker/connector_postgres.go` — Core connectors + test double

- `InMemoryPostgresDB`: exported test double satisfying `postgresBackend`. Provides
  `Seed`, `Rows`, `RowCountTable`, `FailAfterRow`, `BeginTx`, `QueryRows`, `RowCount`.
  Includes `inMemoryPostgresTx` for transaction semantics (commit/rollback/failure injection).
- `PostgreSQLDataSourceConnector`: reads rows via `SELECT * FROM <table>` (or a raw
  `config["query"]`). Applies optional `config["limit"]` cap after fetch.
- `PostgreSQLSinkConnector`: writes records inside a `BeginTx/InsertRow/Commit/Rollback`
  transaction with ADR-003 idempotency guard via `DedupStore`.
- `RegisterPostgreSQLConnectors`: registers both connectors in the provided registry.

### 2. `worker/connector_postgres_pgx.go` — Real pgx adapter

- `PgxBackendAdapter`: wraps `*pgxpool.Pool` and satisfies `postgresBackend`.
- `pgxTxAdapter`: wraps `pgx.Tx` and satisfies `postgresTx`.
- `NewPgxBackendAdapter(dsn string)`: constructs the pool from a DSN string. Returns
  an error on malformed DSN or pool creation failure.
- `QueryRows`, `BeginTx`, `RowCount` wired to real pgx calls.

### 3. `worker/connector_postgres_test.go` — Unit tests

11 tests covering:
- `Type()` string for DataSource and Sink
- `Fetch` with table, with query, with limit, with no config (error), empty table
- `Write` happy path, idempotency (`ErrAlreadyApplied`), rollback on failure, missing table
- `Snapshot` with rows and with missing table config
- `RegisterPostgreSQLConnectors` registers both kinds

### 4. `cmd/worker/main.go` — Wiring

Added `registerPostgresConnectors(reg)` helper and call site. The helper:
- Returns `nil` and logs a warning when `DEMO_POSTGRES_DSN` is unset.
- Calls `NewPgxBackendAdapter(dsn)` and asserts non-nil before registering.
- Nil-wiring guard: explicit `adapter == nil` check with descriptive error.

### 5. `deploy/demo-postgres/01-seed.sql` — Seed script

Creates `sample_data` (10 000 deterministic rows via `generate_series`) and
`demo_output` (empty, for sink tests). Mounted at
`/docker-entrypoint-initdb.d/` — runs exactly once on first container startup.

### 6. `docker-compose.yml` — demo-postgres service

- Added `volumes` mount: `./deploy/demo-postgres:/docker-entrypoint-initdb.d:ro`
- Added `healthcheck`: `pg_isready -U demo -d demo`, 5 retries, 20 s start period
- `worker` service: added `DEMO_POSTGRES_DSN`, `MINIO_ENDPOINT`, `MINIO_ROOT_USER`,
  `MINIO_ROOT_PASSWORD` to env (previously MinIO vars were absent, which could cause
  the worker to miss them in compose-provided envs).

### 7. `.env.example` — Documentation

Added `DEMO_POSTGRES_DSN=postgres://demo:demo@demo-postgres:5432/demo` with commentary.

---

## Nil-Wiring Verification

**Verified in `cmd/worker/main.go`.**

`registerPostgresConnectors` calls `workerPkg.NewPgxBackendAdapter(dsn)` which
returns `(*PgxBackendAdapter, error)`. The function checks the error, then explicitly
guards against a nil adapter:

```go
if adapter == nil {
    return fmt.Errorf("registerPostgresConnectors: NewPgxBackendAdapter returned nil — this is a bug")
}
workerPkg.RegisterPostgreSQLConnectors(reg, adapter)
```

`RegisterPostgreSQLConnectors` calls `NewPostgreSQLDataSourceConnector(db)` and
`NewPostgreSQLSinkConnector(db, ...)`, both of which panic on a nil `db` argument.
The nil guard in `registerPostgresConnectors` ensures neither constructor can be
reached with a nil backend.

---

## Unit Tests

| Test | AC coverage |
|---|---|
| `TestPostgreSQLDataSourceConnector_Type` | PostgreSQL DataSource has type "postgres" |
| `TestPostgreSQLDataSourceConnector_Fetch_ByTable` | DataSource can query data from table |
| `TestPostgreSQLDataSourceConnector_Fetch_ByQuery` | DataSource accepts raw SQL query |
| `TestPostgreSQLDataSourceConnector_Fetch_Limit` | Limit config caps returned rows |
| `TestPostgreSQLDataSourceConnector_Fetch_NoConfig` | Error on missing table and query |
| `TestPostgreSQLDataSourceConnector_Fetch_EmptyTable` | Returns non-nil empty slice |
| `TestPostgreSQLSinkConnector_Type` | PostgreSQL Sink has type "postgres" |
| `TestPostgreSQLSinkConnector_Write_HappyPath` | Sink can write data; rows committed |
| `TestPostgreSQLSinkConnector_Write_Idempotency` | ErrAlreadyApplied on duplicate executionID |
| `TestPostgreSQLSinkConnector_Write_RollbackOnFailure` | Zero rows on insert failure (ADR-009) |
| `TestPostgreSQLSinkConnector_Write_MissingTable` | Error on missing table config |
| `TestPostgreSQLSinkConnector_Snapshot` | row_count matches seeded rows |
| `TestPostgreSQLSinkConnector_Snapshot_MissingTable` | row_count=0 on missing table |
| `TestRegisterPostgreSQLConnectors_RegistersBothKinds` | Both connector types resolvable after registration |

All 13 packages pass (`go test ./... -count=1`). No regressions.

---

## Deviations and Notes

1. **`InMemoryPostgresDB.RowCount` method naming**: The `postgresBackend` interface
   requires `RowCount(ctx, table) (int, error)`. A separate `RowCountTable(table) int`
   helper is provided for test assertions without a context argument. Tests use
   `len(db.Rows(...))` for row-count assertions to avoid confusion between the two.

2. **`QueryRows` in `InMemoryPostgresDB`** returns all committed rows across all tables
   regardless of SQL text. This is intentional for the test double — tests control
   data via `Seed` and the SQL is not evaluated. Integration tests against real
   demo-postgres will exercise the actual query semantics.

3. **Worker env vars in compose**: `MINIO_ENDPOINT`, `MINIO_ROOT_USER`,
   `MINIO_ROOT_PASSWORD` were not previously listed under the worker service's
   environment block. They are now explicit (with empty defaults). This is a
   correctness fix that falls within the scope of the docker-compose changes
   required for TASK-031.

4. **`demo_output` table**: the seed script creates an empty `demo_output` table
   for sink connector pipeline tests. This is within scope — it is required for
   a demo pipeline that uses postgres as both DataSource and Sink.

---

## Acceptance Criteria → Test Mapping

| AC | Covered by |
|---|---|
| `demo-postgres` starts with 10K rows | `deploy/demo-postgres/01-seed.sql` + compose healthcheck; verified by acceptance test Step 2 |
| PostgreSQL DataSource can query data | `TestPostgreSQLDataSourceConnector_Fetch_ByTable` |
| PostgreSQL Sink can write data | `TestPostgreSQLSinkConnector_Write_HappyPath` |
| Demo pipeline can use postgres as both DataSource and Sink | `RegisterPostgreSQLConnectors` wires both types; acceptance test Step 3-5 |
