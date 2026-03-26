// Package api — Task handlers: submission, query, and cancellation.
// See: REQ-001, REQ-009, REQ-010, TASK-005, TASK-008, TASK-012
package api

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/nxlabs/nexusflow/internal/auth"
	"github.com/nxlabs/nexusflow/internal/models"
	"github.com/nxlabs/nexusflow/internal/queue"
)

// TaskHandler handles task-related REST endpoints.
// Depends on TaskRepository, PipelineRepository, and the queue Producer.
// See: TASK-005, TASK-008, TASK-012
type TaskHandler struct {
	server *Server
}

// submitRequest is the JSON body for POST /api/tasks.
type submitRequest struct {
	// PipelineID is the UUID of the Pipeline to execute. Required.
	PipelineID string `json:"pipelineId"`
	// Input holds arbitrary key-value parameters passed to the pipeline at runtime.
	Input map[string]any `json:"input"`
	// Tags are the capability tags used to route the task to a matching Worker (ADR-001).
	// At least one tag is required.
	Tags []string `json:"tags"`
	// RetryConfig is optional. When absent, DefaultRetryConfig() is applied.
	RetryConfig *models.RetryConfig `json:"retryConfig,omitempty"`
}

// submitResponse is the JSON body returned on a successful 201 from POST /api/tasks.
type submitResponse struct {
	TaskID uuid.UUID `json:"taskId"`
	Status string    `json:"status"`
}

// errorResponse is the structured JSON body returned on 4xx responses.
type errorResponse struct {
	Error string `json:"error"`
}

// Submit handles POST /api/tasks.
// Validates the pipeline reference, inserts the task into PostgreSQL (status: submitted),
// enqueues it in the appropriate Redis stream (status: queued), and returns 201.
//
// Request body:
//
//	{
//	  "pipelineId":  "uuid",
//	  "input":       { ... },
//	  "tags":        ["tag1", ...],
//	  "retryConfig": { "maxRetries": 3, "backoff": "exponential" }  // optional
//	}
//
// Responses:
//
//	201 Created:      { "taskId": "uuid", "status": "queued" }
//	400 Bad Request:  malformed JSON, missing required fields, or invalid pipelineId
//	401 Unauthorized: no valid session
//	500 Internal:     database or Redis failure
//
// Preconditions:
//   - Auth middleware has placed a valid Session in the request context.
//   - The referenced Pipeline must exist in PostgreSQL.
//
// Postconditions:
//   - On 201: task exists in PostgreSQL with status "queued"; task message is in queue:{tag}.
//   - State transition submitted -> queued is recorded in task_state_log.
//   - Default RetryConfig applied when not provided in request body.
func (h *TaskHandler) Submit(w http.ResponseWriter, r *http.Request) {
	sess := auth.SessionFromContext(r.Context())
	if sess == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req submitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.PipelineID == "" {
		writeError(w, http.StatusBadRequest, "pipelineId is required")
		return
	}
	if len(req.Tags) == 0 {
		writeError(w, http.StatusBadRequest, "tags must not be empty")
		return
	}

	pipelineID, err := uuid.Parse(req.PipelineID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "pipelineId must be a valid UUID")
		return
	}

	pipeline, err := h.server.pipelines.GetByID(r.Context(), pipelineID)
	if err != nil {
		log.Printf("task.Submit: GetPipelineByID(%v): %v", pipelineID, err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if pipeline == nil {
		writeError(w, http.StatusBadRequest, "pipeline not found")
		return
	}

	retryConfig := resolveRetryConfig(req.RetryConfig)

	taskID := uuid.New()
	input := req.Input
	if input == nil {
		input = map[string]any{}
	}

	// ExecutionID encodes task ID + attempt number for Sink idempotency (ADR-003).
	executionID := taskID.String() + ":0"

	task := &models.Task{
		ID:          taskID,
		PipelineID:  &pipelineID,
		UserID:      sess.UserID,
		Status:      models.TaskStatusSubmitted,
		RetryConfig: retryConfig,
		RetryCount:  0,
		ExecutionID: executionID,
		Input:       input,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}

	created, err := h.server.tasks.Create(r.Context(), task)
	if err != nil {
		log.Printf("task.Submit: Create task: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	_, err = h.server.producer.Enqueue(r.Context(), &queue.ProducerMessage{
		Task: created,
		Tags: req.Tags,
	})
	if err != nil {
		log.Printf("task.Submit: Enqueue task %v: %v", created.ID, err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := h.server.tasks.UpdateStatus(r.Context(), created.ID, models.TaskStatusQueued, "enqueued to Redis stream", nil); err != nil {
		log.Printf("task.Submit: UpdateStatus to queued for task %v: %v", created.ID, err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(submitResponse{
		TaskID: created.ID,
		Status: string(models.TaskStatusQueued),
	})
}

// List handles GET /api/tasks.
// Returns all tasks visible to the caller: own tasks for User role; all tasks for Admin.
// Satisfies Domain Invariant 5 (visibility isolation).
//
// Responses:
//
//	200 OK:           [ { task }, ... ]
//	401 Unauthorized: no valid session
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
//
//	200 OK:           { task, stateLog: [...] }
//	401 Unauthorized: no valid session
//	403 Forbidden:    caller does not own the task and is not Admin
//	404 Not Found:    task does not exist
func (h *TaskHandler) Get(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement in TASK-008 (Cycle 2)
	panic("not implemented")
}

// Cancel handles POST /api/tasks/{id}/cancel.
// Cancels a task in a non-terminal state.
// Domain Invariant 8: only the submitting User or an Admin may cancel.
//
// Responses:
//
//	204 No Content:   task cancelled
//	401 Unauthorized: no valid session
//	403 Forbidden:    caller is not the task owner or Admin
//	404 Not Found:    task does not exist
//	409 Conflict:     task is already in a terminal state (completed, failed, cancelled)
//
// Postconditions:
//   - On 204: task.Status = "cancelled"; SSE event published.
func (h *TaskHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement in TASK-012 (Cycle 2)
	panic("not implemented")
}

// resolveRetryConfig returns the caller-supplied RetryConfig if present and valid,
// otherwise returns the DefaultRetryConfig. This enforces acceptance criterion 5:
// tasks submitted without retry config receive safe defaults (max_retries=3, backoff=exponential).
func resolveRetryConfig(cfg *models.RetryConfig) models.RetryConfig {
	if cfg == nil {
		return models.DefaultRetryConfig()
	}
	return *cfg
}

// writeError writes a structured JSON error response with the given status code and message.
// All 4xx and 5xx responses from task handlers use this function for consistency.
func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorResponse{Error: msg})
}
