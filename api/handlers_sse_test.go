// Package api — unit tests for SSEHandler.
// Verifies that each SSE endpoint delegates to the correct Broker method with the
// correct arguments, and that the chi URL parameter is extracted correctly.
// Uses an in-memory stubSSEBroker instead of a live Redis connection.
// Session injection follows the same pattern as handlers_tasks_test.go:
// auth.Middleware wraps the request so SessionFromContext succeeds in the handler.
// See: ADR-007, TASK-015
package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/nxlabs/nexusflow/internal/auth"
	"github.com/nxlabs/nexusflow/internal/models"
	"github.com/nxlabs/nexusflow/internal/queue"
)

// --- stub broker ---

// stubSSEBroker records which Broker methods were called and with which arguments.
// Used to verify that SSEHandler delegates correctly without exercising Redis.
type stubSSEBroker struct {
	serveTaskCalled   bool
	serveWorkerCalled bool
	serveLogCalled    bool
	serveLogTaskID    string
	serveSinkCalled   bool
	serveSinkTaskID   string
	capturedSession   *models.Session
	publishCalled     bool // set to true when PublishTaskEvent is called
}

func (b *stubSSEBroker) Start(_ context.Context) error { return nil }

func (b *stubSSEBroker) ServeTaskEvents(w http.ResponseWriter, _ *http.Request, session *models.Session) {
	b.serveTaskCalled = true
	b.capturedSession = session
	w.WriteHeader(http.StatusOK)
}

func (b *stubSSEBroker) ServeWorkerEvents(w http.ResponseWriter, _ *http.Request, session *models.Session) {
	b.serveWorkerCalled = true
	b.capturedSession = session
	w.WriteHeader(http.StatusOK)
}

func (b *stubSSEBroker) ServeLogEvents(w http.ResponseWriter, _ *http.Request, session *models.Session, taskID string) {
	b.serveLogCalled = true
	b.serveLogTaskID = taskID
	b.capturedSession = session
	w.WriteHeader(http.StatusOK)
}

func (b *stubSSEBroker) ServeSinkEvents(w http.ResponseWriter, _ *http.Request, session *models.Session, taskID string) {
	b.serveSinkCalled = true
	b.serveSinkTaskID = taskID
	b.capturedSession = session
	w.WriteHeader(http.StatusOK)
}

func (b *stubSSEBroker) PublishTaskEvent(_ context.Context, _ *models.Task, _ string) error {
	b.publishCalled = true
	return nil
}

func (b *stubSSEBroker) PublishWorkerEvent(_ context.Context, _ *models.Worker) error { return nil }

func (b *stubSSEBroker) PublishLogLine(_ context.Context, _ *models.TaskLog) error { return nil }

func (b *stubSSEBroker) PublishSinkSnapshot(_ context.Context, _ *models.SinkSnapshot) error {
	return nil
}

// --- helpers ---

// buildSSEServer creates a minimal Server with the given stub broker.
func buildSSEServer(broker *stubSSEBroker) *Server {
	return &Server{
		broker: broker,
	}
}

// sseRequest builds an HTTP request with the given session injected via auth.Middleware,
// matching the mechanism used by the production auth middleware.
func sseRequest(t *testing.T, method, path string, session *models.Session) *http.Request {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	if session == nil {
		return req
	}

	store := newStubSessionStoreSSE()
	token := "sse-test-token-" + uuid.New().String()
	if err := store.Create(context.Background(), token, session); err != nil {
		t.Fatalf("sseRequest: create session: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	// Run the request through auth.Middleware so SessionFromContext is populated.
	var captured *http.Request
	inner := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		captured = r
	})
	auth.Middleware(store)(inner).ServeHTTP(httptest.NewRecorder(), req)
	if captured == nil {
		t.Fatal("auth.Middleware did not call inner handler — session injection failed")
	}
	return captured
}

// newStubSessionStoreSSE returns a minimal in-memory SessionStore for SSE handler tests.
// Reuses the stubSessionStore already defined in handlers_tasks_test.go (same package).
func newStubSessionStoreSSE() queue.SessionStore {
	return newStubSessionStore()
}

// --- SSEHandler tests ---

