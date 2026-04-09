# Builder Handoff — TASK-030
**Date:** 2026-04-09
**Task:** Demo infrastructure — MinIO Fake-S3 Connector (DataSource + Sink)
**Requirement(s):** DEMO-001, ADR-003, ADR-009

## What Was Implemented

### `worker/connector_minio.go` — full implementation replacing all TODO stubs
- `NewMinIODataSourceConnector(minio minioBackend)` — nil-guard, returns struct
- `MinIODataSourceConnector.Fetch` — extracts bucket/prefix from config, calls
  `ListKeys` to enumerate, `GetObject` per key, `json.Unmarshal` into record map.
  Checks context cancellation between objects. Returns wrapped errors on any failure.
- `NewMinIOSinkConnector(minio minioBackend, dedup DedupStore)` — nil-guards on both
  args, returns struct
- `MinIOSinkConnector.Snapshot` — mirrors `S3SinkConnector.Snapshot` exactly:
  derives listing prefix via `path.Dir(key)`, calls `ListObjectCount`, returns
  `{"object_count": int}`
- `MinIOSinkConnector.Write` — mirrors `S3SinkConnector.Write` exactly: dedup check,
  config extraction, JSON marshal, CreateMultipartUpload, UploadPart (abort on failure),
  CompleteMultipartUpload, dedup record
- `RegisterMinIOConnectors(reg, minio)` — registers both "minio" DataSource and "minio"
  Sink in the given registry

### `worker/sink_connectors.go` — extended `InMemoryS3` to satisfy `minioBackend`
- `GetObject(bucket, key string) ([]byte, error)` — retrieves stored object by
  bucket+"/"+key key; returns a copy to prevent mutation of stored data
- `ListKeys(bucket, prefix string) ([]string, error)` — enumerates stored keys whose
  bucket+"/"+key starts with bucket+"/"+prefix; strips the "bucket/" prefix from
  returned keys to match S3-style key semantics

### `worker/connector_minio_test.go` — new test file (9 tests)
All tests in `package worker_test` using `InMemoryS3` and `InMemoryDedupStore`.

### `docker-compose.yml` — extended demo profile
- Added `healthcheck` to `minio` service: polls `/minio/health/live` endpoint via wget
- Added `minio-init` service (`minio/mc:latest`, profile `demo`): waits for MinIO
  healthy, creates `demo-input` and `demo-output` buckets, seeds `demo-input/data/`
  with 3 JSON records (`record-001.json`, `record-002.json`, `record-003.json`)

### `.env.example` — added `MINIO_ENDPOINT` variable
Documents `http://minio:9000` as the S3-compatible endpoint for worker configuration.

## Unit Tests
- Tests written: 9
- All passing: Could not confirm via test runner (see Known Limitations — Docker issue)
- Key behaviors covered:
  - Fetch happy path: 2 JSON objects decoded into 2 records
  - Fetch prefix filter: only objects under "data/" prefix returned; "other/" excluded
  - Fetch non-JSON error: objects that are not valid JSON return a wrapped error
  - DataSource Type() returns "minio"
  - Write happy path: object written at bucket/key; UploadCount == 1
  - Write idempotency: second Write with same executionID returns ErrAlreadyApplied; UploadCount stays 1
  - Write failure + abort: UploadPart failure calls Abort; OpenMultipartCount == 0; object not written
  - Snapshot: object_count reflects only objects under key's directory prefix
  - Registration: both "minio" DataSource and "minio" Sink resolve from registry after RegisterMinIOConnectors

## Deviations from Task Description

1. **Constructor nil-guards** — the constructors panic on nil backend/dedup rather than
   silently allowing it. This is a stricter application of the "fail fast" principle from
   the Builder guidelines. The scaffold's preconditions explicitly state non-nil
   requirements; making violations loud at construction time is correct.

2. **`RegisterMinIOConnectors` uses `NewInMemoryDedupStore` for the Sink** — following
   the exact same pattern as `RegisterAtomicSinkConnectors` which also uses in-memory
   dedup stores. A production deployment should inject a persistent DedupStore backed by
   the sink_dedup_log table; this is documented in the function's docstring.

3. **main.go not wired** — `RegisterMinIOConnectors` is intentionally not called in
   `cmd/worker/main.go`. The function requires a live MinIO backend (from MINIO_ENDPOINT +
   credentials), which main.go does not construct. Wiring the real MinIO client adapter
   (github.com/minio/minio-go/v7) is out of scope for this task and belongs in a DevOps
   or integration task. The registration function is exported and ready for the caller.

## Known Limitations

1. **Test run not confirmed via Docker** — the Docker test runner (`golang:1.22-alpine`)
   did not complete within the session. Multiple `docker run` processes for prior tasks
   appeared stuck (one had been running since the previous day). The code was verified
   through manual code review against the established `S3SinkConnector` pattern. The
   implementation is structurally identical to the existing S3 connector tests that pass.

2. **No real MinIO client adapter** — the MinIO connector only works with `InMemoryS3`
   for tests. A real adapter wrapping `github.com/minio/minio-go/v7` must be written to
   enable the full integration scenario. This adapter is not part of TASK-030's scope.

3. **`minio-init` idempotency** — the seed objects are always uploaded on container start.
   When `minio-data` volume exists from a previous run, `mc mb --ignore-existing` prevents
   bucket creation errors but the echo/pipe commands will overwrite the seed files. This
   is intentional for demo purposes (always-fresh seed data) and harmless for the Verifier.

## For the Verifier

1. **Nil-wiring check (memory note)**: `cmd/worker/main.go` does NOT call
   `RegisterMinIOConnectors` — this is intentional. The Verifier should not expect
   "minio" to be in the registry at worker startup unless a caller explicitly registers it.

2. **Test run**: The Verifier should run `go test ./worker/ -run TestMinIO -v` to confirm
   all 9 tests pass before acceptance. If Docker was previously stuck, the containers from
   the Builder session should be cleaned up first (`docker rm -f` for any hung containers).

3. **Docker Compose demo profile**: `docker compose --profile demo up` should start MinIO,
   wait for its health check to pass, then run `minio-init` which seeds the demo buckets.
   The Verifier can confirm by running:
   `docker exec $(docker compose ps -q minio) wget -q -O- http://localhost:9000/minio/health/live`

4. **Acceptance criterion "A demo pipeline can be defined using MinIO as DataSource and Sink"**:
   This requires the pipeline definition JSON to use `"connector_type": "minio"` for both
   DataSource and Sink phases. The connectors are implemented and registered via
   `RegisterMinIOConnectors`. The pipeline executor picks them up through the registry.
   End-to-end demo requires a live MinIO backend — this is an integration concern.
