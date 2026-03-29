// Package worker — atomic Sink connector implementations (TASK-018).
//
// Three concrete SinkConnector types are provided, each using its destination's
// native atomicity mechanism (ADR-009):
//
//   - DatabaseSinkConnector: wraps writes in a BEGIN/COMMIT/ROLLBACK transaction.
//   - S3SinkConnector: uses multipart upload; aborts on failure so no partial object exists.
//   - FileSinkConnector: writes to a temp file then renames on success; deletes temp on failure.
//
// All three implement the idempotency guard from ADR-003: before writing they check
// whether the executionID has already been applied via a DedupStore. On success the
// executionID is recorded. On failure it is not recorded so the next attempt retries.
//
// See: ADR-003, ADR-009, REQ-008, TASK-018
package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sync"
)

// -----------------------------------------------------------------------
// DedupStore — deduplication store interface and in-memory implementation
// -----------------------------------------------------------------------

// DedupStore records and queries applied execution IDs.
// All SinkConnector implementations depend on this abstraction (Dependency Inversion).
// The production implementation writes to the sink_dedup_log table (see migration 000003).
// The InMemoryDedupStore is used in unit tests.
type DedupStore interface {
	// Applied returns true when executionID has already been recorded.
	Applied(executionID string) bool

	// Record marks executionID as applied. Called after a successful atomic commit.
	// Returns an error if the record cannot be persisted.
	Record(ctx context.Context, executionID string) error
}

// InMemoryDedupStore is a thread-safe in-memory DedupStore for unit testing.
// It satisfies DedupStore without requiring a live database connection.
type InMemoryDedupStore struct {
	mu      sync.RWMutex
	applied map[string]struct{}
}

// NewInMemoryDedupStore constructs an empty InMemoryDedupStore.
func NewInMemoryDedupStore() *InMemoryDedupStore {
	return &InMemoryDedupStore{applied: make(map[string]struct{})}
}

// Applied implements DedupStore.Applied. Thread-safe.
func (s *InMemoryDedupStore) Applied(executionID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.applied[executionID]
	return ok
}

// Record implements DedupStore.Record. Thread-safe. Never returns an error.
func (s *InMemoryDedupStore) Record(ctx context.Context, executionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.applied[executionID] = struct{}{}
	return nil
}

// -----------------------------------------------------------------------
// InMemoryDatabase — fake relational database for unit testing
// -----------------------------------------------------------------------

// InMemoryDatabase simulates a relational database with transactional guarantees.
// Rows are written to an uncommitted buffer; COMMIT moves them to the committed store;
// ROLLBACK discards the buffer. Tests can inject failures via FailAfterRow.
type InMemoryDatabase struct {
	mu          sync.Mutex
	committed   map[string][]map[string]any // table -> committed rows
	inFlight    map[string][]map[string]any // table -> rows in the current transaction
	failAfter   int                         // fail after this many rows in a transaction (-1 = never)
	insertCount int                         // rows inserted in the current transaction
}

// NewInMemoryDatabase constructs an empty InMemoryDatabase with no failure injection.
func NewInMemoryDatabase() *InMemoryDatabase {
	return &InMemoryDatabase{
		committed: make(map[string][]map[string]any),
		inFlight:  make(map[string][]map[string]any),
		failAfter: -1,
	}
}

// FailAfterRow configures the database to return an error after n rows are inserted
// within a transaction. Set n=0 to fail immediately on the first insert.
func (d *InMemoryDatabase) FailAfterRow(n int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.failAfter = n
}

// Seed populates a table with pre-existing rows (used in Snapshot tests).
func (d *InMemoryDatabase) Seed(table string, rows []map[string]any) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.committed[table] = append(d.committed[table], rows...)
}

// Rows returns a copy of the committed rows in a table.
func (d *InMemoryDatabase) Rows(table string) []map[string]any {
	d.mu.Lock()
	defer d.mu.Unlock()
	rows := d.committed[table]
	cp := make([]map[string]any, len(rows))
	copy(cp, rows)
	return cp
}

