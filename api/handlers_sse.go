// Package api — SSE endpoint handlers.
// Delegates to the SSE Broker for connection management and event fan-out.
// All SSE endpoints require authentication via the auth middleware.
// See: ADR-007, TASK-015
package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/nxlabs/nexusflow/internal/auth"
)

// SSEHandler handles the four SSE streaming endpoints.
// Delegates all connection management and event distribution to the SSE Broker.
// See: ADR-007, TASK-015
type SSEHandler struct {
	server *Server
}

// Tasks handles GET /events/tasks.
// Opens an SSE stream for task state change events.
// User role: own tasks only. Admin role: all tasks.
//
// SSE event types published on this stream:
//
//	task:created, task:state-changed, task:completed, task:failed
//
// Responses:
//
//	200 text/event-stream: streaming SSE connection (blocks until client disconnects)
//	401 Unauthorized:      no valid session
//	500 Internal:          response writer does not support streaming
func (h *SSEHandler) Tasks(w http.ResponseWriter, r *http.Request) {
	session := auth.SessionFromContext(r.Context())
	h.server.broker.ServeTaskEvents(w, r, session)
}

// Workers handles GET /events/workers.
// Opens an SSE stream for worker fleet status changes. Available to all authenticated users.
//
// SSE event types published on this stream:
//
//	worker:registered, worker:heartbeat, worker:down
//
// Responses:
//
//	200 text/event-stream: streaming SSE connection
//	401 Unauthorized:      no valid session
func (h *SSEHandler) Workers(w http.ResponseWriter, r *http.Request) {
	session := auth.SessionFromContext(r.Context())
	h.server.broker.ServeWorkerEvents(w, r, session)
}

// Logs handles GET /events/tasks/{id}/logs.
// Opens an SSE stream of log lines for the specified task.
// Supports Last-Event-ID header for reconnection replay (ADR-007).
//
// SSE event types:
//
//	log:line   — { taskId, line, level, timestamp, id }
//
// Responses:
//
//	200 text/event-stream: streaming SSE connection
//	401 Unauthorized:      no valid session
//	403 Forbidden:         caller is not task owner or Admin
func (h *SSEHandler) Logs(w http.ResponseWriter, r *http.Request) {
	session := auth.SessionFromContext(r.Context())
	taskID := chi.URLParam(r, "id")
	h.server.broker.ServeLogEvents(w, r, session, taskID)
}

// Sink handles GET /events/sink/{taskId}.
// Opens an SSE stream of sink inspector events (before/after snapshots).
// Only available to task owner or Admin.
//
// SSE event types:
//
//	sink:before-snapshot  — { taskId, data, capturedAt }
//	sink:after-result     — { taskId, data, capturedAt }
//
// Responses:
//
//	200 text/event-stream: streaming SSE connection
//	401 Unauthorized:      no valid session
//	403 Forbidden:         caller is not task owner or Admin
func (h *SSEHandler) Sink(w http.ResponseWriter, r *http.Request) {
	session := auth.SessionFromContext(r.Context())
	taskID := chi.URLParam(r, "taskId")
	h.server.broker.ServeSinkEvents(w, r, session, taskID)
}
