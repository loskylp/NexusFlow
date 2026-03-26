// Package api — Task handlers: submission, query, and cancellation.
// See: REQ-001, REQ-009, REQ-010, TASK-005, TASK-008, TASK-012
package api

import "net/http"

// TaskHandler handles task-related REST endpoints.
// Depends on TaskRepository, PipelineRepository, and the queue Producer.
// See: TASK-005, TASK-008, TASK-012
type TaskHandler struct {
	server *Server
}

// Submit handles POST /api/tasks.
// Validates the pipeline reference, inserts the task into PostgreSQL (status: submitted),
// enqueues it in the appropriate Redis stream (status: queued), and returns 201.
//
// Request body:
//   {
//     "pipelineId": "uuid",
//     "input":       { ... },          // arbitrary key-value input params
//     "retryConfig": { "maxRetries": 3, "backoff": "exponential" }  // optional
//   }
//
// Responses:
//   201 Created:       { "taskId": "uuid", "status": "queued" }
//   400 Bad Request:   malformed JSON, missing required fields, or invalid pipelineId
//   401 Unauthorized:  no valid session
//   500 Internal:      database or Redis failure
//
// Preconditions:
//   - The referenced Pipeline exists and is owned by the caller (or caller is Admin).
//   - Auth middleware has placed a valid Session in the request context.
//
// Postconditions:
//   - On 201: task exists in PostgreSQL with status "queued"; task message exists in queue:{tag}.
//   - SSE event published to events:tasks:{userId} (fire-and-forget).
//   - Default RetryConfig applied if not provided in request body.
func (h *TaskHandler) Submit(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement in TASK-005
	panic("not implemented")
}

// List handles GET /api/tasks.
// Returns all tasks visible to the caller: own tasks for User role; all tasks for Admin.
// Satisfies Domain Invariant 5 (visibility isolation).
//
// Responses:
//   200 OK:           [ { task }, ... ]
//   401 Unauthorized: no valid session
//
// Postconditions:
//   - User role: only tasks with user_id matching session.UserID are returned.
//   - Admin role: all tasks across all users are returned.
func (h *TaskHandler) List(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement in TASK-008 (Cycle 2)
	panic("not implemented")
}

// Get handles GET /api/tasks/{id}.
// Returns the task with its current status and state log.
//
// Responses:
//   200 OK:           { task, stateLog: [...] }
//   401 Unauthorized: no valid session
//   403 Forbidden:    caller does not own the task and is not Admin
//   404 Not Found:    task does not exist
func (h *TaskHandler) Get(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement in TASK-008 (Cycle 2)
	panic("not implemented")
}

// Cancel handles POST /api/tasks/{id}/cancel.
// Cancels a task in a non-terminal state.
// Domain Invariant 8: only the submitting User or an Admin may cancel.
//
// Responses:
//   204 No Content:   task cancelled
//   401 Unauthorized: no valid session
//   403 Forbidden:    caller is not the task owner or Admin
//   404 Not Found:    task does not exist
//   409 Conflict:     task is already in a terminal state (completed, failed, cancelled)
//
// Postconditions:
//   - On 204: task.Status = "cancelled"; SSE event published.
func (h *TaskHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement in TASK-012 (Cycle 2)
	panic("not implemented")
}