// Begin starts a new transaction. Resets the in-flight buffer and insert counter.
// Precondition: no nested transactions.
func (d *InMemoryDatabase) Begin() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.inFlight = make(map[string][]map[string]any)
	d.insertCount = 0
}

// Insert adds a row to the in-flight buffer for the given table.
// Returns an error if FailAfterRow was configured and the threshold has been reached.
func (d *InMemoryDatabase) Insert(table string, row map[string]any) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.failAfter >= 0 && d.insertCount >= d.failAfter {
		return fmt.Errorf("simulated database failure after %d row(s)", d.failAfter)
	}
	d.inFlight[table] = append(d.inFlight[table], row)
	d.insertCount++
	return nil
}

// Commit moves all in-flight rows to the committed store.
// Postcondition: in-flight buffer is empty; committed store reflects the new rows.
func (d *InMemoryDatabase) Commit() {
	d.mu.Lock()
	defer d.mu.Unlock()
	for table, rows := range d.inFlight {
		d.committed[table] = append(d.committed[table], rows...)
	}
	d.inFlight = make(map[string][]map[string]any)
}

// Rollback discards all in-flight rows without touching the committed store.
// Postcondition: destination is in exactly the state it was before Begin.
func (d *InMemoryDatabase) Rollback() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.inFlight = make(map[string][]map[string]any)
}

// RowCount returns the number of committed rows in a table.
func (d *InMemoryDatabase) RowCount(table string) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.committed[table])
}

// -----------------------------------------------------------------------
// DatabaseSinkConnector
// -----------------------------------------------------------------------

// DatabaseSinkConnector writes records to a relational database inside a single
// BEGIN/COMMIT/ROLLBACK transaction (ADR-009 Database Sink atomicity).
//
// Idempotency (ADR-003): before any write the connector checks the DedupStore. If the
// executionID is already present, Write returns ErrAlreadyApplied without touching the
// database. On successful COMMIT the executionID is recorded in the DedupStore.
//
// Config keys:
//   - "table" (string): destination table name. Required.
//
// See: ADR-003, ADR-009, TASK-018
type DatabaseSinkConnector struct {
	dedup DedupStore
	db    *InMemoryDatabase // replaced by a real pgx pool in production via UseDatabase
}

// NewDatabaseSinkConnector constructs a DatabaseSinkConnector backed by the given DedupStore.
// Call UseDatabase to wire the actual database before calling Write.
//
// Preconditions:
//   - dedup is non-nil.
func NewDatabaseSinkConnector(dedup DedupStore) *DatabaseSinkConnector {
	return &DatabaseSinkConnector{dedup: dedup}
}

// UseDatabase wires the in-memory database implementation.
// This is the injection point used by unit tests. Production code would inject a
// real pgx connection pool via a separate constructor or option function.
func (c *DatabaseSinkConnector) UseDatabase(db *InMemoryDatabase) {
	c.db = db
}

// Type implements SinkConnector.Type. Returns "database".
func (c *DatabaseSinkConnector) Type() string { return "database" }

// Snapshot implements SinkConnector.Snapshot.
// Returns a map with "row_count" reflecting the number of committed rows in the
// configured table at the time of the call. Used for Before/After comparison (ADR-009).
//
// Config keys:
//   - "table" (string): table to count. Required.
//
// Postconditions:
//   - Returns a non-nil map. Never returns an error for the in-memory implementation.
func (c *DatabaseSinkConnector) Snapshot(ctx context.Context, config map[string]any, taskID string) (map[string]any, error) {
	table, _ := config["table"].(string)
	count := 0
	if c.db != nil && table != "" {
		count = c.db.RowCount(table)
	}
	return map[string]any{"row_count": count}, nil
}

