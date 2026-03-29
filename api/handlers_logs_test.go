// Package api — unit tests for GET /api/tasks/{id}/logs.
// Verifies access control (owner or admin) and correct log line serialisation.
// See: REQ-018, TASK-016
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/nxlabs/nexusflow/internal/auth"
	"github.com/nxlabs/nexusflow/internal/db"
	"github.com/nxlabs/nexusflow/internal/models"
)

// stubTaskLogRepo is an in-memory TaskLogRepository for testing.
type stubTaskLogRepo struct {
	mu   sync.Mutex
	rows []*models.TaskLog
}

func (s *stubTaskLogRepo) BatchInsert(_ context.Context, logs []*models.TaskLog) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rows = append(s.rows, logs...)
	return nil
}

func (s *stubTaskLogRepo) ListByTask(_ context.Context, taskID uuid.UUID, _ string) ([]*models.TaskLog, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*models.TaskLog
	for _, r := range s.rows {
		if r.TaskID == taskID {
			cp := *r
			out = append(out, &cp)
		}
	}
	return out, nil
}

// Compile-time check: stubTaskLogRepo satisfies db.TaskLogRepository.
var _ db.TaskLogRepository = (*stubTaskLogRepo)(nil)

// buildLogsServer constructs a minimal Server with only the dependencies needed
// by the GET /api/tasks/{id}/logs handler.
func buildLogsServer(tasks db.TaskRepository, taskLogs db.TaskLogRepository) *Server {
	return &Server{
		tasks:    tasks,
		taskLogs: taskLogs,
	}
}

// serveLogsRequest routes a GET /api/tasks/{id}/logs request through a real chi router
// with session injection via auth.Middleware. Returns the recorded response.
// Using a real router ensures chi.URLParam("id") works correctly.
func serveLogsRequest(t *testing.T, srv *Server, taskIDStr string, sess *models.Session) *httptest.ResponseRecorder {
	t.Helper()
	h := &LogHandler{server: srv}

	rr := chi.NewRouter()
	rr.Get("/api/tasks/{id}/logs", func(w http.ResponseWriter, r *http.Request) {
		// Inject the session via auth.Middleware.
		store := newStubSessionStore()
		token := "logs-token-" + uuid.New().String()
		_ = store.Create(context.Background(), token, sess)
		r.Header.Set("Authorization", "Bearer "+token)

		var captured *http.Request
		inner := http.HandlerFunc(func(_ http.ResponseWriter, req *http.Request) {
			captured = req
		})
		auth.Middleware(store)(inner).ServeHTTP(httptest.NewRecorder(), r)
		if captured == nil {
			t.Error("auth.Middleware did not call inner handler — session injection failed")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		h.GetLogs(w, captured)
	})

	w := httptest.NewRecorder()
	rr.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/tasks/"+taskIDStr+"/logs", nil))
	return w
}

// TestGetLogs_OwnerReceivesLogs verifies that the task owner can retrieve log lines
// from cold storage and all required fields are present in the JSON response.
// AC-4: GET /api/tasks/{id}/logs returns historical log lines from PostgreSQL.
func TestGetLogs_OwnerReceivesLogs(t *testing.T) {
	ownerID := uuid.New()
	taskID := uuid.New()

	taskRepo := newStubTaskRepo()
	task := &models.Task{
		ID:          taskID,
		UserID:      ownerID,
		Status:      models.TaskStatusCompleted,
		RetryConfig: models.DefaultRetryConfig(),
		Input:       map[string]any{},
	}
	_, _ = taskRepo.Create(context.Background(), task)

	logRepo := &stubTaskLogRepo{}
	_ = logRepo.BatchInsert(context.Background(), []*models.TaskLog{
		{
			ID:        uuid.New(),
			TaskID:    taskID,
			Line:      "[datasource] fetching records",
			Level:     "INFO",
			Timestamp: time.Now().UTC(),
		},
	})

	srv := buildLogsServer(taskRepo, logRepo)
	w := serveLogsRequest(t, srv, taskID.String(), &models.Session{UserID: ownerID, Role: models.RoleUser})

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}

	var body []models.TaskLog
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("cannot decode response: %v", err)
	}
	if len(body) != 1 {
		t.Fatalf("want 1 log line, got %d", len(body))
	}
	if body[0].Level != "INFO" {
		t.Errorf("want level INFO, got %q", body[0].Level)
	}
}

