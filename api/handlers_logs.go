// Package api — handler for GET /api/tasks/{id}/logs.
// Returns historical log lines from PostgreSQL cold storage for the given task.
// Access control: task owner or admin only (AC-6, REQ-018).
// See: REQ-018, TASK-016
package api

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/nxlabs/nexusflow/internal/auth"
	"github.com/nxlabs/nexusflow/internal/models"
)

// LogHandler handles the historical log retrieval endpoint.
// Depends on TaskRepository (for ownership checks) and TaskLogRepository (for log queries).
// See: REQ-018, TASK-016
type LogHandler struct {
	server *Server
}

// GetLogs handles GET /api/tasks/{id}/logs.
// Returns all cold-stored log lines for the given task ordered by timestamp.
// Enforces ownership: only the task owner or an Admin may retrieve logs (AC-6).
//
// Responses:
//
//	200 OK:           [ { "id", "taskId", "line", "level", "timestamp" }, ... ]
//	400 Bad Request:  {id} is not a valid UUID
//	401 Unauthorized: no valid session
//	403 Forbidden:    caller is not the task owner or Admin
//	404 Not Found:    task does not exist
//	500 Internal:     database failure
//
// Preconditions:
//   - Auth middleware has placed a valid Session in the request context.
//   - chi router has parsed the "id" URL parameter.
//
// Postconditions:
//   - On 200: returns a JSON array of log lines (empty array when no logs exist).
//   - On 403: no log data is disclosed.
func (h *LogHandler) GetLogs(w http.ResponseWriter, r *http.Request) {
	sess := auth.SessionFromContext(r.Context())
	if sess == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	rawID := chi.URLParam(r, "id")
	taskID, err := uuid.Parse(rawID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "task id must be a valid UUID")
		return
	}

	task, err := h.server.tasks.GetByID(r.Context(), taskID)
	if err != nil {
		log.Printf("logs.GetLogs: GetByID(%v): %v", taskID, err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if task == nil {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}

	// AC-6: only the task owner or an Admin may retrieve logs.
	if sess.Role != models.RoleAdmin && task.UserID != sess.UserID {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	logs, err := h.server.taskLogs.ListByTask(r.Context(), taskID, "")
	if err != nil {
		log.Printf("logs.GetLogs: ListByTask(%v): %v", taskID, err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Always return a JSON array, never null.
	if logs == nil {
		logs = []*models.TaskLog{}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(logs)
}
