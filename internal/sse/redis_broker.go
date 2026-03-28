// Package sse — Redis-backed implementation of Broker.
// Subscribes to Redis Pub/Sub; fans events out to connected SSE http.ResponseWriters.
// See: ADR-007, TASK-015
package sse

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/nxlabs/nexusflow/internal/models"
	"github.com/redis/go-redis/v9"
)

// subscribeBufferSize is the number of SSE events each client channel can buffer before
// backpressure kicks in. Events that cannot be delivered to a full buffer are dropped
// (ADR-007: fire-and-forget; slow consumers do not stall the Pub/Sub reader).
const subscribeBufferSize = 64

// taskOwnerGetter is a narrow interface that allows RedisBroker to verify task
// ownership for access control on ServeLogEvents and ServeSinkEvents.
// Only GetByID is required; the full TaskRepository satisfies this interface.
// See: SOLID Interface Segregation, ADR-007
type taskOwnerGetter interface {
	GetByID(ctx context.Context, id uuid.UUID) (*models.Task, error)
}

// logReplayer is a narrow interface for replaying missed log lines on SSE reconnect.
// Used by ServeLogEvents when the client sends Last-Event-ID.
// The full TaskLogRepository satisfies this interface.
// See: ADR-007 (Last-Event-ID reconnection strategy)
type logReplayer interface {
	ListByTask(ctx context.Context, taskID uuid.UUID, afterID string) ([]*models.TaskLog, error)
}

// RedisBroker implements Broker using Redis Pub/Sub for event distribution
// and an in-process subscriber registry for fan-out to SSE connections.
//
// Goroutine lifecycle:
//   - Start() launches one goroutine per subscribed Redis Pub/Sub channel pattern.
//     Each goroutine exits when the context passed to Start() is cancelled.
//   - ServeXxx() handlers each subscribe a client channel, block until the request
//     context is cancelled (client disconnect), then unsubscribe and return.
//
// See: ADR-007, TASK-015
type RedisBroker struct {
	client *redis.Client
	mu     sync.RWMutex
	// subscribers maps a Redis channel key to a set of per-client buffered channels.
	// Protected by mu.
	subscribers map[string]map[chan *models.SSEEvent]struct{}
	// tasks is used for ownership checks in ServeLogEvents and ServeSinkEvents.
	// May be nil when the broker is used without access control (e.g. unit tests
	// that only exercise the pub/sub fan-out path).
	tasks taskOwnerGetter
	// logs is used for Last-Event-ID replay on ServeLogEvents reconnection.
	// May be nil when log replay is not needed.
	logs logReplayer
}

// NewRedisBroker constructs a RedisBroker.
//
// Args:
//
//	client: A connected go-redis client. May be nil in unit tests that only exercise
//	        the in-process subscriber registry (publish methods will return errors).
//
// Postconditions:
//   - The subscriber registry is initialised and ready for Subscribe/Unsubscribe calls.
func NewRedisBroker(client *redis.Client) *RedisBroker {
	return &RedisBroker{
		client:      client,
		subscribers: make(map[string]map[chan *models.SSEEvent]struct{}),
	}
}

// WithTaskRepo attaches a TaskRepository for ownership-based access control on log
// and sink SSE endpoints. Returns the receiver for chaining.
func (b *RedisBroker) WithTaskRepo(tasks taskOwnerGetter) *RedisBroker {
	b.tasks = tasks
	return b
}

// WithLogRepo attaches a TaskLogRepository for Last-Event-ID replay on the log SSE
// endpoint. Returns the receiver for chaining.
func (b *RedisBroker) WithLogRepo(logs logReplayer) *RedisBroker {
	b.logs = logs
	return b
}

