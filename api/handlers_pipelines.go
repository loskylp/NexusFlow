// Package api — Pipeline CRUD handlers.
// Pipelines are owned by the creating user; ownership is enforced here.
// Design-time schema mapping validation runs on Create and Update (ADR-008, TASK-026, Cycle 2).
// See: REQ-022, TASK-013
package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/nxlabs/nexusflow/internal/auth"
	"github.com/nxlabs/nexusflow/internal/db"
	"github.com/nxlabs/nexusflow/internal/models"
	"github.com/nxlabs/nexusflow/internal/pipeline"
)

// PipelineHandler handles pipeline CRUD REST endpoints.
// See: TASK-013
type PipelineHandler struct {
	server *Server
}

// createPipelineRequest is the JSON body for POST /api/pipelines.
type createPipelineRequest struct {
	// Name is the human-readable identifier for the pipeline. Required; must be non-empty.
	Name             string                 `json:"name"`
	DataSourceConfig models.DataSourceConfig `json:"dataSourceConfig"`
	ProcessConfig    models.ProcessConfig    `json:"processConfig"`
	SinkConfig       models.SinkConfig       `json:"sinkConfig"`
}

// updatePipelineRequest is the JSON body for PUT /api/pipelines/{id}.
// Replaces all mutable fields; all fields are required.
type updatePipelineRequest struct {
	Name             string                 `json:"name"`
	DataSourceConfig models.DataSourceConfig `json:"dataSourceConfig"`
	ProcessConfig    models.ProcessConfig    `json:"processConfig"`
	SinkConfig       models.SinkConfig       `json:"sinkConfig"`
}

