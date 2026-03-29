// Package acceptance — TASK-018 acceptance tests: Sink atomicity with idempotency
//
// Requirement: REQ-008, ADR-003, ADR-009, TASK-018
//
// These tests verify each of the five acceptance criteria at the component boundary.
// They operate exclusively through the exported SinkConnector interface; no internal
// implementation details are accessed.
//
// All three connector types (DatabaseSinkConnector, S3SinkConnector, FileSinkConnector)
// share the same atomicity and idempotency contracts — each criterion is verified across
// all three connector types where applicable.
//
// Run:
//
//	go test ./tests/acceptance/... -v -run TASK018
package acceptance

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/nxlabs/nexusflow/worker"
)

// ---------------------------------------------------------------------------
// AC-1 (REQ-008 / ADR-009): Database Sink forced failure mid-write rolls back
//   all partial records; destination has zero records.
//
// Negative case: a partial write (fail after first record) must produce zero
//   rows at the destination — no partial commit is acceptable.
// Positive case (AC-3): a successful write commits all records atomically.
// ---------------------------------------------------------------------------

// TestTASK018_AC1_DatabaseSink_ForcedFailureLeavesZeroRecords verifies the
// atomicity invariant (ADR-009) for DatabaseSinkConnector: when the database is
// configured to fail mid-write, the Rollback path leaves zero rows at the destination.
//
// REQ-008: atomic Sink operations; if Sink fails partway through, all partial writes
// are rolled back.
//
// Given: a DatabaseSinkConnector backed by an InMemoryDatabase with FailAfterRow(1)
// When:  Write is called with two records (failure occurs after first Insert)
// Then:  Write returns a non-nil error; destination has exactly 0 committed rows
func TestTASK018_AC1_DatabaseSink_ForcedFailureLeavesZeroRecords(t *testing.T) {
	db := worker.NewInMemoryDatabase()
	db.FailAfterRow(1) // inject failure after first row; second row is never inserted
	dedup := worker.NewInMemoryDedupStore()
	sink := worker.NewDatabaseSinkConnector(dedup)
	sink.UseDatabase(db)

	records := []map[string]any{
		{"id": 1, "name": "alpha"},
		{"id": 2, "name": "beta"},
	}

	// When
	err := sink.Write(context.Background(), map[string]any{"table": "items"}, records, "task018-ac1-db:1")

	// Then: Write must return an error (forced failure occurred)
	if err == nil {
		t.Fatal("AC-1 [REQ-008]: Write must return an error on forced mid-write failure; got nil")
	}

	// Then: destination must have zero rows (full rollback, no partial commit)
	committed := db.Rows("items")
	if len(committed) != 0 {
		t.Errorf("AC-1 [REQ-008]: atomicity violation — destination has %d row(s) after rollback; want 0", len(committed))
	}
}

// TestTASK018_AC1_DatabaseSink_NegativeCase_FailureIsNotSilent verifies that a
// forced failure cannot be misidentified as ErrAlreadyApplied (idempotency false-positive).
//
// [VERIFIER-ADDED] A rollback error must not satisfy errors.Is(err, worker.ErrAlreadyApplied),
// ensuring the caller can distinguish a real failure from a skipped duplicate.
//
// Given: a DatabaseSinkConnector with FailAfterRow(0) (fail immediately)
// When:  Write is called with one record
// Then:  Write returns an error that is NOT ErrAlreadyApplied
func TestTASK018_AC1_DatabaseSink_NegativeCase_FailureIsNotSilent(t *testing.T) {
	db := worker.NewInMemoryDatabase()
	db.FailAfterRow(0)
	dedup := worker.NewInMemoryDedupStore()
	sink := worker.NewDatabaseSinkConnector(dedup)
	sink.UseDatabase(db)

	err := sink.Write(context.Background(), map[string]any{"table": "items"}, []map[string]any{{"x": 1}}, "task018-ac1-db-neg:1")

	if err == nil {
		t.Fatal("AC-1 [VERIFIER-ADDED]: forced-failure Write must not return nil")
	}
	if errors.Is(err, worker.ErrAlreadyApplied) {
		t.Error("AC-1 [VERIFIER-ADDED]: forced-failure error must not be ErrAlreadyApplied; caller cannot distinguish failure from idempotent skip")
	}
}

