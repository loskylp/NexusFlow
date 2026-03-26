// Package queue provides the Redis Streams abstraction for task enqueuing and consumption.
// Stream naming convention:
//   - Task queues:    queue:{tag}        — one stream per capability tag
//   - Dead letter:   queue:dead-letter   — terminally failed tasks
//   - Worker beats:  workers:active      — sorted set used by Monitor (not a stream)
//
// See: ADR-001, ADR-003, TASK-004
package queue

import (
	"context"
	"time"

	"github.com/nxlabs/nexusflow/internal/models"
)

// TaskMessage is the envelope placed on a Redis stream via XADD.
// It carries enough information for a Worker to locate the Task in PostgreSQL
// and execute the Pipeline without further API calls.
// See: ADR-001, TASK-004, TASK-007
type TaskMessage struct {
	TaskID     string `json:"taskId"`
	PipelineID string `json:"pipelineId"`
	UserID     string `json:"userId"`
	// ExecutionID is task_id + ":" + attempt_number, used for Sink idempotency (ADR-003).
	ExecutionID string `json:"executionId"`
	// StreamID is the Redis Streams message ID assigned by XADD. Populated on read.
	StreamID string `json:"-"`
}

// ProducerMessage is the input to Enqueue.
type ProducerMessage struct {
	Task *models.Task
	Tags []string
}

// Producer enqueues tasks onto the Redis Streams layer.
// Implementations must map each tag to its stream (queue:{tag}) via XADD
// and ensure consumer groups are initialized on first use.
// See: ADR-001, TASK-004, TASK-005
type Producer interface {
	// Enqueue adds a task to the per-tag stream for each of its required tags.
	// If a stream or consumer group does not exist, it is created.
	//
	// Args:
	//   ctx:     Request context; cancellation aborts the enqueue.
	//   message: Task and associated tags. Tags must not be empty.
	//
	// Returns:
	//   A slice of Redis stream IDs (one per tag stream the task was added to).
	//   An error if any XADD fails; the task should not be marked "queued" on error.
	//
	// Preconditions:
	//   - message.Tags is non-empty.
	//   - message.Task.ID is populated.
	//
	// Postconditions:
	//   - On success: the task message exists in queue:{tag} for every tag.
	//   - On failure: no partial state guarantee; caller should retry or mark task failed.
	Enqueue(ctx context.Context, message *ProducerMessage) ([]string, error)

	// EnqueueDeadLetter moves a task to the dead letter stream (queue:dead-letter).
	// Called by the Monitor when a task exhausts its retry count.
	//
	// Args:
	//   ctx:    Request context.
	//   taskID: The task that has exhausted retries.
	//   reason: Human-readable failure reason recorded alongside the stream entry.
	//
	// Postconditions:
	//   - On success: task message exists in queue:dead-letter.
	//   - On failure: error returned; caller (Monitor) retries the operation.
	EnqueueDeadLetter(ctx context.Context, taskID string, reason string) error
}

// Consumer reads tasks from Redis Streams on behalf of a Worker.
// Each Worker is a named consumer within the "workers" consumer group for each tag stream.
// See: ADR-001, ADR-002, TASK-004, TASK-007
type Consumer interface {
	// ReadTasks performs a blocking XREADGROUP on all streams matching the given tags.
	// Blocks until at least one message is available or the context is cancelled.
	//
	// Args:
	//   ctx:        Cancellation cancels the blocking read and returns nil, nil.
	//   consumerID: The Worker's unique ID, used as the consumer name within the group.
	//   tags:       The capability tags this Worker handles. One stream per tag is read.
	//   blockFor:   How long to block waiting for messages. 0 means block indefinitely.
	//
	// Returns:
	//   A slice of TaskMessages read across all tag streams. May be empty on timeout.
	//   An error if the XREADGROUP command fails.
	//
	// Preconditions:
	//   - Consumer groups for all tag streams must already exist (created by InitGroups).
	//
	// Postconditions:
	//   - Returned messages are in the consumer's pending entry list until Acknowledge is called.
	ReadTasks(ctx context.Context, consumerID string, tags []string, blockFor time.Duration) ([]*TaskMessage, error)

	// Acknowledge sends XACK for a message, removing it from the pending entry list.
	// Must be called after a Task reaches a terminal state (completed, failed).
	//
	// Args:
	//   ctx:      Request context.
	//   tag:      The capability tag stream the message was read from.
	//   streamID: The Redis stream message ID returned by XREADGROUP.
	//
	// Postconditions:
	//   - On success: the message is no longer in the pending entry list.
	//   - On failure: the message remains pending and is eligible for XCLAIM by the Monitor.
	Acknowledge(ctx context.Context, tag string, streamID string) error

	// InitGroups ensures the "workers" consumer group exists for each tag stream.
	// Creates the stream and group if they do not exist. Called on service startup.
	//
	// Args:
	//   ctx:  Request context.
	//   tags: Capability tags for which groups must exist.
	//
	// Postconditions:
	//   - On success: XGROUP CREATE has been called for every tag stream.
	//   - On failure: error returned; service should not start without group initialization.
	InitGroups(ctx context.Context, tags []string) error
}

