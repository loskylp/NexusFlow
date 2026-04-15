// Package integration — TASK-030 integration tests: MinioClientAdapter against a live MinIO instance.
//
// Requirement: DEMO-001, ADR-009, TASK-030
//
// These tests exercise the MinioClientAdapter (worker/minio_client.go) against a real
// MinIO container. They verify the adapter satisfies the minioBackend contract at the
// component boundary — no internal mocks are used for the MinIO calls.
//
// Skipped unless MINIO_TEST_ENDPOINT is set (e.g. "localhost:19000").
// Set MINIO_TEST_USER / MINIO_TEST_PASSWORD for credentials (default: minioadmin/minioadmin).
//
// Run:
//
//	MINIO_TEST_ENDPOINT=localhost:19000 go test ./tests/integration/... -v -run TASK030
package integration

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/nxlabs/nexusflow/worker"
)

// minioTestAdapter returns a MinioClientAdapter pointed at the live test MinIO instance.
// Skips the test if MINIO_TEST_ENDPOINT is not set.
func minioTestAdapter(t *testing.T) *worker.MinioClientAdapter {
	t.Helper()
	endpoint := os.Getenv("MINIO_TEST_ENDPOINT")
	if endpoint == "" {
		t.Skip("MINIO_TEST_ENDPOINT not set — skipping live MinIO integration tests")
	}
	user := os.Getenv("MINIO_TEST_USER")
	if user == "" {
		user = "minioadmin"
	}
	pass := os.Getenv("MINIO_TEST_PASSWORD")
	if pass == "" {
		pass = "minioadmin"
	}
	adapter, err := worker.NewMinioClientAdapter(endpoint, user, pass, false)
	if err != nil {
		t.Fatalf("NewMinioClientAdapter: %v", err)
	}
	return adapter
}

// ---------------------------------------------------------------------------
// INT-030-1: ListKeys — enumerate objects by prefix
// ---------------------------------------------------------------------------

// TestTASK030_INT1_ListKeysFiltersByPrefix verifies that MinioClientAdapter.ListKeys
// returns only the keys whose prefix matches, not all objects in the bucket.
//
// DEMO-001 / TASK-030: S3 DataSource reads objects from MinIO buckets.
//
// Given: demo-input bucket seeded with data/record-001.json, data/record-002.json,
//
//	data/record-003.json
//
// When:  ListKeys(bucket="demo-input", prefix="data/") is called
// Then:  exactly 3 keys are returned, all starting with "data/"
func TestTASK030_INT1_ListKeysFiltersByPrefix(t *testing.T) {
	a := minioTestAdapter(t)

	keys, err := a.ListKeys("demo-input", "data/")
	if err != nil {
		t.Fatalf("ListKeys returned error: %v", err)
	}
	if len(keys) != 3 {
		t.Errorf("expected 3 keys under data/ prefix, got %d: %v", len(keys), keys)
	}
	for _, k := range keys {
		if len(k) < 5 || k[:5] != "data/" {
			t.Errorf("key %q does not start with expected prefix data/", k)
		}
	}
}

// ---------------------------------------------------------------------------
// INT-030-2: GetObject — retrieve a specific object's bytes
// ---------------------------------------------------------------------------

// TestTASK030_INT2_GetObjectReturnsParsableJSON verifies that GetObject retrieves the
// raw bytes of a seeded object and that those bytes are valid JSON.
//
// DEMO-001 / TASK-030: DataSource reads object bodies and decodes as JSON.
//
// Given: demo-input/data/record-001.json contains {"id":1,"name":"alice","value":100}
// When:  GetObject("demo-input", "data/record-001.json") is called
// Then:  returned bytes are non-empty and decode as a JSON object with key "id"
func TestTASK030_INT2_GetObjectReturnsParsableJSON(t *testing.T) {
	a := minioTestAdapter(t)

	data, err := a.GetObject("demo-input", "data/record-001.json")
	if err != nil {
		t.Fatalf("GetObject returned error: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("GetObject returned empty bytes")
	}
	var record map[string]any
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatalf("GetObject bytes are not valid JSON: %v", err)
	}
	if _, ok := record["id"]; !ok {
		t.Errorf("decoded record missing expected key \"id\": %v", record)
	}
}

// ---------------------------------------------------------------------------
// INT-030-3: PutObject round-trip via multipart upload protocol
// ---------------------------------------------------------------------------

// TestTASK030_INT3_MultipartUploadRoundTrip verifies that the Create/Upload/Complete
// sequence successfully writes an object to MinIO that can then be read back.
//
// DEMO-001 / ADR-009 / TASK-030: Sink writes via multipart upload; adapter must
// satisfy the full minioBackend multipart contract.
//
// Given: demo-output bucket exists and is empty of the test key
// When:  CreateMultipartUpload → UploadPart → CompleteMultipartUpload is called
// Then:  the object exists at the configured bucket/key and its content is correct
func TestTASK030_INT3_MultipartUploadRoundTrip(t *testing.T) {
	a := minioTestAdapter(t)

	const bucket = "demo-output"
	const key = "integration-test/round-trip.json"
	payload := []byte(`[{"id":99,"test":"round-trip"}]`)

	uploadID := a.CreateMultipartUpload(bucket, key)
	if uploadID == "" {
		t.Fatal("CreateMultipartUpload returned empty upload ID")
	}

	if err := a.UploadPart(uploadID, payload); err != nil {
		t.Fatalf("UploadPart returned error: %v", err)
	}

	if err := a.CompleteMultipartUpload(uploadID); err != nil {
		t.Fatalf("CompleteMultipartUpload returned error: %v", err)
	}

	// Verify object is now readable.
	got, err := a.GetObject(bucket, key)
	if err != nil {
		t.Fatalf("GetObject after upload returned error: %v", err)
	}
	if string(got) != string(payload) {
		t.Errorf("stored content mismatch:\n  want: %s\n  got:  %s", payload, got)
	}
}

