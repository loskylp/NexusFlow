// Package api — unit tests for TaskHandler.Submit.
// Uses in-memory stubs for TaskRepository, PipelineRepository, and queue.Producer
// to avoid external dependencies on PostgreSQL or Redis.
// See: REQ-001, REQ-003, REQ-009, TASK-005
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nxlabs/nexusflow/internal/auth"
	"github.com/nxlabs/nexusflow/internal/db"
	"github.com/nxlabs/nexusflow/internal/models"
	"github.com/nxlabs/nexusflow/internal/queue"
)

// --- stubs ---

// stubTaskRepo is an in-memory TaskRepository for testing.
type stubTaskRepo struct {
	tasks       map[uuid.UUID]*models.Task
	statusLog   []*models.TaskStateLog
	createErr   error
	updateErr   error
}

func newStubTaskRepo() *stubTaskRepo {
	return &stubTaskRepo{
		tasks: make(map[uuid.UUID]*models.Task),
	}
}

func (r *stubTaskRepo) Create(_ context.Context, task *models.Task) (*models.Task, error) {
	if r.createErr != nil {
		return nil, r.createErr
	}
	t := *task
	t.CreatedAt = time.Now()
	t.UpdatedAt = time.Now()
	r.tasks[t.ID] = &t
	return &t, nil
}

func (r *stubTaskRepo) GetByID(_ context.Context, id uuid.UUID) (*models.Task, error) {
	t, ok := r.tasks[id]
	if !ok {
		return nil, nil
	}
	return t, nil
}

func (r *stubTaskRepo) ListByUser(_ context.Context, _ uuid.UUID) ([]*models.Task, error) {
	return nil, nil
}

func (r *stubTaskRepo) List(_ context.Context) ([]*models.Task, error) {
	return nil, nil
}

func (r *stubTaskRepo) UpdateStatus(_ context.Context, id uuid.UUID, newStatus models.TaskStatus, reason string, workerID *string) error {
	if r.updateErr != nil {
		return r.updateErr
	}
	if t, ok := r.tasks[id]; ok {
		oldStatus := t.Status
		t.Status = newStatus
		r.statusLog = append(r.statusLog, &models.TaskStateLog{
			ID:        uuid.New(),
			TaskID:    id,
			FromState: oldStatus,
			ToState:   newStatus,
			Reason:    reason,
			Timestamp: time.Now(),
		})
	}
	return nil
}

func (r *stubTaskRepo) IncrementRetryCount(_ context.Context, _ uuid.UUID) (int, error) {
	return 0, nil
}

func (r *stubTaskRepo) Cancel(_ context.Context, _ uuid.UUID, _ string) error {
	return nil
}

func (r *stubTaskRepo) GetStateLog(_ context.Context, taskID uuid.UUID) ([]*models.TaskStateLog, error) {
	var out []*models.TaskStateLog
	for _, entry := range r.statusLog {
		if entry.TaskID == taskID {
			out = append(out, entry)
		}
	}
	return out, nil
}

// stubPipelineRepo is an in-memory PipelineRepository for testing.
type stubPipelineRepo struct {
	pipelines map[uuid.UUID]*models.Pipeline
}

func newStubPipelineRepo() *stubPipelineRepo {
	return &stubPipelineRepo{pipelines: make(map[uuid.UUID]*models.Pipeline)}
}

func (r *stubPipelineRepo) addPipeline(p *models.Pipeline) {
	r.pipelines[p.ID] = p
}

func (r *stubPipelineRepo) Create(_ context.Context, p *models.Pipeline) (*models.Pipeline, error) {
	r.pipelines[p.ID] = p
	return p, nil
}

func (r *stubPipelineRepo) GetByID(_ context.Context, id uuid.UUID) (*models.Pipeline, error) {
	p, ok := r.pipelines[id]
	if !ok {
		return nil, nil
	}
	return p, nil
}

func (r *stubPipelineRepo) ListByUser(_ context.Context, _ uuid.UUID) ([]*models.Pipeline, error) {
	return nil, nil
}

func (r *stubPipelineRepo) List(_ context.Context) ([]*models.Pipeline, error) {
	return nil, nil
}

func (r *stubPipelineRepo) Update(_ context.Context, p *models.Pipeline) (*models.Pipeline, error) {
	r.pipelines[p.ID] = p
	return p, nil
}

