// Package worker_test — unit tests for TASK-030: MinIO fake-S3 connector.
//
// Tests cover MinIODataSourceConnector (Fetch) and MinIOSinkConnector (Write, Snapshot),
// plus connector registration. All tests use InMemoryS3 and InMemoryDedupStore — no live
// MinIO instance is required.
//
// See: DEMO-001, ADR-003, ADR-009, TASK-030
package worker_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/nxlabs/nexusflow/worker"
)

// -----------------------------------------------------------------------
// MinIODataSourceConnector tests
// -----------------------------------------------------------------------

// TestMinIODataSourceConnector_Fetch_HappyPath verifies that Fetch retrieves all
// objects from the bucket and decodes their JSON bodies into records.
func TestMinIODataSourceConnector_Fetch_HappyPath(t *testing.T) {
	s3 := worker.NewInMemoryS3()
	rec1, _ := json.Marshal(map[string]any{"id": 1, "name": "alice"})
	rec2, _ := json.Marshal(map[string]any{"id": 2, "name": "bob"})
	s3.Put("demo-input", "data/record-1.json", rec1)
	s3.Put("demo-input", "data/record-2.json", rec2)

	connector := worker.NewMinIODataSourceConnector(s3)

	records, err := connector.Fetch(context.Background(),
		map[string]any{"bucket": "demo-input", "prefix": "data/"},
		nil,
	)
	if err != nil {
		t.Fatalf("Fetch returned unexpected error: %v", err)
	}
	if len(records) != 2 {
		t.Errorf("expected 2 records, got %d", len(records))
	}
}

// TestMinIODataSourceConnector_Fetch_PrefixFilter verifies that Fetch only returns
// objects whose keys start with the configured prefix.
func TestMinIODataSourceConnector_Fetch_PrefixFilter(t *testing.T) {
	s3 := worker.NewInMemoryS3()
	included, _ := json.Marshal(map[string]any{"id": 1})
	excluded, _ := json.Marshal(map[string]any{"id": 2})
	s3.Put("demo-input", "data/in-scope.json", included)
	s3.Put("demo-input", "other/out-of-scope.json", excluded)

	connector := worker.NewMinIODataSourceConnector(s3)

	records, err := connector.Fetch(context.Background(),
		map[string]any{"bucket": "demo-input", "prefix": "data/"},
		nil,
	)
	if err != nil {
		t.Fatalf("Fetch returned unexpected error: %v", err)
	}
	if len(records) != 1 {
		t.Errorf("expected 1 record with prefix filter, got %d", len(records))
	}
}

// TestMinIODataSourceConnector_Fetch_NonJSONReturnsError verifies that Fetch returns
// an error when an object's body is not valid JSON.
func TestMinIODataSourceConnector_Fetch_NonJSONReturnsError(t *testing.T) {
	s3 := worker.NewInMemoryS3()
	s3.Put("demo-input", "bad.txt", []byte("not json at all"))

	connector := worker.NewMinIODataSourceConnector(s3)

	_, err := connector.Fetch(context.Background(),
		map[string]any{"bucket": "demo-input"},
		nil,
	)
	if err == nil {
		t.Fatal("expected Fetch to return error for non-JSON object, got nil")
	}
}

// TestMinIODataSourceConnector_Type verifies the connector type string is "minio".
func TestMinIODataSourceConnector_Type(t *testing.T) {
	s3 := worker.NewInMemoryS3()
	connector := worker.NewMinIODataSourceConnector(s3)
	if got := connector.Type(); got != "minio" {
		t.Errorf("Type() = %q; want %q", got, "minio")
	}
}

// -----------------------------------------------------------------------
// MinIOSinkConnector tests
// -----------------------------------------------------------------------

// TestMinIOSinkConnector_Write_HappyPath verifies that a successful Write uploads
// a JSON-encoded object to the configured bucket/key.
func TestMinIOSinkConnector_Write_HappyPath(t *testing.T) {
	s3 := worker.NewInMemoryS3()
	dedup := worker.NewInMemoryDedupStore()
	sink := worker.NewMinIOSinkConnector(s3, dedup)

	records := []map[string]any{
		{"id": 1, "name": "alice"},
		{"id": 2, "name": "bob"},
	}
	cfg := map[string]any{"bucket": "demo-output", "key": "results/output.json"}

	err := sink.Write(context.Background(), cfg, records, "task-1:1")
	if err != nil {
		t.Fatalf("Write returned unexpected error: %v", err)
	}
	if !s3.Exists("demo-output", "results/output.json") {
		t.Error("expected object at demo-output/results/output.json, not found")
	}
	if s3.UploadCount() != 1 {
		t.Errorf("expected 1 completed upload, got %d", s3.UploadCount())
	}
}