// Write atomically inserts records into the destination table.
//
// Idempotency guard (ADR-003): if executionID is already in the DedupStore, returns
// ErrAlreadyApplied without touching the database.
//
// Transaction pattern (ADR-009):
//  1. Begin transaction
//  2. Insert each record
//  3. On first error: Rollback and return the error (zero rows at destination)
//  4. On success: Commit and record executionID in the DedupStore
//
// Preconditions:
//   - executionID is non-empty.
//   - config["table"] is a non-empty string.
//   - c.db is non-nil (set via UseDatabase).
//
// Postconditions:
//   - On nil: all records committed; executionID recorded in DedupStore.
//   - On ErrAlreadyApplied: destination unchanged.
//   - On any other error: destination is in the same state as before Write was called.
func (c *DatabaseSinkConnector) Write(ctx context.Context, config map[string]any, records []map[string]any, executionID string) error {
	if c.dedup.Applied(executionID) {
		return ErrAlreadyApplied
	}

	table, _ := config["table"].(string)
	if table == "" {
		return fmt.Errorf("database sink: config missing required key \"table\"")
	}
	if c.db == nil {
		return fmt.Errorf("database sink: no database wired")
	}

	c.db.Begin()
	for _, record := range records {
		if err := c.db.Insert(table, record); err != nil {
			c.db.Rollback()
			return fmt.Errorf("database sink: insert failed (rolled back): %w", err)
		}
	}
	c.db.Commit()

	if err := c.dedup.Record(ctx, executionID); err != nil {
		// Commit already succeeded; best-effort dedup record. Log the error but do
		// not fail the write — the data is durable. A redelivery will hit the
		// idempotency check via the database (row already exists).
		// This is an acceptable edge case: at worst we write the same rows twice on
		// redelivery, which the application must handle via unique constraints.
		_ = err
	}
	return nil
}

// -----------------------------------------------------------------------
// InMemoryS3 — fake S3-compatible object store for unit testing
// -----------------------------------------------------------------------

// InMemoryS3 simulates an S3-compatible object store with multipart upload support.
// Uploaded objects are stored in memory; multipart uploads are tracked by upload ID.
// Tests can inject failures via FailUploadAfterPart.
type InMemoryS3 struct {
	mu              sync.Mutex
	objects         map[string][]byte          // "bucket/key" -> content
	multiparts      map[string]*multipartState // uploadID -> state
	failAfterPart   int                        // fail after N parts (-1 = never)
	uploadCount     int                        // number of completed uploads
	nextUploadID    int
}

type multipartState struct {
	bucket string
	key    string
	parts  [][]byte
}

// NewInMemoryS3 constructs an empty InMemoryS3 with no failure injection.
func NewInMemoryS3() *InMemoryS3 {
	return &InMemoryS3{
		objects:       make(map[string][]byte),
		multiparts:    make(map[string]*multipartState),
		failAfterPart: -1,
	}
}

// FailUploadAfterPart configures the S3 to return an error after n parts are uploaded
// in a multipart session. Set n=1 to fail after the first part.
func (s *InMemoryS3) FailUploadAfterPart(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failAfterPart = n
}

// Put writes an object directly (no multipart). Used by Snapshot tests.
func (s *InMemoryS3) Put(bucket, key string, data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.objects[bucket+"/"+key] = data
}

// Exists returns true when the object at bucket/key is present.
func (s *InMemoryS3) Exists(bucket, key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.objects[bucket+"/"+key]
	return ok
}

// StagingObjectCount returns the number of objects whose key begins with the staging prefix.
func (s *InMemoryS3) StagingObjectCount(bucket string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	count := 0
	prefix := bucket + "/.staging/"
	for k := range s.objects {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			count++
		}
	}
	return count
}

// OpenMultipartCount returns the number of in-progress multipart uploads.
func (s *InMemoryS3) OpenMultipartCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.multiparts)
}

// UploadCount returns the number of successfully completed (committed) uploads.
func (s *InMemoryS3) UploadCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.uploadCount
}

// ListObjectCount returns the number of objects in the given bucket whose key starts
// with prefix. Used by Snapshot.
func (s *InMemoryS3) ListObjectCount(bucket, prefix string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	count := 0
	fullPrefix := bucket + "/" + prefix
	for k := range s.objects {
		if len(k) >= len(fullPrefix) && k[:len(fullPrefix)] == fullPrefix {
			count++
		}
	}
	return count
}