func (r *stubPipelineRepo) Delete(_ context.Context, id uuid.UUID) error {
	delete(r.pipelines, id)
	return nil
}

func (r *stubPipelineRepo) HasActiveTasks(_ context.Context, _ uuid.UUID) (bool, error) {
	return false, nil
}

// stubProducer is an in-memory queue.Producer for testing.
type stubProducer struct {
	enqueued []*queue.ProducerMessage
	enqueueErr error
}

func (p *stubProducer) Enqueue(_ context.Context, msg *queue.ProducerMessage) ([]string, error) {
	if p.enqueueErr != nil {
		return nil, p.enqueueErr
	}
	p.enqueued = append(p.enqueued, msg)
	return []string{"0-1"}, nil
}

func (p *stubProducer) EnqueueDeadLetter(_ context.Context, _ string, _ string) error {
	return nil
}

// --- test helper ---

// newTaskTestServer builds a minimal Server with stub repositories and producer.
func newTaskTestServer(tasks *stubTaskRepo, pipelines *stubPipelineRepo, producer *stubProducer) *Server {
	return &Server{
		tasks:     tasks,
		pipelines: pipelines,
		producer:  producer,
	}
}

// taskSubmitRequest builds a POST /api/tasks request with the given body.
// If sess is non-nil, it is injected into the request context via a stub session store
// and the auth.Middleware wrapper, simulating an authenticated request.
// If sess is nil, no authentication is applied (simulates a missing/invalid token).
func taskSubmitRequest(t *testing.T, body any, sess *models.Session) *http.Request {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("taskSubmitRequest: marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/tasks", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	if sess != nil {
		// Inject session into context using the same mechanism as auth.Middleware.
		// auth.SessionFromContext reads from the same unexported key.
		store := newStubSessionStore()
		token := "test-token"
		_ = store.Create(context.Background(), token, sess)
		req.Header.Set("Authorization", "Bearer "+token)
		// Wrap the request through a no-op handler to have auth.Middleware inject context.
		var captured *http.Request
		inner := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			captured = r
		})
		mw := auth.Middleware(store)
		mw(inner).ServeHTTP(httptest.NewRecorder(), req)
		if captured == nil {
			t.Fatal("auth.Middleware did not call inner handler — session injection failed")
		}
		req = captured
	}
	return req
}

// validPipeline returns a minimal Pipeline with one demo-typed phase config.
func validPipeline(ownerID uuid.UUID) *models.Pipeline {
	return &models.Pipeline{
		ID:     uuid.New(),
		Name:   "test-pipeline",
		UserID: ownerID,
		DataSourceConfig: models.DataSourceConfig{ConnectorType: "demo"},
		ProcessConfig:    models.ProcessConfig{ConnectorType: "demo"},
		SinkConfig:       models.SinkConfig{ConnectorType: "demo"},
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
}

// --- Submit tests ---

// TestSubmit_ValidPayloadReturns201WithTaskID verifies acceptance criterion 1:
// POST /api/tasks with valid payload returns 201 with unique task ID.
func TestSubmit_ValidPayloadReturns201WithTaskID(t *testing.T) {
	tasks := newStubTaskRepo()
	pipelines := newStubPipelineRepo()
	producer := &stubProducer{}

	userID := uuid.New()
	pipeline := validPipeline(userID)
	pipelines.addPipeline(pipeline)

	sess := &models.Session{UserID: userID, Role: models.RoleUser, CreatedAt: time.Now()}
	srv := newTaskTestServer(tasks, pipelines, producer)
	h := &TaskHandler{server: srv}

	reqBody := map[string]any{
		"pipelineId": pipeline.ID.String(),
		"input":      map[string]any{"key": "value"},
		"tags":       []string{"etl"},
	}
	rec := httptest.NewRecorder()
	h.Submit(rec, taskSubmitRequest(t, reqBody, sess))

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp submitResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.TaskID == uuid.Nil {
		t.Error("expected non-nil task ID in response")
	}
	if resp.Status != string(models.TaskStatusQueued) {
		t.Errorf("expected status %q, got %q", models.TaskStatusQueued, resp.Status)
	}
}

// TestSubmit_TaskExistsInPostgreSQL verifies acceptance criterion 2:
// Task record exists in PostgreSQL with status "submitted" then "queued".
func TestSubmit_TaskExistsInPostgreSQL(t *testing.T) {
	tasks := newStubTaskRepo()
	pipelines := newStubPipelineRepo()
	producer := &stubProducer{}

	userID := uuid.New()
	pipeline := validPipeline(userID)
	pipelines.addPipeline(pipeline)

	sess := &models.Session{UserID: userID, Role: models.RoleUser, CreatedAt: time.Now()}
	srv := newTaskTestServer(tasks, pipelines, producer)
	h := &TaskHandler{server: srv}

	reqBody := map[string]any{
		"pipelineId": pipeline.ID.String(),
		"input":      map[string]any{},
		"tags":       []string{"etl"},
	}
	rec := httptest.NewRecorder()
	h.Submit(rec, taskSubmitRequest(t, reqBody, sess))

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}

	var resp submitResponse
	_ = json.NewDecoder(rec.Body).Decode(&resp)

	// Task must exist with final status "queued".
	task, _ := tasks.GetByID(context.Background(), resp.TaskID)
	if task == nil {
		t.Fatal("task not found in repository after submit")
	}
	if task.Status != models.TaskStatusQueued {
		t.Errorf("expected final task status %q, got %q", models.TaskStatusQueued, task.Status)
	}

	// State log must record the submitted -> queued transition.
	log, _ := tasks.GetStateLog(context.Background(), resp.TaskID)
	if len(log) == 0 {
		t.Error("expected at least one state log entry")
	}
	var foundTransition bool
	for _, entry := range log {
		if entry.FromState == models.TaskStatusSubmitted && entry.ToState == models.TaskStatusQueued {
			foundTransition = true
		}
	}
	if !foundTransition {
		t.Error("expected submitted->queued transition in state log")
	}
}

