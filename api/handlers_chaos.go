// Package api — Chaos Controller handlers (TASK-034).
//
// Three chaos action endpoints, all Admin-only:
//   POST /api/chaos/kill-worker     — terminates a named worker container
//   POST /api/chaos/disconnect-db   — simulates database unavailability for a duration
//   POST /api/chaos/flood-queue     — submits a burst of tasks to a named pipeline
//
// These endpoints are used exclusively by the Chaos Controller GUI during demos to
// demonstrate auto-recovery capabilities (ADR-002, DEMO-004).
//
// Container management uses the Docker daemon socket (/var/run/docker.sock) so the
// API container must have it mounted in the demo Docker Compose profile.
//
// See: DEMO-004, ADR-002, TASK-034
package api

import (
	"net/http"
)

// ChaosHandler handles the three chaos action endpoints.
// All endpoints require Admin role (enforced by RequireRole middleware at route registration).
// See: DEMO-004, TASK-034
type ChaosHandler struct {
	server *Server
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

// KillWorker handles POST /api/chaos/kill-worker.
// Stops the Docker container for the named worker.
//
// Request body:
//
//	{ "workerId": "string" }
//
// Responses:
//
//	200 OK:           { "log": [{ timestamp, message, level }, ...] }
//	400 Bad Request:  malformed JSON or missing workerId
//	403 Forbidden:    caller is not Admin
//	404 Not Found:    workerId does not match a registered worker
//	500 Internal:     Docker daemon error or worker container not found
//
// Side effects:
//   - The named worker container is stopped via docker kill.
//   - The worker's status in PostgreSQL is updated to "down" by the Monitor on
//     next heartbeat check (ADR-002). The chaos action does not modify the DB directly.
//   - The activity log returned includes the kill event and any immediate observable
//     system response (e.g., task reassignment visible in the workers:active sorted set).
//
// Preconditions:
//   - Caller is Admin (enforced by RequireRole middleware).
//   - workerId is a non-empty string matching a registered worker.
//   - Docker socket is accessible from the API container.
//
// Postconditions:
//   - On success: the worker container has been stopped; the response log reflects
//     the kill event with a precise timestamp.
//   - On error: no container is stopped; the error is returned with context.
func (h *ChaosHandler) KillWorker(w http.ResponseWriter, r *http.Request) {
	// TODO: implement
	panic("not implemented")
}

// DisconnectDatabase handles POST /api/chaos/disconnect-db.
// Simulates database unavailability for the requested duration by blocking
// TCP connections on the PostgreSQL port from the API and Worker containers.
//
// Request body:
//
//	{ "durationSeconds": 15 | 30 | 60 }
//
// Responses:
//
//	200 OK:           { "log": [{ timestamp, message, level }, ...], "durationSeconds": int }
//	400 Bad Request:  malformed JSON, missing durationSeconds, or invalid value
//	403 Forbidden:    caller is not Admin
//	409 Conflict:     a database disconnect is already active
//	500 Internal:     iptables/network manipulation error
//
// Implementation approach:
//   - The disconnect is simulated via a temporary iptables rule (or equivalent network
//     manipulation) that drops traffic to the PostgreSQL port for the given duration.
//   - A background goroutine restores connectivity after durationSeconds elapses.
//   - Only one disconnect may be active at a time; concurrent requests return 409.
//
// Preconditions:
//   - Caller is Admin.
//   - durationSeconds is one of {15, 30, 60}.
//   - No disconnect is currently active.
//
// Postconditions:
//   - On success: PostgreSQL is unreachable from NexusFlow services for durationSeconds.
//                After the duration, connectivity is restored automatically.
//                The response log reflects the disconnect and scheduled reconnect.
//   - On error: no network change; error returned with context.
func (h *ChaosHandler) DisconnectDatabase(w http.ResponseWriter, r *http.Request) {
	// TODO: implement
	panic("not implemented")
}

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
//	403 Forbidden:    caller is not Admin
//	404 Not Found:    pipelineId does not match an existing pipeline
//	500 Internal:     queue enqueue error
//
// Side effects:
//   - taskCount tasks are created in the tasks table with status "submitted" and
//     immediately enqueued to the appropriate Redis stream(s).
//   - Each task uses an empty input map ({}). The pipeline's connector configuration
//     determines what the task does.
//   - Tasks are submitted sequentially; the response includes the count of
//     successfully submitted tasks.
//
// Preconditions:
//   - Caller is Admin.
//   - pipelineId is a valid UUID referencing an existing pipeline.
//   - taskCount is between 1 and 1000 inclusive.
//
// Postconditions:
//   - On success: exactly taskCount tasks are enqueued; submittedCount equals taskCount.
//   - On partial failure: submittedCount reflects the number successfully enqueued
//     before the error occurred; the error is included in the response log.
func (h *ChaosHandler) FloodQueue(w http.ResponseWriter, r *http.Request) {
	// TODO: implement
	panic("not implemented")
}