// TestMinIOSinkConnector_Write_Idempotency verifies that a second Write with the
// same executionID returns ErrAlreadyApplied without touching MinIO (ADR-003).
func TestMinIOSinkConnector_Write_Idempotency(t *testing.T) {
	s3 := worker.NewInMemoryS3()
	dedup := worker.NewInMemoryDedupStore()
	sink := worker.NewMinIOSinkConnector(s3, dedup)

	records := []map[string]any{{"id": 1}}
	cfg := map[string]any{"bucket": "demo-output", "key": "results/idem.json"}
	execID := "task-idem:1"

	if err := sink.Write(context.Background(), cfg, records, execID); err != nil {
		t.Fatalf("first Write error: %v", err)
	}

	err := sink.Write(context.Background(), cfg, records, execID)
	if !errors.Is(err, worker.ErrAlreadyApplied) {
		t.Errorf("second Write: expected ErrAlreadyApplied, got %v", err)
	}
	// Upload count must remain 1 — no second upload was attempted.
	if s3.UploadCount() != 1 {
		t.Errorf("expected upload count to remain 1, got %d", s3.UploadCount())
	}
}

// TestMinIOSinkConnector_Write_FailureAbortsUpload verifies that when UploadPart fails,
// AbortMultipartUpload is called so no partial object exists (ADR-009 atomicity).
func TestMinIOSinkConnector_Write_FailureAbortsUpload(t *testing.T) {
	s3 := worker.NewInMemoryS3()
	s3.FailUploadAfterPart(0) // fail immediately on first part
	dedup := worker.NewInMemoryDedupStore()
	sink := worker.NewMinIOSinkConnector(s3, dedup)

	records := []map[string]any{{"id": 1}}
	cfg := map[string]any{"bucket": "demo-output", "key": "results/fail.json"}

	err := sink.Write(context.Background(), cfg, records, "task-fail:1")
	if err == nil {
		t.Fatal("expected Write to return error on forced UploadPart failure, got nil")
	}
	// No in-progress uploads must remain after abort.
	if s3.OpenMultipartCount() != 0 {
		t.Errorf("expected 0 open multipart uploads after abort, got %d", s3.OpenMultipartCount())
	}
	// No object must have been written.
	if s3.Exists("demo-output", "results/fail.json") {
		t.Error("no object should exist at the destination after abort (atomicity violation)")
	}
}

// TestMinIOSinkConnector_Snapshot verifies that Snapshot returns "object_count"
// reflecting the number of objects in the bucket under the key's directory prefix.
func TestMinIOSinkConnector_Snapshot(t *testing.T) {
	s3 := worker.NewInMemoryS3()
	s3.Put("demo-output", "results/a.json", []byte("{}"))
	s3.Put("demo-output", "results/b.json", []byte("{}"))
	s3.Put("demo-output", "other/c.json", []byte("{}"))

	dedup := worker.NewInMemoryDedupStore()
	sink := worker.NewMinIOSinkConnector(s3, dedup)

	cfg := map[string]any{"bucket": "demo-output", "key": "results/output.json"}
	snap, err := sink.Snapshot(context.Background(), cfg, "task-snap:1")
	if err != nil {
		t.Fatalf("Snapshot returned unexpected error: %v", err)
	}

	count, ok := snap["object_count"].(int)
	if !ok {
		t.Fatalf("snapshot missing int \"object_count\", got %T: %v", snap["object_count"], snap["object_count"])
	}
	if count != 2 {
		t.Errorf("expected object_count=2 (results/ prefix), got %d", count)
	}
}

// TestMinIOSinkConnector_Type verifies the connector type string is "minio".
func TestMinIOSinkConnector_Type(t *testing.T) {
	s3 := worker.NewInMemoryS3()
	dedup := worker.NewInMemoryDedupStore()
	sink := worker.NewMinIOSinkConnector(s3, dedup)
	if got := sink.Type(); got != "minio" {
		t.Errorf("Type() = %q; want %q", got, "minio")
	}
}

// -----------------------------------------------------------------------
// Registration test
// -----------------------------------------------------------------------

// TestRegisterMinIOConnectors verifies that RegisterMinIOConnectors registers
// "minio" as both a DataSource and a Sink in the registry, and that both can
// be resolved without error.
func TestRegisterMinIOConnectors_RegistersBothKinds(t *testing.T) {
	reg := worker.NewDefaultConnectorRegistry()
	s3 := worker.NewInMemoryS3()

	worker.RegisterMinIOConnectors(reg, s3)

	if _, err := reg.DataSource("minio"); err != nil {
		t.Errorf("DataSource(\"minio\") returned error after registration: %v", err)
	}
	if _, err := reg.Sink("minio"); err != nil {
		t.Errorf("Sink(\"minio\") returned error after registration: %v", err)
	}
}
