// Package api — unit tests for ChainHandler (Create, Get).
// Uses in-memory stubs for ChainRepository and PipelineRepository
// to avoid external dependencies.
// See: REQ-014, TASK-014
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/nxlabs/nexusflow/internal/auth"
	"github.com/nxlabs/nexusflow/internal/db"
	"github.com/nxlabs/nexusflow/internal/models"
)

// --- stub ChainRepository ---

// stubChainRepo is an in-memory ChainRepository for unit tests.
type stubChainRepo struct {
	chains    map[uuid.UUID]*models.Chain
	createErr error
}

func newStubChainRepo() *stubChainRepo {
	return &stubChainRepo{chains: make(map[uuid.UUID]*models.Chain)}
}

func (r *stubChainRepo) Create(_ context.Context, chain *models.Chain) (*models.Chain, error) {
	if r.createErr != nil {
		return nil, r.createErr
	}
	c := *chain
	c.CreatedAt = time.Now().UTC()
	r.chains[c.ID] = &c
	return &c, nil
}

func (r *stubChainRepo) GetByID(_ context.Context, id uuid.UUID) (*models.Chain, error) {
	c, ok := r.chains[id]
	if !ok {
		return nil, nil
	}
	return c, nil
}

func (r *stubChainRepo) FindByPipeline(_ context.Context, pipelineID uuid.UUID) (*models.Chain, error) {
	for _, c := range r.chains {
		for _, pid := range c.PipelineIDs {
			if pid == pipelineID {
				return c, nil
			}
		}
	}
	return nil, nil
}

func (r *stubChainRepo) GetNextPipeline(_ context.Context, chainID uuid.UUID, pipelineID uuid.UUID) (*uuid.UUID, error) {
	c, ok := r.chains[chainID]
	if !ok {
		return nil, nil
	}
	for i, pid := range c.PipelineIDs {
		if pid == pipelineID && i+1 < len(c.PipelineIDs) {
			next := c.PipelineIDs[i+1]
			return &next, nil
		}
	}
	return nil, nil
}

// Compile-time check that stubChainRepo satisfies the ChainRepository interface.
var _ db.ChainRepository = (*stubChainRepo)(nil)

// --- helpers ---

// buildChainServer constructs a Server wired with stub chain and pipeline repos.
func buildChainServer(chainRepo db.ChainRepository, pipelineRepo db.PipelineRepository) *Server {
	return &Server{
		chains:    chainRepo,
		pipelines: pipelineRepo,
	}
}

// withSession wraps a request so that auth.SessionFromContext returns sess.
// Matches the injection pattern used by auth.Middleware in production.
func withSession(t *testing.T, req *http.Request, sess *models.Session) *http.Request {
	t.Helper()
	store := newStubSessionStore()
	token := "test-token-chains"
	_ = store.Create(req.Context(), token, sess)
	req.Header.Set("Authorization", "Bearer "+token)

	var captured *http.Request
	inner := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		captured = r
	})
	auth.Middleware(store)(inner).ServeHTTP(httptest.NewRecorder(), req)
	if captured == nil {
		t.Fatal("auth.Middleware did not inject session — check token header")
	}
	return captured
}

// newChainSession returns a regular-user session for chain endpoint tests.
func newChainSession() *models.Session {
	return &models.Session{
		UserID:    uuid.New(),
		Role:      models.RoleUser,
		CreatedAt: time.Now().UTC(),
	}
}

// --- tests ---

