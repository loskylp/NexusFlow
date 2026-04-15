// Package api — Chaos Controller handlers (TASK-034).
//
// Three chaos action endpoints, all Admin-only:
//
//	POST /api/chaos/kill-worker     — terminates a named worker container
//	POST /api/chaos/disconnect-db   — simulates database unavailability for a duration
//	POST /api/chaos/flood-queue     — submits a burst of tasks to a named pipeline
//
// Admin enforcement is applied via auth.RequireRole(models.RoleAdmin) in server.go at
// route registration. The handlers themselves do not re-check the role; this avoids the
// UI-layer-only pattern observed in OBS-032-1.
//
// Container management uses the Docker daemon socket (/var/run/docker.sock) so the
// API container must have it mounted in the demo Docker Compose profile.
//
// The DB disconnect simulation stops the postgres container and schedules a goroutine
// to restart it after the requested duration. This approach works without NET_ADMIN
// capability and avoids iptables dependency (scaffold ambiguity #2 resolved here).
//
// See: DEMO-004, ADR-002, TASK-034
package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/nxlabs/nexusflow/internal/models"
	"github.com/nxlabs/nexusflow/internal/queue"
)

// ChaosHandler handles the three chaos action endpoints.
// All endpoints require Admin role (enforced by RequireRole middleware at route registration).
// See: DEMO-004, TASK-034
type ChaosHandler struct {
	server *Server
	// disconnectActive is 1 while a database disconnect is in progress, 0 otherwise.
	// Used as an atomic guard to prevent concurrent disconnect requests (409 guard).
	disconnectActive atomic.Int32
	// dockerSocketPath is the path to the Docker daemon socket.
	// Defaults to /var/run/docker.sock; overridden in tests.
	dockerSocketPath string
}

// killWorkerRequest is the JSON body for POST /api/chaos/kill-worker.
type killWorkerRequest struct {
	// WorkerID is the ID of the worker to kill. Must match a currently registered
	// worker in the workers table. Required.
	WorkerID string `json:"workerId"`
}

// disconnectDBRequest is the JSON body for POST /api/chaos/disconnect-db.
type disconnectDBRequest struct {
	// DurationSeconds is the number of seconds to simulate database unavailability.
	// Must be one of 15, 30, or 60.
	DurationSeconds int `json:"durationSeconds"`
}

// floodQueueRequest is the JSON body for POST /api/chaos/flood-queue.
type floodQueueRequest struct {
	// PipelineID is the ID of the pipeline to use for the flood tasks. Required.
	PipelineID string `json:"pipelineId"`

	// TaskCount is the number of tasks to submit in the burst.
	// Must be between 1 and 1000 inclusive.
	TaskCount int `json:"taskCount"`
}

// chaosActivityEntry is one timestamped log line returned from chaos endpoints.
// The GUI appends these to the relevant card's activity log.
type chaosActivityEntry struct {
	Timestamp string `json:"timestamp"`
	Message   string `json:"message"`
	Level     string `json:"level"` // "info", "warn", or "error"
}

// killWorkerResponse is the JSON body returned on 200 from POST /api/chaos/kill-worker.
type killWorkerResponse struct {
	Log []chaosActivityEntry `json:"log"`
}

// disconnectDBResponse is the JSON body returned on 200 from POST /api/chaos/disconnect-db.
type disconnectDBResponse struct {
	Log             []chaosActivityEntry `json:"log"`
	DurationSeconds int                  `json:"durationSeconds"`
}

// floodQueueResponse is the JSON body returned on 200 from POST /api/chaos/flood-queue.
type floodQueueResponse struct {
	SubmittedCount int                  `json:"submittedCount"`
	Log            []chaosActivityEntry `json:"log"`
}

// newActivity creates a timestamped activity log entry at the given level.
func newActivity(level, message string) chaosActivityEntry {
	return chaosActivityEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Message:   message,
		Level:     level,
	}
}

// dockerSocketForHandler returns the configured socket path, defaulting to
// /var/run/docker.sock when no override has been set.
func (h *ChaosHandler) dockerSocketForHandler() string {
	if h.dockerSocketPath != "" {
		return h.dockerSocketPath
	}
	return "/var/run/docker.sock"
}

