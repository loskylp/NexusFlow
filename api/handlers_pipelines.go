// Package api — Pipeline CRUD handlers.
// Pipelines are owned by the creating user; ownership is enforced here.
// Design-time schema mapping validation runs on Create and Update (ADR-008, TASK-026, Cycle 2).
// See: REQ-022, TASK-013
package api

import "net/http"

// PipelineHandler handles pipeline CRUD REST endpoints.
// See: TASK-013
type PipelineHandler struct {
	server *Server
}

// Create handles POST /api/pipelines.
// Creates a pipeline with DataSource, Process, and Sink config, plus schema mappings.
// Design-time validation of schema mappings is a Cycle 2 concern (TASK-026).
//
// Request body:
//   {
//     "name": "string",
//     "dataSourceConfig": { "connectorType": "...", "config": {...}, "outputSchema": [...] },
//     "processConfig":    { "connectorType": "...", "config": {...}, "inputMappings": [...], "outputSchema": [...] },
//     "sinkConfig":       { "connectorType": "...", "config": {...}, "inputMappings": [...] }
//   }
//
// Responses:
//   201 Created:       { pipeline }
//   400 Bad Request:   malformed JSON or invalid schema mappings
//   401 Unauthorized:  no valid session
//   500 Internal:      database failure
//
// Postconditions:
//   - On 201: pipeline exists in PostgreSQL with user_id = session.UserID.
func (h *PipelineHandler) Create(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement in TASK-013
	panic("not implemented")
}

// List handles GET /api/pipelines.
// User role: returns caller's own pipelines.
// Admin role: returns all pipelines across all users.
//
// Responses:
//   200 OK:           [ { pipeline }, ... ]
//   401 Unauthorized: no valid session
func (h *PipelineHandler) List(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement in TASK-013
	panic("not implemented")
}

// Get handles GET /api/pipelines/{id}.
//
// Responses:
//   200 OK:           { pipeline }
//   401 Unauthorized: no valid session
//   403 Forbidden:    caller does not own the pipeline and is not Admin
//   404 Not Found:    pipeline does not exist
func (h *PipelineHandler) Get(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement in TASK-013
	panic("not implemented")
}

// Update handles PUT /api/pipelines/{id}.
// Replaces the pipeline's mutable fields (name, phase configs, schema mappings).
//
// Responses:
//   200 OK:           { pipeline }  — updated pipeline
//   400 Bad Request:  malformed JSON
//   401 Unauthorized: no valid session
//   403 Forbidden:    caller does not own the pipeline and is not Admin
//   404 Not Found:    pipeline does not exist
func (h *PipelineHandler) Update(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement in TASK-013
	panic("not implemented")
}

// Delete handles DELETE /api/pipelines/{id}.
// Rejects deletion if any non-terminal Task references this pipeline.
//
// Responses:
//   204 No Content:   pipeline deleted
//   401 Unauthorized: no valid session
//   403 Forbidden:    caller does not own the pipeline and is not Admin
//   404 Not Found:    pipeline does not exist
//   409 Conflict:     pipeline has active (non-terminal) tasks
//
// Postconditions:
//   - On 204: pipeline no longer exists in PostgreSQL.
//   - On 409: pipeline is unchanged.
func (h *PipelineHandler) Delete(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement in TASK-013
	panic("not implemented")
}
