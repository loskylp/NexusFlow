---
task: TASK-018
title: Sink atomicity with idempotency
result: PASS
date: 2026-03-28
requirements: REQ-008, ADR-003, ADR-009
environment: Go test suite (no running service required — in-memory fakes used)
---

# Demo Script — TASK-018
**Feature:** Sink atomicity with idempotency
**Requirement(s):** REQ-008, ADR-003, ADR-009
**Environment:** Module root with Go 1.23 toolchain (or docker run golang:1.23-alpine). No live PostgreSQL or S3 instance required — all three connector types use in-memory fakes.

---

## Setup: run the acceptance tests

All five acceptance criteria are demonstrated by running the Go acceptance test suite. Execute from the module root:

```bash
docker run --rm \
  -v "$(pwd):/workspace" \
  -w /workspace \
  golang:1.23-alpine \
  go test ./tests/acceptance/... -v -run TASK018
```

Or if a local Go 1.23 toolchain is available:

```bash
go test ./tests/acceptance/... -v -run TASK018
```

Expected: all 16 tests pass, including negative cases.

---

## Scenario 1: Database Sink — forced failure mid-write leaves zero records
**REQ:** REQ-008, ADR-009

**Given:** a DatabaseSinkConnector backed by InMemoryDatabase with FailAfterRow(1) — the database will reject the second Insert, simulating a mid-transaction failure

**When:** Write is called with two records (the first Insert succeeds, the second fails, triggering Rollback)

**Then:** Write returns a non-nil error; the destination has exactly 0 committed rows — the partial first-row write is rolled back atomically; no partial state is visible

**How to verify manually:**

```go
db := worker.NewInMemoryDatabase()
db.FailAfterRow(1)
sink := worker.NewDatabaseSinkConnector(worker.NewInMemoryDedupStore())
sink.UseDatabase(db)

records := []map[string]any{{"id": 1}, {"id": 2}}
err := sink.Write(ctx, map[string]any{"table": "items"}, records, "demo018:1")
// err != nil (forced failure)
// db.Rows("items") == [] (zero rows — atomicity invariant satisfied)
```

**Notes:** The acceptance test for this scenario is `TestTASK018_AC1_DatabaseSink_ForcedFailureLeavesZeroRecords`. The fitness function threshold from ADR-009 is: zero partial writes at Sink destinations.

---

## Scenario 2: S3 Sink — forced failure aborts multipart upload; no partial objects
**REQ:** REQ-008, ADR-009

**Given:** an S3SinkConnector backed by InMemoryS3 with FailUploadAfterPart(0) — the S3 backend will reject the first UploadPart call, simulating a mid-upload failure

**When:** Write is called with records (CreateMultipartUpload succeeds; UploadPart fails; AbortMultipartUpload is called)

**Then:** Write returns a non-nil error; no object exists at the final key in the S3 backend; open multipart upload count is 0 (abort was called); staging object count is 0

**How to verify manually:**

```go
s3 := worker.NewInMemoryS3()
s3.FailUploadAfterPart(0)
sink := worker.NewS3SinkConnector(s3, worker.NewInMemoryDedupStore())

cfg := map[string]any{"bucket": "my-bucket", "key": "output/data.json"}
err := sink.Write(ctx, cfg, []map[string]any{{"id": 1}}, "demo018:1")
// err != nil (forced failure)
// s3.Exists("my-bucket", "output/data.json") == false
// s3.OpenMultipartCount() == 0
```

**Notes:** The acceptance test for this scenario is `TestTASK018_AC2_S3Sink_ForcedFailureAbortsMultipartUpload`. ADR-009 specifies multipart upload abort as the S3 atomicity mechanism.

---

## Scenario 3: Successful Sink write commits all records atomically
**REQ:** REQ-008, ADR-009

**Given:** connector instances with no failure injection (normal operation)

**When:** Write is called with N records on each of the three connector types (DatabaseSinkConnector, S3SinkConnector, FileSinkConnector)

**Then:**
- DatabaseSinkConnector: Write returns nil; all N rows are present in the committed store
- S3SinkConnector: Write returns nil; object exists at the final key; UploadCount == 1; no staging objects remain
- FileSinkConnector: Write returns nil; final file exists at the configured path; no temp files remain

**How to verify manually (Database example):**

```go
db := worker.NewInMemoryDatabase()
sink := worker.NewDatabaseSinkConnector(worker.NewInMemoryDedupStore())
sink.UseDatabase(db)

records := []map[string]any{{"id": 1}, {"id": 2}, {"id": 3}}
err := sink.Write(ctx, map[string]any{"table": "results"}, records, "demo018:1")
// err == nil
// len(db.Rows("results")) == 3
```