// TestSubmit_TaskEnqueuedToRedisStream verifies acceptance criterion 3:
// Task message exists in the appropriate Redis stream (queue:{tag}).
func TestSubmit_TaskEnqueuedToRedisStream(t *testing.T) {
	tasks := newStubTaskRepo()
	pipelines := newStubPipelineRepo()
	producer := &stubProducer{}

	userID := uuid.New()
	pipeline := validPipeline(userID)
	pipelines.addPipeline(pipeline)

	sess := &models.Session{UserID: userID, Role: models.RoleUser, CreatedAt: time.Now()}
	srv := newTaskTestServer(tasks, pipelines, producer)
	h := &TaskHandler{server: srv}

	reqBody := map[string]any{
		"pipelineId": pipeline.ID.String(),
		"input":      map[string]any{},
		"tags":       []string{"etl"},
	}
	rec := httptest.NewRecorder()
	h.Submit(rec, taskSubmitRequest(t, reqBody, sess))

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}

	if len(producer.enqueued) == 0 {
		t.Fatal("expected at least one message enqueued to producer")
	}
	msg := producer.enqueued[0]
	if msg.Task == nil {
		t.Fatal("enqueued message has nil task")
	}
	if len(msg.Tags) == 0 || msg.Tags[0] != "etl" {
		t.Errorf("expected tag %q, got %v", "etl", msg.Tags)
	}
}

// TestSubmit_InvalidPipelineReturns400 verifies acceptance criterion 4:
// POST /api/tasks with invalid pipeline reference returns 400 with structured error.
func TestSubmit_InvalidPipelineReturns400(t *testing.T) {
	tasks := newStubTaskRepo()
	pipelines := newStubPipelineRepo() // empty — no pipelines registered
	producer := &stubProducer{}

	userID := uuid.New()
	sess := &models.Session{UserID: userID, Role: models.RoleUser, CreatedAt: time.Now()}
	srv := newTaskTestServer(tasks, pipelines, producer)
	h := &TaskHandler{server: srv}

	reqBody := map[string]any{
		"pipelineId": uuid.New().String(), // does not exist
		"input":      map[string]any{},
		"tags":       []string{"etl"},
	}
	rec := httptest.NewRecorder()
	h.Submit(rec, taskSubmitRequest(t, reqBody, sess))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for unknown pipeline, got %d", rec.Code)
	}

	// Response must be structured (JSON with an error field).
	var errResp errorResponse
	if err := json.NewDecoder(rec.Body).Decode(&errResp); err != nil {
		t.Errorf("expected structured JSON error response, decode failed: %v", err)
	}
	if errResp.Error == "" {
		t.Error("expected non-empty error field in 400 response")
	}
}