// Start implements Broker.Start.
// Subscribes to all SSE-relevant Redis Pub/Sub patterns and routes inbound messages
// to the appropriate in-process subscriber channels. Blocks until ctx is cancelled.
//
// Redis pattern subscriptions used (ADR-007):
//
//	events:tasks:*   — task events for specific users and the admin "all" feed
//	events:logs:*    — log lines for specific tasks
//	events:workers   — worker fleet status events
//	events:sink:*    — sink inspector events for specific tasks
//
// On ctx cancellation, all pattern subscriptions are closed and the method returns nil.
func (b *RedisBroker) Start(ctx context.Context) error {
	if b.client == nil {
		return errors.New("sse: RedisBroker.Start: Redis client is nil")
	}

	pubsub := b.client.PSubscribe(ctx,
		"events:tasks:*",
		"events:logs:*",
		"events:workers",
		"events:sink:*",
	)
	defer pubsub.Close()

	ch := pubsub.Channel()

	for {
		select {
		case <-ctx.Done():
			return nil
		case msg, ok := <-ch:
			if !ok {
				// Pub/Sub channel was closed (e.g. Redis disconnected).
				return nil
			}
			b.routeMessage(msg)
		}
	}
}

// routeMessage decodes a Redis Pub/Sub message and fans it out to all subscribers
// registered on the message's channel.
// Errors decoding the JSON payload are logged and the message is discarded.
func (b *RedisBroker) routeMessage(msg *redis.Message) {
	var evt models.SSEEvent
	if err := json.Unmarshal([]byte(msg.Payload), &evt); err != nil {
		log.Printf("sse: routeMessage: cannot decode payload on channel %q: %v", msg.Channel, err)
		return
	}
	evt.Channel = msg.Channel
	b.fanOut(msg.Channel, &evt)
}

// fanOut delivers evt to all subscribers registered on channelKey.
// Uses a non-blocking send: if a subscriber's buffer is full (slow consumer),
// the event is dropped for that subscriber. This prevents a single slow client
// from stalling the Pub/Sub reader goroutine (ADR-007 backpressure strategy).
func (b *RedisBroker) fanOut(channelKey string, evt *models.SSEEvent) {
	b.mu.RLock()
	subs := b.subscribers[channelKey]
	b.mu.RUnlock()

	for ch := range subs {
		select {
		case ch <- evt:
		default:
			// Buffer full — drop event for this slow consumer.
		}
	}
}

// Subscribe registers a new client channel on channelKey and returns it.
// The channel is buffered to subscribeBufferSize to allow the Pub/Sub reader to
// proceed without blocking on slow SSE writers.
//
// Postconditions:
//   - The returned channel receives SSE events published to channelKey.
//   - The channel is closed by Unsubscribe when the client disconnects.
func (b *RedisBroker) Subscribe(channelKey string) chan *models.SSEEvent {
	ch := make(chan *models.SSEEvent, subscribeBufferSize)

	b.mu.Lock()
	if b.subscribers[channelKey] == nil {
		b.subscribers[channelKey] = make(map[chan *models.SSEEvent]struct{})
	}
	b.subscribers[channelKey][ch] = struct{}{}
	b.mu.Unlock()

	return ch
}

// Unsubscribe removes ch from the registry for channelKey and closes the channel.
// Safe to call concurrently. Closing the channel signals the ServeXxx handler loop
// to exit if it is ranging over the channel.
//
// Postconditions:
//   - ch is removed from the registry.
//   - ch is closed (callers must not send on it after calling Unsubscribe).
func (b *RedisBroker) Unsubscribe(channelKey string, ch chan *models.SSEEvent) {
	b.mu.Lock()
	if subs, ok := b.subscribers[channelKey]; ok {
		delete(subs, ch)
		// Remove the map entry for this channel key if it is now empty,
		// so the registry does not accumulate empty maps over time.
		if len(subs) == 0 {
			delete(b.subscribers, channelKey)
		}
	}
	b.mu.Unlock()

	close(ch)
}

// ServeTaskEvents implements Broker.ServeTaskEvents.
// Writes SSE headers and blocks delivering task state change events until the client
// disconnects or the request context is cancelled.
//
// Channel selection (ADR-007):
//   - Admin role: subscribes to events:tasks:all (receives all task events).
//   - User role:  subscribes to events:tasks:{userID} (receives own tasks only).
func (b *RedisBroker) ServeTaskEvents(w http.ResponseWriter, r *http.Request, session *models.Session) {
	channelKey := taskChannelKey(session.UserID.String(), session.Role)
	b.serveSSEChannel(w, r, channelKey)
}

