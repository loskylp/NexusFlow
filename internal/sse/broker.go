// Package sse implements the SSE event distribution layer.
// The Broker subscribes to Redis Pub/Sub channels and fans events out to connected
// HTTP clients via Server-Sent Events (Go's http.Flusher).
//
// SSE channel architecture (ADR-007):
//   GET /events/tasks              — task state updates (role-filtered)
//   GET /events/workers            — worker status changes (all authenticated)
//   GET /events/tasks/{id}/logs    — log streaming for a specific task (owner or admin)
//   GET /events/sink/{taskId}      — sink inspector events (owner or admin)
//
// Redis Pub/Sub channels published into the Broker:
//   events:tasks:{userId}          — task events for a specific user
//   events:tasks:all               — task events for admin feeds
//   events:logs:{taskId}           — log lines for a specific task
//   events:workers                 — worker fleet status events
//   events:sink:{taskId}           — sink inspector before/after events
//
// See: ADR-007, TASK-015
package sse

import (
	"context"
	"net/http"

	"github.com/nxlabs/nexusflow/internal/models"
)

// Broker manages SSE client connections and distributes Redis Pub/Sub events
// to the appropriate connected clients.
// See: ADR-007, TASK-015
type Broker interface {
	// Start begins subscribing to Redis Pub/Sub channels and routing events
	// to connected SSE clients. Blocks until ctx is cancelled.
	// Must be called in a dedicated goroutine before serving SSE handlers.
	//
	// Args:
	//   ctx: Cancellation shuts down all Pub/Sub subscriptions and disconnects clients.
	//
	// Postconditions:
	//   - On ctx cancellation: all client connections are closed gracefully.
	Start(ctx context.Context) error

	// ServeTaskEvents writes an SSE stream to w for the authenticated user's task feed.
	// User role receives only their own task events; Admin role receives all task events.
	// Blocks until the client disconnects or ctx is cancelled.
	//
	// Args:
	//   w:       The response writer. Must implement http.Flusher.
	//   r:       The request, used to extract the authenticated session (ADR-006).
	//   session: The authenticated caller's session (userID and role).
	//
	// Preconditions:
	//   - r.Context() is set with a valid session by the auth middleware.
	//   - w implements http.Flusher; returns 500 if it does not.
	//
	// Postconditions:
	//   - On client disconnect: handler returns without error.
	//   - Events are delivered within 2 seconds of publication (NFR-003).
	ServeTaskEvents(w http.ResponseWriter, r *http.Request, session *models.Session)

	// ServeWorkerEvents writes an SSE stream for the worker fleet dashboard.
	// All authenticated users receive all worker events (Domain Invariant 5).
	// Blocks until the client disconnects or ctx is cancelled.
	ServeWorkerEvents(w http.ResponseWriter, r *http.Request, session *models.Session)

	// ServeLogEvents writes an SSE stream of log lines for a specific task.
	// Only the task owner or an Admin may subscribe (REQ-018).
	// Supports Last-Event-ID for reconnection replay: on reconnect, the server
	// replays log lines from the database starting after the given event ID.
	//
	// Args:
	//   w:            Response writer implementing http.Flusher.
	//   r:            Request; Last-Event-ID header is read for replay.
	//   session:      Authenticated caller.
	//   taskID:       The task whose logs are streamed.
	//
	// Postconditions:
	//   - On 403: caller is not the task owner or an Admin; no stream is opened.
	//   - On reconnect with Last-Event-ID: missed log lines are replayed from PostgreSQL.
	ServeLogEvents(w http.ResponseWriter, r *http.Request, session *models.Session, taskID string)

	// ServeSinkEvents writes an SSE stream of sink inspector events for a specific task.
	// Only the task owner or an Admin may subscribe (ADR-009, DEMO-003).
	// Delivers sink:before-snapshot and sink:after-result events.
	ServeSinkEvents(w http.ResponseWriter, r *http.Request, session *models.Session, taskID string)

	// PublishTaskEvent publishes a task state-change event to the Redis Pub/Sub channels
	// consumed by ServeTaskEvents handlers.
	// Called by API handlers and the Monitor after every task state transition.
	//
	// Args:
	//   ctx:    Request context.
	//   task:   The task after its state transition. Used to determine which channel to publish on.
	//   reason: Human-readable reason for the transition (recorded in TaskStateLog).
	//
	// Postconditions:
	//   - On success: event is published to events:tasks:{userId} and events:tasks:all.
	//   - On failure: error logged; SSE clients may miss this event (fire-and-forget, ADR-007).
	PublishTaskEvent(ctx context.Context, task *models.Task, reason string) error

	// PublishWorkerEvent publishes a worker status change event to events:workers.
	// Called by the Monitor after marking a worker down or registering a new worker.
	PublishWorkerEvent(ctx context.Context, worker *models.Worker) error

	// PublishLogLine publishes a single log line to events:logs:{taskId}.
	// Called by the Worker after writing a log line to Redis Streams.
	//
	// Args:
	//   ctx:    Request context.
	//   log:    The log line to publish.
	//
	// Postconditions:
	//   - On success: the log line event is delivered to connected log stream clients within 2s (NFR-003).
	PublishLogLine(ctx context.Context, log *models.TaskLog) error

	// PublishSinkSnapshot publishes a Before or After sink snapshot event to events:sink:{taskId}.
	// Called by the Worker during the Sink phase (ADR-009, TASK-033).
	PublishSinkSnapshot(ctx context.Context, snapshot *models.SinkSnapshot) error
}
