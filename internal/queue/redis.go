// Package queue — Redis implementation of the Producer, Consumer, PendingScanner,
// HeartbeatStore, and EventPublisher interfaces.
// See: ADR-001, ADR-002, ADR-007, TASK-004
package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nxlabs/nexusflow/internal/models"
	"github.com/redis/go-redis/v9"
)

// RedisQueue implements Producer, Consumer, PendingScanner, HeartbeatStore,
// and EventPublisher interfaces backed by go-redis.
// A single RedisQueue instance is shared across all interfaces via injection.
// See: TASK-004
type RedisQueue struct {
	client *redis.Client
}

// NewRedisQueue constructs a RedisQueue from the given go-redis client.
// The client must already be connected; NewRedisQueue does not dial.
// Panics if client is nil (fail-fast: a nil client would cause silent failures
// on every subsequent Redis call rather than surfacing the misconfiguration now).
//
// Args:
//
//	client: A connected go-redis client. Must not be nil.
//
// Preconditions:
//   - client is non-nil.
//   - client.Ping succeeds before any queue operation is attempted.
func NewRedisQueue(client *redis.Client) *RedisQueue {
	if client == nil {
		panic("queue.NewRedisQueue: client must not be nil")
	}
	return &RedisQueue{client: client}
}

// --- Producer ---

// Enqueue implements Producer.Enqueue.
// Routes the task to per-tag streams (queue:{tag}) via XADD (ADR-001).
// Creates the stream and consumer group if they do not exist on first use,
// satisfying the startup initialization contract for dynamically registered tags.
//
// Preconditions:
//   - message.Task is non-nil.
//   - message.Tags is non-empty.
//
// Postconditions:
//   - On success: TaskMessage exists in queue:{tag} for every tag in message.Tags.
//   - On failure: error returned; no partial state guarantee.
//
// See: ADR-001, TASK-004
func (q *RedisQueue) Enqueue(ctx context.Context, message *ProducerMessage) ([]string, error) {
	if message.Task == nil {
		return nil, errors.New("queue.Enqueue: task must not be nil")
	}
	if len(message.Tags) == 0 {
		return nil, errors.New("queue.Enqueue: tags must not be empty")
	}

	tm := TaskMessage{
		TaskID:      message.Task.ID.String(),
		PipelineID:  message.Task.PipelineID.String(),
		UserID:      message.Task.UserID.String(),
		ExecutionID: message.Task.ExecutionID,
	}
	payload, err := json.Marshal(tm)
	if err != nil {
		return nil, fmt.Errorf("queue.Enqueue: marshal task message: %w", err)
	}

	streamIDs := make([]string, 0, len(message.Tags))
	for _, tag := range message.Tags {
		stream := TaskQueueStream(tag)

		// Ensure the consumer group exists before XADD so that any worker that
		// starts consuming after this enqueue can read from the beginning.
		if err := q.ensureGroup(ctx, stream); err != nil {
			return nil, fmt.Errorf("queue.Enqueue: ensure group for stream %q: %w", stream, err)
		}

		id, err := q.client.XAdd(ctx, &redis.XAddArgs{
			Stream: stream,
			ID:     "*", // auto-generate monotonic ID
			Values: map[string]any{"payload": string(payload)},
		}).Result()
		if err != nil {
			return nil, fmt.Errorf("queue.Enqueue: XADD to stream %q: %w", stream, err)
		}
		streamIDs = append(streamIDs, id)
	}
	return streamIDs, nil
}

// EnqueueDeadLetter implements Producer.EnqueueDeadLetter.
// Adds the task to queue:dead-letter via XADD with taskId and reason fields.
// Called by the Monitor when a task exhausts its retry count (TASK-009).
//
// Postconditions:
//   - On success: entry exists in queue:dead-letter with taskId and reason.
//
// See: ADR-001, TASK-009
func (q *RedisQueue) EnqueueDeadLetter(ctx context.Context, taskID string, reason string) error {
	_, err := q.client.XAdd(ctx, &redis.XAddArgs{
		Stream: DeadLetterStream,
		ID:     "*",
		Values: map[string]any{
			"taskId": taskID,
			"reason": reason,
		},
	}).Result()
	if err != nil {
		return fmt.Errorf("queue.EnqueueDeadLetter: XADD to %q: %w", DeadLetterStream, err)
	}
	return nil
}

