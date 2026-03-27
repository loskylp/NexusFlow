// Package api — Worker Fleet status handler.
// All authenticated users can view all workers (Domain Invariant 5).
// See: REQ-016, TASK-025
package api

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/nxlabs/nexusflow/internal/auth"
)

// WorkerHandler handles worker fleet REST endpoints.
// See: TASK-025
type WorkerHandler struct {
	server *Server
}

// workerListItem is the JSON shape returned for each worker in GET /api/workers.
// Fields are named to match the API contract specified in TASK-025 AC-2.
type workerListItem struct {
	// ID is the worker's string identifier (hostname or UUID assigned at startup).
	ID string `json:"id"`
	// Status is "online" or "down" (models.WorkerStatus).
	Status string `json:"status"`
	// Tags are the capability tags that determine which task queue streams the worker consumes.
	Tags []string `json:"tags"`
	// CurrentTaskID is the UUID of the task currently assigned to or running on this worker.
	// Null when the worker is idle.
	CurrentTaskID *uuid.UUID `json:"currentTaskId"`
	// LastHeartbeat is the timestamp of the worker's most recent heartbeat emission.
	LastHeartbeat time.Time `json:"lastHeartbeat"`
}

// List handles GET /api/workers.
// Returns all registered workers with current status, capability tags, current task
// assignment, and last heartbeat timestamp. Available to all authenticated users.
//
// Responses:
//
//	200 OK:           [ { id, status, tags, currentTaskId, lastHeartbeat }, ... ]
//	401 Unauthorized: no valid session in request context
//	500 Internal:     database error
//
// Preconditions:
//   - Auth middleware has placed a valid Session in the request context.
//
// Postconditions:
//   - Response includes workers in all states (online and down).
//   - CurrentTaskID is non-null only for workers that have an active task (assigned or running).
//   - Returns an empty JSON array (not null) when no workers are registered.
func (h *WorkerHandler) List(w http.ResponseWriter, r *http.Request) {
	sess := auth.SessionFromContext(r.Context())
	if sess == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	workers, err := h.server.workers.List(r.Context())
	if err != nil {
		log.Printf("worker.List: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Project domain workers to API response items.
	// Initialise to an empty non-nil slice so the JSON encoder produces [] not null.
	items := make([]workerListItem, 0, len(workers))
	for _, wk := range workers {
		items = append(items, workerListItem{
			ID:            wk.ID,
			Status:        string(wk.Status),
			Tags:          wk.Tags,
			CurrentTaskID: wk.CurrentTaskID,
			LastHeartbeat: wk.LastHeartbeat,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(items)
}
