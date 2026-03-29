// Package worker_test — unit tests for TASK-018: sink atomicity with idempotency.
// Tests cover three concrete SinkConnector implementations:
//   - DatabaseSinkConnector: BEGIN/COMMIT/ROLLBACK with deduplication table
//   - S3SinkConnector: multipart upload with abort on failure
//   - FileSinkConnector: write-to-temp + rename-on-success + delete-on-failure
//
// All tests use in-memory or file-system fakes; no live PostgreSQL or S3 instance
// is required. See: ADR-003, ADR-009, TASK-018, REQ-008
package worker_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/nxlabs/nexusflow/worker"
)

// --- DatabaseSinkConnector tests ---

// TestDatabaseSinkConnector_Type verifies the connector type string is "database".
func TestDatabaseSinkConnector_Type(t *testing.T) {
	sink := worker.NewDatabaseSinkConnector(worker.NewInMemoryDedupStore())
	if got := sink.Type(); got != "database" {
		t.Errorf("DatabaseSinkConnector.Type() = %q; want %q", got, "database")
	}
}

// TestDatabaseSinkConnector_Write_CommitsRecordsAtomically verifies that a successful
// Write records all rows in the in-memory store and returns nil.
func TestDatabaseSinkConnector_Write_CommitsRecordsAtomically(t *testing.T) {
	db := worker.NewInMemoryDatabase()
	dedup := worker.NewInMemoryDedupStore()
	sink := worker.NewDatabaseSinkConnector(dedup)
	sink.UseDatabase(db)

	records := []map[string]any{
		{"id": 1, "name": "alice"},
		{"id": 2, "name": "bob"},
	}

	err := sink.Write(context.Background(), map[string]any{"table": "users"}, records, "task-1:1")
	if err != nil {
		t.Fatalf("Write returned unexpected error: %v", err)
	}

	rows := db.Rows("users")
	if len(rows) != 2 {
		t.Errorf("expected 2 rows in destination, got %d", len(rows))
	}
}

// TestDatabaseSinkConnector_Write_RollsBackOnForcedFailure verifies that when the
// in-memory database is configured to fail on the N-th record, no rows remain in
// the destination (atomic rollback, REQ-008 / ADR-009 fitness function).
func TestDatabaseSinkConnector_Write_RollsBackOnForcedFailure(t *testing.T) {
	db := worker.NewInMemoryDatabase()
	db.FailAfterRow(1) // fail after first insert so second is never written
	dedup := worker.NewInMemoryDedupStore()
	sink := worker.NewDatabaseSinkConnector(dedup)
	sink.UseDatabase(db)

	records := []map[string]any{
		{"id": 1, "name": "alice"},
		{"id": 2, "name": "bob"},
	}

	err := sink.Write(context.Background(), map[string]any{"table": "users"}, records, "task-fail:1")
	if err == nil {
		t.Fatal("expected Write to return an error on forced failure, got nil")
	}

	// After rollback, destination must have zero records from this execution.
	rows := db.Rows("users")
	if len(rows) != 0 {
		t.Errorf("expected 0 rows after rollback, got %d (atomicity violation)", len(rows))
	}
}

// TestDatabaseSinkConnector_Write_IdempotentOnDuplicateExecutionID verifies that a
// second Write with the same executionID returns ErrAlreadyApplied without inserting
// additional rows (ADR-003 idempotency guard).
func TestDatabaseSinkConnector_Write_IdempotentOnDuplicateExecutionID(t *testing.T) {
	db := worker.NewInMemoryDatabase()
	dedup := worker.NewInMemoryDedupStore()
	sink := worker.NewDatabaseSinkConnector(dedup)
	sink.UseDatabase(db)

	records := []map[string]any{{"id": 1, "name": "alice"}}
	execID := "task-idem:1"

	if err := sink.Write(context.Background(), map[string]any{"table": "users"}, records, execID); err != nil {
		t.Fatalf("first Write error: %v", err)
	}

	err := sink.Write(context.Background(), map[string]any{"table": "users"}, records, execID)
	if !errors.Is(err, worker.ErrAlreadyApplied) {
		t.Errorf("second Write: expected ErrAlreadyApplied, got %v", err)
	}

	// The store must still have exactly one row (not two).
	rows := db.Rows("users")
	if len(rows) != 1 {
		t.Errorf("expected 1 row after idempotent second write, got %d", len(rows))
	}
}