// --- Consumer ---

// maxBlockStep is the longest single XREADGROUP block the implementation issues.
// Using short polling steps allows the context cancellation check to fire promptly
// without requiring the caller to use a short blockFor value for shutdown responsiveness.
const maxBlockStep = 200 * time.Millisecond

// ReadTasks implements Consumer.ReadTasks.
// Performs blocking XREADGROUP across all streams matching the given tags.
// The total block window (blockFor) is split into maxBlockStep increments so that
// context cancellation is checked between each increment, enabling clean shutdown
// without waiting for a long Redis block to expire.
//
// Returns nil, nil when the context is cancelled so the caller loop can exit cleanly.
// Returns an empty (non-nil) slice when blockFor elapses with no messages available.
//
// Preconditions:
//   - Consumer groups for all tag streams must already exist (created by InitGroups or Enqueue).
//
// Postconditions:
//   - Returned messages are in the consumer's pending entry list until Acknowledge is called.
//
// See: ADR-001, TASK-004
func (q *RedisQueue) ReadTasks(ctx context.Context, consumerID string, tags []string, blockFor time.Duration) ([]*TaskMessage, error) {
	if len(tags) == 0 {
		return nil, nil
	}

	streams := buildXReadGroupStreams(tags)
	deadline := time.Now().Add(blockFor)

	for {
		if err := ctx.Err(); err != nil {
			// Context cancelled or timed out — signal clean shutdown to the caller.
			return nil, nil
		}

		remaining := time.Until(deadline)
		if remaining <= 0 {
			// blockFor has elapsed with no messages.
			return []*TaskMessage{}, nil
		}

		step := remaining
		if step > maxBlockStep {
			step = maxBlockStep
		}

		result, err := q.client.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    ConsumerGroupName,
			Consumer: consumerID,
			Streams:  streams,
			Count:    10,
			Block:    step,
			NoAck:    false,
		}).Result()

		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil, nil
			}
			if errors.Is(err, redis.Nil) {
				// No messages in this step — continue looping until deadline.
				continue
			}
			return nil, fmt.Errorf("queue.ReadTasks: XREADGROUP: %w", err)
		}

		return parseXReadGroupResult(result)
	}
}

// Acknowledge implements Consumer.Acknowledge.
// Sends XACK to remove the message from the pending entry list.
// Must be called after a Task reaches a terminal state (completed, failed).
//
// Postconditions:
//   - On success: the message is no longer in the pending entry list.
//
// See: ADR-001, TASK-004
func (q *RedisQueue) Acknowledge(ctx context.Context, tag string, streamID string) error {
	stream := TaskQueueStream(tag)
	if err := q.client.XAck(ctx, stream, ConsumerGroupName, streamID).Err(); err != nil {
		return fmt.Errorf("queue.Acknowledge: XACK on stream %q message %q: %w", stream, streamID, err)
	}
	return nil
}