// TestChainCreate_201_linear verifies that POST /api/chains with a linear list of
// pipeline IDs returns 201 Created and the created chain (TASK-014 AC-1).
func TestChainCreate_201_linear(t *testing.T) {
	// Arrange
	pipelineID1 := uuid.New()
	pipelineID2 := uuid.New()
	pipelineID3 := uuid.New()

	pipelineRepo := newCapturingPipelineRepo()
	sess := newChainSession()
	for _, pid := range []uuid.UUID{pipelineID1, pipelineID2, pipelineID3} {
		pipelineRepo.add(&models.Pipeline{
			ID:     pid,
			UserID: sess.UserID,
			Name:   pid.String(),
		})
	}

	chainRepo := newStubChainRepo()
	srv := buildChainServer(chainRepo, pipelineRepo)
	h := &ChainHandler{server: srv}

	body, _ := json.Marshal(map[string]any{
		"name":        "my-chain",
		"pipelineIds": []string{pipelineID1.String(), pipelineID2.String(), pipelineID3.String()},
	})

	req := httptest.NewRequest(http.MethodPost, "/api/chains", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withSession(t, req, sess)
	w := httptest.NewRecorder()

	// Act
	h.Create(w, req)

	// Assert
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var result models.Chain
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.ID == uuid.Nil {
		t.Error("expected non-nil chain ID in response")
	}
	if result.Name != "my-chain" {
		t.Errorf("expected name %q, got %q", "my-chain", result.Name)
	}
	if len(result.PipelineIDs) != 3 {
		t.Errorf("expected 3 pipeline IDs, got %d", len(result.PipelineIDs))
	}
}

// TestChainCreate_400_branching verifies that POST /api/chains with duplicate
// pipeline IDs (branching structure) returns 400 Bad Request (TASK-014 AC-2).
func TestChainCreate_400_branching(t *testing.T) {
	// Arrange
	pipelineID := uuid.New()
	pipelineRepo := newCapturingPipelineRepo()
	sess := newChainSession()
	pipelineRepo.add(&models.Pipeline{ID: pipelineID, UserID: sess.UserID, Name: "p"})

	chainRepo := newStubChainRepo()
	srv := buildChainServer(chainRepo, pipelineRepo)
	h := &ChainHandler{server: srv}

	// Same pipeline ID used twice — constitutes a branching structure.
	body, _ := json.Marshal(map[string]any{
		"name":        "branching-chain",
		"pipelineIds": []string{pipelineID.String(), pipelineID.String()},
	})

	req := httptest.NewRequest(http.MethodPost, "/api/chains", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withSession(t, req, sess)
	w := httptest.NewRecorder()

	// Act
	h.Create(w, req)

	// Assert
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// TestChainCreate_400_empty_pipelines verifies that an empty pipeline list returns 400.
func TestChainCreate_400_empty_pipelines(t *testing.T) {
	chainRepo := newStubChainRepo()
	srv := buildChainServer(chainRepo, newCapturingPipelineRepo())
	h := &ChainHandler{server: srv}

	body, _ := json.Marshal(map[string]any{
		"name":        "empty-chain",
		"pipelineIds": []string{},
	})

	req := httptest.NewRequest(http.MethodPost, "/api/chains", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withSession(t, req, newChainSession())
	w := httptest.NewRecorder()

	h.Create(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty pipelineIds, got %d", w.Code)
	}
}

// TestChainCreate_400_single_pipeline verifies that a single-pipeline chain returns 400.
func TestChainCreate_400_single_pipeline(t *testing.T) {
	chainRepo := newStubChainRepo()
	pipelineRepo := newCapturingPipelineRepo()
	sess := newChainSession()
	pid := uuid.New()
	pipelineRepo.add(&models.Pipeline{ID: pid, UserID: sess.UserID, Name: "p"})
	srv := buildChainServer(chainRepo, pipelineRepo)
	h := &ChainHandler{server: srv}

	body, _ := json.Marshal(map[string]any{
		"name":        "single",
		"pipelineIds": []string{pid.String()},
	})

	req := httptest.NewRequest(http.MethodPost, "/api/chains", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withSession(t, req, sess)
	w := httptest.NewRecorder()

	h.Create(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for single pipelineId, got %d", w.Code)
	}
}

// TestChainCreate_401_unauthenticated verifies that unauthenticated requests receive 401.
func TestChainCreate_401_unauthenticated(t *testing.T) {
	chainRepo := newStubChainRepo()
	pipelineRepo := newCapturingPipelineRepo()
	srv := buildChainServer(chainRepo, pipelineRepo)
	h := &ChainHandler{server: srv}

	body, _ := json.Marshal(map[string]any{"name": "x", "pipelineIds": []string{uuid.New().String(), uuid.New().String()}})
	req := httptest.NewRequest(http.MethodPost, "/api/chains", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	// No session injected.
	w := httptest.NewRecorder()

	h.Create(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

// TestChainGet_200 verifies that GET /api/chains/{id} returns the chain with pipeline
// ordering (TASK-014 AC-5).
func TestChainGet_200(t *testing.T) {
	// Arrange
	chainRepo := newStubChainRepo()
	sess := newChainSession()
	pid1, pid2 := uuid.New(), uuid.New()
	chain := &models.Chain{
		ID:          uuid.New(),
		Name:        "test-chain",
		UserID:      sess.UserID,
		PipelineIDs: []uuid.UUID{pid1, pid2},
		CreatedAt:   time.Now().UTC(),
	}
	chainRepo.chains[chain.ID] = chain

	srv := buildChainServer(chainRepo, newCapturingPipelineRepo())
	h := &ChainHandler{server: srv}

	// Build request with chi URL param
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", chain.ID.String())
	req := httptest.NewRequest(http.MethodGet, "/api/chains/"+chain.ID.String(), nil)
	req = withSession(t, req, sess)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()

	// Act
	h.Get(w, req)

	// Assert
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result models.Chain
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.ID != chain.ID {
		t.Errorf("expected ID %v, got %v", chain.ID, result.ID)
	}
	if len(result.PipelineIDs) != 2 {
		t.Errorf("expected 2 pipeline IDs, got %d", len(result.PipelineIDs))
	}
	// Verify ordering is preserved.
	if result.PipelineIDs[0] != pid1 {
		t.Errorf("expected first pipeline %v, got %v", pid1, result.PipelineIDs[0])
	}
	if result.PipelineIDs[1] != pid2 {
		t.Errorf("expected second pipeline %v, got %v", pid2, result.PipelineIDs[1])
	}
}

// TestChainGet_404 verifies that a non-existent chain ID returns 404.
func TestChainGet_404(t *testing.T) {
	chainRepo := newStubChainRepo()
	srv := buildChainServer(chainRepo, newCapturingPipelineRepo())
	h := &ChainHandler{server: srv}

	missingID := uuid.New()
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", missingID.String())
	req := httptest.NewRequest(http.MethodGet, "/api/chains/"+missingID.String(), nil)
	req = withSession(t, req, newChainSession())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()

	h.Get(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

// TestChainGet_400_invalid_uuid verifies that a malformed {id} returns 400.
func TestChainGet_400_invalid_uuid(t *testing.T) {
	chainRepo := newStubChainRepo()
	srv := buildChainServer(chainRepo, newCapturingPipelineRepo())
	h := &ChainHandler{server: srv}

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "not-a-uuid")
	req := httptest.NewRequest(http.MethodGet, "/api/chains/not-a-uuid", nil)
	req = withSession(t, req, newChainSession())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()

	h.Get(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}
