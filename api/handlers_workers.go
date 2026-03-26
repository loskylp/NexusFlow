// Package api — Worker Fleet status handler.
// All authenticated users can view all workers (Domain Invariant 5).
// See: REQ-016, TASK-025
package api

import "net/http"

// WorkerHandler handles worker fleet REST endpoints.
// See: TASK-025
type WorkerHandler struct {
	server *Server
}

// List handles GET /api/workers.
// Returns all registered workers with current status, tags, current task assignment,
// and last heartbeat timestamp. Available to all authenticated users.
//
// Responses:
//   200 OK:           [ { worker }, ... ]
//   401 Unauthorized: no valid session
//
// Postconditions:
//   - Response includes workers in all states (online and down).
//   - CurrentTaskID is populated for workers currently executing a task.
func (h *WorkerHandler) List(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement in TASK-025
	panic("not implemented")
}
