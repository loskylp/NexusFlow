// Package api — Chain CRUD handlers.
// Chains are strictly linear sequences of pipelines (A -> B -> C).
// Branching structures (one pipeline with multiple successors) are rejected.
// See: REQ-014, ADR-003, TASK-014
package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/nxlabs/nexusflow/internal/auth"
	"github.com/nxlabs/nexusflow/internal/models"
)

// ChainHandler handles chain REST endpoints.
// See: TASK-014
type ChainHandler struct {
	server *Server
}

// createChainRequest is the JSON body for POST /api/chains.
type createChainRequest struct {
	// Name is the human-readable identifier for the chain. Required; must be non-empty.
	Name string `json:"name"`
	// PipelineIDs is the ordered list of pipeline UUIDs forming the chain.
	// Must contain at least two entries. Duplicate entries are rejected (branching).
	PipelineIDs []string `json:"pipelineIds"`
}

// Create handles POST /api/chains.
// Validates that the pipeline list is strictly linear (no duplicates, at least two entries),
// creates the chain record and all steps, and returns 201 with the created chain.
//
// Request body:
//
//	{
//	  "name": "string",
//	  "pipelineIds": ["uuid", "uuid", ...]
//	}
//
// Responses:
//
//	201 Created:      { chain }
//	400 Bad Request:  missing name, fewer than 2 pipelines, duplicate pipeline IDs,
//	                  or any pipelineId is not a valid UUID
//	401 Unauthorized: no valid session
//	500 Internal:     database failure
//
// Preconditions:
//   - Auth middleware has placed a valid Session in the request context.
//
// Postconditions:
//   - On 201: chain and all chain_steps exist in PostgreSQL with user_id = session.UserID.
func (h *ChainHandler) Create(w http.ResponseWriter, r *http.Request) {
	sess := auth.SessionFromContext(r.Context())
	if sess == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req createChainRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	if len(req.PipelineIDs) < 2 {
		writeError(w, http.StatusBadRequest, "pipelineIds must contain at least two entries")
		return
	}

	// Parse and deduplicate-check the pipeline IDs.
	pipelineIDs, err := parseAndValidatePipelineIDs(req.PipelineIDs)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	chain := &models.Chain{
		ID:          uuid.New(),
		Name:        req.Name,
		UserID:      sess.UserID,
		PipelineIDs: pipelineIDs,
	}

	created, err := h.server.chains.Create(r.Context(), chain)
	if err != nil {
		log.Printf("chain.Create: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(created)
}

// Get handles GET /api/chains/{id}.
// Returns the chain definition with its ordered pipeline list.
//
// Responses:
//
//	200 OK:           { chain }
//	400 Bad Request:  {id} is not a valid UUID
//	401 Unauthorized: no valid session
//	404 Not Found:    chain does not exist
func (h *ChainHandler) Get(w http.ResponseWriter, r *http.Request) {
	sess := auth.SessionFromContext(r.Context())
	if sess == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	id, ok := parseChainID(w, r)
	if !ok {
		return
	}

	chain, err := h.server.chains.GetByID(r.Context(), id)
	if err != nil {
		log.Printf("chain.Get: GetByID(%v): %v", id, err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if chain == nil {
		writeError(w, http.StatusNotFound, "chain not found")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(chain)
}

// parseAndValidatePipelineIDs parses the raw pipeline ID strings, validates each
// is a legal UUID, and rejects any duplicate entries.
//
// Duplicate pipeline IDs would constitute a branching structure: the same pipeline
// appearing more than once in a chain is undefined behaviour and is explicitly rejected.
//
// Args:
//
//	rawIDs: Slice of raw UUID string values from the request body.
//
// Returns:
//
//	Parsed UUIDs in the same order as rawIDs.
//	An error naming the first problem: invalid UUID or duplicate pipeline ID.
func parseAndValidatePipelineIDs(rawIDs []string) ([]uuid.UUID, error) {
	seen := make(map[uuid.UUID]struct{}, len(rawIDs))
	result := make([]uuid.UUID, 0, len(rawIDs))

	for _, raw := range rawIDs {
		id, err := uuid.Parse(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid pipeline ID %q: must be a valid UUID", raw)
		}
		if _, exists := seen[id]; exists {
			return nil, fmt.Errorf("duplicate pipeline ID %q: chains must be strictly linear (no branching)", raw)
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}

	return result, nil
}

// parseChainID extracts and parses the {id} URL parameter from the chi router context.
// Writes a 400 response and returns false if the parameter is absent or not a valid UUID.
//
// Preconditions:
//   - r was routed through the chi router with an {id} parameter defined.
func parseChainID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	raw := chi.URLParam(r, "id")
	id, err := uuid.Parse(raw)
	if err != nil {
		writeError(w, http.StatusBadRequest, "id must be a valid UUID")
		return uuid.Nil, false
	}
	return id, true
}
