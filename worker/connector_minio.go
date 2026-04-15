// Package worker — MinIO S3-compatible connector implementations (TASK-030).
//
// MinIODataSourceConnector reads objects from a MinIO bucket and returns their
// contents as records. MinIOSinkConnector writes records to a MinIO bucket using
// the multipart upload atomicity pattern from ADR-009.
//
// Both connectors depend on the minioBackend interface so they can be unit-tested
// against InMemoryS3 and integration-tested against a live MinIO container.
//
// Docker Compose wires a real MinIO client (go-minio) at startup (demo profile only).
//
// See: DEMO-001, ADR-003, ADR-009, TASK-030
package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
)

// minioBackend is the narrow interface the MinIO connectors depend on.
// InMemoryS3 satisfies this interface for unit tests.
// A real MinIO client adapter (wrapping github.com/minio/minio-go/v7) satisfies
// it in the demo Docker Compose environment.
//
// This interface mirrors s3Backend so either backend can be used interchangeably
// for snapshot and write operations.
type minioBackend interface {
	// ListObjects returns the number of objects in the given bucket whose key
	// starts with prefix. Used by Snapshot to count objects in scope.
	ListObjectCount(bucket, prefix string) int

	// CreateMultipartUpload initialises a multipart upload session and returns
	// an upload ID.
	CreateMultipartUpload(bucket, key string) string

	// UploadPart uploads one part of data to an in-progress multipart upload.
	// Returns an error on simulated or real failure.
	UploadPart(uploadID string, data []byte) error

	// CompleteMultipartUpload assembles parts and stores the object at the final key.
	CompleteMultipartUpload(uploadID string) error

	// AbortMultipartUpload cancels the in-progress upload without storing any object.
	AbortMultipartUpload(uploadID string)

	// GetObject retrieves the raw bytes of a single object.
	// Returns nil and an error if the object does not exist.
	GetObject(bucket, key string) ([]byte, error)

	// ListKeys returns the object keys in the given bucket whose key starts with prefix.
	// Used by MinIODataSourceConnector to enumerate objects to fetch.
	ListKeys(bucket, prefix string) ([]string, error)
}

// -----------------------------------------------------------------------
// MinIODataSourceConnector
// -----------------------------------------------------------------------

// MinIODataSourceConnector reads objects from a MinIO bucket and returns their
// contents as records. Each object is fetched by key and its JSON content is
// decoded into a record map. Objects whose keys share a common prefix can be
// filtered by configuring the "prefix" config key.
//
// Config keys:
//   - "bucket" (string): MinIO bucket name. Required.
//   - "prefix" (string): key prefix filter. Optional; defaults to "" (all objects).
//
// Output: each record contains the raw decoded fields from each object's JSON body.
// Non-JSON objects return an error.
//
// See: DEMO-001, TASK-030
type MinIODataSourceConnector struct {
	minio minioBackend
}

// NewMinIODataSourceConnector constructs a MinIODataSourceConnector backed by the
// given minioBackend.
//
// Preconditions:
//   - minio is non-nil.
func NewMinIODataSourceConnector(minio minioBackend) *MinIODataSourceConnector {
	if minio == nil {
		panic("NewMinIODataSourceConnector: minio backend must not be nil")
	}
	return &MinIODataSourceConnector{minio: minio}
}

// Type implements DataSourceConnector.Type.
// Returns "minio" — the connector type string used in pipeline DataSourceConfig.
func (c *MinIODataSourceConnector) Type() string { return "minio" }

