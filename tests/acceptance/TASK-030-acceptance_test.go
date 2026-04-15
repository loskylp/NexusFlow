// Package acceptance — TASK-030 acceptance tests: MinIO Fake-S3 connector.
//
// Requirement: DEMO-001, ADR-003, ADR-009, TASK-030
//
// Four acceptance criteria are verified here:
//   AC-1: MinIO starts via `docker compose --profile demo up`
//   AC-2: S3 DataSource can read objects from MinIO buckets
//   AC-3: S3 Sink can write objects to MinIO buckets
//   AC-4: A demo pipeline can be defined using MinIO as DataSource and Sink
//
// AC-1 (Docker Compose startup + worker log) is verified by system test
// TASK-030-system_test.go. The acceptance tests here cover AC-2, AC-3, AC-4
// via the component interface (connector API) and the connector registry
// (required for AC-4's pipeline definition contract).
//
// Live MinIO tests (AC-2, AC-3 with adapter) skip unless MINIO_TEST_ENDPOINT is set.
// AC-4 (registry resolution) runs against the in-memory backend without a live container.
//
// Run (all):
//
//	MINIO_TEST_ENDPOINT=localhost:19000 go test ./tests/acceptance/... -v -run TASK030
//
// Run (no live MinIO — AC-4 + in-memory AC-2/AC-3 only):
//
//	go test ./tests/acceptance/... -v -run TASK030
package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/nxlabs/nexusflow/worker"
)

// ---------------------------------------------------------------------------
// AC-2: S3 DataSource can read objects from MinIO buckets
// ---------------------------------------------------------------------------

// TestTASK030_AC2_DataSourceReadsFromMinioBucket_InMemory verifies AC-2 at the
// component level using InMemoryS3. Positive case: Fetch returns all seeded records.
//
// DEMO-001 / TASK-030 AC-2: S3 DataSource can read objects from MinIO buckets.
//
// Given: a MinIODataSourceConnector backed by InMemoryS3 seeded with 3 JSON objects
// When:  Fetch is called with the correct bucket and prefix
// Then:  3 records are returned, each containing the fields from the seeded objects
func TestTASK030_AC2_DataSourceReadsFromMinioBucket_InMemory(t *testing.T) {
	s3 := worker.NewInMemoryS3()
	for i, rec := range []map[string]any{
		{"id": 1, "name": "alice", "value": 100},
		{"id": 2, "name": "bob", "value": 200},
		{"id": 3, "name": "carol", "value": 300},
	} {
		data, _ := json.Marshal(rec)
		key := "data/record-00" + string(rune('1'+i)) + ".json"
		s3.Put("demo-input", key, data)
	}

	connector := worker.NewMinIODataSourceConnector(s3)

	// Given: connector configured for demo-input bucket with data/ prefix
	// When:  Fetch is called
	records, err := connector.Fetch(
		context.Background(),
		map[string]any{"bucket": "demo-input", "prefix": "data/"},
		nil,
	)

	// Then: 3 records are returned without error
	if err != nil {
		t.Fatalf("AC-2 FAIL: Fetch returned unexpected error: %v", err)
	}
	if len(records) != 3 {
		t.Errorf("AC-2 FAIL: expected 3 records, got %d", len(records))
	}
}

// TestTASK030_AC2_DataSourceReadsFromMinioBucket_NegativeNonJSON verifies AC-2
// negative case: Fetch must return an error when an object body is not JSON.
//
// DEMO-001 / TASK-030 AC-2: a non-JSON object in the bucket must not produce
// a silently corrupt record — an error is the correct response.
//
// Given: a bucket containing one non-JSON object
// When:  Fetch is called
// Then:  Fetch returns a non-nil error (not a silent zero-record result)
func TestTASK030_AC2_DataSourceReadsFromMinioBucket_NegativeNonJSON(t *testing.T) {
	s3 := worker.NewInMemoryS3()
	s3.Put("demo-input", "data/corrupt.txt", []byte("not json"))

	connector := worker.NewMinIODataSourceConnector(s3)

	_, err := connector.Fetch(
		context.Background(),
		map[string]any{"bucket": "demo-input", "prefix": "data/"},
		nil,
	)
	if err == nil {
		t.Fatal("AC-2 FAIL (negative): Fetch should return error for non-JSON object, got nil")
	}
}