// ServeWorkerEvents implements Broker.ServeWorkerEvents.
// All authenticated users receive all worker events (Domain Invariant 5).
func (b *RedisBroker) ServeWorkerEvents(w http.ResponseWriter, r *http.Request, session *models.Session) {
	b.serveSSEChannel(w, r, "events:workers")
}

// ServeLogEvents implements Broker.ServeLogEvents.
// Verifies task ownership before opening the stream (REQ-018).
// If the client provides Last-Event-ID, replays missed log lines from PostgreSQL
// before switching to live streaming.
func (b *RedisBroker) ServeLogEvents(w http.ResponseWriter, r *http.Request, session *models.Session, taskID string) {
	if !b.authoriseTaskAccess(w, r, session, taskID) {
		return
	}

	channelKey := logChannelKey(taskID)

	// Replay missed log lines if the client provides Last-Event-ID.
	lastEventID := r.Header.Get("Last-Event-ID")
	if lastEventID != "" && b.logs != nil {
		tid, err := uuid.Parse(taskID)
		if err == nil {
			b.replayLogs(w, r.Context(), tid, lastEventID)
		}
	}

	b.serveSSEChannel(w, r, channelKey)
}

// ServeSinkEvents implements Broker.ServeSinkEvents.
// Verifies task ownership before opening the stream (ADR-009, DEMO-003).
func (b *RedisBroker) ServeSinkEvents(w http.ResponseWriter, r *http.Request, session *models.Session, taskID string) {
	if !b.authoriseTaskAccess(w, r, session, taskID) {
		return
	}

	channelKey := sinkChannelKey(taskID)
	b.serveSSEChannel(w, r, channelKey)
}

// PublishTaskEvent implements Broker.PublishTaskEvent.
// Marshals the task as an SSEEvent payload and publishes to:
//   - events:tasks:{userId}  — for per-user task feed subscribers
//   - events:tasks:all       — for admin task feed subscribers
//
// Both channels receive the event so admins and the task owner both see updates.
// Errors from Redis PUBLISH are returned to the caller; SSE clients may miss this
// event (fire-and-forget per ADR-007).
func (b *RedisBroker) PublishTaskEvent(ctx context.Context, task *models.Task, reason string) error {
	if b.client == nil {
		return errors.New("sse: PublishTaskEvent: Redis client is nil")
	}

	evt := &models.SSEEvent{
		Type: taskEventType(task.Status),
		Payload: map[string]any{
			"task":   task,
			"reason": reason,
		},
	}

	data, err := json.Marshal(evt)
	if err != nil {
		return fmt.Errorf("sse: PublishTaskEvent: marshal: %w", err)
	}

	userChannel := taskChannelKey(task.UserID.String(), models.RoleUser)
	adminChannel := "events:tasks:all"

	var errs []string
	if err := b.client.Publish(ctx, userChannel, data).Err(); err != nil {
		errs = append(errs, fmt.Sprintf("user channel: %v", err))
	}
	if err := b.client.Publish(ctx, adminChannel, data).Err(); err != nil {
		errs = append(errs, fmt.Sprintf("admin channel: %v", err))
	}
	if len(errs) > 0 {
		return fmt.Errorf("sse: PublishTaskEvent: %s", strings.Join(errs, "; "))
	}
	return nil
}

// PublishWorkerEvent implements Broker.PublishWorkerEvent.
// Publishes a worker status change event to events:workers.
func (b *RedisBroker) PublishWorkerEvent(ctx context.Context, worker *models.Worker) error {
	if b.client == nil {
		return errors.New("sse: PublishWorkerEvent: Redis client is nil")
	}

	eventType := "worker:heartbeat"
	if worker.Status == models.WorkerStatusDown {
		eventType = "worker:down"
	}

	evt := &models.SSEEvent{
		Type:    eventType,
		Payload: worker,
	}

	data, err := json.Marshal(evt)
	if err != nil {
		return fmt.Errorf("sse: PublishWorkerEvent: marshal: %w", err)
	}

	if err := b.client.Publish(ctx, "events:workers", data).Err(); err != nil {
		return fmt.Errorf("sse: PublishWorkerEvent: publish: %w", err)
	}
	return nil
}

