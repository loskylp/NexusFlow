// Package queue — Redis implementation of the Producer, Consumer, PendingScanner,
// HeartbeatStore, and EventPublisher interfaces.
// See: ADR-001, ADR-002, ADR-007, TASK-004
package queue

import (
	"context"
	"time"

	"github.com/nxlabs/nexusflow/internal/models"
	"github.com/redis/go-redis/v9"
)

// RedisQueue implements all queue interfaces backed by go-redis.
// A single RedisQueue instance is shared across the interfaces via embedding.
// See: TASK-004
type RedisQueue struct {
	client *redis.Client
}

// NewRedisQueue constructs a RedisQueue from the given go-redis client.
// The client must already be connected; NewRedisQueue does not dial.
//
// Args:
//   client: A connected go-redis client. Must not be nil.
//
// Preconditions:
//   - client.Ping succeeds before any queue operation is attempted.
func NewRedisQueue(client *redis.Client) *RedisQueue {
	// TODO: Implement in TASK-004
	panic("not implemented")
}

// --- Producer ---

// Enqueue implements Producer.Enqueue.
// Routes the task to per-tag streams (queue:{tag}) via XADD.
// Creates the stream and consumer group if they do not exist.
// See: ADR-001, TASK-004
func (q *RedisQueue) Enqueue(ctx context.Context, message *ProducerMessage) ([]string, error) {
	// TODO: Implement in TASK-004
	panic("not implemented")
}

// EnqueueDeadLetter implements Producer.EnqueueDeadLetter.
// Adds the task to queue:dead-letter via XADD.
// See: ADR-001, TASK-009
func (q *RedisQueue) EnqueueDeadLetter(ctx context.Context, taskID string, reason string) error {
	// TODO: Implement in TASK-009
	panic("not implemented")
}

// --- Consumer ---

// ReadTasks implements Consumer.ReadTasks.
// Performs a blocking XREADGROUP across all streams matching tags.
// See: ADR-001, TASK-007
func (q *RedisQueue) ReadTasks(ctx context.Context, consumerID string, tags []string, blockFor time.Duration) ([]*TaskMessage, error) {
	// TODO: Implement in TASK-007
	panic("not implemented")
}

// Acknowledge implements Consumer.Acknowledge.
// Sends XACK to remove the message from the pending entry list.
// See: ADR-001, TASK-007
func (q *RedisQueue) Acknowledge(ctx context.Context, tag string, streamID string) error {
	// TODO: Implement in TASK-007
	panic("not implemented")
}

// InitGroups implements Consumer.InitGroups.
// Uses XGROUP CREATE MKSTREAM for each tag to ensure groups exist before consumption.
// Idempotent: returns nil if the group already exists.
// See: ADR-001, TASK-004
func (q *RedisQueue) InitGroups(ctx context.Context, tags []string) error {
	// TODO: Implement in TASK-004
	panic("not implemented")
}

// --- PendingScanner ---

// ListPendingOlderThan implements PendingScanner.ListPendingOlderThan.
// Uses XPENDING IDLE to find messages exceeding olderThan idle time.
// See: ADR-002, TASK-009
func (q *RedisQueue) ListPendingOlderThan(ctx context.Context, tag string, olderThan time.Duration) ([]*PendingEntry, error) {
	// TODO: Implement in TASK-009
	panic("not implemented")
}

// Claim implements PendingScanner.Claim.
// Uses XCLAIM to atomically reassign the message to a healthy worker.
// See: ADR-002, TASK-009
func (q *RedisQueue) Claim(ctx context.Context, tag string, streamID string, newConsumerID string, minIdleTime time.Duration) error {
	// TODO: Implement in TASK-009
	panic("not implemented")
}

// --- HeartbeatStore ---

// RecordHeartbeat implements HeartbeatStore.RecordHeartbeat.
// Executes: ZADD workers:active <now_unix> <workerID>
// See: ADR-002, TASK-006
func (q *RedisQueue) RecordHeartbeat(ctx context.Context, workerID string) error {
	// TODO: Implement in TASK-006
	panic("not implemented")
}

// ListExpired implements HeartbeatStore.ListExpired.
// Executes: ZRANGEBYSCORE workers:active -inf <cutoff_unix>
// See: ADR-002, TASK-009
func (q *RedisQueue) ListExpired(ctx context.Context, cutoff time.Time) ([]string, error) {
	// TODO: Implement in TASK-009
	panic("not implemented")
}

// Remove implements HeartbeatStore.Remove.
// Executes: ZREM workers:active <workerID>
// See: ADR-002, TASK-009
func (q *RedisQueue) Remove(ctx context.Context, workerID string) error {
	// TODO: Implement in TASK-009
	panic("not implemented")
}

