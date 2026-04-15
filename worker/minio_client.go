// Package worker — real MinIO client adapter (TASK-030 wiring).
//
// MinioClientAdapter wraps a github.com/minio/minio-go/v7 client and satisfies
// the minioBackend interface so MinIODataSourceConnector and MinIOSinkConnector
// can be wired to a live MinIO container at worker startup.
//
// Multipart upload semantics are mapped to the minio-go high-level PutObject API:
//   - CreateMultipartUpload records the target bucket and key, returns an upload ID.
//   - UploadPart buffers the payload in memory (single-part only; our payloads are
//     serialised JSON arrays — well within single-request size limits).
//   - CompleteMultipartUpload calls PutObject with the buffered payload.
//   - AbortMultipartUpload discards the buffered payload without writing anything.
//
// This mapping is safe because the connector serialises all records as a single JSON
// array and uploads exactly one part per write (see MinIOSinkConnector.Write).
//
// See: DEMO-001, ADR-009, TASK-030
package worker

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
	"sync/atomic"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// minioUploadState holds the in-flight state for one simulated multipart upload.
type minioUploadState struct {
	bucket string
	key    string
	data   []byte
}

// MinioClientAdapter wraps a *minio.Client and satisfies the minioBackend interface.
// It is safe for concurrent use: in-flight uploads are protected by a mutex, and the
// underlying minio.Client is goroutine-safe per the minio-go documentation.
type MinioClientAdapter struct {
	client    *minio.Client
	mu        sync.Mutex
	uploads   map[string]*minioUploadState
	nextID    atomic.Int64
}

// NewMinioClientAdapter constructs a MinioClientAdapter connected to endpoint
// with the given access and secret keys.
//
// endpoint must include the host and port (e.g. "minio:9000") without a scheme.
// useSSL controls whether TLS is used; set false for local/dev environments.
//
// Preconditions:
//   - endpoint is non-empty.
//   - accessKey and secretKey are non-empty.
//
// Postconditions:
//   - On nil error: returned adapter is ready to use; the client has been
//     initialised but no network call has been made.
//   - On error: returned adapter is nil; the error describes the initialisation failure.
func NewMinioClientAdapter(endpoint, accessKey, secretKey string, useSSL bool) (*MinioClientAdapter, error) {
	if endpoint == "" {
		return nil, fmt.Errorf("NewMinioClientAdapter: endpoint must not be empty")
	}
	if accessKey == "" {
		return nil, fmt.Errorf("NewMinioClientAdapter: accessKey must not be empty")
	}
	if secretKey == "" {
		return nil, fmt.Errorf("NewMinioClientAdapter: secretKey must not be empty")
	}

	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("NewMinioClientAdapter: initialise minio client: %w", err)
	}

	return &MinioClientAdapter{
		client:  client,
		uploads: make(map[string]*minioUploadState),
	}, nil
}

// ListObjectCount returns the number of objects in bucket whose key starts with prefix.
// Implements minioBackend.ListObjectCount for Snapshot support (ADR-009).
func (a *MinioClientAdapter) ListObjectCount(bucket, prefix string) int {
	ctx := context.Background()
	count := 0
	for range a.client.ListObjects(ctx, bucket, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	}) {
		count++
	}
	return count
}

// CreateMultipartUpload records the target bucket and key, allocates a new upload
// ID, and returns it. No network call is made at this point.
// Implements minioBackend.CreateMultipartUpload.
func (a *MinioClientAdapter) CreateMultipartUpload(bucket, key string) string {
	id := fmt.Sprintf("minio-upload-%d", a.nextID.Add(1))
	a.mu.Lock()
	a.uploads[id] = &minioUploadState{bucket: bucket, key: key}
	a.mu.Unlock()
	return id
}

// UploadPart buffers data for the given upload ID.
// A second call with the same uploadID appends to any existing buffer.
// Returns an error if the uploadID is not known.
// Implements minioBackend.UploadPart.
func (a *MinioClientAdapter) UploadPart(uploadID string, data []byte) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	st, ok := a.uploads[uploadID]
	if !ok {
		return fmt.Errorf("MinioClientAdapter.UploadPart: unknown uploadID %q", uploadID)
	}
	st.data = append(st.data, data...)
	return nil
}

// CompleteMultipartUpload uploads the buffered payload to MinIO via PutObject and
// removes the upload entry from the in-flight map.
// Returns an error if the uploadID is not known or if PutObject fails.
// Implements minioBackend.CompleteMultipartUpload.
func (a *MinioClientAdapter) CompleteMultipartUpload(uploadID string) error {
	a.mu.Lock()
	st, ok := a.uploads[uploadID]
	if !ok {
		a.mu.Unlock()
		return fmt.Errorf("MinioClientAdapter.CompleteMultipartUpload: unknown uploadID %q", uploadID)
	}
	payload := st.data
	bucket := st.bucket
	key := st.key
	delete(a.uploads, uploadID)
	a.mu.Unlock()

	_, err := a.client.PutObject(
		context.Background(),
		bucket,
		key,
		bytes.NewReader(payload),
		int64(len(payload)),
		minio.PutObjectOptions{ContentType: "application/json"},
	)
	if err != nil {
		return fmt.Errorf("MinioClientAdapter.CompleteMultipartUpload: PutObject %s/%s: %w", bucket, key, err)
	}
	return nil
}

// AbortMultipartUpload discards the buffered payload for the given upload ID.
// No network call is made; any in-progress data is simply dropped.
// Implements minioBackend.AbortMultipartUpload.
func (a *MinioClientAdapter) AbortMultipartUpload(uploadID string) {
	a.mu.Lock()
	delete(a.uploads, uploadID)
	a.mu.Unlock()
}

// GetObject retrieves the raw bytes of a single object at bucket/key.
// Returns nil and an error if the object does not exist or the download fails.
// Implements minioBackend.GetObject.
func (a *MinioClientAdapter) GetObject(bucket, key string) ([]byte, error) {
	obj, err := a.client.GetObject(context.Background(), bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("MinioClientAdapter.GetObject: %s/%s: %w", bucket, key, err)
	}
	defer obj.Close()

	data, err := io.ReadAll(obj)
	if err != nil {
		return nil, fmt.Errorf("MinioClientAdapter.GetObject: read %s/%s: %w", bucket, key, err)
	}
	return data, nil
}

// ListKeys returns the object keys in bucket whose key starts with prefix.
// Returns an empty slice (not an error) when no matching objects exist.
// Implements minioBackend.ListKeys.
func (a *MinioClientAdapter) ListKeys(bucket, prefix string) ([]string, error) {
	ctx := context.Background()
	var keys []string
	for obj := range a.client.ListObjects(ctx, bucket, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	}) {
		if obj.Err != nil {
			return nil, fmt.Errorf("MinioClientAdapter.ListKeys: listing %s/%s: %w", bucket, prefix, obj.Err)
		}
		keys = append(keys, obj.Key)
	}
	return keys, nil
}