// TestDatabaseSinkConnector_Write_RecordsExecutionID verifies that after a successful
// write the executionID is present in the deduplication store (so future calls detect it).
func TestDatabaseSinkConnector_Write_RecordsExecutionID(t *testing.T) {
	db := worker.NewInMemoryDatabase()
	dedup := worker.NewInMemoryDedupStore()
	sink := worker.NewDatabaseSinkConnector(dedup)
	sink.UseDatabase(db)

	execID := "task-record:1"
	if err := sink.Write(context.Background(), map[string]any{"table": "items"}, []map[string]any{{"x": 1}}, execID); err != nil {
		t.Fatalf("Write error: %v", err)
	}

	if !dedup.Applied(execID) {
		t.Errorf("executionID %q not recorded in dedup store after successful write", execID)
	}
}

// TestDatabaseSinkConnector_Write_DoesNotRecordExecutionIDOnFailure verifies that
// when a Write fails (and rolls back), the executionID is NOT recorded. This ensures
// the next attempt is not mistakenly treated as a duplicate.
func TestDatabaseSinkConnector_Write_DoesNotRecordExecutionIDOnFailure(t *testing.T) {
	db := worker.NewInMemoryDatabase()
	db.FailAfterRow(0) // fail immediately
	dedup := worker.NewInMemoryDedupStore()
	sink := worker.NewDatabaseSinkConnector(dedup)
	sink.UseDatabase(db)

	execID := "task-noid:1"
	_ = sink.Write(context.Background(), map[string]any{"table": "items"}, []map[string]any{{"x": 1}}, execID)

	if dedup.Applied(execID) {
		t.Errorf("executionID %q must not be recorded after failed write", execID)
	}
}

// TestDatabaseSinkConnector_Snapshot_ReturnsDestinationState verifies that Snapshot
// returns a map with a "row_count" key reflecting the current number of rows.
func TestDatabaseSinkConnector_Snapshot_ReturnsDestinationState(t *testing.T) {
	db := worker.NewInMemoryDatabase()
	db.Seed("orders", []map[string]any{{"id": 1}, {"id": 2}})
	dedup := worker.NewInMemoryDedupStore()
	sink := worker.NewDatabaseSinkConnector(dedup)
	sink.UseDatabase(db)

	snap, err := sink.Snapshot(context.Background(), map[string]any{"table": "orders"}, "task-snap:1")
	if err != nil {
		t.Fatalf("Snapshot error: %v", err)
	}
	if snap["row_count"] != 2 {
		t.Errorf("expected row_count=2, got %v", snap["row_count"])
	}
}

// --- S3SinkConnector tests ---

// TestS3SinkConnector_Type verifies the connector type string is "s3".
func TestS3SinkConnector_Type(t *testing.T) {
	sink := worker.NewS3SinkConnector(worker.NewInMemoryS3(), worker.NewInMemoryDedupStore())
	if got := sink.Type(); got != "s3" {
		t.Errorf("S3SinkConnector.Type() = %q; want %q", got, "s3")
	}
}

// TestS3SinkConnector_Write_CommitsObjectOnSuccess verifies that after a successful
// Write the object appears at the final key (not the staging prefix).
func TestS3SinkConnector_Write_CommitsObjectOnSuccess(t *testing.T) {
	s3 := worker.NewInMemoryS3()
	dedup := worker.NewInMemoryDedupStore()
	sink := worker.NewS3SinkConnector(s3, dedup)

	records := []map[string]any{{"id": 1, "name": "alice"}}
	cfg := map[string]any{"bucket": "my-bucket", "key": "output/data.json"}

	if err := sink.Write(context.Background(), cfg, records, "task-s3:1"); err != nil {
		t.Fatalf("Write error: %v", err)
	}

	if !s3.Exists("my-bucket", "output/data.json") {
		t.Error("expected final object to exist at output/data.json after successful write")
	}
	// No staging objects should remain.
	if s3.StagingObjectCount("my-bucket") != 0 {
		t.Errorf("expected 0 staging objects after commit, got %d", s3.StagingObjectCount("my-bucket"))
	}
}