// ---------------------------------------------------------------------------
// AC-2 (REQ-008 / ADR-009): S3 Sink forced failure mid-write aborts multipart
//   upload; no partial objects remain at the destination.
// ---------------------------------------------------------------------------

// TestTASK018_AC2_S3Sink_ForcedFailureAbortsMultipartUpload verifies the atomicity
// invariant (ADR-009) for S3SinkConnector: when a part upload fails, AbortMultipartUpload
// is called and no partial object exists at the final key.
//
// REQ-008: S3 Sink must use multipart abort so no partial object is written.
//
// Given: an S3SinkConnector backed by InMemoryS3 with FailUploadAfterPart(0)
// When:  Write is called with records (part upload fails immediately)
// Then:  Write returns a non-nil error; no object exists at the final key;
//        no in-progress multipart uploads remain open
func TestTASK018_AC2_S3Sink_ForcedFailureAbortsMultipartUpload(t *testing.T) {
	s3 := worker.NewInMemoryS3()
	s3.FailUploadAfterPart(0) // fail on first part upload
	dedup := worker.NewInMemoryDedupStore()
	sink := worker.NewS3SinkConnector(s3, dedup)

	cfg := map[string]any{"bucket": "acceptance-bucket", "key": "output/results.json"}
	records := []map[string]any{{"id": 1}, {"id": 2}, {"id": 3}}

	// When
	err := sink.Write(context.Background(), cfg, records, "task018-ac2-s3:1")

	// Then: Write must return an error
	if err == nil {
		t.Fatal("AC-2 [REQ-008]: Write must return an error on forced S3 part failure; got nil")
	}

	// Then: no partial object at the final key
	if s3.Exists("acceptance-bucket", "output/results.json") {
		t.Error("AC-2 [REQ-008]: partial object must not exist at final key after multipart abort (atomicity violation)")
	}

	// Then: no open multipart uploads (AbortMultipartUpload was called)
	if s3.OpenMultipartCount() != 0 {
		t.Errorf("AC-2 [REQ-008]: %d open multipart upload(s) remain after abort; want 0", s3.OpenMultipartCount())
	}
}

// TestTASK018_AC2_S3Sink_NegativeCase_PartialObjectNeverWritten verifies that the
// InMemoryS3 does not expose a partial payload under any key (including staging prefixes)
// after an aborted multipart upload.
//
// [VERIFIER-ADDED] REQ-008: staging intermediaries must also be cleaned up.
//
// Given: an S3SinkConnector with FailUploadAfterPart(0)
// When:  Write is called and the upload is aborted
// Then:  StagingObjectCount for the bucket is 0
func TestTASK018_AC2_S3Sink_NegativeCase_PartialObjectNeverWritten(t *testing.T) {
	s3 := worker.NewInMemoryS3()
	s3.FailUploadAfterPart(0)
	dedup := worker.NewInMemoryDedupStore()
	sink := worker.NewS3SinkConnector(s3, dedup)

	cfg := map[string]any{"bucket": "bucket-b", "key": "path/to/data.json"}
	_ = sink.Write(context.Background(), cfg, []map[string]any{{"x": 1}}, "task018-ac2-s3-neg:1")

	if s3.StagingObjectCount("bucket-b") != 0 {
		t.Errorf("AC-2 [VERIFIER-ADDED]: %d staging object(s) remain after abort; want 0", s3.StagingObjectCount("bucket-b"))
	}
}

// ---------------------------------------------------------------------------
// AC-3 (REQ-008 / ADR-009): Successful Sink write commits all records atomically.
//   Verified for all three connector types.
// ---------------------------------------------------------------------------