// PublishLogLine implements Broker.PublishLogLine.
// Publishes a log line to events:logs:{taskId} with the log entry's UUID as the event ID
// for Last-Event-ID replay support.
func (b *RedisBroker) PublishLogLine(ctx context.Context, logEntry *models.TaskLog) error {
	if b.client == nil {
		return errors.New("sse: PublishLogLine: Redis client is nil")
	}

	evt := &models.SSEEvent{
		ID:      logEntry.ID.String(),
		Type:    "log:line",
		Payload: logEntry,
	}

	data, err := json.Marshal(evt)
	if err != nil {
		return fmt.Errorf("sse: PublishLogLine: marshal: %w", err)
	}

	channel := logChannelKey(logEntry.TaskID.String())
	if err := b.client.Publish(ctx, channel, data).Err(); err != nil {
		return fmt.Errorf("sse: PublishLogLine: publish: %w", err)
	}
	return nil
}

// PublishSinkSnapshot implements Broker.PublishSinkSnapshot.
// Publishes a Before or After sink snapshot event to events:sink:{taskId}.
func (b *RedisBroker) PublishSinkSnapshot(ctx context.Context, snapshot *models.SinkSnapshot) error {
	if b.client == nil {
		return errors.New("sse: PublishSinkSnapshot: Redis client is nil")
	}

	eventType := "sink:before-snapshot"
	if snapshot.Phase == "after" {
		eventType = "sink:after-result"
	}

	evt := &models.SSEEvent{
		Type:    eventType,
		Payload: snapshot,
	}

	data, err := json.Marshal(evt)
	if err != nil {
		return fmt.Errorf("sse: PublishSinkSnapshot: marshal: %w", err)
	}

	channel := sinkChannelKey(snapshot.TaskID.String())
	if err := b.client.Publish(ctx, channel, data).Err(); err != nil {
		return fmt.Errorf("sse: PublishSinkSnapshot: publish: %w", err)
	}
	return nil
}

// serveSSEChannel sets SSE response headers, subscribes a client channel on channelKey,
// then forwards events to w until the request context is cancelled (client disconnect).
// Unsubscribes and cleans up the channel before returning.
//
// This is the shared delivery loop used by all four SSE handler methods.
func (b *RedisBroker) serveSSEChannel(w http.ResponseWriter, r *http.Request, channelKey string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	setSSEHeaders(w)

	ch := b.Subscribe(channelKey)
	defer b.Unsubscribe(channelKey, ch)

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-ch:
			if !ok {
				return
			}
			if err := writeSSEEvent(w, evt); err != nil {
				// Client disconnected mid-write.
				return
			}
			flusher.Flush()
		}
	}
}

// authoriseTaskAccess checks whether the calling session may access the given task.
// Returns true if the session is an admin or the task owner.
// Writes 403 Forbidden to w and returns false when access is denied.
// Writes 403 when the task is not found (task existence must not leak to unauthorised callers).
//
// Preconditions:
//   - b.tasks must be non-nil when this method is called.
//
// Postconditions:
//   - On true: caller may open an SSE stream for this task.
//   - On false: 403 has been written to w; caller must not write further.
func (b *RedisBroker) authoriseTaskAccess(w http.ResponseWriter, r *http.Request, session *models.Session, taskID string) bool {
	if session.Role == models.RoleAdmin {
		return true
	}

	if b.tasks == nil {
		// No task repository wired: deny access to be safe (fail-closed).
		http.Error(w, "forbidden", http.StatusForbidden)
		return false
	}

	tid, err := uuid.Parse(taskID)
	if err != nil {
		// Unparseable task ID: treat as not found.
		http.Error(w, "forbidden", http.StatusForbidden)
		return false
	}

	task, err := b.tasks.GetByID(r.Context(), tid)
	if err != nil || task == nil {
		// Task not found or DB error: deny (task existence must not leak).
		http.Error(w, "forbidden", http.StatusForbidden)
		return false
	}

	if task.UserID != session.UserID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return false
	}

	return true
}