// TestS3SinkConnector_Write_AbortsMultipartOnFailure verifies that when the in-memory
// S3 is configured to fail mid-upload, the multipart upload is aborted and no partial
// object exists at the final key (ADR-009 atomicity for S3).
func TestS3SinkConnector_Write_AbortsMultipartOnFailure(t *testing.T) {
	s3 := worker.NewInMemoryS3()
	s3.FailUploadAfterPart(0) // fail on the first part so the object is never committed
	dedup := worker.NewInMemoryDedupStore()
	sink := worker.NewS3SinkConnector(s3, dedup)

	records := []map[string]any{{"id": 1}, {"id": 2}, {"id": 3}}
	cfg := map[string]any{"bucket": "my-bucket", "key": "output/data.json"}

	err := sink.Write(context.Background(), cfg, records, "task-s3-fail:1")
	if err == nil {
		t.Fatal("expected Write to return error on forced S3 failure")
	}

	// No partial object at final key.
	if s3.Exists("my-bucket", "output/data.json") {
		t.Error("partial object must not exist at final key after multipart abort")
	}
	// Multipart upload must be aborted.
	if s3.OpenMultipartCount() != 0 {
		t.Errorf("expected 0 open multipart uploads after abort, got %d", s3.OpenMultipartCount())
	}
}

// TestS3SinkConnector_Write_IdempotentOnDuplicateExecutionID verifies that a second
// Write with the same executionID returns ErrAlreadyApplied without uploading again.
func TestS3SinkConnector_Write_IdempotentOnDuplicateExecutionID(t *testing.T) {
	s3 := worker.NewInMemoryS3()
	dedup := worker.NewInMemoryDedupStore()
	sink := worker.NewS3SinkConnector(s3, dedup)

	records := []map[string]any{{"id": 1}}
	cfg := map[string]any{"bucket": "my-bucket", "key": "out.json"}
	execID := "task-s3-idem:1"

	if err := sink.Write(context.Background(), cfg, records, execID); err != nil {
		t.Fatalf("first Write error: %v", err)
	}

	err := sink.Write(context.Background(), cfg, records, execID)
	if !errors.Is(err, worker.ErrAlreadyApplied) {
		t.Errorf("second Write: expected ErrAlreadyApplied, got %v", err)
	}
	// Upload count must still be 1.
	if s3.UploadCount() != 1 {
		t.Errorf("expected exactly 1 upload, got %d", s3.UploadCount())
	}
}

// TestS3SinkConnector_Write_RecordsExecutionIDMarker verifies that after commit, the
// dedup store records the executionID (simulates the marker object for deduplication).
func TestS3SinkConnector_Write_RecordsExecutionIDMarker(t *testing.T) {
	s3 := worker.NewInMemoryS3()
	dedup := worker.NewInMemoryDedupStore()
	sink := worker.NewS3SinkConnector(s3, dedup)

	execID := "task-s3-marker:1"
	cfg := map[string]any{"bucket": "b", "key": "k.json"}
	if err := sink.Write(context.Background(), cfg, []map[string]any{{"x": 1}}, execID); err != nil {
		t.Fatalf("Write error: %v", err)
	}

	if !dedup.Applied(execID) {
		t.Errorf("executionID %q not recorded after successful S3 write", execID)
	}
}