// runDockerCommand executes a docker command using the Docker CLI, routing through
// the configured socket. It captures combined stdout+stderr and returns them as a
// single string alongside any error.
//
// The DOCKER_HOST environment variable is set to "unix://<socket>" so the Docker
// CLI uses the correct socket path without needing root access.
//
// Preconditions:
//   - The Docker daemon socket is mounted and accessible.
//
// Postconditions:
//   - On success: the command completed; combined output returned.
//   - On error: the command failed; combined output and error both returned.
func (h *ChaosHandler) runDockerCommand(args ...string) (string, error) {
	cmd := exec.Command("docker", args...)
	cmd.Env = append(cmd.Environ(), fmt.Sprintf("DOCKER_HOST=unix://%s", h.dockerSocketForHandler()))
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return out.String(), err
}

// KillWorker handles POST /api/chaos/kill-worker.
// Stops the Docker container for the named worker by sending SIGKILL.
//
// Request body:
//
//	{ "workerId": "string" }
//
// Responses:
//
//	200 OK:           { "log": [{ timestamp, message, level }, ...] }
//	400 Bad Request:  malformed JSON or missing workerId
//	403 Forbidden:    caller is not Admin (enforced at route level)
//	404 Not Found:    workerId does not match a registered worker
//	500 Internal:     Docker daemon error or worker container not found
//
// Side effects:
//   - The named worker container is stopped via docker kill.
//   - The worker's status in PostgreSQL is updated to "down" by the Monitor on
//     next heartbeat check (ADR-002). The chaos action does not modify the DB directly.
//
// Preconditions:
//   - Caller is Admin (enforced by RequireRole middleware; not re-checked here per
//     OBS-032-1 avoidance pattern — server.go wires RequireRole before this handler).
//   - workerId is a non-empty string matching a registered worker.
//   - Docker socket is accessible from the API container.
//
// Postconditions:
//   - On success: the worker container has been stopped; response log reflects the kill event.
//   - On error: no container is stopped; the error is returned with context.
func (h *ChaosHandler) KillWorker(w http.ResponseWriter, r *http.Request) {
	var req killWorkerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.WorkerID == "" {
		writeError(w, http.StatusBadRequest, "workerId is required")
		return
	}

	// Verify the worker exists in the database.
	worker, err := h.server.workers.GetByID(r.Context(), req.WorkerID)
	if err != nil {
		log.Printf("chaos.KillWorker: GetByID(%q): %v", req.WorkerID, err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if worker == nil {
		writeError(w, http.StatusNotFound, "worker not found")
		return
	}

	var activityLog []chaosActivityEntry
	activityLog = append(activityLog, newActivity("info",
		fmt.Sprintf("Sending SIGKILL to worker container %q", req.WorkerID)))

	output, dockerErr := h.runDockerCommand("kill", req.WorkerID)
	if dockerErr != nil {
		log.Printf("chaos.KillWorker: docker kill %q: %v (output: %s)", req.WorkerID, dockerErr, output)
		// activityLog is not sent on the error path; write the error directly.
		writeError(w, http.StatusInternalServerError, "docker kill failed: "+dockerErr.Error())
		return
	}

	activityLog = append(activityLog,
		newActivity("info", fmt.Sprintf("Container %q killed successfully", req.WorkerID)))
	activityLog = append(activityLog,
		newActivity("info", "Monitor will detect heartbeat absence and reclaim in-flight tasks (ADR-002)"))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(killWorkerResponse{Log: activityLog})
}

// postgresContainerName is the name used to identify the main PostgreSQL container in Docker.
// This matches the service name in docker-compose.yml and the docker-compose project convention.
const postgresContainerName = "nexusflow-postgres-1"

// DisconnectDatabase handles POST /api/chaos/disconnect-db.
// Simulates database unavailability by stopping the postgres container for the
// requested duration, then restarting it automatically.
//
// This approach avoids iptables (which requires NET_ADMIN capability) and instead
// uses docker stop/start on the postgres container, which is accessible via the
// mounted Docker socket. The API and worker will fail DB operations for the duration.
//
// Request body:
//
//	{ "durationSeconds": 15 | 30 | 60 }
//
// Responses:
//
//	200 OK:           { "log": [{ timestamp, message, level }, ...], "durationSeconds": int }
//	400 Bad Request:  malformed JSON, missing durationSeconds, or invalid value
//	403 Forbidden:    caller is not Admin (enforced at route level)
//	409 Conflict:     a database disconnect is already active
//	500 Internal:     Docker daemon error
//
// Preconditions:
//   - Caller is Admin (enforced by RequireRole middleware at route registration).
//   - durationSeconds is one of {15, 30, 60}.
//   - No disconnect is currently active.
//
// Postconditions:
//   - On success: postgres container is stopped; a background goroutine restarts it after
//     durationSeconds. The response log reflects the disconnect and scheduled restart.
//   - On error: no container change; error returned with context.
func (h *ChaosHandler) DisconnectDatabase(w http.ResponseWriter, r *http.Request) {
	var req disconnectDBRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validate duration: must be 15, 30, or 60.
	if req.DurationSeconds != 15 && req.DurationSeconds != 30 && req.DurationSeconds != 60 {
		writeError(w, http.StatusBadRequest, "durationSeconds must be 15, 30, or 60")
		return
	}

	// 409 guard: prevent concurrent disconnects.
	if !h.disconnectActive.CompareAndSwap(0, 1) {
		writeError(w, http.StatusConflict, "a database disconnect is already active")
		return
	}

	var activityLog []chaosActivityEntry
	activityLog = append(activityLog, newActivity("info",
		fmt.Sprintf("Stopping postgres container for %ds", req.DurationSeconds)))

	output, dockerErr := h.runDockerCommand("stop", postgresContainerName)
	if dockerErr != nil {
		h.disconnectActive.Store(0) // release the guard on failure
		log.Printf("chaos.DisconnectDatabase: docker stop %q: %v (output: %s)", postgresContainerName, dockerErr, output)
		// activityLog is not sent on the error path; write the error directly.
		writeError(w, http.StatusInternalServerError, "docker stop failed: "+dockerErr.Error())
		return
	}

	activityLog = append(activityLog,
		newActivity("info", fmt.Sprintf("Postgres container stopped. Database will be unavailable for %ds.", req.DurationSeconds)))
	activityLog = append(activityLog,
		newActivity("info", fmt.Sprintf("Scheduled restart in %ds. Workers will log connection errors during this window.", req.DurationSeconds)))

	// Background goroutine restores connectivity after the requested duration.
	duration := req.DurationSeconds
	go func() {
		time.Sleep(time.Duration(duration) * time.Second)
		restoreOutput, restoreErr := h.runDockerCommand("start", postgresContainerName)
		if restoreErr != nil {
			log.Printf("chaos.DisconnectDatabase: failed to restart postgres container: %v (output: %s)", restoreErr, restoreOutput)
		} else {
			log.Printf("chaos.DisconnectDatabase: postgres container restarted after %ds", duration)
		}
		h.disconnectActive.Store(0) // release the guard after restore
	}()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(disconnectDBResponse{
		Log:             activityLog,
		DurationSeconds: req.DurationSeconds,
	})
}

// floodDefaultTags are the task queue tags used for flood tasks.
// The worker fleet is configured with WORKER_TAGS=demo,etl in docker-compose.yml,
// so "demo" is guaranteed to have at least one consumer.
var floodDefaultTags = []string{"demo"}

// FloodQueue handles POST /api/chaos/flood-queue.
// Submits taskCount tasks for the named pipeline in rapid succession to saturate
// the queue and demonstrate auto-scaling behaviour.
//
// Request body:
//
//	{ "pipelineId": "string", "taskCount": int }
//
// Responses:
//
//	200 OK:           { "submittedCount": int, "log": [{ timestamp, message, level }, ...] }
//	400 Bad Request:  malformed JSON, missing pipelineId, or taskCount out of [1, 1000]
//	403 Forbidden:    caller is not Admin (enforced at route level)
//	404 Not Found:    pipelineId does not match an existing pipeline
//	500 Internal:     queue enqueue error
//
// Side effects:
//   - taskCount tasks are created in the tasks table with status "submitted" then
//     immediately transitioned to "queued" and enqueued to the appropriate Redis stream.
//   - Each task uses an empty input map ({}).
//   - Tasks are submitted sequentially; the response includes the count of
//     successfully submitted tasks.
//
// Preconditions:
//   - Caller is Admin (enforced by RequireRole middleware at route registration).
//   - pipelineId is a valid UUID referencing an existing pipeline.
//   - taskCount is between 1 and 1000 inclusive.
//
// Postconditions:
//   - On success: exactly taskCount tasks are enqueued; submittedCount equals taskCount.
//   - On partial failure: submittedCount reflects the number successfully enqueued
//     before the error occurred; the error is included in the response log.
func (h *ChaosHandler) FloodQueue(w http.ResponseWriter, r *http.Request) {
	var req floodQueueRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.PipelineID == "" {
		writeError(w, http.StatusBadRequest, "pipelineId is required")
		return
	}
	if req.TaskCount < 1 || req.TaskCount > 1000 {
		writeError(w, http.StatusBadRequest, "taskCount must be between 1 and 1000")
		return
	}

	pipelineID, err := uuid.Parse(req.PipelineID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "pipelineId must be a valid UUID")
		return
	}

	pipeline, err := h.server.pipelines.GetByID(r.Context(), pipelineID)
	if err != nil {
		log.Printf("chaos.FloodQueue: GetByID(%v): %v", pipelineID, err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if pipeline == nil {
		writeError(w, http.StatusNotFound, "pipeline not found")
		return
	}

	var activityLog []chaosActivityEntry
	activityLog = append(activityLog, newActivity("info",
		fmt.Sprintf("Flooding queue: submitting %d tasks for pipeline %q", req.TaskCount, pipeline.Name)))

	submittedCount := 0
	ctx := r.Context()

	for i := 0; i < req.TaskCount; i++ {
		taskID := uuid.New()
		executionID := taskID.String() + ":0"

		task := &models.Task{
			ID:          taskID,
			PipelineID:  &pipelineID,
			UserID:      pipeline.UserID, // attribute flood tasks to the pipeline owner
			Status:      models.TaskStatusSubmitted,
			RetryConfig: models.DefaultRetryConfig(),
			RetryCount:  0,
			ExecutionID: executionID,
			Input:       map[string]any{},
			CreatedAt:   time.Now().UTC(),
			UpdatedAt:   time.Now().UTC(),
		}

		created, createErr := h.server.tasks.Create(ctx, task)
		if createErr != nil {
			log.Printf("chaos.FloodQueue: Create task %d/%d: %v", i+1, req.TaskCount, createErr)
			activityLog = append(activityLog,
				newActivity("error", fmt.Sprintf("Task creation failed at %d/%d: %v", i+1, req.TaskCount, createErr)))
			break
		}

		if statusErr := h.server.tasks.UpdateStatus(ctx, created.ID, models.TaskStatusQueued, "chaos flood enqueue", nil); statusErr != nil {
			log.Printf("chaos.FloodQueue: UpdateStatus task %d/%d: %v", i+1, req.TaskCount, statusErr)
			activityLog = append(activityLog,
				newActivity("error", fmt.Sprintf("Status update failed at %d/%d: %v", i+1, req.TaskCount, statusErr)))
			break
		}

		if _, enqueueErr := h.server.producer.Enqueue(ctx, &queue.ProducerMessage{
			Task: created,
			Tags: floodDefaultTags,
		}); enqueueErr != nil {
			log.Printf("chaos.FloodQueue: Enqueue task %d/%d: %v", i+1, req.TaskCount, enqueueErr)
			activityLog = append(activityLog,
				newActivity("error", fmt.Sprintf("Enqueue failed at %d/%d: %v", i+1, req.TaskCount, enqueueErr)))
			break
		}

		submittedCount++
	}

	if submittedCount == req.TaskCount {
		activityLog = append(activityLog,
			newActivity("info", fmt.Sprintf("Flood complete: %d tasks submitted to queue", submittedCount)))
	} else {
		activityLog = append(activityLog,
			newActivity("warn", fmt.Sprintf("Flood partial: %d/%d tasks submitted before error", submittedCount, req.TaskCount)))
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(floodQueueResponse{
		SubmittedCount: submittedCount,
		Log:            activityLog,
	})
}