// --- EventPublisher ---

// Publish implements EventPublisher.Publish.
// Marshals the event to JSON and executes PUBLISH on the given channel.
// Fire-and-forget: if no subscriber is listening, the message is lost (ADR-007 — acceptable).
// See: ADR-007, TASK-015
func (q *RedisQueue) Publish(ctx context.Context, channel string, event *models.SSEEvent) error {
	// TODO: Implement in TASK-015
	panic("not implemented")
}

// NewLogStream returns a key name for a task's hot log Redis Stream.
// Format: logs:{taskId}
// See: ADR-008, TASK-016
func NewLogStream(taskID string) string {
	return "logs:" + taskID
}

// TaskQueueStream returns the stream key for a given capability tag.
// Format: queue:{tag}
// See: ADR-001, TASK-004
func TaskQueueStream(tag string) string {
	return "queue:" + tag
}

// DeadLetterStream is the Redis Streams key for terminally failed tasks.
// See: ADR-001, TASK-009
const DeadLetterStream = "queue:dead-letter"

// WorkersActiveKey is the Redis sorted set key for liveness tracking.
// See: ADR-002, TASK-006
const WorkersActiveKey = "workers:active"

// ConsumerGroupName is the single consumer group name used across all task streams.
// Each Worker is an individual consumer within this group.
// See: ADR-001, TASK-004
const ConsumerGroupName = "workers"

// SessionStore manages Redis-backed session tokens.
// See: ADR-006, TASK-003
type SessionStore interface {
	// Create stores a new session in Redis at key session:{token} with the configured TTL.
	// The token must be a cryptographically random 256-bit hex string.
	//
	// Args:
	//   ctx:     Request context.
	//   token:   The opaque session token issued to the client.
	//   session: The session payload to store (userID, role, createdAt).
	//
	// Postconditions:
	//   - On success: session is stored with TTL; client can authenticate with this token.
	//   - On failure: error returned; the login response must not be sent to the client.
	Create(ctx context.Context, token string, session *models.Session) error

	// Get retrieves a session by token. Returns nil, nil if the token does not exist or has expired.
	//
	// Args:
	//   ctx:   Request context.
	//   token: The session token from the client's cookie or Authorization header.
	//
	// Returns:
	//   The Session if found and not expired; nil if the token is unknown or expired.
	//   An error only for Redis connectivity failures (not for missing sessions).
	Get(ctx context.Context, token string) (*models.Session, error)

	// Delete invalidates a session immediately.
	// Called by POST /api/auth/logout and by admin user deactivation (REQ-020).
	//
	// Args:
	//   ctx:   Request context.
	//   token: The session token to invalidate.
	//
	// Postconditions:
	//   - On success: subsequent Get calls with this token return nil.
	Delete(ctx context.Context, token string) error

	// DeleteAllForUser removes all active sessions for a given user.
	// Called when an admin deactivates a user account (REQ-020 — immediate invalidation).
	//
	// Args:
	//   ctx:    Request context.
	//   userID: The user whose sessions are all to be invalidated.
	//
	// Postconditions:
	//   - On success: no active session remains for this user.
	DeleteAllForUser(ctx context.Context, userID string) error
}

// RedisSessionStore implements SessionStore using go-redis.
// See: ADR-006, TASK-003
type RedisSessionStore struct {
	client *redis.Client
	ttl    time.Duration
}

// NewRedisSessionStore constructs a RedisSessionStore.
//
// Args:
//   client: A connected go-redis client.
//   ttl:    Session time-to-live (default 24h per ADR-006).
func NewRedisSessionStore(client *redis.Client, ttl time.Duration) *RedisSessionStore {
	// TODO: Implement in TASK-003
	panic("not implemented")
}

// Create implements SessionStore.Create.
func (s *RedisSessionStore) Create(ctx context.Context, token string, session *models.Session) error {
	// TODO: Implement in TASK-003
	panic("not implemented")
}

// Get implements SessionStore.Get.
func (s *RedisSessionStore) Get(ctx context.Context, token string) (*models.Session, error) {
	// TODO: Implement in TASK-003
	panic("not implemented")
}

// Delete implements SessionStore.Delete.
func (s *RedisSessionStore) Delete(ctx context.Context, token string) error {
	// TODO: Implement in TASK-003
	panic("not implemented")
}

// DeleteAllForUser implements SessionStore.DeleteAllForUser.
// Uses SCAN to find all session:{*} keys belonging to the user, then DEL.
// Complexity note: this is O(N) in total session count; acceptable for single-org scale.
func (s *RedisSessionStore) DeleteAllForUser(ctx context.Context, userID string) error {
	// TODO: Implement in TASK-003
	panic("not implemented")
}