// TestS3SinkConnector_Snapshot_ReturnsDestinationState verifies that Snapshot returns
// a map describing the current objects in the target prefix.
func TestS3SinkConnector_Snapshot_ReturnsDestinationState(t *testing.T) {
	s3 := worker.NewInMemoryS3()
	s3.Put("my-bucket", "output/a.json", []byte(`{}`))
	s3.Put("my-bucket", "output/b.json", []byte(`{}`))
	dedup := worker.NewInMemoryDedupStore()
	sink := worker.NewS3SinkConnector(s3, dedup)

	cfg := map[string]any{"bucket": "my-bucket", "key": "output/data.json"}
	snap, err := sink.Snapshot(context.Background(), cfg, "task-snap")
	if err != nil {
		t.Fatalf("Snapshot error: %v", err)
	}
	if snap["object_count"] != 2 {
		t.Errorf("expected object_count=2, got %v", snap["object_count"])
	}
}

// --- FileSinkConnector tests ---

// TestFileSinkConnector_Type verifies the connector type string is "file".
func TestFileSinkConnector_Type(t *testing.T) {
	sink := worker.NewFileSinkConnector(worker.NewInMemoryDedupStore())
	if got := sink.Type(); got != "file" {
		t.Errorf("FileSinkConnector.Type() = %q; want %q", got, "file")
	}
}

// TestFileSinkConnector_Write_CreatesFileOnSuccess verifies that after a successful
// Write the final file exists at the configured path.
func TestFileSinkConnector_Write_CreatesFileOnSuccess(t *testing.T) {
	dir := t.TempDir()
	finalPath := filepath.Join(dir, "output.json")
	dedup := worker.NewInMemoryDedupStore()
	sink := worker.NewFileSinkConnector(dedup)

	records := []map[string]any{{"id": 1, "name": "alice"}}
	cfg := map[string]any{"path": finalPath}

	if err := sink.Write(context.Background(), cfg, records, "task-file:1"); err != nil {
		t.Fatalf("Write error: %v", err)
	}

	if _, err := os.Stat(finalPath); os.IsNotExist(err) {
		t.Errorf("expected final file at %s after successful write", finalPath)
	}
}

// TestFileSinkConnector_Write_DeletesTempOnFailure verifies that when a write failure
// occurs, the temp file is removed and the final file does not exist
// (atomic rollback via delete-temp-on-failure, ADR-009).
func TestFileSinkConnector_Write_DeletesTempOnFailure(t *testing.T) {
	dir := t.TempDir()
	finalPath := filepath.Join(dir, "output.json")
	dedup := worker.NewInMemoryDedupStore()

	// A write to a path inside a non-existent subdirectory will fail at
	// the rename step.
	badPath := filepath.Join(dir, "nonexistent", "nested", "output.json")
	sink := worker.NewFileSinkConnector(dedup)

	records := []map[string]any{{"id": 1}}
	cfg := map[string]any{"path": badPath}

	err := sink.Write(context.Background(), cfg, records, "task-file-fail:1")
	if err == nil {
		t.Fatal("expected Write to return error for bad path")
	}

	// The final file must not exist.
	if _, statErr := os.Stat(badPath); !os.IsNotExist(statErr) {
		t.Errorf("final file must not exist after failed write at %s", badPath)
	}

	// No temp files should be left in the dir.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.Name() != filepath.Base(finalPath) {
			t.Errorf("unexpected temp file left in dir: %s", e.Name())
		}
	}
}

// TestFileSinkConnector_Write_IdempotentOnDuplicateExecutionID verifies that a second
// Write with the same executionID returns ErrAlreadyApplied without overwriting the file.
func TestFileSinkConnector_Write_IdempotentOnDuplicateExecutionID(t *testing.T) {
	dir := t.TempDir()
	finalPath := filepath.Join(dir, "output.json")
	dedup := worker.NewInMemoryDedupStore()
	sink := worker.NewFileSinkConnector(dedup)

	records := []map[string]any{{"id": 1}}
	cfg := map[string]any{"path": finalPath}
	execID := "task-file-idem:1"

	if err := sink.Write(context.Background(), cfg, records, execID); err != nil {
		t.Fatalf("first Write error: %v", err)
	}

	err := sink.Write(context.Background(), cfg, records, execID)
	if !errors.Is(err, worker.ErrAlreadyApplied) {
		t.Errorf("second Write: expected ErrAlreadyApplied, got %v", err)
	}
}

