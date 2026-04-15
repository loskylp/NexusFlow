# Builder Handoff — TASK-030 Wiring Fix
**Date:** 2026-04-15
**Task:** Wire RegisterMinIOConnectors into cmd/worker/main.go
**Requirement(s):** DEMO-001, ADR-009, TASK-030

## What Was Implemented

### `worker/minio_client.go` — new file
`MinioClientAdapter` wraps a `*minio.Client` (minio-go/v7) and satisfies the
`minioBackend` interface. Key design decisions:

- `CreateMultipartUpload` / `UploadPart` / `CompleteMultipartUpload` map to minio-go's
  high-level `PutObject`. Data is buffered in memory between Create and Complete; on
  Complete, PutObject is called with the full payload. This is safe because
  `MinIOSinkConnector.Write` uploads exactly one part (a single JSON array).
- `AbortMultipartUpload` discards the buffered payload with no network call.
- `GetObject` calls `client.GetObject` and drains the response via `io.ReadAll`.
- `ListKeys` and `ListObjectCount` use `client.ListObjects` with `Recursive: true`.
- The struct is safe for concurrent use: in-flight upload state is guarded by a mutex;
  the minio-go client is goroutine-safe per its documentation.

### `cmd/worker/main.go` — updated
Added `registerMinIOConnectors(reg)` call in the connector registration section.
The helper function:
- Reads `MINIO_ENDPOINT`, `MINIO_ROOT_USER`, `MINIO_ROOT_PASSWORD` from env.
- If `MINIO_ENDPOINT` is empty: logs a warning and returns nil (worker starts without
  MinIO; non-demo deployments are unaffected).
- Parses the endpoint URL to derive the bare `host:port` and `useSSL` flag.
- Constructs a `MinioClientAdapter` and calls `RegisterMinIOConnectors`.

### `go.mod` / `go.sum`
Added `github.com/minio/minio-go/v7 v7.0.91` as a direct dependency.
v7.0.91 was selected because v7.0.100 (latest) requires Go 1.25 but the project
targets Go 1.23.

## Unit Tests
- Tests added: 0 (the adapter is a thin I/O wrapper; unit tests require a live
  MinIO container and belong in integration tests)
- Existing tests: 9 (TestMinIO*) — all pass

## Deviations

1. **Adapter uses PutObject instead of the low-level multipart API** — minio-go/v7
   does not expose `CreateMultipartUpload` / `CompleteMultipartUpload` at the high
   level. The workaround (buffer + PutObject) is semantically equivalent for the
   single-part upload pattern used by this connector.

2. **v7.0.91 not latest** — v7.0.100 requires Go 1.25 which is above the project's
   `go 1.23` directive. v7.0.91 is the most recent version compatible with Go 1.23.

## Known Limitations

1. **No unit tests for MinioClientAdapter** — the adapter talks directly to the
   MinIO network API. Its correctness is verified by integration tests against a
   live MinIO container (Verifier domain).

2. **In-memory buffering for multipart uploads** — all payload bytes are held in
   memory until CompleteMultipartUpload is called. For the current use case (JSON
   array of pipeline records) this is fine. Very large record sets could exceed
   available heap; a streaming approach would be needed if payloads grow.

## For the Verifier

1. `go build ./cmd/worker/` compiles cleanly.
2. `go test ./worker/ -run TestMinIO -v` — 9 tests, all PASS.
3. Integration check: `docker compose --profile demo up` — worker log should contain
   `worker: MinIO connectors registered (endpoint=minio:9000 ssl=false)` on startup.
4. With MINIO_ENDPOINT unset: worker log should contain
   `worker: MINIO_ENDPOINT not set — MinIO connectors not registered` and startup
   should complete normally.
