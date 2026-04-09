// Package worker — Sink snapshot capture (TASK-033).
//
// SnapshotCapturer is responsible for capturing the Before and After state of a
// Sink destination around a Sink phase execution. Both snapshots are stored in
// the task execution record and published to the events:sink:{taskId} Redis
// Pub/Sub channel for SSE consumption by the Sink Inspector GUI.
//
// The Before snapshot is taken by calling SinkConnector.Snapshot before Write
// begins. The After snapshot is taken after Write returns (success or rollback).
//
// See: DEMO-003, ADR-009, TASK-033
package worker

import (
	"context"

	"github.com/nxlabs/nexusflow/internal/models"
	"github.com/redis/go-redis/v9"
)

// sinkEventType enumerates the SSE event types published to events:sink:{taskId}.
// The Sink Inspector GUI subscribes to these event types via the SSE endpoint.
// See: ADR-007, TASK-033, TASK-032
const (
	// SinkEventBeforeSnapshot is published when the Before snapshot has been captured,
	// just before the Sink write phase begins.
	SinkEventBeforeSnapshot = "sink:before-snapshot"

	// SinkEventAfterResult is published when the Sink write has completed (success or
	// rollback) and the After snapshot has been captured.
	SinkEventAfterResult = "sink:after-result"
)

// SnapshotCapturer captures Before/After sink snapshots and publishes them via
// Redis Pub/Sub for the Sink Inspector.
//
// It wraps a SinkConnector and intercepts the Write call to capture snapshots
// around the execution boundary.
//
// Thread safety: SnapshotCapturer is safe for single-task concurrent use; each
// Worker goroutine creates its own SnapshotCapturer per task execution.
//
// See: ADR-009, TASK-033
type SnapshotCapturer struct {
	connector SinkConnector
	publisher snapshotPublisher
}

// snapshotPublisher is the narrow interface SnapshotCapturer depends on for
// publishing snapshot events. Satisfied by *redis.Client in production and by
// an in-memory stub in unit tests.
type snapshotPublisher interface {
	// Publish publishes msg to the given Redis Pub/Sub channel.
	// Returns an error if publishing fails.
	Publish(ctx context.Context, channel string, message any) *redis.IntCmd
}

// NewSnapshotCapturer constructs a SnapshotCapturer that wraps the given
// SinkConnector and publishes snapshots via the given publisher.
//
// Preconditions:
//   - connector is non-nil.
//   - publisher is non-nil.
func NewSnapshotCapturer(connector SinkConnector, publisher snapshotPublisher) *SnapshotCapturer {
	// TODO: implement
	panic("not implemented")
}

// CaptureAndWrite executes the full Sink phase with snapshot capture:
//
//  1. Call connector.Snapshot to capture the Before state.
//  2. Publish a sink:before-snapshot event to events:sink:{taskID}.
//  3. Call connector.Write with the given records and executionID.
//  4. Call connector.Snapshot again to capture the After state (regardless of write outcome).
//  5. Publish a sink:after-result event to events:sink:{taskID} with both
//     Before and After snapshots and the write error (nil on success).
//
// The write error from step 3 is always returned to the caller after step 5
// completes. If snapshot capture or publishing fails, those errors are logged
// but do not affect the write result or the task's success/failure status.
//
// Args:
//   - ctx:         Execution context. Cancellation propagates to the underlying connector.
//   - config:      SinkConfig.Config from the pipeline definition.
//   - records:     Records after Process->Sink schema mapping.
//   - executionID: Unique identifier for this execution attempt.
//   - taskID:      The task being executed; used to scope snapshots and channel name.
//
// Returns:
//   - nil on successful write (records durably written).
//   - ErrAlreadyApplied if the executionID was already committed (idempotent no-op).
//   - The underlying connector error on write failure (destination unchanged).
//
// Preconditions:
//   - executionID is non-empty.
//   - taskID is a valid UUID string.
//
// Postconditions:
//   - On nil: records written; Before and After snapshots published to
//             events:sink:{taskID}; After snapshot differs from Before.
//   - On error: rollback completed by the underlying connector; Before and After
//               snapshots published to events:sink:{taskID}; After matches Before
//               (confirming rollback succeeded).
func (s *SnapshotCapturer) CaptureAndWrite(
	ctx context.Context,
	config map[string]any,
	records []map[string]any,
	executionID string,
	taskID string,
) error {
	// TODO: implement
	panic("not implemented")
}

// sinkSnapshotEvent is the payload published to events:sink:{taskID}.
// The Sink Inspector GUI receives this via SSE and populates its Before/After panels.
// See: DEMO-003, ADR-007, TASK-032, TASK-033
type sinkSnapshotEvent struct {
	// EventType is either SinkEventBeforeSnapshot or SinkEventAfterResult.
	EventType string `json:"eventType"`

	// TaskID is the UUID of the task whose Sink phase generated this snapshot.
	TaskID string `json:"taskId"`

	// Before is the destination state before the Sink write began.
	// Non-nil for both SinkEventBeforeSnapshot and SinkEventAfterResult events.
	Before *models.SinkSnapshot `json:"before,omitempty"`

	// After is the destination state after the Sink write completed.
	// Non-nil only for SinkEventAfterResult events.
	After *models.SinkSnapshot `json:"after,omitempty"`

	// RolledBack is true when the Sink write failed and the destination was
	// restored to the Before state. Set only on SinkEventAfterResult events.
	RolledBack bool `json:"rolledBack"`

	// WriteError is the error message from the failed write, if any.
	// Empty string on success.
	WriteError string `json:"writeError,omitempty"`
}