// replayLogs writes missed log lines from cold storage to w, starting after afterID.
// Called by ServeLogEvents when the client provides a Last-Event-ID header.
// Errors are logged and discarded — the live stream continues regardless.
func (b *RedisBroker) replayLogs(w http.ResponseWriter, ctx context.Context, taskID uuid.UUID, afterID string) {
	logs, err := b.logs.ListByTask(ctx, taskID, afterID)
	if err != nil {
		log.Printf("sse: replayLogs: ListByTask(%s, after=%q): %v", taskID, afterID, err)
		return
	}

	for _, logEntry := range logs {
		evt := &models.SSEEvent{
			ID:      logEntry.ID.String(),
			Type:    "log:line",
			Payload: logEntry,
		}
		if err := writeSSEEvent(w, evt); err != nil {
			// Client disconnected during replay.
			return
		}
	}
}

// writeSSEEvent writes a single SSE-formatted event to w and flushes the connection.
//
// Wire format (SSE spec):
//
//	[id: {id}\n]           — omitted when ID is empty
//	event: {type}\n
//	data: {json}\n
//	\n
//
// Returns an error if w does not implement http.Flusher (streaming not supported)
// or if a write to w fails (client disconnected).
func writeSSEEvent(w http.ResponseWriter, event *models.SSEEvent) error {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return errors.New("sse: writeSSEEvent: response writer does not implement http.Flusher")
	}

	data, err := json.Marshal(event.Payload)
	if err != nil {
		return fmt.Errorf("sse: writeSSEEvent: marshal payload: %w", err)
	}

	var sb strings.Builder
	if event.ID != "" {
		sb.WriteString("id: ")
		sb.WriteString(event.ID)
		sb.WriteByte('\n')
	}
	sb.WriteString("event: ")
	sb.WriteString(event.Type)
	sb.WriteByte('\n')
	sb.WriteString("data: ")
	sb.Write(data)
	sb.WriteString("\n\n")

	if _, err := fmt.Fprint(w, sb.String()); err != nil {
		return fmt.Errorf("sse: writeSSEEvent: write: %w", err)
	}

	flusher.Flush()
	return nil
}

// setSSEHeaders sets the HTTP headers required for a Server-Sent Events stream
// and immediately flushes the 200 OK status line and headers to the client.
//
// Go's net/http buffers the status line and headers until the first Write call.
// For SSE, the client must receive the 200 OK and Content-Type header before any
// events arrive so the browser's EventSource can confirm the connection is live.
// Without an explicit WriteHeader + Flush here, a connection with no immediate
// events appears to hang from the browser's perspective ("Reconnecting...").
//
// Must be called before any data is written to w. w must implement http.Flusher.
func setSSEHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Disable nginx buffering (ADR-005).
	// Flush the 200 OK + headers to the client immediately.
	// This ensures the browser's EventSource sees a successful response before
	// any events are published, preventing spurious reconnection attempts.
	w.WriteHeader(http.StatusOK)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

// taskChannelKey returns the Redis Pub/Sub channel key for a task feed subscription.
// Admin role receives the all-tasks channel; User role receives their own user channel.
func taskChannelKey(userID string, role models.Role) string {
	if role == models.RoleAdmin {
		return "events:tasks:all"
	}
	return "events:tasks:" + userID
}

// logChannelKey returns the Redis Pub/Sub channel key for a task's log stream.
func logChannelKey(taskID string) string {
	return "events:logs:" + taskID
}

// sinkChannelKey returns the Redis Pub/Sub channel key for a task's sink inspector stream.
func sinkChannelKey(taskID string) string {
	return "events:sink:" + taskID
}

// taskEventType maps a TaskStatus to the SSE event type string published on the task feed.
func taskEventType(status models.TaskStatus) string {
	switch status {
	case models.TaskStatusSubmitted:
		return "task:created"
	case models.TaskStatusCompleted:
		return "task:completed"
	case models.TaskStatusFailed:
		return "task:failed"
	default:
		return "task:state-changed"
	}
}
