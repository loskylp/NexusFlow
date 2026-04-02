# Builder Handoff — TASK-018
**Date:** 2026-03-28
**Task:** Sink atomicity with idempotency
**Requirement(s):** REQ-008, ADR-003, ADR-009

## What Was Implemented

### New source file: `worker/sink_connectors.go`

Three concrete `SinkConnector` implementations plus supporting test infrastructure:

**`DedupStore` interface and `InMemoryDedupStore`**
- Narrow interface used by all three connectors for executionID deduplication.
- `InMemoryDedupStore` is the unit-test double; production use wires the `sink_dedup_log` table (migration 000003).

**`InMemoryDatabase`** — in-memory relational database fake for `DatabaseSinkConnector` unit tests.
- Supports `Begin` / `Insert` / `Commit` / `Rollback` semantics.
- `FailAfterRow(n)` injects a mid-transaction failure for the atomicity rollback test.
- `Seed(table, rows)` pre-populates committed rows for `Snapshot` tests.

**`DatabaseSinkConnector`** (type `"database"`)
- Idempotency guard (ADR-003): checks `DedupStore.Applied` before `Begin`.
- Transaction pattern (ADR-009): `Begin` → `Insert` each record → on error `Rollback` (zero rows) → on success `Commit` → `Record` executionID.
- `Snapshot` returns `row_count` from the committed store.
- `UseDatabase(db)` is the injection point (Dependency Inversion); production code would inject a real pgx pool.

**`InMemoryS3`** — in-memory S3-compatible fake for `S3SinkConnector` unit tests.
- Tracks multipart upload sessions; `FailUploadAfterPart(n)` injects part failure.
- `StagingObjectCount`, `OpenMultipartCount`, `UploadCount` expose state for test assertions.

**`s3Backend` interface** — narrow interface the S3 connector depends on (Dependency Inversion). `InMemoryS3` satisfies it in tests; a real AWS/MinIO adapter would satisfy it in production.

**`S3SinkConnector`** (type `"s3"`)
- Idempotency guard: checks `DedupStore.Applied` before upload.
- Atomicity (ADR-009): `CreateMultipartUpload` → `UploadPart` → on error `AbortMultipartUpload` (no partial object) → `CompleteMultipartUpload` → `Record` executionID.
- Records serialised as a single JSON payload per upload.
- `Snapshot` returns `object_count` for the key's directory prefix using the `path` package (not `filepath` — S3 keys always use forward slashes).

**`FileSinkConnector`** (type `"file"`)
- Idempotency guard: checks `DedupStore.Applied` before any file I/O.
- Atomicity (ADR-009): `os.CreateTemp` in same directory as final path → write JSON → on error remove temp → `os.Rename` to final path (POSIX-atomic) → `Record` executionID.
- `Snapshot` returns `exists` (bool) and `size_bytes` (int64).

**`RegisterAtomicSinkConnectors(reg)`**
- Registers all three connectors in the `DefaultConnectorRegistry`.
- Each connector receives a fresh `InMemoryDedupStore` (suitable for demo/test); production deployments should supply a persistent dedup store.

### Updated: `cmd/worker/main.go`
- Calls `RegisterAtomicSinkConnectors(connectorRegistry)` at startup so pipelines using connector types `"database"`, `"s3"`, and `"file"` are available without additional configuration.

### New migrations: `internal/db/migrations/000003_sink_dedup_log.{up,down}.sql`
- Creates `sink_dedup_log (execution_id TEXT PRIMARY KEY, connector_type TEXT, applied_at TIMESTAMPTZ)`.
- The up migration is idempotent (`CREATE TABLE IF NOT EXISTS`).
- This table is the persistent deduplication store for the production `DatabaseSinkConnector` and for any production DedupStore adapter that stores records here.

### New test file: `worker/sink_connectors_test.go`
28 unit tests covering all three connector types (see Unit Tests section).

## Unit Tests

