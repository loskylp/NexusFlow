// Package api — unit tests for WorkerHandler.List (GET /api/workers).
// Uses an in-memory stub WorkerRepository to avoid external dependencies.
// See: REQ-016, TASK-025
package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nxlabs/nexusflow/internal/auth"
	"github.com/nxlabs/nexusflow/internal/models"
)

// --- worker-specific stub ---

// stubWorkerRepo is an in-memory WorkerRepository for testing WorkerHandler.
type stubWorkerRepo struct {
	workers []*models.Worker
	listErr error
}

func newStubWorkerRepo() *stubWorkerRepo {
	return &stubWorkerRepo{}
}

func (r *stubWorkerRepo) add(w *models.Worker) {
	r.workers = append(r.workers, w)
}

func (r *stubWorkerRepo) Register(_ context.Context, w *models.Worker) (*models.Worker, error) {
	r.workers = append(r.workers, w)
	return w, nil
}

func (r *stubWorkerRepo) GetByID(_ context.Context, id string) (*models.Worker, error) {
	for _, w := range r.workers {
		if w.ID == id {
			return w, nil
		}
	}
	return nil, nil
}

func (r *stubWorkerRepo) List(_ context.Context) ([]*models.Worker, error) {
	if r.listErr != nil {
		return nil, r.listErr
	}
	if r.workers == nil {
		return []*models.Worker{}, nil
	}
	return r.workers, nil
}

func (r *stubWorkerRepo) UpdateStatus(_ context.Context, id string, status models.WorkerStatus) error {
	for _, w := range r.workers {
		if w.ID == id {
			w.Status = status
			return nil
		}
	}
	return nil
}

// workerResponse mirrors the JSON shape the handler returns.
// Used only in tests to decode and assert individual fields.
type workerResponse struct {
	ID            string     `json:"id"`
	Status        string     `json:"status"`
	Tags          []string   `json:"tags"`
	CurrentTaskID *uuid.UUID `json:"currentTaskId"`
	LastHeartbeat time.Time  `json:"lastHeartbeat"`
}

// --- test helpers ---

// newWorkerTestServer builds a minimal Server with the given worker repository.
func newWorkerTestServer(workers *stubWorkerRepo) *Server {
	return &Server{workers: workers}
}

// workerGetRequest builds an authenticated GET /api/workers request.
// When sess is non-nil the session is injected into the request context via
// auth.Middleware so WorkerHandler.List can read it with auth.SessionFromContext.
// When sess is nil the request carries no authentication (unauthenticated path).
func workerGetRequest(t *testing.T, sess *models.Session) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/workers", nil)
	if sess == nil {
		return req
	}

	store := newStubSessionStore()
	token := "worker-test-token"
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

// makeTestWorker returns a populated models.Worker for use in tests.
// When taskID is non-nil it is set as CurrentTaskID.
func makeTestWorker(id string, status models.WorkerStatus, tags []string, taskID *uuid.UUID) *models.Worker {
	return &models.Worker{
		ID:            id,
		Tags:          tags,
		Status:        status,
		LastHeartbeat: time.Now().UTC().Truncate(time.Second),
		RegisteredAt:  time.Now().UTC().Truncate(time.Second),
		CurrentTaskID: taskID,
	}
}

// --- List tests ---

// TestWorkerList_AuthenticatedReturns200WithAllWorkers verifies AC-1 and AC-2:
// GET /api/workers returns 200 with a JSON array of all registered workers,
// each including id, status, tags, currentTaskId (nullable), and lastHeartbeat.
func TestWorkerList_AuthenticatedReturns200WithAllWorkers(t *testing.T) {
	repo := newStubWorkerRepo()
	taskID := uuid.New()
	repo.add(makeTestWorker("worker-1", models.WorkerStatusOnline, []string{"gpu", "large"}, &taskID))
	repo.add(makeTestWorker("worker-2", models.WorkerStatusDown, []string{"cpu"}, nil))

	srv := newWorkerTestServer(repo)
	h := &WorkerHandler{server: srv}

	sess := &models.Session{UserID: uuid.New(), Role: models.RoleUser, CreatedAt: time.Now()}
	rec := httptest.NewRecorder()
	h.List(rec, workerGetRequest(t, sess))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var workers []workerResponse
	if err := json.NewDecoder(rec.Body).Decode(&workers); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if len(workers) != 2 {
		t.Fatalf("expected 2 workers, got %d", len(workers))
	}

	// Verify first worker: online, two tags, task assigned.
	w1 := workers[0]
	if w1.ID != "worker-1" {
		t.Errorf("worker[0].id: expected worker-1, got %s", w1.ID)
	}
	if w1.Status != "online" {
		t.Errorf("worker[0].status: expected online, got %s", w1.Status)
	}
	if len(w1.Tags) != 2 {
		t.Errorf("worker[0].tags: expected 2 tags, got %d", len(w1.Tags))
	}
	if w1.CurrentTaskID == nil || *w1.CurrentTaskID != taskID {
		t.Errorf("worker[0].currentTaskId: expected %s, got %v", taskID, w1.CurrentTaskID)
	}
	if w1.LastHeartbeat.IsZero() {
		t.Error("worker[0].lastHeartbeat: expected non-zero")
	}

	// Verify second worker: down, no task assignment.
	w2 := workers[1]
	if w2.ID != "worker-2" {
		t.Errorf("worker[1].id: expected worker-2, got %s", w2.ID)
	}
	if w2.Status != "down" {
		t.Errorf("worker[1].status: expected down, got %s", w2.Status)
	}
	if w2.CurrentTaskID != nil {
		t.Errorf("worker[1].currentTaskId: expected nil, got %v", w2.CurrentTaskID)
	}
}

