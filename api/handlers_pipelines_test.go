// Package api — unit tests for PipelineHandler (Create, List, Get, Update, Delete).
// Uses in-memory stubs for PipelineRepository to avoid external dependencies.
// See: REQ-022, TASK-013
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

// --- pipeline-specific stub ---

// capturingPipelineRepo extends stubPipelineRepo with error injection and call tracking.
// Used in pipeline handler tests to simulate repository failures and active-task guards.
type capturingPipelineRepo struct {
	pipelines      map[uuid.UUID]*models.Pipeline
	createErr      error
	updateErr      error
	deleteErr      error
	hasActiveTasks bool
}

func newCapturingPipelineRepo() *capturingPipelineRepo {
	return &capturingPipelineRepo{
		pipelines: make(map[uuid.UUID]*models.Pipeline),
	}
}

func (r *capturingPipelineRepo) add(p *models.Pipeline) {
	r.pipelines[p.ID] = p
}

func (r *capturingPipelineRepo) Create(_ context.Context, p *models.Pipeline) (*models.Pipeline, error) {
	if r.createErr != nil {
		return nil, r.createErr
	}
	created := *p
	created.CreatedAt = time.Now().UTC()
	created.UpdatedAt = time.Now().UTC()
	r.pipelines[created.ID] = &created
	return &created, nil
}

func (r *capturingPipelineRepo) GetByID(_ context.Context, id uuid.UUID) (*models.Pipeline, error) {
	p, ok := r.pipelines[id]
	if !ok {
		return nil, nil
	}
	return p, nil
}

func (r *capturingPipelineRepo) ListByUser(_ context.Context, userID uuid.UUID) ([]*models.Pipeline, error) {
	var out []*models.Pipeline
	for _, p := range r.pipelines {
		if p.UserID == userID {
			out = append(out, p)
		}
	}
	return out, nil
}

func (r *capturingPipelineRepo) List(_ context.Context) ([]*models.Pipeline, error) {
	out := make([]*models.Pipeline, 0, len(r.pipelines))
	for _, p := range r.pipelines {
		out = append(out, p)
	}
	return out, nil
}

func (r *capturingPipelineRepo) Update(_ context.Context, p *models.Pipeline) (*models.Pipeline, error) {
	if r.updateErr != nil {
		return nil, r.updateErr
	}
	existing, ok := r.pipelines[p.ID]
	if !ok {
		return nil, db.ErrNotFound
	}
	updated := *existing
	updated.Name = p.Name
	updated.DataSourceConfig = p.DataSourceConfig
	updated.ProcessConfig = p.ProcessConfig
	updated.SinkConfig = p.SinkConfig
	updated.UpdatedAt = time.Now().UTC()
	r.pipelines[p.ID] = &updated
	return &updated, nil
}

func (r *capturingPipelineRepo) Delete(_ context.Context, id uuid.UUID) error {
	if r.deleteErr != nil {
		return r.deleteErr
	}
	if r.hasActiveTasks {
		return db.ErrActiveTasks
	}
	delete(r.pipelines, id)
	return nil
}

func (r *capturingPipelineRepo) HasActiveTasks(_ context.Context, _ uuid.UUID) (bool, error) {
	return r.hasActiveTasks, nil
}

// Compile-time assertion: capturingPipelineRepo satisfies db.PipelineRepository.
var _ db.PipelineRepository = (*capturingPipelineRepo)(nil)

// --- test helpers ---

// newPipelineTestServer builds a minimal Server with the given pipeline repository.
func newPipelineTestServer(pipelines db.PipelineRepository) *Server {
	return &Server{pipelines: pipelines}
}