// TestGetLogs_AdminReceivesAnyTaskLogs verifies that an admin can retrieve logs for
// any task regardless of ownership (AC-6).
func TestGetLogs_AdminReceivesAnyTaskLogs(t *testing.T) {
	ownerID := uuid.New()
	adminID := uuid.New()
	taskID := uuid.New()

	taskRepo := newStubTaskRepo()
	task := &models.Task{
		ID:          taskID,
		UserID:      ownerID,
		Status:      models.TaskStatusCompleted,
		RetryConfig: models.DefaultRetryConfig(),
		Input:       map[string]any{},
	}
	_, _ = taskRepo.Create(context.Background(), task)

	logRepo := &stubTaskLogRepo{}
	_ = logRepo.BatchInsert(context.Background(), []*models.TaskLog{
		{ID: uuid.New(), TaskID: taskID, Line: "[sink] writing", Level: "INFO", Timestamp: time.Now()},
	})

	srv := buildLogsServer(taskRepo, logRepo)
	w := serveLogsRequest(t, srv, taskID.String(), &models.Session{UserID: adminID, Role: models.RoleAdmin})

	if w.Code != http.StatusOK {
		t.Fatalf("admin: want 200, got %d", w.Code)
	}
}

// TestGetLogs_NonOwnerReceivesForbidden verifies that a non-admin user who does not
// own the task receives 403 (AC-6: access control).
func TestGetLogs_NonOwnerReceivesForbidden(t *testing.T) {
	ownerID := uuid.New()
	otherID := uuid.New()
	taskID := uuid.New()

	taskRepo := newStubTaskRepo()
	task := &models.Task{
		ID:          taskID,
		UserID:      ownerID,
		Status:      models.TaskStatusCompleted,
		RetryConfig: models.DefaultRetryConfig(),
		Input:       map[string]any{},
	}
	_, _ = taskRepo.Create(context.Background(), task)

	srv := buildLogsServer(taskRepo, &stubTaskLogRepo{})
	w := serveLogsRequest(t, srv, taskID.String(), &models.Session{UserID: otherID, Role: models.RoleUser})

	if w.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d", w.Code)
	}
}

// TestGetLogs_NotFoundTask returns 404 when the task does not exist.
func TestGetLogs_NotFoundTask(t *testing.T) {
	taskID := uuid.New()
	srv := buildLogsServer(newStubTaskRepo(), &stubTaskLogRepo{})
	w := serveLogsRequest(t, srv, taskID.String(), &models.Session{UserID: uuid.New(), Role: models.RoleUser})

	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}
}

// TestGetLogs_InvalidUUID returns 400 for a malformed task ID in the URL path.
// The chi router matches "not-a-uuid" as the {id} parameter; the handler parses it.
func TestGetLogs_InvalidUUID(t *testing.T) {
	srv := buildLogsServer(newStubTaskRepo(), &stubTaskLogRepo{})
	w := serveLogsRequest(t, srv, "not-a-uuid", &models.Session{UserID: uuid.New(), Role: models.RoleUser})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

// TestGetLogs_EmptyListReturnsArray verifies that when a task has no cold logs yet
// the response is an empty JSON array (never null).
func TestGetLogs_EmptyListReturnsArray(t *testing.T) {
	ownerID := uuid.New()
	taskID := uuid.New()

	taskRepo := newStubTaskRepo()
	task := &models.Task{
		ID:          taskID,
		UserID:      ownerID,
		Status:      models.TaskStatusRunning,
		RetryConfig: models.DefaultRetryConfig(),
		Input:       map[string]any{},
	}
	_, _ = taskRepo.Create(context.Background(), task)

	srv := buildLogsServer(taskRepo, &stubTaskLogRepo{})
	w := serveLogsRequest(t, srv, taskID.String(), &models.Session{UserID: ownerID, Role: models.RoleUser})

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	body := w.Body.String()
	if body == "null\n" || body == "null" {
		t.Error("response body must be [] not null")
	}
}