// TestTASK018_AC3_DatabaseSink_SuccessfulWriteCommitsAllRecords verifies that a
// successful DatabaseSinkConnector.Write persists all records in the committed store.
//
// REQ-008: all records must be committed together; no partial state is visible.
//
// Given: a DatabaseSinkConnector backed by InMemoryDatabase (no failure injection)
// When:  Write is called with 3 records
// Then:  Write returns nil; all 3 rows are present in the committed store
func TestTASK018_AC3_DatabaseSink_SuccessfulWriteCommitsAllRecords(t *testing.T) {
	db := worker.NewInMemoryDatabase()
	dedup := worker.NewInMemoryDedupStore()
	sink := worker.NewDatabaseSinkConnector(dedup)
	sink.UseDatabase(db)

	records := []map[string]any{
		{"id": 1, "v": "x"},
		{"id": 2, "v": "y"},
		{"id": 3, "v": "z"},
	}

	// When
	err := sink.Write(context.Background(), map[string]any{"table": "results"}, records, "task018-ac3-db:1")

	// Then: no error
	if err != nil {
		t.Fatalf("AC-3 [REQ-008]: DatabaseSinkConnector.Write returned unexpected error: %v", err)
	}

	// Then: all 3 rows committed atomically
	rows := db.Rows("results")
	if len(rows) != 3 {
		t.Errorf("AC-3 [REQ-008]: expected 3 committed rows, got %d (partial commit detected)", len(rows))
	}
}

// TestTASK018_AC3_S3Sink_SuccessfulWriteCommitsObject verifies that a successful
// S3SinkConnector.Write produces an object at the final key with no staging residue.
//
// Given: an S3SinkConnector backed by InMemoryS3 (no failure injection)
// When:  Write is called with records
// Then:  Write returns nil; object exists at final key; no staging objects remain;
//        upload count is exactly 1
func TestTASK018_AC3_S3Sink_SuccessfulWriteCommitsObject(t *testing.T) {
	s3 := worker.NewInMemoryS3()
	dedup := worker.NewInMemoryDedupStore()
	sink := worker.NewS3SinkConnector(s3, dedup)

	cfg := map[string]any{"bucket": "bkt", "key": "data/output.json"}
	records := []map[string]any{{"id": 1}, {"id": 2}}

	// When
	err := sink.Write(context.Background(), cfg, records, "task018-ac3-s3:1")

	// Then
	if err != nil {
		t.Fatalf("AC-3 [REQ-008]: S3SinkConnector.Write returned unexpected error: %v", err)
	}
	if !s3.Exists("bkt", "data/output.json") {
		t.Error("AC-3 [REQ-008]: object does not exist at final key after successful Write")
	}
	if s3.UploadCount() != 1 {
		t.Errorf("AC-3 [REQ-008]: expected 1 completed upload, got %d", s3.UploadCount())
	}
}

// TestTASK018_AC3_FileSink_SuccessfulWriteCreatesFile verifies that a successful
// FileSinkConnector.Write creates a file at the configured path and leaves no temp files.
//
// Given: a FileSinkConnector and a valid writable directory
// When:  Write is called with records
// Then:  Write returns nil; final file exists at the configured path; no temp files remain
func TestTASK018_AC3_FileSink_SuccessfulWriteCreatesFile(t *testing.T) {
	dir := t.TempDir()
	finalPath := filepath.Join(dir, "output.json")
	dedup := worker.NewInMemoryDedupStore()
	sink := worker.NewFileSinkConnector(dedup)

	records := []map[string]any{{"id": 1, "name": "alice"}, {"id": 2, "name": "bob"}}
	cfg := map[string]any{"path": finalPath}

	// When
	err := sink.Write(context.Background(), cfg, records, "task018-ac3-file:1")

	// Then
	if err != nil {
		t.Fatalf("AC-3 [REQ-008]: FileSinkConnector.Write returned unexpected error: %v", err)
	}
	if _, statErr := os.Stat(finalPath); os.IsNotExist(statErr) {
		t.Errorf("AC-3 [REQ-008]: final file does not exist at %s after successful Write", finalPath)
	}

	// No temp files should remain
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.Name() != filepath.Base(finalPath) {
			t.Errorf("AC-3 [REQ-008] [VERIFIER-ADDED]: unexpected temp file remains: %s", e.Name())
		}
	}
}