// TestWorkerList_EmptyRegistryReturnsEmptyArray verifies AC-1 boundary case:
// GET /api/workers returns 200 with an empty JSON array when no workers are registered.
func TestWorkerList_EmptyRegistryReturnsEmptyArray(t *testing.T) {
	repo := newStubWorkerRepo()
	srv := newWorkerTestServer(repo)
	h := &WorkerHandler{server: srv}

	sess := &models.Session{UserID: uuid.New(), Role: models.RoleAdmin, CreatedAt: time.Now()}
	rec := httptest.NewRecorder()
	h.List(rec, workerGetRequest(t, sess))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Body must decode as an array, not null.
	var workers []workerResponse
	if err := json.NewDecoder(rec.Body).Decode(&workers); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if workers == nil {
		t.Error("expected empty array, got null")
	}
	if len(workers) != 0 {
		t.Errorf("expected 0 workers, got %d", len(workers))
	}
}

// TestWorkerList_UnauthenticatedReturns401 verifies AC-3:
// GET /api/workers returns 401 when no session is present in the request context.
func TestWorkerList_UnauthenticatedReturns401(t *testing.T) {
	repo := newStubWorkerRepo()
	srv := newWorkerTestServer(repo)
	h := &WorkerHandler{server: srv}

	rec := httptest.NewRecorder()
	// nil sess produces a request with no auth context.
	h.List(rec, workerGetRequest(t, nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestWorkerList_RepositoryErrorReturns500 verifies that a database error surfaces
// as 500 Internal Server Error without leaking implementation details to the caller.
func TestWorkerList_RepositoryErrorReturns500(t *testing.T) {
	repo := newStubWorkerRepo()
	repo.listErr = errors.New("simulated db failure")
	srv := newWorkerTestServer(repo)
	h := &WorkerHandler{server: srv}

	sess := &models.Session{UserID: uuid.New(), Role: models.RoleUser, CreatedAt: time.Now()}
	rec := httptest.NewRecorder()
	h.List(rec, workerGetRequest(t, sess))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestWorkerList_UserRoleSeesAllWorkers verifies Domain Invariant 5:
// All authenticated users (not just admins) can see all workers.
func TestWorkerList_UserRoleSeesAllWorkers(t *testing.T) {
	repo := newStubWorkerRepo()
	repo.add(makeTestWorker("worker-a", models.WorkerStatusOnline, []string{"cpu"}, nil))
	repo.add(makeTestWorker("worker-b", models.WorkerStatusOnline, []string{"gpu"}, nil))
	srv := newWorkerTestServer(repo)
	h := &WorkerHandler{server: srv}

	// Regular (non-admin) user — must still see all workers.
	sess := &models.Session{UserID: uuid.New(), Role: models.RoleUser, CreatedAt: time.Now()}
	rec := httptest.NewRecorder()
	h.List(rec, workerGetRequest(t, sess))

	if rec.Code != http.StatusOK {
		t.Fatalf("user role: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var workers []workerResponse
	if err := json.NewDecoder(rec.Body).Decode(&workers); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(workers) != 2 {
		t.Errorf("user role: expected 2 workers, got %d", len(workers))
	}
}

// TestWorkerList_ContentTypeIsJSON verifies the response carries the correct Content-Type header.
func TestWorkerList_ContentTypeIsJSON(t *testing.T) {
	repo := newStubWorkerRepo()
	srv := newWorkerTestServer(repo)
	h := &WorkerHandler{server: srv}

	sess := &models.Session{UserID: uuid.New(), Role: models.RoleUser, CreatedAt: time.Now()}
	rec := httptest.NewRecorder()
	h.List(rec, workerGetRequest(t, sess))

	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type: expected application/json, got %s", ct)
	}
}