// TestFileSinkConnector_Write_RecordsExecutionID verifies that after a successful write
// the executionID is recorded in the dedup store.
func TestFileSinkConnector_Write_RecordsExecutionID(t *testing.T) {
	dir := t.TempDir()
	finalPath := filepath.Join(dir, "output.json")
	dedup := worker.NewInMemoryDedupStore()
	sink := worker.NewFileSinkConnector(dedup)

	execID := "task-file-rec:1"
	cfg := map[string]any{"path": finalPath}
	if err := sink.Write(context.Background(), cfg, []map[string]any{{"x": 1}}, execID); err != nil {
		t.Fatalf("Write error: %v", err)
	}

	if !dedup.Applied(execID) {
		t.Errorf("executionID %q not recorded after successful file write", execID)
	}
}

// TestFileSinkConnector_Write_DoesNotRecordExecutionIDOnFailure verifies that when a
// Write fails the executionID is NOT recorded (so the next attempt is not treated as
// a duplicate and the system retries correctly).
func TestFileSinkConnector_Write_DoesNotRecordExecutionIDOnFailure(t *testing.T) {
	dir := t.TempDir()
	badPath := filepath.Join(dir, "nonexistent", "output.json")
	dedup := worker.NewInMemoryDedupStore()
	sink := worker.NewFileSinkConnector(dedup)

	execID := "task-file-noid:1"
	_ = sink.Write(context.Background(), map[string]any{"path": badPath}, []map[string]any{{"x": 1}}, execID)

	if dedup.Applied(execID) {
		t.Errorf("executionID %q must not be recorded after failed file write", execID)
	}
}

// TestFileSinkConnector_Snapshot_WhenFileExists verifies that Snapshot returns a map
// with "exists"=true and a positive "size_bytes" when the final file is present.
func TestFileSinkConnector_Snapshot_WhenFileExists(t *testing.T) {
	dir := t.TempDir()
	finalPath := filepath.Join(dir, "output.json")
	if err := os.WriteFile(finalPath, []byte(`{"id":1}`), 0o600); err != nil {
		t.Fatalf("setup: write file: %v", err)
	}

	dedup := worker.NewInMemoryDedupStore()
	sink := worker.NewFileSinkConnector(dedup)

	snap, err := sink.Snapshot(context.Background(), map[string]any{"path": finalPath}, "task-snap")
	if err != nil {
		t.Fatalf("Snapshot error: %v", err)
	}
	if snap["exists"] != true {
		t.Errorf("expected exists=true, got %v", snap["exists"])
	}
	if snap["size_bytes"].(int64) <= 0 {
		t.Errorf("expected positive size_bytes, got %v", snap["size_bytes"])
	}
}

// TestFileSinkConnector_Snapshot_WhenFileAbsent verifies that Snapshot returns
// "exists"=false and size_bytes=0 when no file exists at the path.
func TestFileSinkConnector_Snapshot_WhenFileAbsent(t *testing.T) {
	dir := t.TempDir()
	finalPath := filepath.Join(dir, "missing.json")

	dedup := worker.NewInMemoryDedupStore()
	sink := worker.NewFileSinkConnector(dedup)

	snap, err := sink.Snapshot(context.Background(), map[string]any{"path": finalPath}, "task-snap")
	if err != nil {
		t.Fatalf("Snapshot error: %v", err)
	}
	if snap["exists"] != false {
		t.Errorf("expected exists=false, got %v", snap["exists"])
	}
}

// --- Registry integration tests ---

// TestAtomicSinkConnectors_RegisteredInRegistry verifies that all three atomic sink
// connector types register and resolve correctly from the DefaultConnectorRegistry.
func TestAtomicSinkConnectors_RegisteredInRegistry(t *testing.T) {
	reg := worker.NewDefaultConnectorRegistry()
	worker.RegisterAtomicSinkConnectors(reg)

	for _, typ := range []string{"database", "s3", "file"} {
		if _, err := reg.Sink(typ); err != nil {
			t.Errorf("registry.Sink(%q) error: %v", typ, err)
		}
	}
}
