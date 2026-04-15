<!-- SPDX-License-Identifier: Apache-2.0 -->

# Builder Handoff — TASK-033
**Date:** 2026-04-15
**Task:** Sink Before/After snapshot capture
**Requirement(s):** DEMO-003, ADR-009

## What Was Implemented

### `worker/snapshot.go` (rewritten from scaffold)

- `SnapshotCapturer` struct wrapping a `SinkConnector` and a `snapshotPublisher`.
- `NewSnapshotCapturer(connector, publisher)` — fail-fast precondition guards (panics on nil).
- `CaptureAndWrite(ctx, config, records, executionID, taskID)` — full snapshot-capture cycle:
  1. Calls `connector.Snapshot` to capture the Before state.
  2. Publishes `sink:before-snapshot` event to `events:sink:{taskID}`.
  3. Calls `connector.Write` with the provided records.
  4. Calls `connector.Snapshot` again to capture the After state (regardless of write outcome).
  5. Publishes `sink:after-result` event with Before, After, RolledBack, and WriteError fields.
- `publishEvent` — internal helper that serialises a `sinkSnapshotEvent` as JSON and publishes it; errors are logged and discarded (fire-and-forget per ADR-007).
- `sinkChannelName(taskID)` — returns `events:sink:{taskID}`.
- Removed all three `//lint:ignore U1000` scaffold suppressions (fields and struct are now live).

### `worker/worker.go`

- Added `snapshotPub snapshotPublisher` field to `Worker`.
- Added `WithSnapshotPublisher(pub snapshotPublisher) *Worker` fluent setter.
- Updated `runSink` to create a `SnapshotCapturer` when `snapshotPub` is wired; falls back to direct `connector.Write` when it is nil (backward-compatible with existing unit tests that do not wire a publisher).

### `cmd/worker/main.go`

- Wired `redisClient` as the snapshot publisher via `.WithSnapshotPublisher(redisClient)`.
- `*redis.Client` satisfies `snapshotPublisher` structurally (it has `Publish(ctx, channel, message any) *redis.IntCmd`).
- The nil-wiring gap noted in the project memory (`feedback_nil_wiring.md`) is resolved: the field is non-nil in production.

### `worker/snapshot_test.go` (new)

17 unit tests covering all 6 acceptance criteria. See Unit Tests section below.

## Unit Tests

- Tests written: 17
- All passing: yes
- `go test ./... -count=1` — green across all packages
- `go vet ./...` — clean

Key behaviours covered:

| Test | AC |
|---|---|
| `PublishesBeforeEvent` | AC1, AC3 — before-snapshot event published to `events:sink:{taskId}` |
| `PublishesAfterEvent` | AC2, AC3 — after-result event published after write |
| `ReturnsWriteError` | AC2 — write error returned; both events still published on failure |
| `DatabaseSink_BeforeSnapshotReflectsRowCount` | AC4 — Before `row_count` matches pre-write table state |
| `DatabaseSink_AfterSnapshotReflectsNewRowCount` | AC4 — After `row_count` incremented after commit |
| `S3Sink_BeforeSnapshotReflectsObjectCount` | AC5 — Before `object_count` reflects prefix objects |
| `S3Sink_AfterSnapshotReflectsNewObjectCount` | AC5 — After `object_count` incremented after upload |
| `DatabaseSink_AfterMatchesBeforeOnRollback` | AC6 — After `row_count` equals Before on rollback; `rolledBack=true`; `writeError` non-empty |
| `S3Sink_AfterMatchesBeforeOnRollback` | AC6 — After `object_count` equals Before on S3 abort |
| `BeforePublishedBeforeWrite` | AC1 — Before snapshot captured with pre-write state (ordering guarantee) |
| `SnapshotPhaseLabels` | — Before.phase="before", After.phase="after" |
| `SnapshotsCapturedAtIsSet` | — RFC3339 CapturedAt timestamps in both snapshots |
| `NilConnectorPanics` | — fail-fast precondition |
| `NilPublisherPanics` | — fail-fast precondition |

The `inMemoryPublisher` fake in the test file implements `snapshotPublisher` structurally — no Redis instance required.

## Deviations from Task Description

None. All 6 acceptance criteria implemented as specified.

## Known Limitations

- Snapshot publish errors are fire-and-forget (logged, not surfaced). This is per the existing ADR-007 pattern for SSE events and explicitly stated in the `CaptureAndWrite` contract.
- The `snapshotPublisher` interface is unexported. Callers in `main.go` pass `*redis.Client`, which satisfies it structurally. If a future task needs to mock the publisher from outside the worker package, the interface will need to be exported at that time.

## For the Verifier

- AC1–AC6 each have direct unit test coverage. The test names map to ACs in the table above.
- The `Worker.runSink` path change in `worker.go` is exercised by existing `executor_test.go` tests (which don't wire a publisher, so the nil-guard path runs). The snapshot path is exercised by the new `snapshot_test.go` tests.
- The `cmd/worker/main.go` wiring is not directly unit-tested (no test files for `cmd/worker`), but it is verified by `go build ./...` succeeding.
- No integration tests were written in this session; those are the Verifier's domain.