// ---------------------------------------------------------------------------
// INT-030-4: AbortMultipartUpload — no object written after abort
// ---------------------------------------------------------------------------

// TestTASK030_INT4_AbortMultipartUploadLeavesNoObject verifies that AbortMultipartUpload
// discards any buffered data without writing to MinIO (ADR-009 atomicity guarantee).
//
// DEMO-001 / ADR-009 / TASK-030: on Sink failure, Abort is called and no partial object exists.
//
// Given: an upload is started and data is buffered
// When:  AbortMultipartUpload is called before CompleteMultipartUpload
// Then:  no object is written; ListObjectCount for the key returns 0 extra objects
//
// [VERIFIER-ADDED]: abort semantics are required by ADR-009 but not explicitly
// exercised in the AC scenarios; tested here to close the atomicity coverage gap.
func TestTASK030_INT4_AbortMultipartUploadLeavesNoObject(t *testing.T) {
	a := minioTestAdapter(t)

	const bucket = "demo-output"
	const key = "integration-test/aborted.json"

	// Capture count before to avoid coupling on bucket cleanliness.
	before := a.ListObjectCount(bucket, "integration-test/aborted")

	uploadID := a.CreateMultipartUpload(bucket, key)
	if err := a.UploadPart(uploadID, []byte(`[{"id":0}]`)); err != nil {
		t.Fatalf("UploadPart: %v", err)
	}
	a.AbortMultipartUpload(uploadID)

	after := a.ListObjectCount(bucket, "integration-test/aborted")
	if after != before {
		t.Errorf("AbortMultipartUpload: expected object count %d (unchanged), got %d", before, after)
	}
}

// ---------------------------------------------------------------------------
// INT-030-5: ListObjectCount — counts objects under prefix only
// ---------------------------------------------------------------------------

// TestTASK030_INT5_ListObjectCountScopedToPrefix verifies that ListObjectCount
// returns the count scoped to the given prefix, not the full bucket count.
//
// DEMO-001 / ADR-009 / TASK-030: Snapshot uses ListObjectCount to count objects
// in the Sink's output scope.
//
// Given: demo-input bucket has 3 objects under "data/" and 0 elsewhere
// When:  ListObjectCount("demo-input", "data/") is called
// Then:  the count is exactly 3
func TestTASK030_INT5_ListObjectCountScopedToPrefix(t *testing.T) {
	a := minioTestAdapter(t)

	count := a.ListObjectCount("demo-input", "data/")
	if count != 3 {
		t.Errorf("expected ListObjectCount=3 for demo-input/data/, got %d", count)
	}
}

// ---------------------------------------------------------------------------
// INT-030-6: MinIODataSourceConnector.Fetch against live MinIO
// ---------------------------------------------------------------------------

// TestTASK030_INT6_DataSourceFetchFromLiveMinIO verifies that MinIODataSourceConnector.Fetch
// retrieves and decodes all three seeded records from the live demo-input bucket.
//
// DEMO-001 / TASK-030 AC-2: S3 DataSource can read objects from MinIO buckets.
//
// Given: demo-input/data/ contains 3 JSON records (record-001.json, -002.json, -003.json)
// When:  Fetch is called with bucket="demo-input" prefix="data/"
// Then:  3 records are returned, each with "id", "name", and "value" fields
func TestTASK030_INT6_DataSourceFetchFromLiveMinIO(t *testing.T) {
	a := minioTestAdapter(t)
	connector := worker.NewMinIODataSourceConnector(a)

	records, err := connector.Fetch(
		context.Background(),
		map[string]any{"bucket": "demo-input", "prefix": "data/"},
		nil,
	)
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}
	if len(records) != 3 {
		t.Errorf("expected 3 records from live MinIO, got %d", len(records))
	}
	for i, rec := range records {
		for _, field := range []string{"id", "name", "value"} {
			if _, ok := rec[field]; !ok {
				t.Errorf("record[%d] missing field %q: %v", i, field, rec)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// INT-030-7: MinIOSinkConnector.Write against live MinIO (round-trip)
// ---------------------------------------------------------------------------

// TestTASK030_INT7_SinkWriteToLiveMinIO verifies that MinIOSinkConnector.Write
// successfully uploads a JSON array to the live MinIO demo-output bucket.
//
// DEMO-001 / TASK-030 AC-3: S3 Sink can write objects to MinIO buckets.
//
// Given: demo-output bucket is accessible
// When:  Write is called with 2 records and a unique executionID
// Then:  the object is written at the configured key and is readable as a JSON array
func TestTASK030_INT7_SinkWriteToLiveMinIO(t *testing.T) {
	a := minioTestAdapter(t)
	dedup := worker.NewInMemoryDedupStore()
	sink := worker.NewMinIOSinkConnector(a, dedup)

	records := []map[string]any{
		{"id": float64(10), "name": "integration-test-1"},
		{"id": float64(11), "name": "integration-test-2"},
	}
	cfg := map[string]any{
		"bucket": "demo-output",
		"key":    "integration-test/sink-output.json",
	}

	if err := sink.Write(context.Background(), cfg, records, "int-test-sink:1"); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}

	// Read the written object back to confirm content.
	data, err := a.GetObject("demo-output", "integration-test/sink-output.json")
	if err != nil {
		t.Fatalf("GetObject after Write: %v", err)
	}
	var result []map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("written object is not valid JSON array: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 records in written object, got %d", len(result))
	}
}