// ---------------------------------------------------------------------------
// AC-4 (ADR-003): Duplicate execution (same task ID + attempt number) is
//   detected and skipped (no duplicate writes). Verified for all three connectors.
// ---------------------------------------------------------------------------

// TestTASK018_AC4_DatabaseSink_DuplicateExecutionSkipped verifies that a second
// Write with the same executionID returns ErrAlreadyApplied and inserts no new rows.
//
// ADR-003: at-least-once delivery with idempotent Sink writes.
//
// Given: a DatabaseSinkConnector that has already committed executionID "task018-ac4-db:1"
// When:  Write is called again with the same executionID
// Then:  Write returns ErrAlreadyApplied; row count in destination is unchanged (still 1)
func TestTASK018_AC4_DatabaseSink_DuplicateExecutionSkipped(t *testing.T) {
	db := worker.NewInMemoryDatabase()
	dedup := worker.NewInMemoryDedupStore()
	sink := worker.NewDatabaseSinkConnector(dedup)
	sink.UseDatabase(db)

	execID := "task018-ac4-db:1"
	records := []map[string]any{{"id": 1, "v": "first"}}
	cfg := map[string]any{"table": "results"}

	// First Write — must succeed
	if err := sink.Write(context.Background(), cfg, records, execID); err != nil {
		t.Fatalf("AC-4 [ADR-003]: first Write error: %v", err)
	}

	// When: duplicate Write with same executionID
	err := sink.Write(context.Background(), cfg, records, execID)

	// Then: must return ErrAlreadyApplied
	if !errors.Is(err, worker.ErrAlreadyApplied) {
		t.Errorf("AC-4 [ADR-003]: DatabaseSink second Write must return ErrAlreadyApplied; got %v", err)
	}

	// Then: exactly 1 row in destination (no duplicate insert)
	if len(db.Rows("results")) != 1 {
		t.Errorf("AC-4 [ADR-003]: expected 1 row after idempotent skip, got %d (duplicate write occurred)", len(db.Rows("results")))
	}
}

// TestTASK018_AC4_S3Sink_DuplicateExecutionSkipped verifies that a second Write
// with the same executionID returns ErrAlreadyApplied and issues no additional upload.
//
// Given: an S3SinkConnector that has already committed executionID "task018-ac4-s3:1"
// When:  Write is called again with the same executionID
// Then:  Write returns ErrAlreadyApplied; UploadCount is still 1
func TestTASK018_AC4_S3Sink_DuplicateExecutionSkipped(t *testing.T) {
	s3 := worker.NewInMemoryS3()
	dedup := worker.NewInMemoryDedupStore()
	sink := worker.NewS3SinkConnector(s3, dedup)

	execID := "task018-ac4-s3:1"
	cfg := map[string]any{"bucket": "bkt", "key": "out.json"}
	records := []map[string]any{{"id": 1}}

	if err := sink.Write(context.Background(), cfg, records, execID); err != nil {
		t.Fatalf("AC-4 [ADR-003]: first S3 Write error: %v", err)
	}

	// When: duplicate
	err := sink.Write(context.Background(), cfg, records, execID)

	if !errors.Is(err, worker.ErrAlreadyApplied) {
		t.Errorf("AC-4 [ADR-003]: S3Sink second Write must return ErrAlreadyApplied; got %v", err)
	}
	if s3.UploadCount() != 1 {
		t.Errorf("AC-4 [ADR-003]: expected 1 upload after idempotent skip, got %d (duplicate upload occurred)", s3.UploadCount())
	}
}