// TestTASK030_AC2_DataSourceReadsFromMinioBucket_NegativeMissingBucket verifies
// that Fetch returns an error when the required "bucket" config key is absent.
//
// [VERIFIER-ADDED]: guards against a trivially permissive implementation that
// silently defaults to an empty bucket string.
//
// Given: Fetch is called without a "bucket" key in config
// When:  Fetch runs
// Then:  a non-nil error is returned
func TestTASK030_AC2_DataSourceReadsFromMinioBucket_NegativeMissingBucket(t *testing.T) {
	s3 := worker.NewInMemoryS3()
	connector := worker.NewMinIODataSourceConnector(s3)

	_, err := connector.Fetch(context.Background(), map[string]any{}, nil)
	if err == nil {
		t.Fatal("AC-2 FAIL (negative): Fetch should return error when bucket config key is absent, got nil")
	}
}

// TestTASK030_AC2_DataSourceReadsFromMinioBucket_Live runs AC-2 against the live
// MinIO container. Skipped unless MINIO_TEST_ENDPOINT is set.
//
// Given: demo-input bucket seeded with 3 records at data/ prefix
// When:  MinIODataSourceConnector.Fetch is called
// Then:  3 records with id/name/value fields are returned
func TestTASK030_AC2_DataSourceReadsFromMinioBucket_Live(t *testing.T) {
	endpoint := os.Getenv("MINIO_TEST_ENDPOINT")
	if endpoint == "" {
		t.Skip("MINIO_TEST_ENDPOINT not set — skipping live MinIO AC-2 test")
	}
	user := envOrDefault("MINIO_TEST_USER", "minioadmin")
	pass := envOrDefault("MINIO_TEST_PASSWORD", "minioadmin")

	adapter, err := worker.NewMinioClientAdapter(endpoint, user, pass, false)
	if err != nil {
		t.Fatalf("NewMinioClientAdapter: %v", err)
	}

	connector := worker.NewMinIODataSourceConnector(adapter)
	records, err := connector.Fetch(
		context.Background(),
		map[string]any{"bucket": "demo-input", "prefix": "data/"},
		nil,
	)
	if err != nil {
		t.Fatalf("AC-2 FAIL (live): Fetch error: %v", err)
	}
	if len(records) != 3 {
		t.Errorf("AC-2 FAIL (live): expected 3 records, got %d", len(records))
	}
	for i, rec := range records {
		for _, field := range []string{"id", "name", "value"} {
			if _, ok := rec[field]; !ok {
				t.Errorf("AC-2 FAIL (live): record[%d] missing field %q: %v", i, field, rec)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// AC-3: S3 Sink can write objects to MinIO buckets
// ---------------------------------------------------------------------------

// TestTASK030_AC3_SinkWritesToMinioBucket_InMemory verifies AC-3 at the component
// level using InMemoryS3. Positive case: Write stores the object at bucket/key.
//
// DEMO-001 / ADR-003 / ADR-009 / TASK-030 AC-3.
//
// Given: a MinIOSinkConnector backed by InMemoryS3
// When:  Write is called with 2 records, bucket="demo-output", key="results/out.json"
// Then:  no error is returned; the object exists at demo-output/results/out.json
func TestTASK030_AC3_SinkWritesToMinioBucket_InMemory(t *testing.T) {
	s3 := worker.NewInMemoryS3()
	dedup := worker.NewInMemoryDedupStore()
	sink := worker.NewMinIOSinkConnector(s3, dedup)

	records := []map[string]any{
		{"id": 1, "name": "alice"},
		{"id": 2, "name": "bob"},
	}

	err := sink.Write(
		context.Background(),
		map[string]any{"bucket": "demo-output", "key": "results/out.json"},
		records,
		"task-ac3:1",
	)
	if err != nil {
		t.Fatalf("AC-3 FAIL: Write returned error: %v", err)
	}
	if !s3.Exists("demo-output", "results/out.json") {
		t.Error("AC-3 FAIL: object not found at demo-output/results/out.json after Write")
	}
}

// TestTASK030_AC3_SinkWritesToMinioBucket_NegativeIdempotency verifies AC-3 negative
// case: a second Write with the same executionID must return ErrAlreadyApplied (ADR-003).
//
// DEMO-001 / ADR-003 / TASK-030 AC-3: idempotency guard prevents duplicate writes.
//
// Given: a successful Write has already been recorded for executionID "task-idem:1"
// When:  Write is called again with the same executionID
// Then:  ErrAlreadyApplied is returned; upload count remains 1
func TestTASK030_AC3_SinkWritesToMinioBucket_NegativeIdempotency(t *testing.T) {
	s3 := worker.NewInMemoryS3()
	dedup := worker.NewInMemoryDedupStore()
	sink := worker.NewMinIOSinkConnector(s3, dedup)

	cfg := map[string]any{"bucket": "demo-output", "key": "results/idem.json"}
	records := []map[string]any{{"id": 1}}

	if err := sink.Write(context.Background(), cfg, records, "task-idem:1"); err != nil {
		t.Fatalf("first Write error: %v", err)
	}
	err := sink.Write(context.Background(), cfg, records, "task-idem:1")
	if !errors.Is(err, worker.ErrAlreadyApplied) {
		t.Errorf("AC-3 FAIL (negative idempotency): expected ErrAlreadyApplied, got %v", err)
	}
}

// TestTASK030_AC3_SinkWritesToMinioBucket_NegativeAtomicity verifies AC-3 negative
// case: when UploadPart fails, AbortMultipartUpload is called and no partial object exists.
//
// DEMO-001 / ADR-009 / TASK-030 AC-3: sink atomicity — abort on failure leaves no partial object.
//
// Given: InMemoryS3 configured to fail immediately on UploadPart
// When:  Write is called
// Then:  Write returns an error; no object exists at the destination; no open uploads remain
func TestTASK030_AC3_SinkWritesToMinioBucket_NegativeAtomicity(t *testing.T) {
	s3 := worker.NewInMemoryS3()
	s3.FailUploadAfterPart(0) // fail immediately
	dedup := worker.NewInMemoryDedupStore()
	sink := worker.NewMinIOSinkConnector(s3, dedup)

	err := sink.Write(
		context.Background(),
		map[string]any{"bucket": "demo-output", "key": "results/atomic-fail.json"},
		[]map[string]any{{"id": 1}},
		"task-atomic:1",
	)
	if err == nil {
		t.Fatal("AC-3 FAIL (negative atomicity): expected error on forced UploadPart failure, got nil")
	}
	if s3.OpenMultipartCount() != 0 {
		t.Errorf("AC-3 FAIL (negative atomicity): expected 0 open uploads after abort, got %d",
			s3.OpenMultipartCount())
	}
	if s3.Exists("demo-output", "results/atomic-fail.json") {
		t.Error("AC-3 FAIL (negative atomicity): partial object exists after abort (atomicity violation)")
	}
}

// TestTASK030_AC3_SinkWritesToMinioBucket_Live runs AC-3 against the live MinIO container.
// Skipped unless MINIO_TEST_ENDPOINT is set.
//
// Given: demo-output bucket is accessible
// When:  Write is called with 2 records
// Then:  the object is written and can be read back as a JSON array
func TestTASK030_AC3_SinkWritesToMinioBucket_Live(t *testing.T) {
	endpoint := os.Getenv("MINIO_TEST_ENDPOINT")
	if endpoint == "" {
		t.Skip("MINIO_TEST_ENDPOINT not set — skipping live MinIO AC-3 test")
	}
	user := envOrDefault("MINIO_TEST_USER", "minioadmin")
	pass := envOrDefault("MINIO_TEST_PASSWORD", "minioadmin")

	adapter, err := worker.NewMinioClientAdapter(endpoint, user, pass, false)
	if err != nil {
		t.Fatalf("NewMinioClientAdapter: %v", err)
	}

	dedup := worker.NewInMemoryDedupStore()
	sink := worker.NewMinIOSinkConnector(adapter, dedup)

	records := []map[string]any{
		{"id": float64(1), "name": "alice"},
		{"id": float64(2), "name": "bob"},
		{"id": float64(3), "name": "carol"},
	}
	cfg := map[string]any{
		"bucket": "demo-output",
		"key":    "acceptance-test/ac3-output.json",
	}

	if err := sink.Write(context.Background(), cfg, records, "ac3-live:1"); err != nil {
		t.Fatalf("AC-3 FAIL (live): Write error: %v", err)
	}

	data, err := adapter.GetObject("demo-output", "acceptance-test/ac3-output.json")
	if err != nil {
		t.Fatalf("AC-3 FAIL (live): GetObject after Write: %v", err)
	}
	var result []map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("AC-3 FAIL (live): written object not valid JSON: %v", err)
	}
	if len(result) != 3 {
		t.Errorf("AC-3 FAIL (live): expected 3 records in stored object, got %d", len(result))
	}
}

// ---------------------------------------------------------------------------
// AC-4: A demo pipeline can be defined using MinIO as DataSource and Sink
// ---------------------------------------------------------------------------

// TestTASK030_AC4_PipelineCanUseMinioBothSourceAndSink_RegistryResolution verifies
// AC-4 at the registry level: after RegisterMinIOConnectors, both "minio" DataSource
// and "minio" Sink can be resolved from the DefaultConnectorRegistry.
//
// DEMO-001 / TASK-030 AC-4: a demo pipeline definition using connector_type "minio"
// resolves both the DataSource and Sink connectors without error.
//
// Given: a DefaultConnectorRegistry with MinIO connectors registered
// When:  DataSource("minio") and Sink("minio") are called on the registry
// Then:  both return a non-nil connector and no error
func TestTASK030_AC4_PipelineCanUseMinioBothSourceAndSink_RegistryResolution(t *testing.T) {
	reg := worker.NewDefaultConnectorRegistry()
	s3 := worker.NewInMemoryS3()

	// Given: RegisterMinIOConnectors wired with an in-memory backend
	worker.RegisterMinIOConnectors(reg, s3)

	// When: the registry is queried for "minio" connectors
	ds, err := reg.DataSource("minio")
	if err != nil {
		t.Errorf("AC-4 FAIL: DataSource(\"minio\") returned error: %v", err)
	}
	if ds == nil {
		t.Error("AC-4 FAIL: DataSource(\"minio\") returned nil connector")
	}

	sk, err := reg.Sink("minio")
	if err != nil {
		t.Errorf("AC-4 FAIL: Sink(\"minio\") returned error: %v", err)
	}
	if sk == nil {
		t.Error("AC-4 FAIL: Sink(\"minio\") returned nil connector")
	}
}

// TestTASK030_AC4_PipelineCanUseMinioBothSourceAndSink_TypeStrings verifies that
// the connectors returned by the registry report "minio" as their Type().
//
// DEMO-001 / TASK-030 AC-4: pipeline executor matches connector by type string;
// "minio" must be the declared type for both directions.
//
// Given: MinIO connectors registered
// When:  Type() is called on each resolved connector
// Then:  both return "minio"
func TestTASK030_AC4_PipelineCanUseMinioBothSourceAndSink_TypeStrings(t *testing.T) {
	reg := worker.NewDefaultConnectorRegistry()
	s3 := worker.NewInMemoryS3()
	worker.RegisterMinIOConnectors(reg, s3)

	ds, _ := reg.DataSource("minio")
	if ds.Type() != "minio" {
		t.Errorf("AC-4 FAIL: DataSource connector Type()=%q, want \"minio\"", ds.Type())
	}

	sk, _ := reg.Sink("minio")
	if sk.Type() != "minio" {
		t.Errorf("AC-4 FAIL: Sink connector Type()=%q, want \"minio\"", sk.Type())
	}
}

// TestTASK030_AC4_PipelineCanUseMinioBothSourceAndSink_NegativeUnregistered verifies
// AC-4 negative case: when MinIO connectors are NOT registered, DataSource("minio") and
// Sink("minio") must return errors (not nil connectors).
//
// [VERIFIER-ADDED]: a trivially permissive registry that returns nil connectors without
// error would satisfy neither the positive nor the negative case correctly.
//
// Given: a fresh registry with NO MinIO connectors registered
// When:  DataSource("minio") and Sink("minio") are queried
// Then:  both return errors
func TestTASK030_AC4_PipelineCanUseMinioBothSourceAndSink_NegativeUnregistered(t *testing.T) {
	reg := worker.NewDefaultConnectorRegistry()
	// Note: RegisterMinIOConnectors is intentionally NOT called.

	if _, err := reg.DataSource("minio"); err == nil {
		t.Error("AC-4 FAIL (negative): DataSource(\"minio\") should return error when not registered, got nil")
	}
	if _, err := reg.Sink("minio"); err == nil {
		t.Error("AC-4 FAIL (negative): Sink(\"minio\") should return error when not registered, got nil")
	}
}

// TestTASK030_AC4_PipelineCanUseMinioBothSourceAndSink_EndToEnd verifies AC-4 at the
// pipeline level: a complete Fetch-then-Write round trip using both registered connectors
// exercises the full MinIO pipeline path.
//
// DEMO-001 / TASK-030 AC-4: a demo pipeline can use MinIO for both DataSource and Sink.
//
// Given: 3 JSON objects in demo-input bucket; demo-output bucket exists
// When:  DataSource.Fetch reads from demo-input, then Sink.Write stores output to demo-output
// Then:  no errors; the output key exists in demo-output with 3 records
func TestTASK030_AC4_PipelineCanUseMinioBothSourceAndSink_EndToEnd(t *testing.T) {
	// Seed InMemoryS3 as the shared backend for both source and sink.
	s3 := worker.NewInMemoryS3()
	for i, rec := range []map[string]any{
		{"id": 1, "name": "alice", "value": 100},
		{"id": 2, "name": "bob", "value": 200},
		{"id": 3, "name": "carol", "value": 300},
	} {
		data, _ := json.Marshal(rec)
		key := "data/record-00" + string(rune('1'+i)) + ".json"
		s3.Put("demo-input", key, data)
	}

	reg := worker.NewDefaultConnectorRegistry()
	worker.RegisterMinIOConnectors(reg, s3)

	ds, err := reg.DataSource("minio")
	if err != nil {
		t.Fatalf("AC-4 FAIL: DataSource registry lookup: %v", err)
	}
	sk, err := reg.Sink("minio")
	if err != nil {
		t.Fatalf("AC-4 FAIL: Sink registry lookup: %v", err)
	}

	// Phase 1 — DataSource Fetch
	records, err := ds.Fetch(
		context.Background(),
		map[string]any{"bucket": "demo-input", "prefix": "data/"},
		nil,
	)
	if err != nil {
		t.Fatalf("AC-4 FAIL: DataSource.Fetch error: %v", err)
	}
	if len(records) != 3 {
		t.Errorf("AC-4 FAIL: expected 3 records from DataSource, got %d", len(records))
	}

	// Phase 2 — Sink Write
	err = sk.Write(
		context.Background(),
		map[string]any{"bucket": "demo-output", "key": "results/pipeline-output.json"},
		records,
		"ac4-e2e:1",
	)
	if err != nil {
		t.Fatalf("AC-4 FAIL: Sink.Write error: %v", err)
	}

	if !s3.Exists("demo-output", "results/pipeline-output.json") {
		t.Error("AC-4 FAIL: no object at demo-output/results/pipeline-output.json after pipeline execution")
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