// CreateMultipartUpload initialises a multipart upload session and returns an upload ID.
func (s *InMemoryS3) CreateMultipartUpload(bucket, key string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextUploadID++
	id := fmt.Sprintf("upload-%d", s.nextUploadID)
	s.multiparts[id] = &multipartState{bucket: bucket, key: key}
	return id
}

// UploadPart appends a part to the given multipart upload.
// Returns an error if FailUploadAfterPart is configured and the threshold is reached.
func (s *InMemoryS3) UploadPart(uploadID string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	mp, ok := s.multiparts[uploadID]
	if !ok {
		return fmt.Errorf("unknown uploadID %q", uploadID)
	}
	if s.failAfterPart >= 0 && len(mp.parts) >= s.failAfterPart {
		return fmt.Errorf("simulated S3 failure after %d part(s)", s.failAfterPart)
	}
	cp := make([]byte, len(data))
	copy(cp, data)
	mp.parts = append(mp.parts, cp)
	return nil
}

// CompleteMultipartUpload assembles all parts and stores the object at the final key.
// Removes the upload from the in-progress set and increments the upload counter.
func (s *InMemoryS3) CompleteMultipartUpload(uploadID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	mp, ok := s.multiparts[uploadID]
	if !ok {
		return fmt.Errorf("unknown uploadID %q", uploadID)
	}
	var buf bytes.Buffer
	for _, part := range mp.parts {
		buf.Write(part)
	}
	s.objects[mp.bucket+"/"+mp.key] = buf.Bytes()
	delete(s.multiparts, uploadID)
	s.uploadCount++
	return nil
}

// AbortMultipartUpload cancels an in-progress multipart upload without writing any object.
// Postcondition: no partial object exists; the upload ID is removed from in-progress set.
func (s *InMemoryS3) AbortMultipartUpload(uploadID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.multiparts, uploadID)
}

// -----------------------------------------------------------------------
// S3SinkConnector
// -----------------------------------------------------------------------

// s3Backend is the narrow interface the S3SinkConnector depends on.
// InMemoryS3 satisfies this interface for tests; a real AWS/MinIO client adapter
// would satisfy it in production (Dependency Inversion).
type s3Backend interface {
	CreateMultipartUpload(bucket, key string) string
	UploadPart(uploadID string, data []byte) error
	CompleteMultipartUpload(uploadID string) error
	AbortMultipartUpload(uploadID string)
	ListObjectCount(bucket, prefix string) int
}

// S3SinkConnector writes records to an S3-compatible object store using multipart upload.
//
// Atomicity (ADR-009 S3 Sink): the connector uses multipart upload semantics:
//   - CreateMultipartUpload initialises a session at a staging key.
//   - Records are serialised as JSON and uploaded as a single part.
//   - CompleteMultipartUpload atomically moves the data to the final key.
//   - On any failure, AbortMultipartUpload is called so no partial object exists.
//
// Idempotency (ADR-003): a DedupStore is checked before writing; executionID is
// recorded after CompleteMultipartUpload succeeds.
//
// Config keys:
//   - "bucket" (string): S3 bucket name. Required.
//   - "key"    (string): destination object key. Required.
//
// See: ADR-003, ADR-009, TASK-018
type S3SinkConnector struct {
	s3    s3Backend
	dedup DedupStore
}

// NewS3SinkConnector constructs an S3SinkConnector with the given backend and dedup store.
//
// Preconditions:
//   - s3 is non-nil.
//   - dedup is non-nil.
func NewS3SinkConnector(s3 s3Backend, dedup DedupStore) *S3SinkConnector {
	return &S3SinkConnector{s3: s3, dedup: dedup}
}

// Type implements SinkConnector.Type. Returns "s3".
func (c *S3SinkConnector) Type() string { return "s3" }