func TestSSEHandler_Tasks_DelegatesToBrokerServeTaskEvents(t *testing.T) {
	stub := &stubSSEBroker{}
	h := &SSEHandler{server: buildSSEServer(stub)}

	session := &models.Session{
		UserID:    uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"),
		Role:      models.RoleUser,
		CreatedAt: time.Now(),
	}
	r := sseRequest(t, http.MethodGet, "/events/tasks", session)
	w := httptest.NewRecorder()

	h.Tasks(w, r)

	if !stub.serveTaskCalled {
		t.Error("expected SSEHandler.Tasks to call Broker.ServeTaskEvents; it was not called")
	}
	if stub.capturedSession == nil || stub.capturedSession.UserID != session.UserID {
		t.Error("expected the authenticated session to be passed to ServeTaskEvents")
	}
}

func TestSSEHandler_Workers_DelegatesToBrokerServeWorkerEvents(t *testing.T) {
	stub := &stubSSEBroker{}
	h := &SSEHandler{server: buildSSEServer(stub)}

	session := &models.Session{
		UserID:    uuid.MustParse("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"),
		Role:      models.RoleUser,
		CreatedAt: time.Now(),
	}
	r := sseRequest(t, http.MethodGet, "/events/workers", session)
	w := httptest.NewRecorder()

	h.Workers(w, r)

	if !stub.serveWorkerCalled {
		t.Error("expected SSEHandler.Workers to call Broker.ServeWorkerEvents; it was not called")
	}
}

func TestSSEHandler_Logs_ExtractsTaskIDAndDelegatesToBroker(t *testing.T) {
	stub := &stubSSEBroker{}
	h := &SSEHandler{server: buildSSEServer(stub)}

	taskID := uuid.New().String()
	session := &models.Session{
		UserID:    uuid.MustParse("cccccccc-cccc-cccc-cccc-cccccccccccc"),
		Role:      models.RoleUser,
		CreatedAt: time.Now(),
	}

	// Wire the chi URL parameter so chi.URLParam("id") returns taskID.
	rr := chi.NewRouter()
	rr.Get("/events/tasks/{id}/logs", func(w http.ResponseWriter, r *http.Request) {
		// Re-apply the auth context (chi router recreates the request).
		store := newStubSessionStoreSSE()
		token := "sse-token"
		_ = store.Create(context.Background(), token, session)
		r.Header.Set("Authorization", "Bearer "+token)

		var captured *http.Request
		inner := http.HandlerFunc(func(_ http.ResponseWriter, req *http.Request) {
			captured = req
		})
		auth.Middleware(store)(inner).ServeHTTP(httptest.NewRecorder(), r)
		if captured == nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		h.Logs(w, captured)
	})

	r := httptest.NewRequest(http.MethodGet, "/events/tasks/"+taskID+"/logs", nil)
	w := httptest.NewRecorder()
	rr.ServeHTTP(w, r)

	if !stub.serveLogCalled {
		t.Error("expected SSEHandler.Logs to call Broker.ServeLogEvents; it was not called")
	}
	if stub.serveLogTaskID != taskID {
		t.Errorf("expected taskID %q passed to ServeLogEvents, got %q", taskID, stub.serveLogTaskID)
	}
}

func TestSSEHandler_Sink_ExtractsTaskIDAndDelegatesToBroker(t *testing.T) {
	stub := &stubSSEBroker{}
	h := &SSEHandler{server: buildSSEServer(stub)}

	taskID := uuid.New().String()
	session := &models.Session{
		UserID:    uuid.MustParse("dddddddd-dddd-dddd-dddd-dddddddddddd"),
		Role:      models.RoleAdmin,
		CreatedAt: time.Now(),
	}

	rr := chi.NewRouter()
	rr.Get("/events/sink/{taskId}", func(w http.ResponseWriter, r *http.Request) {
		store := newStubSessionStoreSSE()
		token := "sse-sink-token"
		_ = store.Create(context.Background(), token, session)
		r.Header.Set("Authorization", "Bearer "+token)

		var captured *http.Request
		inner := http.HandlerFunc(func(_ http.ResponseWriter, req *http.Request) {
			captured = req
		})
		auth.Middleware(store)(inner).ServeHTTP(httptest.NewRecorder(), r)
		if captured == nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		h.Sink(w, captured)
	})

	r := httptest.NewRequest(http.MethodGet, "/events/sink/"+taskID, nil)
	w := httptest.NewRecorder()
	rr.ServeHTTP(w, r)

	if !stub.serveSinkCalled {
		t.Error("expected SSEHandler.Sink to call Broker.ServeSinkEvents; it was not called")
	}
	if stub.serveSinkTaskID != taskID {
		t.Errorf("expected taskID %q passed to ServeSinkEvents, got %q", taskID, stub.serveSinkTaskID)
	}
}
