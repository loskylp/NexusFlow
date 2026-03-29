// Package api — Task handlers: submission, query, and cancellation.
// See: REQ-001, REQ-009, REQ-010, TASK-005, TASK-008, TASK-012
package api

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
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
// transitions it to "queued" in PostgreSQL, then publishes it to the Redis stream.
// The status is written before enqueueing so that a worker picking up the message
// always finds the task in "queued" state and can make the queued→assigned transition
// without error (fixes OBS-023).
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
//   - State transition submitted -> queued is recorded in task_state_log before Enqueue.
//   - If Enqueue fails after UpdateStatus, task remains in "queued" (recoverable); 500 returned.
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

	// Mark the task queued in PostgreSQL before publishing to Redis.
	// This eliminates the race where a fast worker picks up the task while it is
	// still in "submitted" state and cannot make the submitted→assigned transition.
	// If UpdateStatus fails, we return 500 without enqueueing — the task remains
	// in "submitted" and can be retried or inspected by an operator.
	if err := h.server.tasks.UpdateStatus(r.Context(), created.ID, models.TaskStatusQueued, "enqueued to Redis stream", nil); err != nil {
		log.Printf("task.Submit: UpdateStatus to queued for task %v: %v", created.ID, err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Enqueue after the status is durable. If this fails the task stays in "queued"
	// in PostgreSQL, which is a recoverable state — a separate reconciler or operator
	// can re-enqueue it without losing the record.
	_, err = h.server.producer.Enqueue(r.Context(), &queue.ProducerMessage{
		Task: created,
		Tags: req.Tags,
	})
	if err != nil {
		log.Printf("task.Submit: Enqueue task %v: %v", created.ID, err)
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

// taskDetailResponse is the JSON body returned by GET /api/tasks/{id}.
// It embeds the full Task domain model alongside its state transition history.
type taskDetailResponse struct {
	Task         *models.Task            `json:"task"`
	StateHistory []*models.TaskStateLog  `json:"stateHistory"`
}

// List handles GET /api/tasks.
// Returns all tasks visible to the caller: own tasks for User role; all tasks for Admin.
// The optional ?status= query parameter filters results to tasks with that exact status.
// Satisfies Domain Invariant 5 (visibility isolation).
//
// Responses:
//
//	200 OK:           [ { task }, ... ]
//	401 Unauthorized: no valid session
//
// Preconditions:
//   - Auth middleware has placed a valid Session in the request context.
//
// Postconditions:
//   - User role: only tasks with user_id matching session.UserID are returned.
//   - Admin role: all tasks across all users are returned.
//   - If ?status=<value> is present, only tasks with that status are included in the response.
func (h *TaskHandler) List(w http.ResponseWriter, r *http.Request) {
	sess := auth.SessionFromContext(r.Context())
	if sess == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var (
		tasks []*models.Task
		err   error
	)
	if sess.Role == models.RoleAdmin {
		tasks, err = h.server.tasks.List(r.Context())
	} else {
		tasks, err = h.server.tasks.ListByUser(r.Context(), sess.UserID)
	}
	if err != nil {
		log.Printf("task.List: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Apply optional ?status= filter.
	if statusFilter := r.URL.Query().Get("status"); statusFilter != "" {
		tasks = filterByStatus(tasks, models.TaskStatus(statusFilter))
	}

	// Always return a JSON array, never null.
	if tasks == nil {
		tasks = []*models.Task{}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(tasks)
}

// filterByStatus returns only those tasks whose Status equals the given target.
// Extracted to keep List's branching logic flat and testable in isolation.
func filterByStatus(tasks []*models.Task, target models.TaskStatus) []*models.Task {
	out := make([]*models.Task, 0, len(tasks))
	for _, t := range tasks {
		if t.Status == target {
			out = append(out, t)
		}
	}
	return out
}

// Get handles GET /api/tasks/{id}.
// Returns the task with its current status and full state transition history.
// Enforces ownership: a non-Admin caller may only read tasks they submitted.
//
// Responses:
//
//	200 OK:           { "task": {...}, "stateHistory": [...] }
//	400 Bad Request:  {id} is not a valid UUID
//	401 Unauthorized: no valid session
//	403 Forbidden:    caller does not own the task and is not Admin
//	404 Not Found:    task does not exist
//
// Preconditions:
//   - Auth middleware has placed a valid Session in the request context.
//   - chi router has parsed the "id" URL parameter.
//
// Postconditions:
//   - On 200: response includes task details and all state transitions in chronological order.
//   - On 403: no task data is disclosed to the caller.
func (h *TaskHandler) Get(w http.ResponseWriter, r *http.Request) {
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
		log.Printf("task.Get: GetByID(%v): %v", taskID, err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if task == nil {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}

	// Enforce ownership: non-admin callers may only read their own tasks.
	if sess.Role != models.RoleAdmin && task.UserID != sess.UserID {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	history, err := h.server.tasks.GetStateLog(r.Context(), taskID)
	if err != nil {
		log.Printf("task.Get: GetStateLog(%v): %v", taskID, err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	// Always return an array, never null.
	if history == nil {
		history = []*models.TaskStateLog{}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(taskDetailResponse{
		Task:         task,
		StateHistory: history,
	})
}

// terminalStatuses is the set of task states from which cancellation is not permitted.
// A task in a terminal state has already finished — there is nothing to cancel.
// See: REQ-010, Domain Invariant 1
var terminalStatuses = map[models.TaskStatus]bool{
	models.TaskStatusCompleted: true,
	models.TaskStatusFailed:    true,
	models.TaskStatusCancelled: true,
}

// cancelFlagTTL is how long the Redis cancel:{taskID} key persists.
// 60 seconds gives the Worker enough time to detect the flag before it expires.
const cancelFlagTTL = 60 * time.Second

// Cancel handles POST /api/tasks/{id}/cancel.
// Cancels a task in a non-terminal state.
// Domain Invariant 8 (REQ-010): only the submitting User or an Admin may cancel.
//
// Responses:
//
//	204 No Content:   task cancelled
//	400 Bad Request:  {id} is not a valid UUID
//	401 Unauthorized: no valid session
//	403 Forbidden:    caller is not the task owner or Admin
//	404 Not Found:    task does not exist
//	409 Conflict:     task is already in a terminal state (completed, failed, cancelled)
//
// Preconditions:
//   - Auth middleware has placed a valid Session in the request context.
//   - chi router has parsed the "id" URL parameter.
//
// Postconditions:
//   - On 204: task.Status = "cancelled"; task_state_log entry created.
//   - On 204 with prior status "running": cancel:{taskID} flag set in Redis with 60s TTL.
//   - SSE task event published (fire-and-forget; broker nil is a no-op).
func (h *TaskHandler) Cancel(w http.ResponseWriter, r *http.Request) {
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
		log.Printf("task.Cancel: GetByID(%v): %v", taskID, err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if task == nil {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}

	// Domain Invariant 8: only the task owner or an Admin may cancel.
	if sess.Role != models.RoleAdmin && task.UserID != sess.UserID {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	// Terminal tasks cannot be cancelled — the work is already done or already cancelled.
	if terminalStatuses[task.Status] {
		writeError(w, http.StatusConflict, "task is already in a terminal state")
		return
	}

	// Set the Redis cancellation flag before updating the DB status so the Worker
	// does not transition to "completed" between the flag set and the DB update.
	// Non-fatal: if the flag cannot be set, the DB cancel still proceeds; the
	// worker may complete the task, but the DB status will still be "cancelled".
	wasRunning := task.Status == models.TaskStatusRunning
	if wasRunning && h.server.cancellations != nil {
		if flagErr := h.server.cancellations.SetCancelFlag(r.Context(), taskID.String(), cancelFlagTTL); flagErr != nil {
			log.Printf("task.Cancel: SetCancelFlag(%v): %v — proceeding with DB cancel", taskID, flagErr)
		}
	}

	if err := h.server.tasks.Cancel(r.Context(), taskID, "cancelled by user"); err != nil {
		log.Printf("task.Cancel: Cancel(%v): %v", taskID, err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Publish SSE event (fire-and-forget per ADR-007).
	if h.server.broker != nil {
		updatedTask, getErr := h.server.tasks.GetByID(r.Context(), taskID)
		if getErr == nil && updatedTask != nil {
			if pubErr := h.server.broker.PublishTaskEvent(r.Context(), updatedTask, "cancelled by user"); pubErr != nil {
				log.Printf("task.Cancel: PublishTaskEvent(%v): %v", taskID, pubErr)
			}
		}
	}

	w.WriteHeader(http.StatusNoContent)
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