// Create handles POST /api/pipelines.
// Creates a pipeline with DataSource, Process, and Sink config, plus schema mappings.
// Schema mappings are validated at design time (TASK-026, ADR-008): all SourceFields in
// ProcessConfig.InputMappings must exist in DataSourceConfig.OutputSchema, and all
// SourceFields in SinkConfig.InputMappings must exist in ProcessConfig.OutputSchema.
//
// Request body:
//
//	{
//	  "name": "string",
//	  "dataSourceConfig": { "connectorType": "...", "config": {...}, "outputSchema": [...] },
//	  "processConfig":    { "connectorType": "...", "config": {...}, "inputMappings": [...], "outputSchema": [...] },
//	  "sinkConfig":       { "connectorType": "...", "config": {...}, "inputMappings": [...] }
//	}
//
// Responses:
//
//	201 Created:       { pipeline }
//	400 Bad Request:   malformed JSON, missing required fields, or invalid schema mapping
//	401 Unauthorized:  no valid session
//	500 Internal:      database failure
//
// Preconditions:
//   - Auth middleware has placed a valid Session in the request context.
//
// Postconditions:
//   - On 201: pipeline exists in PostgreSQL with user_id = session.UserID.
//   - On 400 (schema mapping error): no pipeline is persisted.
func (h *PipelineHandler) Create(w http.ResponseWriter, r *http.Request) {
	sess := auth.SessionFromContext(r.Context())
	if sess == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req createPipelineRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	candidate := models.Pipeline{
		DataSourceConfig: req.DataSourceConfig,
		ProcessConfig:    req.ProcessConfig,
		SinkConfig:       req.SinkConfig,
	}
	if err := pipeline.ValidateSchemaMappings(candidate); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	p := &models.Pipeline{
		ID:               uuid.New(),
		Name:             req.Name,
		UserID:           sess.UserID,
		DataSourceConfig: req.DataSourceConfig,
		ProcessConfig:    req.ProcessConfig,
		SinkConfig:       req.SinkConfig,
	}

	created, err := h.server.pipelines.Create(r.Context(), p)
	if err != nil {
		log.Printf("pipeline.Create: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(created)
}

// List handles GET /api/pipelines.
// User role: returns caller's own pipelines.
// Admin role: returns all pipelines across all users.
//
// Responses:
//
//	200 OK:           [ { pipeline }, ... ]
//	401 Unauthorized: no valid session
//
// Postconditions:
//   - User role: only pipelines with user_id matching session.UserID are returned.
//   - Admin role: all pipelines across all users are returned.
func (h *PipelineHandler) List(w http.ResponseWriter, r *http.Request) {
	sess := auth.SessionFromContext(r.Context())
	if sess == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var (
		pipelines []*models.Pipeline
		err       error
	)

	if sess.Role == models.RoleAdmin {
		pipelines, err = h.server.pipelines.List(r.Context())
	} else {
		pipelines, err = h.server.pipelines.ListByUser(r.Context(), sess.UserID)
	}

	if err != nil {
		log.Printf("pipeline.List: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Return an empty JSON array rather than null when no pipelines exist.
	if pipelines == nil {
		pipelines = []*models.Pipeline{}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(pipelines)
}

// Get handles GET /api/pipelines/{id}.
// Ownership enforcement: non-owner, non-admin callers receive 403.
//
// Responses:
//
//	200 OK:           { pipeline }
//	400 Bad Request:  {id} is not a valid UUID
//	401 Unauthorized: no valid session
//	403 Forbidden:    caller does not own the pipeline and is not Admin
//	404 Not Found:    pipeline does not exist
func (h *PipelineHandler) Get(w http.ResponseWriter, r *http.Request) {
	sess := auth.SessionFromContext(r.Context())
	if sess == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	id, ok := parsePipelineID(w, r)
	if !ok {
		return
	}

	pipeline, err := h.server.pipelines.GetByID(r.Context(), id)
	if err != nil {
		log.Printf("pipeline.Get: GetByID(%v): %v", id, err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if pipeline == nil {
		writeError(w, http.StatusNotFound, "pipeline not found")
		return
	}

	if !canAccessPipeline(sess, pipeline) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(pipeline)
}

// Update handles PUT /api/pipelines/{id}.
// Replaces the pipeline's mutable fields (name, phase configs, schema mappings).
// Schema mappings are validated at design time (TASK-026, ADR-008) before any
// database write: all SourceFields in ProcessConfig.InputMappings must exist in
// DataSourceConfig.OutputSchema, and all SourceFields in SinkConfig.InputMappings
// must exist in ProcessConfig.OutputSchema.
// Ownership enforcement: non-owner, non-admin callers receive 403.
//
// Responses:
//
//	200 OK:           { pipeline }  — updated pipeline
//	400 Bad Request:  malformed JSON, invalid {id}, or invalid schema mapping
//	401 Unauthorized: no valid session
//	403 Forbidden:    caller does not own the pipeline and is not Admin
//	404 Not Found:    pipeline does not exist
func (h *PipelineHandler) Update(w http.ResponseWriter, r *http.Request) {
	sess := auth.SessionFromContext(r.Context())
	if sess == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	id, ok := parsePipelineID(w, r)
	if !ok {
		return
	}

	var req updatePipelineRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	existing, err := h.server.pipelines.GetByID(r.Context(), id)
	if err != nil {
		log.Printf("pipeline.Update: GetByID(%v): %v", id, err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if existing == nil {
		writeError(w, http.StatusNotFound, "pipeline not found")
		return
	}

	if !canAccessPipeline(sess, existing) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	candidate := models.Pipeline{
		DataSourceConfig: req.DataSourceConfig,
		ProcessConfig:    req.ProcessConfig,
		SinkConfig:       req.SinkConfig,
	}
	if err := pipeline.ValidateSchemaMappings(candidate); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	updated := &models.Pipeline{
		ID:               existing.ID,
		UserID:           existing.UserID,
		Name:             req.Name,
		DataSourceConfig: req.DataSourceConfig,
		ProcessConfig:    req.ProcessConfig,
		SinkConfig:       req.SinkConfig,
	}

	result, err := h.server.pipelines.Update(r.Context(), updated)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeError(w, http.StatusNotFound, "pipeline not found")
			return
		}
		log.Printf("pipeline.Update: Update(%v): %v", id, err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

// Delete handles DELETE /api/pipelines/{id}.
// Rejects deletion if any non-terminal Task references this pipeline (409 Conflict).
// Ownership enforcement: non-owner, non-admin callers receive 403.
//
// Responses:
//
//	204 No Content:   pipeline deleted
//	400 Bad Request:  {id} is not a valid UUID
//	401 Unauthorized: no valid session
//	403 Forbidden:    caller does not own the pipeline and is not Admin
//	404 Not Found:    pipeline does not exist
//	409 Conflict:     pipeline has active (non-terminal) tasks
//
// Postconditions:
//   - On 204: pipeline no longer exists in PostgreSQL.
//   - On 409: pipeline is unchanged.
func (h *PipelineHandler) Delete(w http.ResponseWriter, r *http.Request) {
	sess := auth.SessionFromContext(r.Context())
	if sess == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	id, ok := parsePipelineID(w, r)
	if !ok {
		return
	}

	existing, err := h.server.pipelines.GetByID(r.Context(), id)
	if err != nil {
		log.Printf("pipeline.Delete: GetByID(%v): %v", id, err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if existing == nil {
		writeError(w, http.StatusNotFound, "pipeline not found")
		return
	}

	if !canAccessPipeline(sess, existing) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	if err := h.server.pipelines.Delete(r.Context(), id); err != nil {
		if errors.Is(err, db.ErrActiveTasks) {
			writeError(w, http.StatusConflict, "pipeline has active tasks")
			return
		}
		log.Printf("pipeline.Delete: Delete(%v): %v", id, err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// parsePipelineID extracts and parses the {id} URL parameter from the chi router context.
// Writes a 400 response and returns false if the parameter is absent or not a valid UUID.
//
// Preconditions:
//   - r was routed through the chi router with an {id} parameter defined.
func parsePipelineID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	raw := chi.URLParam(r, "id")
	id, err := uuid.Parse(raw)
	if err != nil {
		writeError(w, http.StatusBadRequest, "id must be a valid UUID")
		return uuid.Nil, false
	}
	return id, true
}

// canAccessPipeline returns true when the session user is permitted to read or mutate
// the given pipeline. Access is granted when:
//   - The session user is an Admin, or
//   - The session user owns the pipeline (session.UserID == pipeline.UserID).
func canAccessPipeline(sess *models.Session, pipeline *models.Pipeline) bool {
	return sess.Role == models.RoleAdmin || sess.UserID == pipeline.UserID
}