// TestTASK018_AC4_FileSink_DuplicateExecutionSkipped verifies that a second Write
// with the same executionID returns ErrAlreadyApplied without overwriting the file.
//
// Given: a FileSinkConnector that has already committed executionID "task018-ac4-file:1"
// When:  Write is called again with the same executionID and different records
// Then:  Write returns ErrAlreadyApplied; file content is unchanged from first Write
func TestTASK018_AC4_FileSink_DuplicateExecutionSkipped(t *testing.T) {
	dir := t.TempDir()
	finalPath := filepath.Join(dir, "out.json")
	dedup := worker.NewInMemoryDedupStore()
	sink := worker.NewFileSinkConnector(dedup)

	execID := "task018-ac4-file:1"
	cfg := map[string]any{"path": finalPath}

	// First Write
	if err := sink.Write(context.Background(), cfg, []map[string]any{{"id": 1}}, execID); err != nil {
		t.Fatalf("AC-4 [ADR-003]: first File Write error: %v", err)
	}

	// Capture file size after first Write
	info1, _ := os.Stat(finalPath)

	// When: duplicate Write with different records (second write must be a no-op)
	err := sink.Write(context.Background(), cfg, []map[string]any{{"id": 1}, {"id": 2}, {"id": 3}}, execID)

	if !errors.Is(err, worker.ErrAlreadyApplied) {
		t.Errorf("AC-4 [ADR-003]: FileSink second Write must return ErrAlreadyApplied; got %v", err)
	}

	// File must not have been modified
	info2, _ := os.Stat(finalPath)
	if info1 != nil && info2 != nil && info2.ModTime() != info1.ModTime() {
		t.Error("AC-4 [ADR-003] [VERIFIER-ADDED]: file was modified by duplicate Write — first Write's data was overwritten")
	}
}

// TestTASK018_AC4_NegativeCase_DifferentExecutionIDIsNotBlocked verifies that the
// idempotency guard does NOT block a Write with a different executionID.
//
// [VERIFIER-ADDED] REQ-008: the dedup guard must not be over-eager — distinct execution
// attempts must be processed.
//
// Given: a DatabaseSinkConnector that has committed executionID "task018-ac4-db-diff:1"
// When:  Write is called with executionID "task018-ac4-db-diff:2" (different attempt)
// Then:  Write returns nil; destination has 2 rows total
func TestTASK018_AC4_NegativeCase_DifferentExecutionIDIsNotBlocked(t *testing.T) {
	db := worker.NewInMemoryDatabase()
	dedup := worker.NewInMemoryDedupStore()
	sink := worker.NewDatabaseSinkConnector(dedup)
	sink.UseDatabase(db)

	cfg := map[string]any{"table": "results"}

	if err := sink.Write(context.Background(), cfg, []map[string]any{{"id": 1}}, "task018-ac4-db-diff:1"); err != nil {
		t.Fatalf("AC-4 [VERIFIER-ADDED]: first Write error: %v", err)
	}

	// Different executionID — must not be blocked
	err := sink.Write(context.Background(), cfg, []map[string]any{{"id": 2}}, "task018-ac4-db-diff:2")
	if err != nil {
		t.Errorf("AC-4 [VERIFIER-ADDED]: second Write with different executionID must succeed; got %v", err)
	}
	if len(db.Rows("results")) != 2 {
		t.Errorf("AC-4 [VERIFIER-ADDED]: expected 2 rows after two distinct executions, got %d", len(db.Rows("results")))
	}
}

// ---------------------------------------------------------------------------
// AC-5 (ADR-003 / ADR-009): Execution ID is recorded at the destination for
//   deduplication after a successful Write. Verified for all three connectors.
// ---------------------------------------------------------------------------