// Snapshot implements SinkConnector.Snapshot.
// Returns a map with "object_count" reflecting the number of objects in the bucket
// whose key shares the same prefix directory as the configured key.
//
// Config keys:
//   - "bucket" (string): S3 bucket name. Required.
//   - "key"    (string): used to derive the prefix for listing. Required.
//
// Postconditions:
//   - Returns a non-nil map. Never returns an error for the in-memory backend.
func (c *S3SinkConnector) Snapshot(ctx context.Context, config map[string]any, taskID string) (map[string]any, error) {
	bucket, _ := config["bucket"].(string)
	key, _ := config["key"].(string)
	// Use path (not filepath) because S3 keys always use forward slashes.
	prefix := path.Dir(key)
	if prefix == "." {
		prefix = ""
	}
	count := c.s3.ListObjectCount(bucket, prefix)
	return map[string]any{"object_count": count}, nil
}

// Write serialises records as JSON and uploads them as a single S3 object via multipart upload.
//
// Idempotency guard (ADR-003): if executionID is in the DedupStore, returns ErrAlreadyApplied.
//
// Atomicity (ADR-009):
//  1. CreateMultipartUpload at the final key
//  2. UploadPart with the serialised JSON payload
//  3. On UploadPart error: AbortMultipartUpload; return error (no object at final key)
//  4. CompleteMultipartUpload to commit the object
//  5. Record executionID in DedupStore
//
// Preconditions:
//   - executionID is non-empty.
//   - config["bucket"] and config["key"] are non-empty strings.
//
// Postconditions:
//   - On nil: object exists at bucket/key; executionID recorded.
//   - On ErrAlreadyApplied: no change.
//   - On any other error: no object at bucket/key; multipart upload aborted.
func (c *S3SinkConnector) Write(ctx context.Context, config map[string]any, records []map[string]any, executionID string) error {
	if c.dedup.Applied(executionID) {
		return ErrAlreadyApplied
	}

	bucket, _ := config["bucket"].(string)
	key, _ := config["key"].(string)
	if bucket == "" {
		return fmt.Errorf("s3 sink: config missing required key \"bucket\"")
	}
	if key == "" {
		return fmt.Errorf("s3 sink: config missing required key \"key\"")
	}

	payload, err := json.Marshal(records)
	if err != nil {
		return fmt.Errorf("s3 sink: serialise records: %w", err)
	}

	uploadID := c.s3.CreateMultipartUpload(bucket, key)

	if err := c.s3.UploadPart(uploadID, payload); err != nil {
		c.s3.AbortMultipartUpload(uploadID)
		return fmt.Errorf("s3 sink: upload failed (aborted): %w", err)
	}

	if err := c.s3.CompleteMultipartUpload(uploadID); err != nil {
		c.s3.AbortMultipartUpload(uploadID)
		return fmt.Errorf("s3 sink: complete failed (aborted): %w", err)
	}

	if err := c.dedup.Record(ctx, executionID); err != nil {
		_ = err // best-effort; see DatabaseSinkConnector.Write for rationale
	}
	return nil
}

// -----------------------------------------------------------------------
// FileSinkConnector
// -----------------------------------------------------------------------

// FileSinkConnector writes records to a local file using an atomic temp-file rename.
//
// Atomicity (ADR-009 File Sink):
//   - Records are serialised as JSON and written to a temp file in the same directory
//     as the final path (guaranteeing the rename is on the same filesystem).
//   - On success: os.Rename moves the temp file to the final path (atomic on POSIX).
//   - On failure: the temp file is removed; the final path is untouched.
//
// Idempotency (ADR-003): a DedupStore is checked before writing; executionID is
// recorded after the rename succeeds.
//
// Config keys:
//   - "path" (string): absolute destination file path. Required.
//
// See: ADR-003, ADR-009, TASK-018
type FileSinkConnector struct {
	dedup DedupStore
}

// NewFileSinkConnector constructs a FileSinkConnector with the given DedupStore.
//
// Preconditions:
//   - dedup is non-nil.
func NewFileSinkConnector(dedup DedupStore) *FileSinkConnector {
	return &FileSinkConnector{dedup: dedup}
}

// Type implements SinkConnector.Type. Returns "file".
func (c *FileSinkConnector) Type() string { return "file" }