// TestSubmit_DefaultRetryConfig verifies acceptance criterion 5:
// POST /api/tasks without retry config creates task with default retry settings.
func TestSubmit_DefaultRetryConfig(t *testing.T) {
	tasks := newStubTaskRepo()
	pipelines := newStubPipelineRepo()
	producer := &stubProducer{}

	userID := uuid.New()
	pipeline := validPipeline(userID)
	pipelines.addPipeline(pipeline)

	sess := &models.Session{UserID: userID, Role: models.RoleUser, CreatedAt: time.Now()}
	srv := newTaskTestServer(tasks, pipelines, producer)
	h := &TaskHandler{server: srv}

	// No retryConfig field in the request body.
	reqBody := map[string]any{
		"pipelineId": pipeline.ID.String(),
		"input":      map[string]any{},
		"tags":       []string{"etl"},
	}
	rec := httptest.NewRecorder()
	h.Submit(rec, taskSubmitRequest(t, reqBody, sess))

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp submitResponse
	_ = json.NewDecoder(rec.Body).Decode(&resp)

	task, _ := tasks.GetByID(context.Background(), resp.TaskID)
	if task == nil {
		t.Fatal("task not found after submit")
	}

	defaults := models.DefaultRetryConfig()
	if task.RetryConfig.MaxRetries != defaults.MaxRetries {
		t.Errorf("expected MaxRetries=%d, got %d", defaults.MaxRetries, task.RetryConfig.MaxRetries)
	}
	if task.RetryConfig.Backoff != defaults.Backoff {
		t.Errorf("expected Backoff=%q, got %q", defaults.Backoff, task.RetryConfig.Backoff)
	}
}

// TestSubmit_UnauthenticatedReturns401 verifies acceptance criterion 6:
// Unauthenticated request returns 401.
func TestSubmit_UnauthenticatedReturns401(t *testing.T) {
	tasks := newStubTaskRepo()
	pipelines := newStubPipelineRepo()
	producer := &stubProducer{}

	srv := newTaskTestServer(tasks, pipelines, producer)
	h := &TaskHandler{server: srv}

	reqBody := map[string]any{
		"pipelineId": uuid.New().String(),
		"input":      map[string]any{},
		"tags":       []string{"etl"},
	}
	// No session injected — simulates missing auth middleware.
	rec := httptest.NewRecorder()
	h.Submit(rec, taskSubmitRequest(t, reqBody, nil))

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for unauthenticated request, got %d", rec.Code)
	}
}

// TestSubmit_MalformedBodyReturns400 verifies that non-JSON bodies are rejected early.
func TestSubmit_MalformedBodyReturns400(t *testing.T) {
	tasks := newStubTaskRepo()
	pipelines := newStubPipelineRepo()
	producer := &stubProducer{}

	userID := uuid.New()
	sess := &models.Session{UserID: userID, Role: models.RoleUser, CreatedAt: time.Now()}
	srv := newTaskTestServer(tasks, pipelines, producer)
	h := &TaskHandler{server: srv}

	// Build an authenticated request with a malformed body via the middleware path.
	store := newStubSessionStore()
	token := "test-token-malformed"
	_ = store.Create(context.Background(), token, sess)
	req := httptest.NewRequest(http.MethodPost, "/api/tasks", bytes.NewBufferString("not json"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	var captured *http.Request
	inner := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { captured = r })
	auth.Middleware(store)(inner).ServeHTTP(httptest.NewRecorder(), req)
	if captured == nil {
		t.Fatal("auth middleware did not call inner handler")
	}

	rec := httptest.NewRecorder()
	h.Submit(rec, captured)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for malformed body, got %d", rec.Code)
	}
}

