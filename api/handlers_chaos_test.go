// Package api — unit tests for ChaosHandler (KillWorker, DisconnectDatabase, FloodQueue).
//
// Tests use in-memory stubs defined in handlers_tasks_test.go (stubTaskRepo,
// stubPipelineRepo, stubProducer) and handlers_workers_test.go (stubWorkerRepo).
//
// Docker CLI calls are not invoked in tests. KillWorker tests verify the worker
// lookup path; DisconnectDatabase tests verify the 409 guard and validation;
// FloodQueue tests exercise the full task submission loop.
//
// See: DEMO-004, TASK-034
package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nxlabs/nexusflow/internal/models"
)

// newStubProducer constructs a fresh stubProducer with no enqueued messages.
func newStubProducer() *stubProducer {
	return &stubProducer{}
}

// newChaosTestServer constructs a minimal Server wired with the given stubs.
// The ChaosHandler uses dockerSocketPath set to a non-existent path so that
// docker CLI calls fail with a clear error rather than hitting the real daemon.
func newChaosTestServer(workers *stubWorkerRepo, tasks *stubTaskRepo, pipelines *stubPipelineRepo, producer *stubProducer) (*Server, *ChaosHandler) {
	srv := &Server{
		workers:   workers,
		tasks:     tasks,
		pipelines: pipelines,
		producer:  producer,
	}
	h := &ChaosHandler{
		server:           srv,
		dockerSocketPath: "/nonexistent/docker.sock", // prevents real Docker calls in tests
	}
	return srv, h
}

// --- KillWorker tests ---