// Fetch retrieves all objects from the configured bucket (filtered by prefix) and
// returns each object's JSON body as a record map.
//
// Args:
//   - ctx:    Execution context. Cancellation aborts the fetch.
//   - config: DataSourceConfig.Config from the pipeline definition.
//             Required key: "bucket" (string).
//             Optional key: "prefix" (string).
//   - input:  Task input parameters. Not used by this connector; passed for
//             interface compliance.
//
// Returns:
//   - A slice of records, one per object fetched. Each record is a
//     map[string]any decoded from the object's JSON body.
//   - An error if the bucket is unreachable, ListKeys fails, or any object
//     body cannot be decoded as JSON.
//
// Preconditions:
//   - ctx is not cancelled.
//   - config["bucket"] is a non-empty string.
//
// Postconditions:
//   - On success: returns a non-nil slice (may be empty if bucket is empty or
//     no objects match prefix).
//   - On error: returns nil slice and a wrapped error describing the failure.
func (c *MinIODataSourceConnector) Fetch(ctx context.Context, config map[string]any, input map[string]any) ([]map[string]any, error) {
	bucket, _ := config["bucket"].(string)
	if bucket == "" {
		return nil, fmt.Errorf("minio datasource: config missing required key \"bucket\"")
	}
	prefix, _ := config["prefix"].(string)

	keys, err := c.minio.ListKeys(bucket, prefix)
	if err != nil {
		return nil, fmt.Errorf("minio datasource: list keys in bucket %q prefix %q: %w", bucket, prefix, err)
	}

	records := make([]map[string]any, 0, len(keys))
	for _, key := range keys {
		// Check for context cancellation between objects.
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("minio datasource: fetch cancelled after %d objects: %w", len(records), ctx.Err())
		default:
		}

		data, err := c.minio.GetObject(bucket, key)
		if err != nil {
			return nil, fmt.Errorf("minio datasource: get object %q/%q: %w", bucket, key, err)
		}

		var record map[string]any
		if err := json.Unmarshal(data, &record); err != nil {
			return nil, fmt.Errorf("minio datasource: decode object %q/%q as JSON: %w", bucket, key, err)
		}
		records = append(records, record)
	}

	return records, nil
}

// -----------------------------------------------------------------------
// MinIOSinkConnector
// -----------------------------------------------------------------------

// MinIOSinkConnector writes processed records to a MinIO bucket using the
// multipart upload atomicity pattern from ADR-009.
//
// Atomicity (ADR-009 S3/MinIO Sink):
//   - CreateMultipartUpload initialises a session at the configured key.
//   - Records are serialised as a JSON array and uploaded as a single part.
//   - CompleteMultipartUpload atomically commits the object at the final key.
//   - On any failure, AbortMultipartUpload is called so no partial object exists.
//
// Idempotency (ADR-003): a DedupStore is checked before writing; executionID is
// recorded after CompleteMultipartUpload succeeds.
//
// Config keys:
//   - "bucket" (string): MinIO bucket name. Required.
//   - "key"    (string): destination object key. Required.
//
// See: DEMO-001, ADR-003, ADR-009, TASK-030
type MinIOSinkConnector struct {
	minio minioBackend
	dedup DedupStore
}

// NewMinIOSinkConnector constructs a MinIOSinkConnector with the given backend
// and dedup store.
//
// Preconditions:
//   - minio is non-nil.
//   - dedup is non-nil.
func NewMinIOSinkConnector(minio minioBackend, dedup DedupStore) *MinIOSinkConnector {
	if minio == nil {
		panic("NewMinIOSinkConnector: minio backend must not be nil")
	}
	if dedup == nil {
		panic("NewMinIOSinkConnector: dedup store must not be nil")
	}
	return &MinIOSinkConnector{minio: minio, dedup: dedup}
}

// Type implements SinkConnector.Type.
// Returns "minio" — the connector type string used in pipeline SinkConfig.
func (c *MinIOSinkConnector) Type() string { return "minio" }

// Snapshot implements SinkConnector.Snapshot.
// Returns a map with "object_count" reflecting the number of objects in the
// configured bucket whose key shares the same prefix directory as the configured key.
// Used for Before/After comparison in the Sink Inspector (ADR-009).
//
// Config keys:
//   - "bucket" (string): MinIO bucket name. Required.
//   - "key"    (string): used to derive the listing prefix. Required.
//
// Preconditions:
//   - config["bucket"] and config["key"] are non-empty strings.
//
// Postconditions:
//   - Returns a non-nil map. Never returns an error for the in-memory backend.
//   - Map contains "object_count" (int).
func (c *MinIOSinkConnector) Snapshot(ctx context.Context, config map[string]any, taskID string) (map[string]any, error) {
	bucket, _ := config["bucket"].(string)
	key, _ := config["key"].(string)
	// Use path (not filepath) because S3/MinIO keys always use forward slashes.
	prefix := path.Dir(key)
	if prefix == "." {
		prefix = ""
	}
	count := c.minio.ListObjectCount(bucket, prefix)
	return map[string]any{"object_count": count}, nil
}