// InitGroups implements Consumer.InitGroups.
// Uses XGROUP CREATE MKSTREAM for each tag to ensure consumer groups exist before
// any worker attempts to consume. Idempotent: returns nil if the group already exists.
// Called on service startup (AC-2).
//
// Postconditions:
//   - On success: the "workers" consumer group exists on queue:{tag} for every tag.
//
// See: ADR-001, TASK-004
func (q *RedisQueue) InitGroups(ctx context.Context, tags []string) error {
	for _, tag := range tags {
		stream := TaskQueueStream(tag)
		if err := q.ensureGroup(ctx, stream); err != nil {
			return fmt.Errorf("queue.InitGroups: stream %q: %w", stream, err)
		}
	}
	return nil
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

// --- Key helpers (public) ---

// NewLogStream returns the Redis Streams key for a task's hot log stream.
// Format: logs:{taskId}
// See: ADR-008, TASK-016
func NewLogStream(taskID string) string {
	return "logs:" + taskID
}

// TaskQueueStream returns the stream key for the given capability tag.
// Format: queue:{tag}
// See: ADR-001, TASK-004
func TaskQueueStream(tag string) string {
	return "queue:" + tag
}

// DeadLetterStream is the Redis Streams key for terminally failed tasks.
// See: ADR-001, TASK-009
const DeadLetterStream = "queue:dead-letter"

// WorkersActiveKey is the Redis sorted set key for worker liveness tracking.
// See: ADR-002, TASK-006
const WorkersActiveKey = "workers:active"

// ConsumerGroupName is the single consumer group name used across all task streams.
// Each Worker is an individual consumer within this group identified by its worker ID.
// See: ADR-001, TASK-004
const ConsumerGroupName = "workers"

// --- SessionStore ---

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
// Panics if client is nil (fail-fast: a nil client causes silent failures on every call).
//
// Args:
//
//	client: A connected go-redis client. Must not be nil.
//	ttl:    Session time-to-live (default 24h per ADR-006).
func NewRedisSessionStore(client *redis.Client, ttl time.Duration) *RedisSessionStore {
	if client == nil {
		panic("queue.NewRedisSessionStore: client must not be nil")
	}
	return &RedisSessionStore{client: client, ttl: ttl}
}

// sessionKey returns the Redis key for the given session token.
// Format: session:{token} per ADR-006.
func sessionKey(token string) string {
	return "session:" + token
}

// Create implements SessionStore.Create.
// Marshals the session to JSON and stores it at session:{token} with the configured TTL.
//
// Postconditions:
//   - On success: session:{token} exists in Redis with configured TTL.
func (s *RedisSessionStore) Create(ctx context.Context, token string, session *models.Session) error {
	payload, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("queue.SessionStore.Create: marshal session: %w", err)
	}
	if err := s.client.Set(ctx, sessionKey(token), payload, s.ttl).Err(); err != nil {
		return fmt.Errorf("queue.SessionStore.Create: SET session:%s: %w", token, err)
	}
	return nil
}

// Get implements SessionStore.Get.
// Returns nil, nil when the token does not exist or has expired (redis.Nil treated as missing).
//
// Postconditions:
//   - Returns (*models.Session, nil) when found.
//   - Returns (nil, nil) when the key is absent or expired.
//   - Returns (nil, error) only for Redis connectivity failures.
func (s *RedisSessionStore) Get(ctx context.Context, token string) (*models.Session, error) {
	data, err := s.client.Get(ctx, sessionKey(token)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("queue.SessionStore.Get: GET session:%s: %w", token, err)
	}
	var sess models.Session
	if err := json.Unmarshal(data, &sess); err != nil {
		return nil, fmt.Errorf("queue.SessionStore.Get: unmarshal session: %w", err)
	}
	return &sess, nil
}

// Delete implements SessionStore.Delete.
// Removes the session key from Redis immediately. Idempotent: returns nil if the key is absent.
//
// Postconditions:
//   - On success: subsequent Get calls with this token return nil.
func (s *RedisSessionStore) Delete(ctx context.Context, token string) error {
	if err := s.client.Del(ctx, sessionKey(token)).Err(); err != nil {
		return fmt.Errorf("queue.SessionStore.Delete: DEL session:%s: %w", token, err)
	}
	return nil
}