// TestTASK018_AC5_DatabaseSink_ExecutionIDRecordedAfterCommit verifies that after a
// successful Write, the executionID is present in the DedupStore.
//
// ADR-003: the execution ID must be persisted so future redeliveries are detected.
//
// Given: a DatabaseSinkConnector with a fresh DedupStore
// When:  Write succeeds with executionID "task018-ac5-db:1"
// Then:  dedup.Applied("task018-ac5-db:1") returns true
func TestTASK018_AC5_DatabaseSink_ExecutionIDRecordedAfterCommit(t *testing.T) {
	db := worker.NewInMemoryDatabase()
	dedup := worker.NewInMemoryDedupStore()
	sink := worker.NewDatabaseSinkConnector(dedup)
	sink.UseDatabase(db)

	execID := "task018-ac5-db:1"
	err := sink.Write(context.Background(), map[string]any{"table": "t"}, []map[string]any{{"x": 1}}, execID)
	if err != nil {
		t.Fatalf("AC-5 [ADR-003]: Write error: %v", err)
	}

	if !dedup.Applied(execID) {
		t.Errorf("AC-5 [ADR-003]: executionID %q not recorded in DedupStore after successful DatabaseSink Write", execID)
	}
}

// TestTASK018_AC5_S3Sink_ExecutionIDRecordedAfterCommit verifies that after a
// successful Write, the executionID is present in the DedupStore.
//
// Given: an S3SinkConnector with a fresh DedupStore
// When:  Write succeeds with executionID "task018-ac5-s3:1"
// Then:  dedup.Applied("task018-ac5-s3:1") returns true
func TestTASK018_AC5_S3Sink_ExecutionIDRecordedAfterCommit(t *testing.T) {
	s3 := worker.NewInMemoryS3()
	dedup := worker.NewInMemoryDedupStore()
	sink := worker.NewS3SinkConnector(s3, dedup)

	execID := "task018-ac5-s3:1"
	err := sink.Write(context.Background(), map[string]any{"bucket": "bkt", "key": "k.json"}, []map[string]any{{"x": 1}}, execID)
	if err != nil {
		t.Fatalf("AC-5 [ADR-003]: Write error: %v", err)
	}

	if !dedup.Applied(execID) {
		t.Errorf("AC-5 [ADR-003]: executionID %q not recorded in DedupStore after successful S3Sink Write", execID)
	}
}

// TestTASK018_AC5_FileSink_ExecutionIDRecordedAfterCommit verifies that after a
// successful Write, the executionID is present in the DedupStore.
//
// Given: a FileSinkConnector with a fresh DedupStore
// When:  Write succeeds with executionID "task018-ac5-file:1"
// Then:  dedup.Applied("task018-ac5-file:1") returns true
func TestTASK018_AC5_FileSink_ExecutionIDRecordedAfterCommit(t *testing.T) {
	dir := t.TempDir()
	finalPath := filepath.Join(dir, "out.json")
	dedup := worker.NewInMemoryDedupStore()
	sink := worker.NewFileSinkConnector(dedup)

	execID := "task018-ac5-file:1"
	err := sink.Write(context.Background(), map[string]any{"path": finalPath}, []map[string]any{{"x": 1}}, execID)
	if err != nil {
		t.Fatalf("AC-5 [ADR-003]: Write error: %v", err)
	}

	if !dedup.Applied(execID) {
		t.Errorf("AC-5 [ADR-003]: executionID %q not recorded in DedupStore after successful FileSink Write", execID)
	}
}

// TestTASK018_AC5_ExecutionIDNotRecordedOnFailure verifies that after a failed Write,
// the executionID is NOT recorded — so the next attempt is not mistakenly skipped.
//
// [VERIFIER-ADDED] ADR-003: recording the execution ID before commit would break
// at-least-once delivery by blocking retry after failure.
//
// Given: a DatabaseSinkConnector with FailAfterRow(0)
// When:  Write fails with executionID "task018-ac5-noid:1"
// Then:  dedup.Applied("task018-ac5-noid:1") returns false
func TestTASK018_AC5_ExecutionIDNotRecordedOnFailure(t *testing.T) {
	db := worker.NewInMemoryDatabase()
	db.FailAfterRow(0)
	dedup := worker.NewInMemoryDedupStore()
	sink := worker.NewDatabaseSinkConnector(dedup)
	sink.UseDatabase(db)

	execID := "task018-ac5-noid:1"
	_ = sink.Write(context.Background(), map[string]any{"table": "t"}, []map[string]any{{"x": 1}}, execID)

	if dedup.Applied(execID) {
		t.Errorf("AC-5 [ADR-003] [VERIFIER-ADDED]: executionID %q must NOT be recorded in DedupStore after failed Write (would block retry)", execID)
	}
}