// Write serialises records as a JSON array and uploads them to MinIO as a single
// object via multipart upload.
//
// Idempotency guard (ADR-003): if executionID is already in the DedupStore,
// returns ErrAlreadyApplied without touching MinIO.
//
// Atomicity (ADR-009):
//  1. CreateMultipartUpload at the configured key.
//  2. Serialise all records as a JSON array.
//  3. UploadPart with the serialised payload.
//  4. On UploadPart error: AbortMultipartUpload; return error (no object stored).
//  5. CompleteMultipartUpload to commit the object.
//  6. Record executionID in DedupStore.
//
// Args:
//   - ctx:         Execution context. Cancellation signals the upload should abort.
//   - config:      SinkConfig.Config from the pipeline definition.
//   - records:     Records after Process->Sink schema mapping.
//   - executionID: Unique identifier for this execution attempt (taskID:attempt).
//
// Preconditions:
//   - executionID is non-empty.
//   - config["bucket"] and config["key"] are non-empty strings.
//   - c.minio is a live MinIO backend (or InMemoryS3 for tests).
//
// Postconditions:
//   - On nil: object exists at bucket/key containing JSON-encoded records;
//             executionID is recorded in DedupStore.
//   - On ErrAlreadyApplied: no change to MinIO state.
//   - On any other error: no object at bucket/key; multipart upload aborted.
func (c *MinIOSinkConnector) Write(ctx context.Context, config map[string]any, records []map[string]any, executionID string) error {
	if c.dedup.Applied(executionID) {
		return ErrAlreadyApplied
	}

	bucket, _ := config["bucket"].(string)
	key, _ := config["key"].(string)
	if bucket == "" {
		return fmt.Errorf("minio sink: config missing required key \"bucket\"")
	}
	if key == "" {
		return fmt.Errorf("minio sink: config missing required key \"key\"")
	}

	payload, err := json.Marshal(records)
	if err != nil {
		return fmt.Errorf("minio sink: serialise records: %w", err)
	}

	uploadID := c.minio.CreateMultipartUpload(bucket, key)

	if err := c.minio.UploadPart(uploadID, payload); err != nil {
		c.minio.AbortMultipartUpload(uploadID)
		return fmt.Errorf("minio sink: upload failed (aborted): %w", err)
	}

	if err := c.minio.CompleteMultipartUpload(uploadID); err != nil {
		c.minio.AbortMultipartUpload(uploadID)
		return fmt.Errorf("minio sink: complete failed (aborted): %w", err)
	}

	if err := c.dedup.Record(ctx, executionID); err != nil {
		_ = err // best-effort; see DatabaseSinkConnector.Write for rationale
	}
	return nil
}

// -----------------------------------------------------------------------
// Registration
// -----------------------------------------------------------------------

// RegisterMinIOConnectors registers the MinIO DataSource and Sink connectors
// in the given registry using the provided minioBackend.
//
// Intended to be called at worker startup when the "demo" Docker Compose profile
// is active and a live MinIO container is available.
//
// The caller is responsible for constructing the minioBackend from the
// MINIO_ENDPOINT, MINIO_ROOT_USER, and MINIO_ROOT_PASSWORD environment variables.
//
// Panics on duplicate registration (fail-fast: calling this twice is a startup bug).
//
// See: DEMO-001, TASK-030, cmd/worker/main.go
func RegisterMinIOConnectors(reg *DefaultConnectorRegistry, minio minioBackend) {
	reg.Register("datasource", NewMinIODataSourceConnector(minio))
	reg.Register("sink", NewMinIOSinkConnector(minio, NewInMemoryDedupStore()))
}