// pipelineRequest builds an authenticated HTTP request for the given method, path, and body.
// If sess is non-nil, the session is injected into the request context via auth.Middleware.
// If sess is nil, the request carries no authentication (simulates an unauthenticated call).
func pipelineRequest(t *testing.T, method, path string, body any, sess *models.Session) *http.Request {
	t.Helper()
	var rawBody []byte
	if body != nil {
		var err error
		rawBody, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("pipelineRequest: marshal: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(rawBody))
	req.Header.Set("Content-Type", "application/json")
	if sess == nil {
		return req
	}

	store := newStubSessionStore()
	token := "pipeline-test-token"
	_ = store.Create(context.Background(), token, sess)
	req.Header.Set("Authorization", "Bearer "+token)

	var captured *http.Request
	inner := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { captured = r })
	auth.Middleware(store)(inner).ServeHTTP(httptest.NewRecorder(), req)
	if captured == nil {
		t.Fatal("auth.Middleware did not call inner handler — session injection failed")
	}
	return captured
}

// withChiID wraps the request with a chi URL parameter {id} set to the given value.
// Required because chi.URLParam reads from the chi router context.
func withChiID(r *http.Request, id string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

// userSession returns a Session for a regular (non-admin) user.
func userSession(userID uuid.UUID) *models.Session {
	return &models.Session{UserID: userID, Role: models.RoleUser, CreatedAt: time.Now()}
}

// adminSession returns a Session for an admin user.
func adminSession() *models.Session {
	return &models.Session{UserID: uuid.New(), Role: models.RoleAdmin, CreatedAt: time.Now()}
}

// minimalPipeline returns a minimal valid pipeline owned by userID.
func minimalPipeline(userID uuid.UUID) *models.Pipeline {
	return &models.Pipeline{
		ID:               uuid.New(),
		Name:             "my-pipeline",
		UserID:           userID,
		DataSourceConfig: models.DataSourceConfig{ConnectorType: "demo"},
		ProcessConfig:    models.ProcessConfig{ConnectorType: "demo"},
		SinkConfig:       models.SinkConfig{ConnectorType: "demo"},
		CreatedAt:        time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
	}
}

// createBody returns a request body map suitable for POST /api/pipelines.
func createBody(name string) map[string]any {
	return map[string]any{
		"name": name,
		"dataSourceConfig": map[string]any{
			"connectorType": "demo",
			"config":        map[string]any{},
			"outputSchema":  []string{},
		},
		"processConfig": map[string]any{
			"connectorType": "demo",
			"config":        map[string]any{},
			"inputMappings": []any{},
			"outputSchema":  []string{},
		},
		"sinkConfig": map[string]any{
			"connectorType": "demo",
			"config":        map[string]any{},
			"inputMappings": []any{},
		},
	}
}

// --- Create tests ---

// TestCreate_ValidPayloadReturns201WithPipeline verifies AC-1:
// POST /api/pipelines creates a pipeline and returns 201 with the new pipeline.
func TestCreate_ValidPayloadReturns201WithPipeline(t *testing.T) {
	repo := newCapturingPipelineRepo()
	srv := newPipelineTestServer(repo)
	h := &PipelineHandler{server: srv}

	userID := uuid.New()
	sess := userSession(userID)

	rec := httptest.NewRecorder()
	h.Create(rec, pipelineRequest(t, http.MethodPost, "/api/pipelines", createBody("my-pipe"), sess))

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp models.Pipeline
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.ID == uuid.Nil {
		t.Error("expected non-nil pipeline ID in response")
	}
	if resp.Name != "my-pipe" {
		t.Errorf("expected name %q, got %q", "my-pipe", resp.Name)
	}
	if resp.UserID != userID {
		t.Errorf("expected UserID %v, got %v", userID, resp.UserID)
	}
}

// TestCreate_UnauthenticatedReturns401 verifies that no session returns 401.
func TestCreate_UnauthenticatedReturns401(t *testing.T) {
	repo := newCapturingPipelineRepo()
	srv := newPipelineTestServer(repo)
	h := &PipelineHandler{server: srv}

	rec := httptest.NewRecorder()
	h.Create(rec, pipelineRequest(t, http.MethodPost, "/api/pipelines", createBody("p"), nil))

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

// TestCreate_MalformedBodyReturns400 verifies that non-JSON body returns 400.
func TestCreate_MalformedBodyReturns400(t *testing.T) {
	repo := newCapturingPipelineRepo()
	srv := newPipelineTestServer(repo)
	h := &PipelineHandler{server: srv}

	userID := uuid.New()
	sess := userSession(userID)

	store := newStubSessionStore()
	token := "bad-body-token"
	_ = store.Create(context.Background(), token, sess)
	req := httptest.NewRequest(http.MethodPost, "/api/pipelines", bytes.NewBufferString("not json"))
	req.Header.Set("Authorization", "Bearer "+token)

	var captured *http.Request
	inner := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { captured = r })
	auth.Middleware(store)(inner).ServeHTTP(httptest.NewRecorder(), req)

	rec := httptest.NewRecorder()
	h.Create(rec, captured)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

// TestCreate_MissingNameReturns400 verifies that an empty name is rejected.
func TestCreate_MissingNameReturns400(t *testing.T) {
	repo := newCapturingPipelineRepo()
	srv := newPipelineTestServer(repo)
	h := &PipelineHandler{server: srv}

	userID := uuid.New()
	body := createBody("") // empty name

	rec := httptest.NewRecorder()
	h.Create(rec, pipelineRequest(t, http.MethodPost, "/api/pipelines", body, userSession(userID)))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty name, got %d", rec.Code)
	}
}

// TestCreate_UserIDFromSessionIsUsed verifies that user_id comes from the session, not the body.
func TestCreate_UserIDFromSessionIsUsed(t *testing.T) {
	repo := newCapturingPipelineRepo()
	srv := newPipelineTestServer(repo)
	h := &PipelineHandler{server: srv}

	userID := uuid.New()
	sess := userSession(userID)

	rec := httptest.NewRecorder()
	h.Create(rec, pipelineRequest(t, http.MethodPost, "/api/pipelines", createBody("pipe"), sess))

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp models.Pipeline
	_ = json.NewDecoder(rec.Body).Decode(&resp)

	if resp.UserID != userID {
		t.Errorf("expected UserID %v from session, got %v", userID, resp.UserID)
	}
}

// --- List tests ---

// TestList_UserSeeOwnPipelinesOnly verifies AC-2:
// GET /api/pipelines for a User role returns only that user's pipelines.
func TestList_UserSeeOwnPipelinesOnly(t *testing.T) {
	repo := newCapturingPipelineRepo()

	userID := uuid.New()
	otherID := uuid.New()
	repo.add(minimalPipeline(userID))
	repo.add(minimalPipeline(userID))
	repo.add(minimalPipeline(otherID)) // another user's pipeline

	srv := newPipelineTestServer(repo)
	h := &PipelineHandler{server: srv}

	rec := httptest.NewRecorder()
	h.List(rec, pipelineRequest(t, http.MethodGet, "/api/pipelines", nil, userSession(userID)))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp []models.Pipeline
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp) != 2 {
		t.Errorf("expected 2 pipelines for user, got %d", len(resp))
	}
	for _, p := range resp {
		if p.UserID != userID {
			t.Errorf("got pipeline belonging to other user: %v", p.UserID)
		}
	}
}

// TestList_AdminSeesAllPipelines verifies AC-2:
// GET /api/pipelines for an Admin role returns all pipelines.
func TestList_AdminSeesAllPipelines(t *testing.T) {
	repo := newCapturingPipelineRepo()
	repo.add(minimalPipeline(uuid.New()))
	repo.add(minimalPipeline(uuid.New()))
	repo.add(minimalPipeline(uuid.New()))

	srv := newPipelineTestServer(repo)
	h := &PipelineHandler{server: srv}

	rec := httptest.NewRecorder()
	h.List(rec, pipelineRequest(t, http.MethodGet, "/api/pipelines", nil, adminSession()))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp []models.Pipeline
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if len(resp) != 3 {
		t.Errorf("expected 3 pipelines for admin, got %d", len(resp))
	}
}

// TestList_UnauthenticatedReturns401 verifies that no session returns 401.
func TestList_UnauthenticatedReturns401(t *testing.T) {
	repo := newCapturingPipelineRepo()
	srv := newPipelineTestServer(repo)
	h := &PipelineHandler{server: srv}

	rec := httptest.NewRecorder()
	h.List(rec, pipelineRequest(t, http.MethodGet, "/api/pipelines", nil, nil))

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

// TestList_EmptyListReturnsEmptyArray verifies that an empty result is a JSON array (not null).
func TestList_EmptyListReturnsEmptyArray(t *testing.T) {
	repo := newCapturingPipelineRepo()
	srv := newPipelineTestServer(repo)
	h := &PipelineHandler{server: srv}

	userID := uuid.New()
	rec := httptest.NewRecorder()
	h.List(rec, pipelineRequest(t, http.MethodGet, "/api/pipelines", nil, userSession(userID)))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp []models.Pipeline
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp == nil {
		t.Error("expected empty JSON array, got null")
	}
}

// --- Get tests ---

// TestGet_ExistingOwnedPipelineReturns200 verifies AC-3:
// GET /api/pipelines/{id} for the owning user returns 200 with the pipeline.
func TestGet_ExistingOwnedPipelineReturns200(t *testing.T) {
	repo := newCapturingPipelineRepo()

	userID := uuid.New()
	p := minimalPipeline(userID)
	repo.add(p)

	srv := newPipelineTestServer(repo)
	h := &PipelineHandler{server: srv}

	req := pipelineRequest(t, http.MethodGet, "/api/pipelines/"+p.ID.String(), nil, userSession(userID))
	req = withChiID(req, p.ID.String())

	rec := httptest.NewRecorder()
	h.Get(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp models.Pipeline
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.ID != p.ID {
		t.Errorf("expected pipeline ID %v, got %v", p.ID, resp.ID)
	}
}

// TestGet_NonExistentPipelineReturns404 verifies AC-3:
// GET /api/pipelines/{id} for a non-existent pipeline returns 404.
func TestGet_NonExistentPipelineReturns404(t *testing.T) {
	repo := newCapturingPipelineRepo()
	srv := newPipelineTestServer(repo)
	h := &PipelineHandler{server: srv}

	unknownID := uuid.New().String()
	req := pipelineRequest(t, http.MethodGet, "/api/pipelines/"+unknownID, nil, adminSession())
	req = withChiID(req, unknownID)

	rec := httptest.NewRecorder()
	h.Get(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

// TestGet_NonOwnerNonAdminReturns403 verifies AC-7:
// A User who does not own the pipeline and is not Admin receives 403.
func TestGet_NonOwnerNonAdminReturns403(t *testing.T) {
	repo := newCapturingPipelineRepo()

	ownerID := uuid.New()
	p := minimalPipeline(ownerID)
	repo.add(p)

	srv := newPipelineTestServer(repo)
	h := &PipelineHandler{server: srv}

	otherUserID := uuid.New()
	req := pipelineRequest(t, http.MethodGet, "/api/pipelines/"+p.ID.String(), nil, userSession(otherUserID))
	req = withChiID(req, p.ID.String())

	rec := httptest.NewRecorder()
	h.Get(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 for non-owner, got %d", rec.Code)
	}
}

// TestGet_AdminCanAccessAnyPipeline verifies that admin bypasses ownership check.
func TestGet_AdminCanAccessAnyPipeline(t *testing.T) {
	repo := newCapturingPipelineRepo()

	ownerID := uuid.New()
	p := minimalPipeline(ownerID)
	repo.add(p)

	srv := newPipelineTestServer(repo)
	h := &PipelineHandler{server: srv}

	req := pipelineRequest(t, http.MethodGet, "/api/pipelines/"+p.ID.String(), nil, adminSession())
	req = withChiID(req, p.ID.String())

	rec := httptest.NewRecorder()
	h.Get(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for admin, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestGet_InvalidIDFormatReturns400 verifies that a non-UUID {id} returns 400.
func TestGet_InvalidIDFormatReturns400(t *testing.T) {
	repo := newCapturingPipelineRepo()
	srv := newPipelineTestServer(repo)
	h := &PipelineHandler{server: srv}

	req := pipelineRequest(t, http.MethodGet, "/api/pipelines/not-a-uuid", nil, adminSession())
	req = withChiID(req, "not-a-uuid")

	rec := httptest.NewRecorder()
	h.Get(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid UUID, got %d", rec.Code)
	}
}

// TestGet_UnauthenticatedReturns401 verifies that no session returns 401.
func TestGet_UnauthenticatedReturns401(t *testing.T) {
	repo := newCapturingPipelineRepo()
	srv := newPipelineTestServer(repo)
	h := &PipelineHandler{server: srv}

	id := uuid.New().String()
	req := pipelineRequest(t, http.MethodGet, "/api/pipelines/"+id, nil, nil)
	req = withChiID(req, id)

	rec := httptest.NewRecorder()
	h.Get(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

// --- Update tests ---

// TestUpdate_OwnerCanUpdatePipeline verifies AC-4:
// PUT /api/pipelines/{id} by the owner returns 200 with the updated pipeline.
func TestUpdate_OwnerCanUpdatePipeline(t *testing.T) {
	repo := newCapturingPipelineRepo()

	userID := uuid.New()
	p := minimalPipeline(userID)
	repo.add(p)

	srv := newPipelineTestServer(repo)
	h := &PipelineHandler{server: srv}

	body := createBody("updated-name")
	req := pipelineRequest(t, http.MethodPut, "/api/pipelines/"+p.ID.String(), body, userSession(userID))
	req = withChiID(req, p.ID.String())

	rec := httptest.NewRecorder()
	h.Update(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp models.Pipeline
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Name != "updated-name" {
		t.Errorf("expected name %q, got %q", "updated-name", resp.Name)
	}
}

// TestUpdate_NonOwnerNonAdminReturns403 verifies AC-7:
// A User who does not own the pipeline cannot update it.
func TestUpdate_NonOwnerNonAdminReturns403(t *testing.T) {
	repo := newCapturingPipelineRepo()

	ownerID := uuid.New()
	p := minimalPipeline(ownerID)
	repo.add(p)

	srv := newPipelineTestServer(repo)
	h := &PipelineHandler{server: srv}

	otherID := uuid.New()
	req := pipelineRequest(t, http.MethodPut, "/api/pipelines/"+p.ID.String(), createBody("x"), userSession(otherID))
	req = withChiID(req, p.ID.String())

	rec := httptest.NewRecorder()
	h.Update(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

// TestUpdate_AdminCanUpdateAnyPipeline verifies that admin bypasses ownership check.
func TestUpdate_AdminCanUpdateAnyPipeline(t *testing.T) {
	repo := newCapturingPipelineRepo()

	ownerID := uuid.New()
	p := minimalPipeline(ownerID)
	repo.add(p)

	srv := newPipelineTestServer(repo)
	h := &PipelineHandler{server: srv}

	req := pipelineRequest(t, http.MethodPut, "/api/pipelines/"+p.ID.String(), createBody("admin-updated"), adminSession())
	req = withChiID(req, p.ID.String())

	rec := httptest.NewRecorder()
	h.Update(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for admin update, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestUpdate_NonExistentPipelineReturns404 verifies that updating a missing pipeline returns 404.
func TestUpdate_NonExistentPipelineReturns404(t *testing.T) {
	repo := newCapturingPipelineRepo()
	srv := newPipelineTestServer(repo)
	h := &PipelineHandler{server: srv}

	unknownID := uuid.New().String()
	req := pipelineRequest(t, http.MethodPut, "/api/pipelines/"+unknownID, createBody("x"), adminSession())
	req = withChiID(req, unknownID)

	rec := httptest.NewRecorder()
	h.Update(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 for missing pipeline, got %d", rec.Code)
	}
}

// TestUpdate_MalformedBodyReturns400 verifies that invalid JSON body returns 400.
func TestUpdate_MalformedBodyReturns400(t *testing.T) {
	repo := newCapturingPipelineRepo()

	userID := uuid.New()
	p := minimalPipeline(userID)
	repo.add(p)

	srv := newPipelineTestServer(repo)
	h := &PipelineHandler{server: srv}

	store := newStubSessionStore()
	token := "update-bad-body"
	sess := userSession(userID)
	_ = store.Create(context.Background(), token, sess)
	req := httptest.NewRequest(http.MethodPut, "/api/pipelines/"+p.ID.String(), bytes.NewBufferString("not json"))
	req.Header.Set("Authorization", "Bearer "+token)

	var captured *http.Request
	inner := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { captured = r })
	auth.Middleware(store)(inner).ServeHTTP(httptest.NewRecorder(), req)
	captured = withChiID(captured, p.ID.String())

	rec := httptest.NewRecorder()
	h.Update(rec, captured)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

// TestUpdate_UnauthenticatedReturns401 verifies that no session returns 401.
func TestUpdate_UnauthenticatedReturns401(t *testing.T) {
	repo := newCapturingPipelineRepo()
	srv := newPipelineTestServer(repo)
	h := &PipelineHandler{server: srv}

	id := uuid.New().String()
	req := pipelineRequest(t, http.MethodPut, "/api/pipelines/"+id, createBody("x"), nil)
	req = withChiID(req, id)

	rec := httptest.NewRecorder()
	h.Update(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

// --- Delete tests ---

// TestDelete_OwnerCanDeletePipeline verifies AC-5:
// DELETE /api/pipelines/{id} by the owner returns 204.
func TestDelete_OwnerCanDeletePipeline(t *testing.T) {
	repo := newCapturingPipelineRepo()

	userID := uuid.New()
	p := minimalPipeline(userID)
	repo.add(p)

	srv := newPipelineTestServer(repo)
	h := &PipelineHandler{server: srv}

	req := pipelineRequest(t, http.MethodDelete, "/api/pipelines/"+p.ID.String(), nil, userSession(userID))
	req = withChiID(req, p.ID.String())

	rec := httptest.NewRecorder()
	h.Delete(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify the pipeline was removed from the repository.
	remaining, _ := repo.GetByID(context.Background(), p.ID)
	if remaining != nil {
		t.Error("expected pipeline to be deleted from repository")
	}
}

// TestDelete_ActiveTasksReturns409 verifies AC-6:
// DELETE /api/pipelines/{id} returns 409 when active tasks reference the pipeline.
func TestDelete_ActiveTasksReturns409(t *testing.T) {
	repo := newCapturingPipelineRepo()
	repo.hasActiveTasks = true

	userID := uuid.New()
	p := minimalPipeline(userID)
	repo.add(p)

	srv := newPipelineTestServer(repo)
	h := &PipelineHandler{server: srv}

	req := pipelineRequest(t, http.MethodDelete, "/api/pipelines/"+p.ID.String(), nil, userSession(userID))
	req = withChiID(req, p.ID.String())

	rec := httptest.NewRecorder()
	h.Delete(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("expected 409 for active tasks, got %d", rec.Code)
	}
}

// TestDelete_NonOwnerNonAdminReturns403 verifies AC-7:
// A User who does not own the pipeline cannot delete it.
func TestDelete_NonOwnerNonAdminReturns403(t *testing.T) {
	repo := newCapturingPipelineRepo()

	ownerID := uuid.New()
	p := minimalPipeline(ownerID)
	repo.add(p)

	srv := newPipelineTestServer(repo)
	h := &PipelineHandler{server: srv}

	otherID := uuid.New()
	req := pipelineRequest(t, http.MethodDelete, "/api/pipelines/"+p.ID.String(), nil, userSession(otherID))
	req = withChiID(req, p.ID.String())

	rec := httptest.NewRecorder()
	h.Delete(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 for non-owner, got %d", rec.Code)
	}
}

// TestDelete_AdminCanDeleteAnyPipeline verifies that admin bypasses ownership check.
func TestDelete_AdminCanDeleteAnyPipeline(t *testing.T) {
	repo := newCapturingPipelineRepo()

	ownerID := uuid.New()
	p := minimalPipeline(ownerID)
	repo.add(p)

	srv := newPipelineTestServer(repo)
	h := &PipelineHandler{server: srv}

	req := pipelineRequest(t, http.MethodDelete, "/api/pipelines/"+p.ID.String(), nil, adminSession())
	req = withChiID(req, p.ID.String())

	rec := httptest.NewRecorder()
	h.Delete(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204 for admin delete, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestDelete_NonExistentPipelineReturns404 verifies that deleting a missing pipeline returns 404.
func TestDelete_NonExistentPipelineReturns404(t *testing.T) {
	repo := newCapturingPipelineRepo()
	srv := newPipelineTestServer(repo)
	h := &PipelineHandler{server: srv}

	unknownID := uuid.New().String()
	req := pipelineRequest(t, http.MethodDelete, "/api/pipelines/"+unknownID, nil, adminSession())
	req = withChiID(req, unknownID)

	rec := httptest.NewRecorder()
	h.Delete(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 for missing pipeline, got %d", rec.Code)
	}
}

// TestDelete_UnauthenticatedReturns401 verifies that no session returns 401.
func TestDelete_UnauthenticatedReturns401(t *testing.T) {
	repo := newCapturingPipelineRepo()
	srv := newPipelineTestServer(repo)
	h := &PipelineHandler{server: srv}

	id := uuid.New().String()
	req := pipelineRequest(t, http.MethodDelete, "/api/pipelines/"+id, nil, nil)
	req = withChiID(req, id)

	rec := httptest.NewRecorder()
	h.Delete(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}