// ---------------------------------------------------------------------------
// Migration SQL structural verification
// AC-5 integration: sink_dedup_log migration exists and has correct schema
// ---------------------------------------------------------------------------

// TestTASK018_Migration_DedupLogSQLExists verifies that the migration files for
// migration 000003 exist on disk and contain the expected DDL artifacts.
//
// REQ-008, ADR-009: The production DedupStore depends on sink_dedup_log. The
// migration must exist for this feature to function in production.
//
// Given: the project migration directory
// When:  migration 000003 up/down files are read
// Then:  up file contains CREATE TABLE sink_dedup_log with execution_id PRIMARY KEY;
//        down file contains DROP TABLE sink_dedup_log
func TestTASK018_Migration_DedupLogSQLExists(t *testing.T) {
	// Resolve migration paths relative to the module root. The module root can be
	// found by searching upward from the current working directory for go.mod.
	// In CI and Docker the working directory is always the module root, so the
	// relative paths below are correct. On host machines go test sets the cwd to
	// the package directory (tests/acceptance/), so we walk up two levels.
	upPath := resolveFromModuleRoot("internal/db/migrations/000003_sink_dedup_log.up.sql")
	downPath := resolveFromModuleRoot("internal/db/migrations/000003_sink_dedup_log.down.sql")

	upSQL, err := os.ReadFile(upPath)
	if err != nil {
		t.Fatalf("AC-5 migration [REQ-008]: up migration 000003 not found at %s: %v", upPath, err)
	}

	downSQL, err := os.ReadFile(downPath)
	if err != nil {
		t.Fatalf("AC-5 migration [REQ-008]: down migration 000003 not found at %s: %v", downPath, err)
	}

	// Up migration must declare sink_dedup_log with execution_id as PRIMARY KEY
	upStr := string(upSQL)
	for _, token := range []string{"sink_dedup_log", "execution_id", "PRIMARY KEY"} {
		if !containsToken(upStr, token) {
			t.Errorf("AC-5 migration [REQ-008]: up SQL missing expected token %q", token)
		}
	}

	// Down migration must drop sink_dedup_log
	downStr := string(downSQL)
	for _, token := range []string{"sink_dedup_log", "DROP"} {
		if !containsToken(downStr, token) {
			t.Errorf("AC-5 migration [REQ-008]: down SQL missing expected token %q", token)
		}
	}
}

// resolveFromModuleRoot returns an absolute path to rel by walking up from the
// current working directory until a directory containing go.mod is found. This
// allows the test to be run from any directory (go test sets cwd to the package
// directory; docker/CI sets cwd to the module root).
func resolveFromModuleRoot(rel string) string {
	dir, err := os.Getwd()
	if err != nil {
		return rel // fallback
	}
	for {
		candidate := filepath.Join(dir, "go.mod")
		if _, err := os.Stat(candidate); err == nil {
			return filepath.Join(dir, filepath.FromSlash(rel))
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break // filesystem root — give up
		}
		dir = parent
	}
	return rel // unreachable in practice
}

// containsToken returns true if s contains substr (case-insensitive substring match).
func containsToken(s, substr string) bool {
	return len(s) >= len(substr) && containsCI(s, substr)
}

func containsCI(s, sub string) bool {
	if len(sub) == 0 {
		return true
	}
	sl, subl := len(s), len(sub)
	for i := 0; i <= sl-subl; i++ {
		match := true
		for j := 0; j < subl; j++ {
			a, b := s[i+j], sub[j]
			if a >= 'A' && a <= 'Z' {
				a += 32
			}
			if b >= 'A' && b <= 'Z' {
				b += 32
			}
			if a != b {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