// TestSubmit_MissingPipelineIDReturns400 verifies that omitting pipelineId returns 400.
func TestSubmit_MissingPipelineIDReturns400(t *testing.T) {
	tasks := newStubTaskRepo()
	pipelines := newStubPipelineRepo()
	producer := &stubProducer{}

	userID := uuid.New()
	sess := &models.Session{UserID: userID, Role: models.RoleUser, CreatedAt: time.Now()}
	srv := newTaskTestServer(tasks, pipelines, producer)
	h := &TaskHandler{server: srv}

	reqBody := map[string]any{
		// pipelineId intentionally omitted
		"input": map[string]any{},
		"tags":  []string{"etl"},
	}
	rec := httptest.NewRecorder()
	h.Submit(rec, taskSubmitRequest(t, reqBody, sess))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing pipelineId, got %d", rec.Code)
	}
}

// TestSubmit_MissingTagsReturns400 verifies that an empty tags list is rejected.
func TestSubmit_MissingTagsReturns400(t *testing.T) {
	tasks := newStubTaskRepo()
	pipelines := newStubPipelineRepo()
	producer := &stubProducer{}

	userID := uuid.New()
	pipeline := validPipeline(userID)
	pipelines.addPipeline(pipeline)

	sess := &models.Session{UserID: userID, Role: models.RoleUser, CreatedAt: time.Now()}
	srv := newTaskTestServer(tasks, pipelines, producer)
	h := &TaskHandler{server: srv}

	reqBody := map[string]any{
		"pipelineId": pipeline.ID.String(),
		"input":      map[string]any{},
		"tags":       []string{}, // empty
	}
	rec := httptest.NewRecorder()
	h.Submit(rec, taskSubmitRequest(t, reqBody, sess))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty tags, got %d", rec.Code)
	}
}

// TestSubmit_ExplicitRetryConfigIsPreserved verifies that a caller-supplied retry config
// overrides the default when provided.
func TestSubmit_ExplicitRetryConfigIsPreserved(t *testing.T) {
	tasks := newStubTaskRepo()
	pipelines := newStubPipelineRepo()
	producer := &stubProducer{}

	userID := uuid.New()
	pipeline := validPipeline(userID)
	pipelines.addPipeline(pipeline)

	sess := &models.Session{UserID: userID, Role: models.RoleUser, CreatedAt: time.Now()}
	srv := newTaskTestServer(tasks, pipelines, producer)
	h := &TaskHandler{server: srv}

	reqBody := map[string]any{
		"pipelineId":  pipeline.ID.String(),
		"input":       map[string]any{},
		"tags":        []string{"etl"},
		"retryConfig": map[string]any{"maxRetries": 5, "backoff": "linear"},
	}
	rec := httptest.NewRecorder()
	h.Submit(rec, taskSubmitRequest(t, reqBody, sess))

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp submitResponse
	_ = json.NewDecoder(rec.Body).Decode(&resp)

	task, _ := tasks.GetByID(context.Background(), resp.TaskID)
	if task == nil {
		t.Fatal("task not found after submit")
	}
	if task.RetryConfig.MaxRetries != 5 {
		t.Errorf("expected MaxRetries=5, got %d", task.RetryConfig.MaxRetries)
	}
	if task.RetryConfig.Backoff != models.BackoffLinear {
		t.Errorf("expected Backoff=%q, got %q", models.BackoffLinear, task.RetryConfig.Backoff)
	}
}

// TestSubmit_UserIDFromSessionIsAttachedToTask verifies the task is owned by the
// authenticated user (session.UserID, not a caller-supplied field).
func TestSubmit_UserIDFromSessionIsAttachedToTask(t *testing.T) {
	tasks := newStubTaskRepo()
	pipelines := newStubPipelineRepo()
	producer := &stubProducer{}

	userID := uuid.New()
	pipeline := validPipeline(userID)
	pipelines.addPipeline(pipeline)

	sess := &models.Session{UserID: userID, Role: models.RoleUser, CreatedAt: time.Now()}
	srv := newTaskTestServer(tasks, pipelines, producer)
	h := &TaskHandler{server: srv}

	reqBody := map[string]any{
		"pipelineId": pipeline.ID.String(),
		"input":      map[string]any{},
		"tags":       []string{"etl"},
	}
	rec := httptest.NewRecorder()
	h.Submit(rec, taskSubmitRequest(t, reqBody, sess))

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}

	var resp submitResponse
	_ = json.NewDecoder(rec.Body).Decode(&resp)

	task, _ := tasks.GetByID(context.Background(), resp.TaskID)
	if task == nil {
		t.Fatal("task not found after submit")
	}
	if task.UserID != userID {
		t.Errorf("expected UserID=%v, got %v", userID, task.UserID)
	}
}