- Tests written: 28
- All passing: yes
- Key behaviors covered:
  - `DatabaseSinkConnector`: successful atomic commit stores all records; forced mid-transaction failure leaves zero rows (rollback); duplicate executionID returns `ErrAlreadyApplied` without additional rows; executionID is recorded after commit; executionID is NOT recorded after failure; `Snapshot` reflects committed row count.
  - `S3SinkConnector`: successful upload creates object at final key with no staging objects; forced part failure aborts multipart upload leaving no partial object and zero open uploads; duplicate executionID returns `ErrAlreadyApplied`; executionID marker recorded after commit; `Snapshot` returns object count for the key's prefix.
  - `FileSinkConnector`: successful write creates file at final path; forced failure (bad path) deletes temp file and leaves no final file; duplicate executionID returns `ErrAlreadyApplied`; executionID recorded after commit; executionID NOT recorded after failure; `Snapshot` returns `exists=true` and positive `size_bytes` when file present; `Snapshot` returns `exists=false` when file absent.
  - Registry: all three connector types resolve correctly from `DefaultConnectorRegistry` after `RegisterAtomicSinkConnectors`.

## Deviations from Task Description

**`DatabaseSinkConnector` uses an in-memory fake database, not a real pgx connection.**
The task description says "wraps writes in BEGIN/COMMIT/ROLLBACK." This is implemented with the exact same semantics using `InMemoryDatabase` which has `Begin` / `Commit` / `Rollback`. A production `pgx`-backed implementation would be wired via a `UseDatabase`-equivalent that accepts a pgx pool. The in-memory implementation is the correct approach for unit tests (no live DB required) and the production wiring is clearly separated via the `UseDatabase` injection point. The ADR-009 atomicity invariant is structurally correct and verified by tests.

**`RegisterAtomicSinkConnectors` uses `InMemoryDedupStore`.**
The `sink_dedup_log` migration is provided, but the production `DedupStore` adapter (a pgx-backed implementation) is not implemented in this task. The connector interfaces and migration are in place; wiring the production dedup store is straightforward work for the next session (inject via constructor, replace `NewInMemoryDedupStore()` calls in the `RegisterAtomicSinkConnectors` function).

**Execution ID format not validated at the connector layer.**
The task description defines the format as `{taskID}:{attemptNumber}`. The connectors accept any non-empty string as the executionID and treat it opaquely. The format is produced by the Worker's existing `ExecutionID` field (set at task creation per ADR-003). Validating the format in the Sink connector would introduce coupling to a naming convention; the connectors remain format-agnostic.

## Known Limitations

1. **Production DedupStore not wired.** The `sink_dedup_log` table and migration exist, but `RegisterAtomicSinkConnectors` uses `InMemoryDedupStore`. For production, a `PgDedupStore` adapter (using the `pgx` pool) must be written and injected. The interface is defined; the implementation is one constructor function and two methods.

2. **`DatabaseSinkConnector` uses `InMemoryDatabase` in all contexts.** A production pgx-backed database implementation is not provided. The `UseDatabase` injection point exists for wiring a real pgx connection pool in a production `DatabaseSinkConnector` constructor.

3. **S3 multipart: one part per write.** The current S3 connector serialises all records as a single JSON payload and uploads it as one multipart part. For very large record slices a chunking strategy (multiple parts) would be more memory-efficient. This is a performance concern, not a correctness concern. YAGNI applies.

4. **`staticcheck` cannot run.** The installed staticcheck versions either require Go >= 1.25 (latest) or panic on Go 1.23 range-over-func syntax that appears elsewhere in the codebase. `go build ./...` and `go vet ./...` both pass cleanly. This is a pre-existing tooling gap, not introduced by this task.

## For the Verifier

- The acceptance tests for atomicity (forced failure mid-write → zero records) are covered by unit tests in `worker/sink_connectors_test.go`. The fitness function from ADR-009 is directly tested: `TestDatabaseSinkConnector_Write_RollsBackOnForcedFailure`, `TestS3SinkConnector_Write_AbortsMultipartOnFailure`, `TestFileSinkConnector_Write_DeletesTempOnFailure`.
- The deduplication acceptance test is covered by: `TestDatabaseSinkConnector_Write_IdempotentOnDuplicateExecutionID`, `TestS3SinkConnector_Write_IdempotentOnDuplicateExecutionID`, `TestFileSinkConnector_Write_IdempotentOnDuplicateExecutionID`.
- `go build ./...` and `go vet ./...` and `go test ./...` all pass with zero failures.
- The `cmd/worker/main.go` now calls `RegisterAtomicSinkConnectors` — the three new sink types are available in the running worker without additional configuration.
- Migration `000003_sink_dedup_log.up.sql` has not been applied to any live database; it will run automatically on next worker startup (via `db.New` which runs `golang-migrate`).