// PendingScanner is used by the Monitor to detect and reclaim tasks from downed workers.
// See: ADR-002, TASK-009
type PendingScanner interface {
	// ListPendingOlderThan returns all pending messages on a tag stream whose idle time
	// exceeds the given threshold. These are candidates for reclamation via Claim.
	//
	// Args:
	//   ctx:       Request context.
	//   tag:       The capability tag stream to inspect via XPENDING.
	//   olderThan: Messages idle longer than this duration are returned.
	//
	// Returns:
	//   Pending messages with their consumer IDs and idle times.
	//   An error if XPENDING fails.
	ListPendingOlderThan(ctx context.Context, tag string, olderThan time.Duration) ([]*PendingEntry, error)

	// Claim atomically reassigns a pending message from its current consumer to a new one
	// via XCLAIM. Used by the Monitor to reclaim tasks from downed workers.
	//
	// Args:
	//   ctx:             Request context.
	//   tag:             The capability tag stream.
	//   streamID:        The message ID to claim.
	//   newConsumerID:   The Worker that will take over the message.
	//   minIdleTime:     Minimum idle time before XCLAIM is permitted (safety guard).
	//
	// Postconditions:
	//   - On success: the message is now in newConsumerID's pending list.
	//   - On failure: error returned; the original consumer retains the message.
	Claim(ctx context.Context, tag string, streamID string, newConsumerID string, minIdleTime time.Duration) error
}

// PendingEntry describes a message in the pending entry list (XPENDING output).
// See: ADR-002, TASK-009
type PendingEntry struct {
	StreamID   string
	ConsumerID string
	IdleTime   time.Duration
	// TaskID is extracted from the message payload for cross-referencing with PostgreSQL.
	TaskID string
}

// HeartbeatStore manages the Redis sorted set (workers:active) used for liveness detection.
// See: ADR-002, TASK-006, TASK-009
type HeartbeatStore interface {
	// RecordHeartbeat writes or updates a Worker's entry in workers:active with the
	// current Unix timestamp as the score (ZADD workers:active <now> <workerID>).
	//
	// Args:
	//   ctx:      Request context.
	//   workerID: The Worker emitting the heartbeat.
	//
	// Postconditions:
	//   - On success: workers:active contains an entry for workerID with score = now.
	RecordHeartbeat(ctx context.Context, workerID string) error

	// ListExpired returns all Worker IDs in workers:active whose score is older than
	// the given cutoff (i.e., workers that have missed enough heartbeats to be declared down).
	// Uses ZRANGEBYSCORE workers:active -inf <cutoff>.
	//
	// Args:
	//   ctx:    Request context.
	//   cutoff: Workers with last heartbeat before this time are returned.
	//
	// Returns:
	//   Worker IDs of expired workers. Empty slice if none are expired.
	ListExpired(ctx context.Context, cutoff time.Time) ([]string, error)

	// Remove deletes a Worker from workers:active. Called by the Monitor after declaring a worker down.
	//
	// Args:
	//   ctx:      Request context.
	//   workerID: The Worker to remove.
	Remove(ctx context.Context, workerID string) error
}

// EventPublisher publishes events to Redis Pub/Sub channels consumed by the SSE broker.
// See: ADR-007, TASK-015
type EventPublisher interface {
	// Publish sends an event to the given Redis Pub/Sub channel.
	//
	// Args:
	//   ctx:     Request context.
	//   channel: The Pub/Sub channel name (e.g. "events:tasks:{userId}", "events:workers").
	//   event:   The SSEEvent payload, marshalled to JSON before publishing.
	//
	// Postconditions:
	//   - On success: all active Pub/Sub subscribers on channel receive the message.
	//   - On failure: error returned; event may be lost (fire-and-forget semantics, ADR-007).
	Publish(ctx context.Context, channel string, event *models.SSEEvent) error
}