// TestSubmit_InvalidPipelineIDFormatReturns400 verifies that a non-UUID pipelineId
// is rejected before any DB lookup.
func TestSubmit_InvalidPipelineIDFormatReturns400(t *testing.T) {
	tasks := newStubTaskRepo()
	pipelines := newStubPipelineRepo()
	producer := &stubProducer{}

	userID := uuid.New()
	sess := &models.Session{UserID: userID, Role: models.RoleUser, CreatedAt: time.Now()}
	srv := newTaskTestServer(tasks, pipelines, producer)
	h := &TaskHandler{server: srv}

	reqBody := map[string]any{
		"pipelineId": "not-a-valid-uuid",
		"input":      map[string]any{},
		"tags":       []string{"etl"},
	}
	rec := httptest.NewRecorder()
	h.Submit(rec, taskSubmitRequest(t, reqBody, sess))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid UUID, got %d", rec.Code)
	}
}

// orderingProducer is a queue.Producer spy that records the task's status in the
// TaskRepository at the moment Enqueue is called. This lets tests assert that
// UpdateStatus(queued) precedes Enqueue — the fix for OBS-023.
type orderingProducer struct {
	tasks          *stubTaskRepo
	statusAtEnqueue models.TaskStatus
	enqueued        []*queue.ProducerMessage
}

func (p *orderingProducer) Enqueue(_ context.Context, msg *queue.ProducerMessage) ([]string, error) {
	// Snapshot the task's current status from the repository at the instant of enqueue.
	if t, ok := p.tasks.tasks[msg.Task.ID]; ok {
		p.statusAtEnqueue = t.Status
	}
	p.enqueued = append(p.enqueued, msg)
	return []string{"0-1"}, nil
}

func (p *orderingProducer) EnqueueDeadLetter(_ context.Context, _ string, _ string) error {
	return nil
}

// Compile-time assertion that orderingProducer satisfies queue.Producer.
var _ queue.Producer = (*orderingProducer)(nil)

// TestSubmit_StatusQueuedBeforeEnqueue verifies OBS-023 is fixed:
// UpdateStatus(queued) must complete before Enqueue is called so that a fast
// worker picking up the task from the Redis stream finds it already in "queued"
// state and can make the queued→assigned transition without error.
func TestSubmit_StatusQueuedBeforeEnqueue(t *testing.T) {
	tasks := newStubTaskRepo()
	pipelines := newStubPipelineRepo()

	// The spy producer reads back the task status from the repo at enqueue time.
	spy := &orderingProducer{tasks: tasks}

	userID := uuid.New()
	pipeline := validPipeline(userID)
	pipelines.addPipeline(pipeline)

	sess := &models.Session{UserID: userID, Role: models.RoleUser, CreatedAt: time.Now()}
	srv := &Server{
		tasks:     tasks,
		pipelines: pipelines,
		producer:  spy,
	}
	h := &TaskHandler{server: srv}

	reqBody := map[string]any{
		"pipelineId": pipeline.ID.String(),
		"input":      map[string]any{},
		"tags":       []string{"etl"},
	}
	rec := httptest.NewRecorder()
	h.Submit(rec, taskSubmitRequest(t, reqBody, sess))

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	if len(spy.enqueued) == 0 {
		t.Fatal("expected producer.Enqueue to be called")
	}

	// The status captured inside Enqueue must already be "queued".
	// Before the OBS-023 fix it would be "submitted".
	if spy.statusAtEnqueue != models.TaskStatusQueued {
		t.Errorf("expected task status %q at enqueue time, got %q — OBS-023 not fixed",
			models.TaskStatusQueued, spy.statusAtEnqueue)
	}
}

// Compile-time assertion that stubTaskRepo satisfies db.TaskRepository.
var _ db.TaskRepository = (*stubTaskRepo)(nil)

// Compile-time assertion that stubPipelineRepo satisfies db.PipelineRepository.
var _ db.PipelineRepository = (*stubPipelineRepo)(nil)

// Compile-time assertion that stubProducer satisfies queue.Producer.
var _ queue.Producer = (*stubProducer)(nil)