// DeleteAllForUser implements SessionStore.DeleteAllForUser.
// Uses SCAN to find all session:{*} keys, deserialises each, and deletes those
// belonging to the given userID. Complexity: O(N) in total session count.
// Acceptable for single-org scale (ADR-006 — Warning threshold: >1000 active sessions).
//
// Postconditions:
//   - On success: no active session remains for this user.
func (s *RedisSessionStore) DeleteAllForUser(ctx context.Context, userID string) error {
	var cursor uint64
	for {
		keys, nextCursor, err := s.client.Scan(ctx, cursor, "session:*", 100).Result()
		if err != nil {
			return fmt.Errorf("queue.SessionStore.DeleteAllForUser: SCAN: %w", err)
		}

		for _, key := range keys {
			data, err := s.client.Get(ctx, key).Bytes()
			if errors.Is(err, redis.Nil) {
				continue // expired between SCAN and GET
			}
			if err != nil {
				return fmt.Errorf("queue.SessionStore.DeleteAllForUser: GET %s: %w", key, err)
			}
			var sess models.Session
			if err := json.Unmarshal(data, &sess); err != nil {
				continue // malformed entry — skip rather than halt
			}
			if sess.UserID.String() == userID {
				if err := s.client.Del(ctx, key).Err(); err != nil {
					return fmt.Errorf("queue.SessionStore.DeleteAllForUser: DEL %s: %w", key, err)
				}
			}
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
	return nil
}

// --- internal helpers ---

// ensureGroup calls XGROUP CREATE MKSTREAM on the given stream with the "workers" group.
// Returns nil if the group already exists (BUSYGROUP error is swallowed).
// The "$" start ID means consumers created after the group only see new messages;
// "0" would deliver all historical messages. We use "$" because consumer groups
// created at startup should only process messages written after creation — previously
// enqueued messages that survived a restart are still readable via the stream itself.
//
// Postconditions:
//   - On success: the "workers" group exists on the stream and is ready for XREADGROUP.
func (q *RedisQueue) ensureGroup(ctx context.Context, stream string) error {
	err := q.client.XGroupCreateMkStream(ctx, stream, ConsumerGroupName, "$").Err()
	if err != nil && !isBusyGroup(err) {
		return fmt.Errorf("ensureGroup: XGROUP CREATE MKSTREAM on %q: %w", stream, err)
	}
	return nil
}

// isBusyGroup returns true when the Redis error indicates the consumer group
// already exists (BUSYGROUP error string from Redis).
func isBusyGroup(err error) bool {
	return strings.Contains(err.Error(), "BUSYGROUP")
}

// buildXReadGroupStreams constructs the streams slice for XReadGroupArgs.
// Redis requires the format: [stream1, stream2, ..., ">", ">", ...] where ">" means
// "deliver new messages not yet delivered to any other consumer in this group".
//
// Args:
//
//	tags: capability tags whose streams will be read.
//
// Returns:
//
//	A slice with all stream keys followed by ">" for each, as required by XReadGroup.
func buildXReadGroupStreams(tags []string) []string {
	streams := make([]string, 0, len(tags)*2)
	for _, tag := range tags {
		streams = append(streams, TaskQueueStream(tag))
	}
	for range tags {
		streams = append(streams, ">")
	}
	return streams
}

// parseXReadGroupResult converts the go-redis XReadGroup result into TaskMessage slices.
// Each XStream in the result corresponds to one tag stream; each XMessage within it
// is one pending task for this consumer. The StreamID field is populated from the
// Redis message ID so that Acknowledge can call XACK with the correct ID.
//
// Args:
//
//	result: The raw XREADGROUP response from go-redis.
//
// Returns:
//
//	A flat slice of TaskMessages across all streams, in the order returned by Redis.
func parseXReadGroupResult(result []redis.XStream) ([]*TaskMessage, error) {
	var messages []*TaskMessage
	for _, xstream := range result {
		for _, xmsg := range xstream.Messages {
			rawPayload, ok := xmsg.Values["payload"]
			if !ok {
				// Malformed entry: skip rather than halt the consumer.
				// This can happen if a different producer wrote to the stream
				// without the expected payload field.
				continue
			}
			payloadStr := fmt.Sprintf("%v", rawPayload)
			var tm TaskMessage
			if err := json.Unmarshal([]byte(payloadStr), &tm); err != nil {
				// Unparseable payload: skip and continue to allow other messages through.
				continue
			}
			tm.StreamID = xmsg.ID
			messages = append(messages, &tm)
		}
	}
	return messages, nil
}