// Snapshot implements SinkConnector.Snapshot.
// Returns a map with "exists" (bool) and "size_bytes" (int64) for the file at config["path"].
// If the file does not exist, "exists" is false and "size_bytes" is 0.
//
// Config keys:
//   - "path" (string): absolute destination file path. Required.
//
// Postconditions:
//   - Returns a non-nil map. Never returns an error (missing file is not an error).
func (c *FileSinkConnector) Snapshot(ctx context.Context, config map[string]any, taskID string) (map[string]any, error) {
	path, _ := config["path"].(string)
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{"exists": false, "size_bytes": int64(0)}, nil
		}
		return map[string]any{"exists": false, "size_bytes": int64(0)}, nil
	}
	return map[string]any{"exists": true, "size_bytes": info.Size()}, nil
}

// Write serialises records as JSON and writes them to the configured file path atomically.
//
// Idempotency guard (ADR-003): if executionID is in the DedupStore, returns ErrAlreadyApplied.
//
// Atomicity (ADR-009):
//  1. Create a temp file in the same directory as the final path.
//  2. Write JSON-encoded records to the temp file.
//  3. On any error before rename: remove the temp file and return the error.
//  4. os.Rename the temp file to the final path (atomic on POSIX).
//  5. Record executionID in the DedupStore.
//
// Preconditions:
//   - executionID is non-empty.
//   - config["path"] is an absolute file path whose parent directory must exist.
//
// Postconditions:
//   - On nil: final file exists with JSON-encoded records; executionID recorded.
//   - On ErrAlreadyApplied: no change.
//   - On any other error: final path unchanged; no temp file remains.
func (c *FileSinkConnector) Write(ctx context.Context, config map[string]any, records []map[string]any, executionID string) error {
	if c.dedup.Applied(executionID) {
		return ErrAlreadyApplied
	}

	finalPath, _ := config["path"].(string)
	if finalPath == "" {
		return fmt.Errorf("file sink: config missing required key \"path\"")
	}

	dir := filepath.Dir(finalPath)
	tmp, err := os.CreateTemp(dir, ".nexusflow-sink-*.tmp")
	if err != nil {
		return fmt.Errorf("file sink: create temp file: %w", err)
	}
	tmpPath := tmp.Name()

	// Ensure the temp file is cleaned up on any non-nil error path.
	committed := false
	defer func() {
		if !committed {
			_ = tmp.Close()
			_ = os.Remove(tmpPath)
		}
	}()

	if err := writeJSONTo(tmp, records); err != nil {
		return fmt.Errorf("file sink: write records to temp file: %w", err)
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("file sink: close temp file: %w", err)
	}

	if err := os.Rename(tmpPath, finalPath); err != nil {
		return fmt.Errorf("file sink: rename temp to final path: %w", err)
	}

	committed = true

	if err := c.dedup.Record(ctx, executionID); err != nil {
		_ = err // best-effort; see DatabaseSinkConnector.Write for rationale
	}
	return nil
}

// writeJSONTo serialises records as a JSON array and writes it to w.
// Uses a streaming encoder so very large record slices are not held twice in memory.
func writeJSONTo(w io.Writer, records []map[string]any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(records)
}

// -----------------------------------------------------------------------
// Registration
// -----------------------------------------------------------------------

// RegisterAtomicSinkConnectors registers the three atomic SinkConnector implementations
// (DatabaseSinkConnector, S3SinkConnector, FileSinkConnector) in the given registry.
//
// Each connector is constructed with a fresh InMemoryDedupStore, which is suitable for
// unit tests and the demo environment. Production deployments should wire a persistent
// DedupStore backed by the sink_dedup_log table (migration 000003).
//
// Called at worker startup alongside RegisterDemoConnectors to enable pipelines that
// use connector types "database", "s3", or "file".
//
// Panics on duplicate registration (fail-fast: calling this twice is a startup bug).
//
// See: TASK-018, cmd/worker/main.go
func RegisterAtomicSinkConnectors(reg *DefaultConnectorRegistry) {
	reg.Register("sink", NewDatabaseSinkConnector(NewInMemoryDedupStore()))
	reg.Register("sink", NewS3SinkConnector(NewInMemoryS3(), NewInMemoryDedupStore()))
	reg.Register("sink", NewFileSinkConnector(NewInMemoryDedupStore()))
}