**Notes:** Acceptance tests: `TestTASK018_AC3_DatabaseSink_SuccessfulWriteCommitsAllRecords`, `TestTASK018_AC3_S3Sink_SuccessfulWriteCommitsObject`, `TestTASK018_AC3_FileSink_SuccessfulWriteCreatesFile`.

---

## Scenario 4: Duplicate execution ID is detected and skipped
**REQ:** ADR-003 (idempotency), REQ-008

**Given:** a SinkConnector that has already successfully committed executionID "task018:1"

**When:** Write is called again with the same executionID "task018:1" and the same (or different) records

**Then:** Write returns `worker.ErrAlreadyApplied` (not nil, not a system error); the destination state is unchanged — no additional rows inserted, no additional upload, no file overwrite

**How to verify manually:**

```go
db := worker.NewInMemoryDatabase()
dedup := worker.NewInMemoryDedupStore()
sink := worker.NewDatabaseSinkConnector(dedup)
sink.UseDatabase(db)

cfg := map[string]any{"table": "results"}
sink.Write(ctx, cfg, []map[string]any{{"id": 1}}, "task018:1") // first write succeeds

err := sink.Write(ctx, cfg, []map[string]any{{"id": 1}}, "task018:1") // duplicate
// errors.Is(err, worker.ErrAlreadyApplied) == true
// len(db.Rows("results")) == 1 (unchanged — second write was skipped)
```

**Notes:** Acceptance tests: `TestTASK018_AC4_DatabaseSink_DuplicateExecutionSkipped`, `TestTASK018_AC4_S3Sink_DuplicateExecutionSkipped`, `TestTASK018_AC4_FileSink_DuplicateExecutionSkipped`. A different executionID (same task, next attempt number) must NOT be blocked — verified by `TestTASK018_AC4_NegativeCase_DifferentExecutionIDIsNotBlocked`.

---

## Scenario 5: Execution ID is recorded at the destination for deduplication
**REQ:** ADR-003, ADR-009

**Given:** a fresh DedupStore (no entries) and a SinkConnector

**When:** Write succeeds with executionID "task018:1"

**Then:** `dedup.Applied("task018:1")` returns true — the execution ID is persisted in the DedupStore so future redeliveries are correctly detected

**When:** Write fails (forced failure) with executionID "task018:2"

**Then:** `dedup.Applied("task018:2")` returns false — the execution ID must not be recorded after failure, so the next retry attempt is not mistakenly treated as a duplicate

**How to verify manually:**

```go
dedup := worker.NewInMemoryDedupStore()
db := worker.NewInMemoryDatabase()
sink := worker.NewDatabaseSinkConnector(dedup)
sink.UseDatabase(db)

sink.Write(ctx, map[string]any{"table": "t"}, []map[string]any{{"x": 1}}, "task018:1")
dedup.Applied("task018:1") // true

db.FailAfterRow(0)
sink.Write(ctx, map[string]any{"table": "t"}, []map[string]any{{"x": 2}}, "task018:2")
dedup.Applied("task018:2") // false — failure does not record the ID
```

**Notes:** Acceptance tests: `TestTASK018_AC5_DatabaseSink_ExecutionIDRecordedAfterCommit`, `TestTASK018_AC5_S3Sink_ExecutionIDRecordedAfterCommit`, `TestTASK018_AC5_FileSink_ExecutionIDRecordedAfterCommit`, `TestTASK018_AC5_ExecutionIDNotRecordedOnFailure`. The production deduplication store is the `sink_dedup_log` table (migration 000003); the InMemoryDedupStore is the unit/acceptance test double.

---

## Scenario 6: Migration 000003 sink_dedup_log exists with correct schema
**REQ:** REQ-008, ADR-009

**Given:** the module's migration directory

**When:** migration file `internal/db/migrations/000003_sink_dedup_log.up.sql` is read

**Then:** the file contains `CREATE TABLE` for `sink_dedup_log` with `execution_id` as `PRIMARY KEY`; the down migration contains `DROP TABLE sink_dedup_log`

**How to verify manually:**

```bash
cat internal/db/migrations/000003_sink_dedup_log.up.sql
# Expected: CREATE TABLE IF NOT EXISTS sink_dedup_log (execution_id TEXT PRIMARY KEY, ...)

cat internal/db/migrations/000003_sink_dedup_log.down.sql
# Expected: DROP TABLE IF EXISTS sink_dedup_log
```

**Notes:** Acceptance test: `TestTASK018_Migration_DedupLogSQLExists`. The table is not applied to any live database by this task — it will be applied automatically by `db.New` on next worker startup (golang-migrate runs on boot, ADR-008).