// TestKillWorker_MissingWorkerIdReturns400 verifies that an empty workerId returns 400.
func TestKillWorker_MissingWorkerIdReturns400(t *testing.T) {
	workers := newStubWorkerRepo()
	_, h := newChaosTestServer(workers, newStubTaskRepo(), newStubPipelineRepo(), newStubProducer())

	req := httptest.NewRequest(http.MethodPost, "/api/chaos/kill-worker",
		bytes.NewBufferString(`{"workerId":""}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.KillWorker(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestKillWorker_MalformedBodyReturns400 verifies that invalid JSON returns 400.
func TestKillWorker_MalformedBodyReturns400(t *testing.T) {
	_, h := newChaosTestServer(newStubWorkerRepo(), newStubTaskRepo(), newStubPipelineRepo(), newStubProducer())

	req := httptest.NewRequest(http.MethodPost, "/api/chaos/kill-worker",
		bytes.NewBufferString(`not json`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.KillWorker(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestKillWorker_UnknownWorkerReturns404 verifies that an unregistered workerId returns 404.
func TestKillWorker_UnknownWorkerReturns404(t *testing.T) {
	workers := newStubWorkerRepo()
	_, h := newChaosTestServer(workers, newStubTaskRepo(), newStubPipelineRepo(), newStubProducer())

	body, _ := json.Marshal(killWorkerRequest{WorkerID: "unknown-worker-id"})
	req := httptest.NewRequest(http.MethodPost, "/api/chaos/kill-worker", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.KillWorker(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestKillWorker_DockerFailureReturns500 verifies that a Docker daemon error returns 500.
// The worker exists in the database but the Docker call fails (non-existent socket).
func TestKillWorker_DockerFailureReturns500(t *testing.T) {
	workers := newStubWorkerRepo()
	workers.add(&models.Worker{
		ID:            "worker-abc",
		Tags:          []string{"demo"},
		Status:        models.WorkerStatusOnline,
		LastHeartbeat: time.Now(),
		RegisteredAt:  time.Now(),
	})
	_, h := newChaosTestServer(workers, newStubTaskRepo(), newStubPipelineRepo(), newStubProducer())

	body, _ := json.Marshal(killWorkerRequest{WorkerID: "worker-abc"})
	req := httptest.NewRequest(http.MethodPost, "/api/chaos/kill-worker", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.KillWorker(rec, req)

	// Docker CLI fails because the socket path is non-existent.
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d: %s", rec.Code, rec.Body.String())
	}
}

// --- DisconnectDatabase tests ---

// TestDisconnectDatabase_MalformedBodyReturns400 verifies JSON decode error returns 400.
func TestDisconnectDatabase_MalformedBodyReturns400(t *testing.T) {
	_, h := newChaosTestServer(newStubWorkerRepo(), newStubTaskRepo(), newStubPipelineRepo(), newStubProducer())

	req := httptest.NewRequest(http.MethodPost, "/api/chaos/disconnect-db",
		bytes.NewBufferString(`not json`))
	rec := httptest.NewRecorder()

	h.DisconnectDatabase(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestDisconnectDatabase_InvalidDurationReturns400 verifies that a duration not in
// {15, 30, 60} returns 400.
func TestDisconnectDatabase_InvalidDurationReturns400(t *testing.T) {
	_, h := newChaosTestServer(newStubWorkerRepo(), newStubTaskRepo(), newStubPipelineRepo(), newStubProducer())

	body, _ := json.Marshal(disconnectDBRequest{DurationSeconds: 45})
	req := httptest.NewRequest(http.MethodPost, "/api/chaos/disconnect-db", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.DisconnectDatabase(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid duration, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestDisconnectDatabase_ZeroDurationReturns400 verifies that duration=0 returns 400.
func TestDisconnectDatabase_ZeroDurationReturns400(t *testing.T) {
	_, h := newChaosTestServer(newStubWorkerRepo(), newStubTaskRepo(), newStubPipelineRepo(), newStubProducer())

	body, _ := json.Marshal(disconnectDBRequest{DurationSeconds: 0})
	req := httptest.NewRequest(http.MethodPost, "/api/chaos/disconnect-db", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.DisconnectDatabase(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for zero duration, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestDisconnectDatabase_ConcurrentRequestReturns409 verifies the 409 guard.
// Sets the atomic flag to 1 (simulating an active disconnect) and verifies 409.
func TestDisconnectDatabase_ConcurrentRequestReturns409(t *testing.T) {
	_, h := newChaosTestServer(newStubWorkerRepo(), newStubTaskRepo(), newStubPipelineRepo(), newStubProducer())
	h.disconnectActive.Store(1) // simulate active disconnect

	body, _ := json.Marshal(disconnectDBRequest{DurationSeconds: 15})
	req := httptest.NewRequest(http.MethodPost, "/api/chaos/disconnect-db", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.DisconnectDatabase(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestDisconnectDatabase_DockerFailureReturns500AndReleasesGuard verifies that a
// Docker failure returns 500 and releases the atomic guard so subsequent requests
// are not blocked.
func TestDisconnectDatabase_DockerFailureReturns500AndReleasesGuard(t *testing.T) {
	_, h := newChaosTestServer(newStubWorkerRepo(), newStubTaskRepo(), newStubPipelineRepo(), newStubProducer())

	body, _ := json.Marshal(disconnectDBRequest{DurationSeconds: 15})
	req := httptest.NewRequest(http.MethodPost, "/api/chaos/disconnect-db", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.DisconnectDatabase(rec, req)

	// Docker fails (non-existent socket).
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d: %s", rec.Code, rec.Body.String())
	}

	// Guard must be released so a follow-up request is not blocked.
	if h.disconnectActive.Load() != 0 {
		t.Error("disconnectActive guard was not released after Docker failure")
	}
}

// --- FloodQueue tests ---

// TestFloodQueue_MalformedBodyReturns400 verifies JSON decode error returns 400.
func TestFloodQueue_MalformedBodyReturns400(t *testing.T) {
	_, h := newChaosTestServer(newStubWorkerRepo(), newStubTaskRepo(), newStubPipelineRepo(), newStubProducer())

	req := httptest.NewRequest(http.MethodPost, "/api/chaos/flood-queue",
		bytes.NewBufferString(`not json`))
	rec := httptest.NewRecorder()

	h.FloodQueue(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestFloodQueue_MissingPipelineIdReturns400 verifies that an empty pipelineId returns 400.
func TestFloodQueue_MissingPipelineIdReturns400(t *testing.T) {
	_, h := newChaosTestServer(newStubWorkerRepo(), newStubTaskRepo(), newStubPipelineRepo(), newStubProducer())

	body, _ := json.Marshal(floodQueueRequest{PipelineID: "", TaskCount: 10})
	req := httptest.NewRequest(http.MethodPost, "/api/chaos/flood-queue", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.FloodQueue(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestFloodQueue_TaskCountZeroReturns400 verifies that taskCount=0 returns 400.
func TestFloodQueue_TaskCountZeroReturns400(t *testing.T) {
	_, h := newChaosTestServer(newStubWorkerRepo(), newStubTaskRepo(), newStubPipelineRepo(), newStubProducer())

	body, _ := json.Marshal(floodQueueRequest{PipelineID: uuid.New().String(), TaskCount: 0})
	req := httptest.NewRequest(http.MethodPost, "/api/chaos/flood-queue", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.FloodQueue(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for taskCount=0, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestFloodQueue_TaskCountOver1000Returns400 verifies that taskCount=1001 returns 400.
func TestFloodQueue_TaskCountOver1000Returns400(t *testing.T) {
	_, h := newChaosTestServer(newStubWorkerRepo(), newStubTaskRepo(), newStubPipelineRepo(), newStubProducer())

	body, _ := json.Marshal(floodQueueRequest{PipelineID: uuid.New().String(), TaskCount: 1001})
	req := httptest.NewRequest(http.MethodPost, "/api/chaos/flood-queue", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.FloodQueue(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for taskCount>1000, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestFloodQueue_InvalidUUIDReturns400 verifies that a non-UUID pipelineId returns 400.
func TestFloodQueue_InvalidUUIDReturns400(t *testing.T) {
	_, h := newChaosTestServer(newStubWorkerRepo(), newStubTaskRepo(), newStubPipelineRepo(), newStubProducer())

	body, _ := json.Marshal(floodQueueRequest{PipelineID: "not-a-uuid", TaskCount: 5})
	req := httptest.NewRequest(http.MethodPost, "/api/chaos/flood-queue", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.FloodQueue(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid UUID, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestFloodQueue_UnknownPipelineReturns404 verifies that a non-existent pipeline returns 404.
func TestFloodQueue_UnknownPipelineReturns404(t *testing.T) {
	_, h := newChaosTestServer(newStubWorkerRepo(), newStubTaskRepo(), newStubPipelineRepo(), newStubProducer())

	body, _ := json.Marshal(floodQueueRequest{PipelineID: uuid.New().String(), TaskCount: 5})
	req := httptest.NewRequest(http.MethodPost, "/api/chaos/flood-queue", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.FloodQueue(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestFloodQueue_SubmitsAllTasksAndReturns200 verifies the happy path:
// taskCount tasks are created, queued, and enqueued; submittedCount equals taskCount.
func TestFloodQueue_SubmitsAllTasksAndReturns200(t *testing.T) {
	pipelines := newStubPipelineRepo()
	pipelineID := uuid.New()
	ownerID := uuid.New()
	pipelines.addPipeline(&models.Pipeline{
		ID:     pipelineID,
		Name:   "flood-test-pipeline",
		UserID: ownerID,
		DataSourceConfig: models.DataSourceConfig{
			ConnectorType: "demo",
			Config:        map[string]any{},
			OutputSchema:  []string{"field"},
		},
		ProcessConfig: models.ProcessConfig{
			ConnectorType: "simulated",
			Config:        map[string]any{},
			InputMappings: []models.SchemaMapping{},
			OutputSchema:  []string{"field"},
		},
		SinkConfig: models.SinkConfig{
			ConnectorType: "demo",
			Config:        map[string]any{},
			InputMappings: []models.SchemaMapping{},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	})

	tasks := newStubTaskRepo()
	producer := newStubProducer()
	_, h := newChaosTestServer(newStubWorkerRepo(), tasks, pipelines, producer)

	const count = 5
	body, _ := json.Marshal(floodQueueRequest{PipelineID: pipelineID.String(), TaskCount: count})
	req := httptest.NewRequest(http.MethodPost, "/api/chaos/flood-queue", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.FloodQueue(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp floodQueueResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.SubmittedCount != count {
		t.Errorf("expected submittedCount=%d, got %d", count, resp.SubmittedCount)
	}
	if len(resp.Log) == 0 {
		t.Error("expected non-empty activity log")
	}
	// Verify the producer received exactly count Enqueue calls.
	if len(producer.enqueued) != count {
		t.Errorf("expected producer.enqueued len=%d, got %d", count, len(producer.enqueued))
	}
}

// TestFloodQueue_TaskCountOneBoundary verifies taskCount=1 (lower bound) is accepted.
func TestFloodQueue_TaskCountOneBoundary(t *testing.T) {
	pipelines := newStubPipelineRepo()
	pipelineID := uuid.New()
	pipelines.addPipeline(&models.Pipeline{
		ID:               pipelineID,
		Name:             "p1",
		UserID:           uuid.New(),
		DataSourceConfig: models.DataSourceConfig{ConnectorType: "demo", Config: map[string]any{}, OutputSchema: []string{}},
		ProcessConfig:    models.ProcessConfig{ConnectorType: "simulated", Config: map[string]any{}, InputMappings: []models.SchemaMapping{}, OutputSchema: []string{}},
		SinkConfig:       models.SinkConfig{ConnectorType: "demo", Config: map[string]any{}, InputMappings: []models.SchemaMapping{}},
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	})
	_, h := newChaosTestServer(newStubWorkerRepo(), newStubTaskRepo(), pipelines, newStubProducer())

	body, _ := json.Marshal(floodQueueRequest{PipelineID: pipelineID.String(), TaskCount: 1})
	req := httptest.NewRequest(http.MethodPost, "/api/chaos/flood-queue", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.FloodQueue(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for taskCount=1, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestFloodQueue_TaskCountMaxBoundary verifies taskCount=1000 (upper bound) is accepted.
func TestFloodQueue_TaskCountMaxBoundary(t *testing.T) {
	pipelines := newStubPipelineRepo()
	pipelineID := uuid.New()
	pipelines.addPipeline(&models.Pipeline{
		ID:               pipelineID,
		Name:             "p-max",
		UserID:           uuid.New(),
		DataSourceConfig: models.DataSourceConfig{ConnectorType: "demo", Config: map[string]any{}, OutputSchema: []string{}},
		ProcessConfig:    models.ProcessConfig{ConnectorType: "simulated", Config: map[string]any{}, InputMappings: []models.SchemaMapping{}, OutputSchema: []string{}},
		SinkConfig:       models.SinkConfig{ConnectorType: "demo", Config: map[string]any{}, InputMappings: []models.SchemaMapping{}},
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	})
	_, h := newChaosTestServer(newStubWorkerRepo(), newStubTaskRepo(), pipelines, newStubProducer())

	body, _ := json.Marshal(floodQueueRequest{PipelineID: pipelineID.String(), TaskCount: 1000})
	req := httptest.NewRequest(http.MethodPost, "/api/chaos/flood-queue", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.FloodQueue(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for taskCount=1000, got %d: %s", rec.Code, rec.Body.String())
	}
}
