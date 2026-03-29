// Package monitor — unit tests for the Monitor service.
// Uses in-process fakes for all dependencies to keep tests hermetic.
// No Redis or PostgreSQL required.
//
// See: ADR-002, TASK-009
package monitor

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nxlabs/nexusflow/internal/config"
	"github.com/nxlabs/nexusflow/internal/models"
	"github.com/nxlabs/nexusflow/internal/queue"
)

// --- fakes ---

// fakeWorkerRepository implements db.WorkerRepository for testing.
type fakeWorkerRepository struct {
	mu      sync.Mutex
	workers map[string]*models.Worker
	updates []workerStatusUpdate
}

type workerStatusUpdate struct {
	id     string
	status models.WorkerStatus
}

func newFakeWorkerRepository() *fakeWorkerRepository {
	return &fakeWorkerRepository{
		workers: make(map[string]*models.Worker),
	}
}

func (r *fakeWorkerRepository) Register(_ context.Context, w *models.Worker) (*models.Worker, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.workers[w.ID] = w
	return w, nil
}

func (r *fakeWorkerRepository) GetByID(_ context.Context, id string) (*models.Worker, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	w, ok := r.workers[id]
	if !ok {
		return nil, nil
	}
	return w, nil
}

func (r *fakeWorkerRepository) List(_ context.Context) ([]*models.Worker, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*models.Worker, 0, len(r.workers))
	for _, w := range r.workers {
		out = append(out, w)
	}
	return out, nil
}

func (r *fakeWorkerRepository) UpdateStatus(_ context.Context, id string, status models.WorkerStatus) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.updates = append(r.updates, workerStatusUpdate{id: id, status: status})
	if w, ok := r.workers[id]; ok {
		w.Status = status
	}
	return nil
}

// fakeTaskRepository implements db.TaskRepository for testing.
type fakeTaskRepository struct {
	mu           sync.Mutex
	tasks        map[uuid.UUID]*models.Task
	statusUpdates []taskStatusUpdate
	retryCounts  map[uuid.UUID]int
	retryAfters  map[uuid.UUID]*time.Time
}

type taskStatusUpdate struct {
	id     uuid.UUID
	status models.TaskStatus
	reason string
}

func newFakeTaskRepository() *fakeTaskRepository {
	return &fakeTaskRepository{
		tasks:       make(map[uuid.UUID]*models.Task),
		retryCounts: make(map[uuid.UUID]int),
		retryAfters: make(map[uuid.UUID]*time.Time),
	}
}

func (r *fakeTaskRepository) Create(_ context.Context, task *models.Task) (*models.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tasks[task.ID] = task
	return task, nil
}

func (r *fakeTaskRepository) GetByID(_ context.Context, id uuid.UUID) (*models.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.tasks[id]
	if !ok {
		return nil, nil
	}
	return t, nil
}

func (r *fakeTaskRepository) ListByUser(_ context.Context, _ uuid.UUID) ([]*models.Task, error) {
	return nil, nil
}

func (r *fakeTaskRepository) List(_ context.Context) ([]*models.Task, error) {
	return nil, nil
}

func (r *fakeTaskRepository) UpdateStatus(_ context.Context, id uuid.UUID, newStatus models.TaskStatus, reason string, _ *string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.statusUpdates = append(r.statusUpdates, taskStatusUpdate{id: id, status: newStatus, reason: reason})
	if t, ok := r.tasks[id]; ok {
		t.Status = newStatus
	}
	return nil
}

func (r *fakeTaskRepository) IncrementRetryCount(_ context.Context, id uuid.UUID) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.retryCounts[id]++
	if t, ok := r.tasks[id]; ok {
		t.RetryCount = r.retryCounts[id]
	}
	return r.retryCounts[id], nil
}

func (r *fakeTaskRepository) SetRetryAfterAndTags(_ context.Context, id uuid.UUID, retryAfter *time.Time, retryTags []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.retryAfters[id] = retryAfter
	if t, ok := r.tasks[id]; ok {
		t.RetryAfter = retryAfter
		t.RetryTags = retryTags
	}
	return nil
}

func (r *fakeTaskRepository) ListRetryReady(_ context.Context) ([]*models.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	var ready []*models.Task
	for _, t := range r.tasks {
		if t.Status == models.TaskStatusQueued && t.RetryAfter != nil && !t.RetryAfter.After(now) {
			ready = append(ready, t)
		}
	}
	return ready, nil
}

func (r *fakeTaskRepository) Cancel(_ context.Context, id uuid.UUID, _ string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if t, ok := r.tasks[id]; ok {
		t.Status = models.TaskStatusCancelled
	}
	return nil
}

func (r *fakeTaskRepository) GetStateLog(_ context.Context, _ uuid.UUID) ([]*models.TaskStateLog, error) {
	return nil, nil
}

func (r *fakeTaskRepository) ListByPipelineAndStatuses(_ context.Context, pipelineID uuid.UUID, statuses []models.TaskStatus) ([]*models.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	statusSet := make(map[models.TaskStatus]struct{}, len(statuses))
	for _, s := range statuses {
		statusSet[s] = struct{}{}
	}
	var out []*models.Task
	for _, t := range r.tasks {
		if t.PipelineID != nil && *t.PipelineID == pipelineID {
			if _, ok := statusSet[t.Status]; ok {
				out = append(out, t)
			}
		}
	}
	return out, nil
}

// fakeHeartbeatStore implements queue.HeartbeatStore for testing.
type fakeHeartbeatStore struct {
	mu      sync.Mutex
	active  map[string]time.Time // workerID -> last heartbeat time
	removed []string
}

func newFakeHeartbeatStore() *fakeHeartbeatStore {
	return &fakeHeartbeatStore{active: make(map[string]time.Time)}
}

func (s *fakeHeartbeatStore) RecordHeartbeat(_ context.Context, workerID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.active[workerID] = time.Now()
	return nil
}

func (s *fakeHeartbeatStore) ListExpired(_ context.Context, cutoff time.Time) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var expired []string
	for id, ts := range s.active {
		if ts.Before(cutoff) {
			expired = append(expired, id)
		}
	}
	return expired, nil
}

func (s *fakeHeartbeatStore) Remove(_ context.Context, workerID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.active, workerID)
	s.removed = append(s.removed, workerID)
	return nil
}

// fakePendingScanner implements queue.PendingScanner for testing.
type fakePendingScanner struct {
	mu           sync.Mutex
	pending      map[string][]*queue.PendingEntry // tag -> entries
	claimed      []claimCall
	acknowledged []ackCall
	claimErr     error
}

type claimCall struct {
	tag      string
	streamID string
	consumer string
}

type ackCall struct {
	tag      string
	streamID string
}

func newFakePendingScanner() *fakePendingScanner {
	return &fakePendingScanner{
		pending: make(map[string][]*queue.PendingEntry),
	}
}

func (s *fakePendingScanner) addPending(tag string, entry *queue.PendingEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending[tag] = append(s.pending[tag], entry)
}

func (s *fakePendingScanner) ListPendingOlderThan(_ context.Context, tag string, _ time.Duration) ([]*queue.PendingEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pending[tag], nil
}

func (s *fakePendingScanner) Claim(_ context.Context, tag string, streamID string, newConsumerID string, _ time.Duration) error {
	if s.claimErr != nil {
		return s.claimErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.claimed = append(s.claimed, claimCall{tag: tag, streamID: streamID, consumer: newConsumerID})
	return nil
}

func (s *fakePendingScanner) AcknowledgePending(_ context.Context, tag string, streamID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.acknowledged = append(s.acknowledged, ackCall{tag: tag, streamID: streamID})
	return nil
}

// fakeProducer implements queue.Producer for testing.
type fakeProducer struct {
	mu             sync.Mutex
	enqueued       []*queue.ProducerMessage
	deadLettered   []deadLetterCall
}

type deadLetterCall struct {
	taskID string
	reason string
}

func newFakeProducer() *fakeProducer { return &fakeProducer{} }

func (p *fakeProducer) Enqueue(_ context.Context, msg *queue.ProducerMessage) ([]string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.enqueued = append(p.enqueued, msg)
	return []string{"test-stream-id"}, nil
}

func (p *fakeProducer) EnqueueDeadLetter(_ context.Context, taskID string, reason string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.deadLettered = append(p.deadLettered, deadLetterCall{taskID: taskID, reason: reason})
	return nil
}

// fakeBroker implements sse.Broker for testing (only the methods the Monitor uses).
// The full sse.Broker interface has many HTTP methods; we use a minimal stub.
type fakeBroker struct {
	mu              sync.Mutex
	workerEvents    []*models.Worker
	taskEvents      []*models.Task
	publishErr      error
}

func newFakeBroker() *fakeBroker { return &fakeBroker{} }

// Only the Broker methods the Monitor calls are implemented.
// The rest satisfy the interface via noop stubs.

func (b *fakeBroker) PublishWorkerEvent(_ context.Context, w *models.Worker) error {
	if b.publishErr != nil {
		return b.publishErr
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.workerEvents = append(b.workerEvents, w)
	return nil
}

func (b *fakeBroker) PublishTaskEvent(_ context.Context, t *models.Task, _ string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.taskEvents = append(b.taskEvents, t)
	return nil
}

// Remaining sse.Broker methods not used by the Monitor — noop stubs.

func (b *fakeBroker) Start(_ context.Context) error { return nil }

func (b *fakeBroker) ServeTaskEvents(_ http.ResponseWriter, _ *http.Request, _ *models.Session) {}

func (b *fakeBroker) ServeWorkerEvents(_ http.ResponseWriter, _ *http.Request, _ *models.Session) {}

func (b *fakeBroker) ServeLogEvents(_ http.ResponseWriter, _ *http.Request, _ *models.Session, _ string) {
}

func (b *fakeBroker) ServeSinkEvents(_ http.ResponseWriter, _ *http.Request, _ *models.Session, _ string) {
}

func (b *fakeBroker) PublishLogLine(_ context.Context, _ *models.TaskLog) error { return nil }

func (b *fakeBroker) PublishSinkSnapshot(_ context.Context, _ *models.SinkSnapshot) error {
	return nil
}

// fakeChainRepository implements db.ChainRepository for testing cascading cancellation.
type fakeChainRepository struct {
	mu     sync.Mutex
	chains map[uuid.UUID]*models.Chain // chain ID -> chain
	// byPipeline maps pipeline ID -> chain (for FindByPipeline)
	byPipeline map[uuid.UUID]*models.Chain
}

func newFakeChainRepository() *fakeChainRepository {
	return &fakeChainRepository{
		chains:     make(map[uuid.UUID]*models.Chain),
		byPipeline: make(map[uuid.UUID]*models.Chain),
	}
}

// addChain registers a chain and indexes all its pipelines for FindByPipeline lookups.
func (r *fakeChainRepository) addChain(chain *models.Chain) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.chains[chain.ID] = chain
	for _, pid := range chain.PipelineIDs {
		r.byPipeline[pid] = chain
	}
}

func (r *fakeChainRepository) Create(_ context.Context, chain *models.Chain) (*models.Chain, error) {
	r.addChain(chain)
	return chain, nil
}

func (r *fakeChainRepository) GetByID(_ context.Context, id uuid.UUID) (*models.Chain, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.chains[id]
	if !ok {
		return nil, nil
	}
	return c, nil
}

func (r *fakeChainRepository) FindByPipeline(_ context.Context, pipelineID uuid.UUID) (*models.Chain, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.byPipeline[pipelineID]
	if !ok {
		return nil, nil
	}
	return c, nil
}

func (r *fakeChainRepository) GetNextPipeline(_ context.Context, chainID uuid.UUID, pipelineID uuid.UUID) (*uuid.UUID, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	chain, ok := r.chains[chainID]
	if !ok {
		return nil, nil
	}
	for i, pid := range chain.PipelineIDs {
		if pid == pipelineID {
			if i+1 < len(chain.PipelineIDs) {
				next := chain.PipelineIDs[i+1]
				return &next, nil
			}
			return nil, nil // last step
		}
	}
	return nil, nil
}

// --- helpers ---

// testCfg returns a minimal config suitable for unit tests.
func testCfg() *config.Config {
	return &config.Config{
		HeartbeatTimeout:    15 * time.Second,
		PendingScanInterval: 10 * time.Second,
	}
}

// newTask returns a minimal models.Task with the given retry config.
func newTask(id uuid.UUID, retryConfig models.RetryConfig) *models.Task {
	return &models.Task{
		ID:          id,
		Status:      models.TaskStatusRunning,
		RetryConfig: retryConfig,
		RetryCount:  0,
	}
}

// --- tests ---

// TestNewMonitor_NonNilDependencies verifies that NewMonitor stores all injected
// dependencies on the returned Monitor without panicking on valid input.
func TestNewMonitor_NonNilDependencies(t *testing.T) {
	cfg := testCfg()
	workers := newFakeWorkerRepository()
	tasks := newFakeTaskRepository()
	heartbeat := newFakeHeartbeatStore()
	scanner := newFakePendingScanner()
	producer := newFakeProducer()
	broker := newFakeBroker()

	m := NewMonitor(cfg, workers, tasks, nil, heartbeat, scanner, producer, broker)

	if m == nil {
		t.Fatal("NewMonitor: returned nil")
	}
	if m.cfg != cfg {
		t.Error("NewMonitor: cfg not stored")
	}
	if m.workers == nil {
		t.Error("NewMonitor: workers not stored")
	}
	if m.tasks == nil {
		t.Error("NewMonitor: tasks not stored")
	}
}

// TestCheckHeartbeats_MarksExpiredWorkerDown verifies that a worker whose last
// heartbeat is older than HeartbeatTimeout is marked "down" in the repository.
//
// Red criteria: worker status must be updated to "down" after checkHeartbeats.
func TestCheckHeartbeats_MarksExpiredWorkerDown(t *testing.T) {
	cfg := testCfg()
	workers := newFakeWorkerRepository()
	tasks := newFakeTaskRepository()
	heartbeat := newFakeHeartbeatStore()
	scanner := newFakePendingScanner()
	producer := newFakeProducer()
	broker := newFakeBroker()

	// Register a worker that last heartbeated 30 seconds ago (beyond the 15s timeout).
	staleWorkerID := "worker-stale-1"
	staleTime := time.Now().Add(-30 * time.Second)
	heartbeat.active[staleWorkerID] = staleTime
	workers.workers[staleWorkerID] = &models.Worker{
		ID:     staleWorkerID,
		Status: models.WorkerStatusOnline,
		Tags:   []string{"demo"},
	}

	m := NewMonitor(cfg, workers, tasks, nil, heartbeat, scanner, producer, broker)

	ctx := context.Background()
	if err := m.checkHeartbeats(ctx); err != nil {
		t.Fatalf("checkHeartbeats: unexpected error: %v", err)
	}

	// The stale worker must be marked down.
	if len(workers.updates) == 0 {
		t.Fatal("checkHeartbeats: no WorkerRepository.UpdateStatus call recorded")
	}
	update := workers.updates[0]
	if update.id != staleWorkerID {
		t.Errorf("checkHeartbeats: UpdateStatus called with id=%q, want %q", update.id, staleWorkerID)
	}
	if update.status != models.WorkerStatusDown {
		t.Errorf("checkHeartbeats: UpdateStatus called with status=%q, want %q", update.status, models.WorkerStatusDown)
	}
}

// TestCheckHeartbeats_RemovesExpiredWorkerFromHeartbeatStore verifies that the
// expired worker is removed from workers:active after being marked down.
func TestCheckHeartbeats_RemovesExpiredWorkerFromHeartbeatStore(t *testing.T) {
	cfg := testCfg()
	workers := newFakeWorkerRepository()
	tasks := newFakeTaskRepository()
	heartbeat := newFakeHeartbeatStore()
	scanner := newFakePendingScanner()
	producer := newFakeProducer()
	broker := newFakeBroker()

	staleWorkerID := "worker-stale-2"
	heartbeat.active[staleWorkerID] = time.Now().Add(-30 * time.Second)
	workers.workers[staleWorkerID] = &models.Worker{
		ID:     staleWorkerID,
		Status: models.WorkerStatusOnline,
		Tags:   []string{"demo"},
	}

	m := NewMonitor(cfg, workers, tasks, nil, heartbeat, scanner, producer, broker)

	if err := m.checkHeartbeats(context.Background()); err != nil {
		t.Fatalf("checkHeartbeats: %v", err)
	}

	// The worker must have been removed from the heartbeat store.
	if len(heartbeat.removed) == 0 {
		t.Fatal("checkHeartbeats: Remove not called on HeartbeatStore")
	}
	if heartbeat.removed[0] != staleWorkerID {
		t.Errorf("checkHeartbeats: removed %q, want %q", heartbeat.removed[0], staleWorkerID)
	}
}

// TestCheckHeartbeats_PublishesWorkerDownEvent verifies that a worker:down event
// is published to the SSE broker after the worker is marked down.
func TestCheckHeartbeats_PublishesWorkerDownEvent(t *testing.T) {
	cfg := testCfg()
	workers := newFakeWorkerRepository()
	tasks := newFakeTaskRepository()
	heartbeat := newFakeHeartbeatStore()
	scanner := newFakePendingScanner()
	producer := newFakeProducer()
	broker := newFakeBroker()

	staleWorkerID := "worker-stale-3"
	heartbeat.active[staleWorkerID] = time.Now().Add(-30 * time.Second)
	workers.workers[staleWorkerID] = &models.Worker{
		ID:     staleWorkerID,
		Status: models.WorkerStatusOnline,
		Tags:   []string{"demo"},
	}

	m := NewMonitor(cfg, workers, tasks, nil, heartbeat, scanner, producer, broker)

	if err := m.checkHeartbeats(context.Background()); err != nil {
		t.Fatalf("checkHeartbeats: %v", err)
	}

	if len(broker.workerEvents) == 0 {
		t.Fatal("checkHeartbeats: no worker event published to broker")
	}
	evt := broker.workerEvents[0]
	if evt.Status != models.WorkerStatusDown {
		t.Errorf("checkHeartbeats: published event status=%q, want %q", evt.Status, models.WorkerStatusDown)
	}
}

// TestCheckHeartbeats_HealthyWorkerIgnored verifies that a worker whose heartbeat
// is recent is NOT marked down.
func TestCheckHeartbeats_HealthyWorkerIgnored(t *testing.T) {
	cfg := testCfg()
	workers := newFakeWorkerRepository()
	tasks := newFakeTaskRepository()
	heartbeat := newFakeHeartbeatStore()
	scanner := newFakePendingScanner()
	producer := newFakeProducer()
	broker := newFakeBroker()

	healthyWorkerID := "worker-healthy-1"
	heartbeat.active[healthyWorkerID] = time.Now() // fresh heartbeat

	m := NewMonitor(cfg, workers, tasks, nil, heartbeat, scanner, producer, broker)

	if err := m.checkHeartbeats(context.Background()); err != nil {
		t.Fatalf("checkHeartbeats: %v", err)
	}

	if len(workers.updates) != 0 {
		t.Errorf("checkHeartbeats: UpdateStatus called %d times for healthy worker, want 0", len(workers.updates))
	}
}

// TestReclaimTask_IncrementsRetryCount verifies that reclaimTask increments the
// task's retry_count in the repository.
func TestReclaimTask_IncrementsRetryCount(t *testing.T) {
	cfg := testCfg()
	workers := newFakeWorkerRepository()
	tasks := newFakeTaskRepository()
	heartbeat := newFakeHeartbeatStore()
	scanner := newFakePendingScanner()
	producer := newFakeProducer()
	broker := newFakeBroker()

	taskID := uuid.New()
	tasks.tasks[taskID] = newTask(taskID, models.RetryConfig{MaxRetries: 3, Backoff: models.BackoffExponential})

	entry := &queue.PendingEntry{
		StreamID:   "1-1",
		ConsumerID: "worker-dead",
		IdleTime:   30 * time.Second,
		TaskID:     taskID.String(),
	}

	m := NewMonitor(cfg, workers, tasks, nil, heartbeat, scanner, producer, broker)

	if err := m.reclaimTask(context.Background(), entry, "demo"); err != nil {
		t.Fatalf("reclaimTask: %v", err)
	}

	if tasks.retryCounts[taskID] != 1 {
		t.Errorf("reclaimTask: retry_count=%d, want 1", tasks.retryCounts[taskID])
	}
}

// TestReclaimTask_TransitionsTaskToQueued verifies that reclaimTask sets the task
// status back to "queued" so a healthy worker can pick it up.
func TestReclaimTask_TransitionsTaskToQueued(t *testing.T) {
	cfg := testCfg()
	workers := newFakeWorkerRepository()
	tasks := newFakeTaskRepository()
	heartbeat := newFakeHeartbeatStore()
	scanner := newFakePendingScanner()
	producer := newFakeProducer()
	broker := newFakeBroker()

	taskID := uuid.New()
	tasks.tasks[taskID] = newTask(taskID, models.RetryConfig{MaxRetries: 3})

	entry := &queue.PendingEntry{
		StreamID:   "2-1",
		ConsumerID: "worker-dead",
		IdleTime:   30 * time.Second,
		TaskID:     taskID.String(),
	}

	m := NewMonitor(cfg, workers, tasks, nil, heartbeat, scanner, producer, broker)

	if err := m.reclaimTask(context.Background(), entry, "demo"); err != nil {
		t.Fatalf("reclaimTask: %v", err)
	}

	found := false
	for _, u := range tasks.statusUpdates {
		if u.id == taskID && u.status == models.TaskStatusQueued {
			found = true
			break
		}
	}
	if !found {
		t.Error("reclaimTask: task status not updated to 'queued'")
	}
}

// TestReclaimTask_ClaimsPendingEntry verifies that reclaimTask calls Claim on the
// PendingScanner to transfer the stream entry to the monitor consumer.
func TestReclaimTask_ClaimsPendingEntry(t *testing.T) {
	cfg := testCfg()
	workers := newFakeWorkerRepository()
	tasks := newFakeTaskRepository()
	heartbeat := newFakeHeartbeatStore()
	scanner := newFakePendingScanner()
	producer := newFakeProducer()
	broker := newFakeBroker()

	taskID := uuid.New()
	tasks.tasks[taskID] = newTask(taskID, models.RetryConfig{MaxRetries: 3})

	entry := &queue.PendingEntry{
		StreamID:   "3-1",
		ConsumerID: "worker-dead",
		IdleTime:   30 * time.Second,
		TaskID:     taskID.String(),
	}

	m := NewMonitor(cfg, workers, tasks, nil, heartbeat, scanner, producer, broker)

	if err := m.reclaimTask(context.Background(), entry, "demo"); err != nil {
		t.Fatalf("reclaimTask: %v", err)
	}

	if len(scanner.claimed) == 0 {
		t.Fatal("reclaimTask: Claim was not called on PendingScanner")
	}
	c := scanner.claimed[0]
	if c.streamID != entry.StreamID {
		t.Errorf("reclaimTask: Claim called with streamID=%q, want %q", c.streamID, entry.StreamID)
	}
	if c.tag != "demo" {
		t.Errorf("reclaimTask: Claim called with tag=%q, want %q", c.tag, "demo")
	}
}

// TestDeadLetterTask_ExhaustedRetries verifies that deadLetterTask routes the task
// to queue:dead-letter and marks it "failed" when retries are exhausted.
func TestDeadLetterTask_ExhaustedRetries(t *testing.T) {
	cfg := testCfg()
	workers := newFakeWorkerRepository()
	tasks := newFakeTaskRepository()
	heartbeat := newFakeHeartbeatStore()
	scanner := newFakePendingScanner()
	producer := newFakeProducer()
	broker := newFakeBroker()

	taskID := uuid.New()
	tasks.tasks[taskID] = &models.Task{
		ID:          taskID,
		Status:      models.TaskStatusRunning,
		RetryConfig: models.RetryConfig{MaxRetries: 3},
		RetryCount:  3, // already exhausted
	}

	entry := &queue.PendingEntry{
		StreamID:   "4-1",
		ConsumerID: "worker-dead",
		IdleTime:   30 * time.Second,
		TaskID:     taskID.String(),
	}

	m := NewMonitor(cfg, workers, tasks, nil, heartbeat, scanner, producer, broker)

	if err := m.deadLetterTask(context.Background(), entry, "demo"); err != nil {
		t.Fatalf("deadLetterTask: %v", err)
	}

	// Must enqueue to dead letter.
	if len(producer.deadLettered) == 0 {
		t.Fatal("deadLetterTask: EnqueueDeadLetter not called")
	}
	if producer.deadLettered[0].taskID != taskID.String() {
		t.Errorf("deadLetterTask: EnqueueDeadLetter taskID=%q, want %q", producer.deadLettered[0].taskID, taskID.String())
	}

	// Must mark task failed.
	found := false
	for _, u := range tasks.statusUpdates {
		if u.id == taskID && u.status == models.TaskStatusFailed {
			found = true
			break
		}
	}
	if !found {
		t.Error("deadLetterTask: task status not updated to 'failed'")
	}
}

// TestScanPendingEntries_ReclaimsAndDeadLetters verifies the full scan loop:
// tasks within retry limit are reclaimed; tasks beyond retry limit go to dead letter.
func TestScanPendingEntries_ReclaimsAndDeadLetters(t *testing.T) {
	cfg := testCfg()
	workers := newFakeWorkerRepository()
	tasks := newFakeTaskRepository()
	heartbeat := newFakeHeartbeatStore()
	scanner := newFakePendingScanner()
	producer := newFakeProducer()
	broker := newFakeBroker()

	// Register workers with tags so scanPendingEntries knows which streams to check.
	workers.workers["worker-a"] = &models.Worker{
		ID:   "worker-a",
		Tags: []string{"demo"},
	}

	// Task 1: has retries remaining — should be reclaimed.
	taskID1 := uuid.New()
	tasks.tasks[taskID1] = &models.Task{
		ID:          taskID1,
		Status:      models.TaskStatusRunning,
		RetryConfig: models.RetryConfig{MaxRetries: 3},
		RetryCount:  1,
	}

	// Task 2: retries exhausted — should go to dead letter.
	taskID2 := uuid.New()
	tasks.tasks[taskID2] = &models.Task{
		ID:          taskID2,
		Status:      models.TaskStatusRunning,
		RetryConfig: models.RetryConfig{MaxRetries: 3},
		RetryCount:  3,
	}

	scanner.addPending("demo", &queue.PendingEntry{
		StreamID: "5-1", ConsumerID: "worker-a",
		IdleTime: 30 * time.Second, TaskID: taskID1.String(),
	})
	scanner.addPending("demo", &queue.PendingEntry{
		StreamID: "5-2", ConsumerID: "worker-a",
		IdleTime: 30 * time.Second, TaskID: taskID2.String(),
	})

	m := NewMonitor(cfg, workers, tasks, nil, heartbeat, scanner, producer, broker)

	if err := m.scanPendingEntries(context.Background()); err != nil {
		t.Fatalf("scanPendingEntries: %v", err)
	}

	// Task 1 must have been claimed and re-queued.
	if len(scanner.claimed) == 0 {
		t.Error("scanPendingEntries: no pending entries claimed for retry task")
	}

	// Task 2 must have been dead-lettered.
	if len(producer.deadLettered) == 0 {
		t.Error("scanPendingEntries: no dead-letter enqueue for exhausted task")
	}
	if producer.deadLettered[0].taskID != taskID2.String() {
		t.Errorf("scanPendingEntries: dead-lettered taskID=%q, want %q", producer.deadLettered[0].taskID, taskID2.String())
	}
}

// TestReclaimTask_DeferredEnqueueViaRetryAfter verifies that reclaimTask schedules
// deferred re-enqueue by setting retry_after and retry_tags rather than calling
// Producer.Enqueue immediately. The ACK of the monitor's pending entry must still
// happen so the pending list stays clean.
//
// The actual re-XADD happens in scanRetryReady once retry_after elapses.
// This satisfies TASK-010: AC-2 (backoff delay) and the XCLAIM + deferred-enqueue pattern.
func TestReclaimTask_DeferredEnqueueViaRetryAfter(t *testing.T) {
	cfg := testCfg()
	workers := newFakeWorkerRepository()
	tasks := newFakeTaskRepository()
	heartbeat := newFakeHeartbeatStore()
	scanner := newFakePendingScanner()
	producer := newFakeProducer()
	broker := newFakeBroker()

	taskID := uuid.New()
	tasks.tasks[taskID] = newTask(taskID, models.RetryConfig{MaxRetries: 3})

	entry := &queue.PendingEntry{
		StreamID:   "6-1",
		ConsumerID: "worker-dead",
		IdleTime:   30 * time.Second,
		TaskID:     taskID.String(),
	}

	m := NewMonitor(cfg, workers, tasks, nil, heartbeat, scanner, producer, broker)

	if err := m.reclaimTask(context.Background(), entry, "demo"); err != nil {
		t.Fatalf("reclaimTask: %v", err)
	}

	// With deferred enqueue, Producer.Enqueue must NOT be called by reclaimTask.
	// Healthy workers pick up the task only after retry_after elapses (via scanRetryReady).
	if len(producer.enqueued) != 0 {
		t.Errorf("reclaimTask: Enqueue called immediately — backoff violated; want deferred enqueue")
	}

	// retry_after must be set so scanRetryReady can dispatch when the gate opens.
	if tasks.retryAfters[taskID] == nil {
		t.Error("reclaimTask: retry_after not set — scanRetryReady cannot dispatch the task")
	}

	// retry_tags must record the tag so scanRetryReady knows which stream to target.
	task, _ := tasks.GetByID(context.Background(), taskID)
	if task == nil || len(task.RetryTags) == 0 || task.RetryTags[0] != "demo" {
		t.Errorf("reclaimTask: retry_tags not set to [demo], got %v", func() []string {
			if task != nil {
				return task.RetryTags
			}
			return nil
		}())
	}

	// The monitor's pending entry must be ACKed to keep the pending list clean.
	if len(scanner.acknowledged) == 0 {
		t.Fatal("reclaimTask: AcknowledgePending not called — monitor pending list will accumulate")
	}
	ack := scanner.acknowledged[0]
	if ack.streamID != entry.StreamID {
		t.Errorf("reclaimTask: AcknowledgePending streamID=%q, want %q", ack.streamID, entry.StreamID)
	}
}

// TestRun_StopsOnContextCancel verifies that Run returns nil when the context
// is cancelled, without blocking indefinitely.
func TestRun_StopsOnContextCancel(t *testing.T) {
	cfg := &config.Config{
		HeartbeatTimeout:    15 * time.Second,
		PendingScanInterval: 100 * time.Millisecond, // fast for test
	}
	workers := newFakeWorkerRepository()
	tasks := newFakeTaskRepository()
	heartbeat := newFakeHeartbeatStore()
	scanner := newFakePendingScanner()
	producer := newFakeProducer()
	broker := newFakeBroker()

	m := NewMonitor(cfg, workers, tasks, nil, heartbeat, scanner, producer, broker)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- m.Run(ctx)
	}()

	// Cancel after a brief period.
	time.Sleep(150 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run: expected nil error on context cancellation, got: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("Run: did not stop within 2 seconds after context cancellation")
	}
}

// TestCheckHeartbeats_BrokerErrorIsNonFatal verifies that a broker publish error
// does not abort the heartbeat check — logging and continuing is the expected behaviour
// (fire-and-forget per ADR-007).
func TestCheckHeartbeats_BrokerErrorIsNonFatal(t *testing.T) {
	cfg := testCfg()
	workers := newFakeWorkerRepository()
	tasks := newFakeTaskRepository()
	heartbeat := newFakeHeartbeatStore()
	scanner := newFakePendingScanner()
	producer := newFakeProducer()
	broker := newFakeBroker()
	broker.publishErr = fmt.Errorf("redis: connection refused")

	staleWorkerID := "worker-stale-4"
	heartbeat.active[staleWorkerID] = time.Now().Add(-30 * time.Second)
	workers.workers[staleWorkerID] = &models.Worker{
		ID:     staleWorkerID,
		Status: models.WorkerStatusOnline,
		Tags:   []string{"demo"},
	}

	m := NewMonitor(cfg, workers, tasks, nil, heartbeat, scanner, producer, broker)

	// Broker publish fails, but checkHeartbeats must still return nil and
	// continue processing (fire-and-forget SSE events per ADR-007).
	err := m.checkHeartbeats(context.Background())
	if err != nil {
		t.Errorf("checkHeartbeats: expected nil on broker error, got: %v", err)
	}

	// Worker must still have been marked down despite the broker error.
	if len(workers.updates) == 0 {
		t.Error("checkHeartbeats: WorkerRepository.UpdateStatus not called when broker failed")
	}
}

// --- TASK-010 acceptance criterion tests ---

// TestReclaimTask_SetsRetryAfterWithExponentialBackoff verifies AC-2:
// "Backoff delay applied between retries (exponential: 1s, 2s, 4s)".
// After reclaimTask with retryCount=0, retry_after must be approximately now+1s.
func TestReclaimTask_SetsRetryAfterWithExponentialBackoff(t *testing.T) {
	cfg := testCfg()
	workers := newFakeWorkerRepository()
	tasks := newFakeTaskRepository()
	heartbeat := newFakeHeartbeatStore()
	scanner := newFakePendingScanner()
	producer := newFakeProducer()
	broker := newFakeBroker()

	taskID := uuid.New()
	tasks.tasks[taskID] = newTask(taskID, models.RetryConfig{MaxRetries: 3, Backoff: models.BackoffExponential})
	// RetryCount=0 → first retry → delay=1s

	entry := &queue.PendingEntry{
		StreamID:   "10-1",
		ConsumerID: "worker-dead",
		IdleTime:   30 * time.Second,
		TaskID:     taskID.String(),
	}

	before := time.Now()
	m := NewMonitor(cfg, workers, tasks, nil, heartbeat, scanner, producer, broker)
	if err := m.reclaimTask(context.Background(), entry, "demo"); err != nil {
		t.Fatalf("reclaimTask: %v", err)
	}

	// retry_after must have been set.
	ra := tasks.retryAfters[taskID]
	if ra == nil {
		t.Fatal("reclaimTask: retry_after not set — backoff delay not applied")
	}

	// For retryCount=0 with exponential strategy: delay = 1s.
	// retry_after must be approximately before+1s (within a 200ms window to allow for test execution time).
	expectedMin := before.Add(900 * time.Millisecond)
	expectedMax := before.Add(1100 * time.Millisecond)
	if ra.Before(expectedMin) || ra.After(expectedMax) {
		t.Errorf("reclaimTask: retry_after=%v, want approximately %v (±100ms)",
			*ra, before.Add(time.Second))
	}
}

// TestReclaimTask_SetsRetryAfterWithLinearBackoff verifies linear backoff:
// retryCount=1 → delay=2s.
func TestReclaimTask_SetsRetryAfterWithLinearBackoff(t *testing.T) {
	cfg := testCfg()
	workers := newFakeWorkerRepository()
	tasks := newFakeTaskRepository()
	heartbeat := newFakeHeartbeatStore()
	scanner := newFakePendingScanner()
	producer := newFakeProducer()
	broker := newFakeBroker()

	taskID := uuid.New()
	task := newTask(taskID, models.RetryConfig{MaxRetries: 3, Backoff: models.BackoffLinear})
	task.RetryCount = 1 // second retry → delay = 2s
	tasks.tasks[taskID] = task

	entry := &queue.PendingEntry{
		StreamID:   "11-1",
		ConsumerID: "worker-dead",
		IdleTime:   30 * time.Second,
		TaskID:     taskID.String(),
	}

	before := time.Now()
	m := NewMonitor(cfg, workers, tasks, nil, heartbeat, scanner, producer, broker)
	if err := m.reclaimTask(context.Background(), entry, "demo"); err != nil {
		t.Fatalf("reclaimTask: %v", err)
	}

	ra := tasks.retryAfters[taskID]
	if ra == nil {
		t.Fatal("reclaimTask: retry_after not set for linear backoff")
	}

	// linear retryCount=1: delay = 1s*(1+1) = 2s.
	expectedMin := before.Add(1900 * time.Millisecond)
	expectedMax := before.Add(2100 * time.Millisecond)
	if ra.Before(expectedMin) || ra.After(expectedMax) {
		t.Errorf("reclaimTask: retry_after=%v, want approximately %v (±100ms)",
			*ra, before.Add(2*time.Second))
	}
}

// TestReclaimTask_DoesNotImmediatelyReEnqueue verifies that reclaimTask does NOT
// re-enqueue the task to Redis when a backoff delay is set.
// The re-enqueue must be deferred until retry_after elapses (AC-2).
func TestReclaimTask_DoesNotImmediatelyReEnqueue(t *testing.T) {
	cfg := testCfg()
	workers := newFakeWorkerRepository()
	tasks := newFakeTaskRepository()
	heartbeat := newFakeHeartbeatStore()
	scanner := newFakePendingScanner()
	producer := newFakeProducer()
	broker := newFakeBroker()

	taskID := uuid.New()
	tasks.tasks[taskID] = newTask(taskID, models.RetryConfig{MaxRetries: 3, Backoff: models.BackoffExponential})

	entry := &queue.PendingEntry{
		StreamID:   "12-1",
		ConsumerID: "worker-dead",
		IdleTime:   30 * time.Second,
		TaskID:     taskID.String(),
	}

	m := NewMonitor(cfg, workers, tasks, nil, heartbeat, scanner, producer, broker)
	if err := m.reclaimTask(context.Background(), entry, "demo"); err != nil {
		t.Fatalf("reclaimTask: %v", err)
	}

	// Producer.Enqueue must NOT have been called — the task is gated by retry_after.
	if len(producer.enqueued) != 0 {
		t.Errorf("reclaimTask: Enqueue called %d time(s) before retry_after elapsed — backoff violated",
			len(producer.enqueued))
	}
}

// TestScanRetryReady_ReEnqueuesTasksWhoseRetryAfterHasElapsed verifies that
// scanRetryReady picks up tasks with elapsed retry_after and re-enqueues them.
func TestScanRetryReady_ReEnqueuesTasksWhoseRetryAfterHasElapsed(t *testing.T) {
	cfg := testCfg()
	workers := newFakeWorkerRepository()
	tasks := newFakeTaskRepository()
	heartbeat := newFakeHeartbeatStore()
	scanner := newFakePendingScanner()
	producer := newFakeProducer()
	broker := newFakeBroker()

	taskID := uuid.New()
	past := time.Now().Add(-500 * time.Millisecond) // retry_after in the past
	task := &models.Task{
		ID:          taskID,
		Status:      models.TaskStatusQueued,
		RetryConfig: models.RetryConfig{MaxRetries: 3, Backoff: models.BackoffExponential},
		RetryCount:  1,
		RetryAfter:  &past,
		RetryTags:   []string{"demo"}, // stored when reclaimTask set retry_after
		Input:       map[string]any{},
	}
	tasks.tasks[taskID] = task

	m := NewMonitor(cfg, workers, tasks, nil, heartbeat, scanner, producer, broker)

	if err := m.scanRetryReady(context.Background()); err != nil {
		t.Fatalf("scanRetryReady: %v", err)
	}

	// The task should have been re-enqueued.
	if len(producer.enqueued) == 0 {
		t.Fatal("scanRetryReady: task with elapsed retry_after was not re-enqueued")
	}
}

// TestProcessEntry_SkipsTaskWithFutureRetryAfter verifies AC-2 from the scan side:
// processEntry must not reclaim a task whose retry_after is still in the future.
func TestProcessEntry_SkipsTaskWithFutureRetryAfter(t *testing.T) {
	cfg := testCfg()
	workers := newFakeWorkerRepository()
	tasks := newFakeTaskRepository()
	heartbeat := newFakeHeartbeatStore()
	scanner := newFakePendingScanner()
	producer := newFakeProducer()
	broker := newFakeBroker()

	taskID := uuid.New()
	future := time.Now().Add(10 * time.Second) // retry_after far in the future
	task := &models.Task{
		ID:          taskID,
		Status:      models.TaskStatusQueued,
		RetryConfig: models.RetryConfig{MaxRetries: 3, Backoff: models.BackoffExponential},
		RetryCount:  1,
		RetryAfter:  &future,
	}
	tasks.tasks[taskID] = task

	entry := &queue.PendingEntry{
		StreamID:   "13-1",
		ConsumerID: "monitor",
		IdleTime:   30 * time.Second,
		TaskID:     taskID.String(),
	}

	m := NewMonitor(cfg, workers, tasks, nil, heartbeat, scanner, producer, broker)
	if err := m.processEntry(context.Background(), entry, "demo"); err != nil {
		t.Fatalf("processEntry: %v", err)
	}

	// No claim or re-enqueue should have happened.
	if len(scanner.claimed) != 0 {
		t.Errorf("processEntry: Claim called %d time(s) for task with future retry_after — must be skipped",
			len(scanner.claimed))
	}
}

// TestReclaimTask_UpToMaxRetries verifies AC-1:
// "Task with {max_retries: 3, backoff: exponential} is retried up to 3 times on infrastructure failure".
// This test verifies that retrying is allowed as long as RetryCount < MaxRetries.
func TestReclaimTask_UpToMaxRetries(t *testing.T) {
	cfg := testCfg()
	workers := newFakeWorkerRepository()
	tasks := newFakeTaskRepository()
	heartbeat := newFakeHeartbeatStore()
	scanner := newFakePendingScanner()
	producer := newFakeProducer()
	broker := newFakeBroker()

	taskID := uuid.New()
	tasks.tasks[taskID] = &models.Task{
		ID:          taskID,
		Status:      models.TaskStatusRunning,
		RetryConfig: models.RetryConfig{MaxRetries: 3, Backoff: models.BackoffExponential},
		RetryCount:  2, // two retries done; one more allowed
	}

	entry := &queue.PendingEntry{
		StreamID:   "14-1",
		ConsumerID: "worker-dead",
		IdleTime:   30 * time.Second,
		TaskID:     taskID.String(),
	}

	m := NewMonitor(cfg, workers, tasks, nil, heartbeat, scanner, producer, broker)

	// processEntry with RetryCount=2, MaxRetries=3: should reclaim (not dead-letter).
	if err := m.processEntry(context.Background(), entry, "demo"); err != nil {
		t.Fatalf("processEntry: %v", err)
	}

	// Must have incremented retry count (reclaim path) not dead-lettered.
	if tasks.retryCounts[taskID] == 0 {
		t.Error("processEntry: IncrementRetryCount not called — task should have been reclaimed (retry 3 of 3)")
	}
	if len(producer.deadLettered) != 0 {
		t.Error("processEntry: task was dead-lettered but retries were not exhausted (RetryCount=2, MaxRetries=3)")
	}
}

// TestProcessEntry_DeadLettersWhenRetriesExhausted verifies AC-4:
// "Task that exhausts retries transitions to 'failed' and is placed in dead letter queue".
func TestProcessEntry_DeadLettersWhenRetriesExhausted(t *testing.T) {
	cfg := testCfg()
	workers := newFakeWorkerRepository()
	tasks := newFakeTaskRepository()
	heartbeat := newFakeHeartbeatStore()
	scanner := newFakePendingScanner()
	producer := newFakeProducer()
	broker := newFakeBroker()

	taskID := uuid.New()
	tasks.tasks[taskID] = &models.Task{
		ID:          taskID,
		Status:      models.TaskStatusRunning,
		RetryConfig: models.RetryConfig{MaxRetries: 3, Backoff: models.BackoffExponential},
		RetryCount:  3, // all retries exhausted
	}

	entry := &queue.PendingEntry{
		StreamID:   "15-1",
		ConsumerID: "worker-dead",
		IdleTime:   30 * time.Second,
		TaskID:     taskID.String(),
	}

	m := NewMonitor(cfg, workers, tasks, nil, heartbeat, scanner, producer, broker)
	if err := m.processEntry(context.Background(), entry, "demo"); err != nil {
		t.Fatalf("processEntry: %v", err)
	}

	// Must be dead-lettered and marked failed.
	if len(producer.deadLettered) == 0 {
		t.Fatal("processEntry: task not dead-lettered after exhausting max_retries=3")
	}

	failed := false
	for _, u := range tasks.statusUpdates {
		if u.id == taskID && u.status == models.TaskStatusFailed {
			failed = true
			break
		}
	}
	if !failed {
		t.Error("processEntry: task status not set to 'failed' after retries exhausted")
	}
}

// TestRetryCountVisibleInTaskState verifies AC-5:
// "Retry count is visible in task state".
// After reclaimTask, the task's RetryCount in the repository must be incremented.
func TestRetryCountVisibleInTaskState(t *testing.T) {
	cfg := testCfg()
	workers := newFakeWorkerRepository()
	tasks := newFakeTaskRepository()
	heartbeat := newFakeHeartbeatStore()
	scanner := newFakePendingScanner()
	producer := newFakeProducer()
	broker := newFakeBroker()

	taskID := uuid.New()
	tasks.tasks[taskID] = newTask(taskID, models.RetryConfig{MaxRetries: 3, Backoff: models.BackoffExponential})

	entry := &queue.PendingEntry{
		StreamID:   "16-1",
		ConsumerID: "worker-dead",
		IdleTime:   30 * time.Second,
		TaskID:     taskID.String(),
	}

	m := NewMonitor(cfg, workers, tasks, nil, heartbeat, scanner, producer, broker)
	if err := m.reclaimTask(context.Background(), entry, "demo"); err != nil {
		t.Fatalf("reclaimTask: %v", err)
	}

	// RetryCount must be visible in the task record (AC-5).
	task, _ := tasks.GetByID(context.Background(), taskID)
	if task == nil {
		t.Fatal("GetByID returned nil for task that should exist")
	}
	if task.RetryCount != 1 {
		t.Errorf("task.RetryCount = %d, want 1 — retry count must be visible in task state", task.RetryCount)
	}
}

// --- TASK-011 acceptance criterion tests ---

// TestDeadLetterTask_CascadeCancelsDownstreamTasks verifies AC-2:
// Pipeline chain A -> B -> C: when task A enters the dead letter queue,
// tasks B and C are cancelled with reason "upstream task failed".
func TestDeadLetterTask_CascadeCancelsDownstreamTasks(t *testing.T) {
	cfg := testCfg()
	workers := newFakeWorkerRepository()
	tasks := newFakeTaskRepository()
	heartbeat := newFakeHeartbeatStore()
	scanner := newFakePendingScanner()
	producer := newFakeProducer()
	broker := newFakeBroker()
	chains := newFakeChainRepository()

	// Set up three pipelines in a chain: A -> B -> C.
	pipelineA := uuid.New()
	pipelineB := uuid.New()
	pipelineC := uuid.New()

	chain := &models.Chain{
		ID:          uuid.New(),
		Name:        "test-chain",
		UserID:      uuid.New(),
		PipelineIDs: []uuid.UUID{pipelineA, pipelineB, pipelineC},
	}
	chains.addChain(chain)

	// Task A is the failing task (will be dead-lettered).
	taskAID := uuid.New()
	taskA := &models.Task{
		ID:          taskAID,
		PipelineID:  &pipelineA,
		Status:      models.TaskStatusRunning,
		RetryConfig: models.RetryConfig{MaxRetries: 3},
		RetryCount:  3,
	}
	tasks.tasks[taskAID] = taskA

	// Task B is queued downstream.
	taskBID := uuid.New()
	taskB := &models.Task{
		ID:         taskBID,
		PipelineID: &pipelineB,
		Status:     models.TaskStatusQueued,
		RetryConfig: models.RetryConfig{MaxRetries: 3},
	}
	tasks.tasks[taskBID] = taskB

	// Task C is submitted downstream.
	taskCID := uuid.New()
	taskC := &models.Task{
		ID:         taskCID,
		PipelineID: &pipelineC,
		Status:     models.TaskStatusSubmitted,
		RetryConfig: models.RetryConfig{MaxRetries: 3},
	}
	tasks.tasks[taskCID] = taskC

	entry := &queue.PendingEntry{
		StreamID:   "100-1",
		ConsumerID: "worker-dead",
		IdleTime:   30 * time.Second,
		TaskID:     taskAID.String(),
	}

	m := NewMonitor(cfg, workers, tasks, chains, heartbeat, scanner, producer, broker)

	if err := m.deadLetterTask(context.Background(), entry, "demo"); err != nil {
		t.Fatalf("deadLetterTask: %v", err)
	}

	// Task A must be dead-lettered and failed.
	if len(producer.deadLettered) == 0 {
		t.Fatal("deadLetterTask: EnqueueDeadLetter not called for task A")
	}
	if producer.deadLettered[0].taskID != taskAID.String() {
		t.Errorf("deadLetterTask: dead-lettered taskID=%q, want %q", producer.deadLettered[0].taskID, taskAID.String())
	}

	// Task B must be cancelled.
	if tasks.tasks[taskBID].Status != models.TaskStatusCancelled {
		t.Errorf("cascade: task B status=%q, want %q", tasks.tasks[taskBID].Status, models.TaskStatusCancelled)
	}

	// Task C must be cancelled.
	if tasks.tasks[taskCID].Status != models.TaskStatusCancelled {
		t.Errorf("cascade: task C status=%q, want %q", tasks.tasks[taskCID].Status, models.TaskStatusCancelled)
	}
}

// TestDeadLetterTask_StandaloneTaskNoCascade verifies AC-3:
// A standalone task (not part of any chain) enters the dead letter queue
// without cascading cancellation.
func TestDeadLetterTask_StandaloneTaskNoCascade(t *testing.T) {
	cfg := testCfg()
	workers := newFakeWorkerRepository()
	tasks := newFakeTaskRepository()
	heartbeat := newFakeHeartbeatStore()
	scanner := newFakePendingScanner()
	producer := newFakeProducer()
	broker := newFakeBroker()
	chains := newFakeChainRepository() // empty — no chains defined

	// Task is standalone: has a pipeline but pipeline is not in any chain.
	standalonePipeline := uuid.New()
	taskID := uuid.New()
	tasks.tasks[taskID] = &models.Task{
		ID:          taskID,
		PipelineID:  &standalonePipeline,
		Status:      models.TaskStatusRunning,
		RetryConfig: models.RetryConfig{MaxRetries: 3},
		RetryCount:  3,
	}

	// An unrelated queued task that must NOT be cancelled.
	otherPipeline := uuid.New()
	otherTaskID := uuid.New()
	tasks.tasks[otherTaskID] = &models.Task{
		ID:         otherTaskID,
		PipelineID: &otherPipeline,
		Status:     models.TaskStatusQueued,
		RetryConfig: models.RetryConfig{MaxRetries: 3},
	}

	entry := &queue.PendingEntry{
		StreamID:   "101-1",
		ConsumerID: "worker-dead",
		IdleTime:   30 * time.Second,
		TaskID:     taskID.String(),
	}

	m := NewMonitor(cfg, workers, tasks, chains, heartbeat, scanner, producer, broker)

	if err := m.deadLetterTask(context.Background(), entry, "demo"); err != nil {
		t.Fatalf("deadLetterTask: %v", err)
	}

	// Task must be dead-lettered.
	if len(producer.deadLettered) == 0 {
		t.Fatal("deadLetterTask: EnqueueDeadLetter not called")
	}

	// Other task must NOT have been cancelled (no cascade for standalone).
	if tasks.tasks[otherTaskID].Status == models.TaskStatusCancelled {
		t.Error("cascade: unrelated task was incorrectly cancelled for standalone pipeline")
	}
}

// TestDeadLetterTask_CascadePublishesSSEEvents verifies that SSE task events are
// published for each task cancelled as part of cascading cancellation (NFR-003).
func TestDeadLetterTask_CascadePublishesSSEEvents(t *testing.T) {
	cfg := testCfg()
	workers := newFakeWorkerRepository()
	tasks := newFakeTaskRepository()
	heartbeat := newFakeHeartbeatStore()
	scanner := newFakePendingScanner()
	producer := newFakeProducer()
	broker := newFakeBroker()
	chains := newFakeChainRepository()

	pipelineA := uuid.New()
	pipelineB := uuid.New()

	chain := &models.Chain{
		ID:          uuid.New(),
		Name:        "sse-chain",
		UserID:      uuid.New(),
		PipelineIDs: []uuid.UUID{pipelineA, pipelineB},
	}
	chains.addChain(chain)

	taskAID := uuid.New()
	taskA := &models.Task{
		ID:          taskAID,
		PipelineID:  &pipelineA,
		Status:      models.TaskStatusRunning,
		RetryConfig: models.RetryConfig{MaxRetries: 3},
		RetryCount:  3,
	}
	tasks.tasks[taskAID] = taskA

	taskBID := uuid.New()
	taskB := &models.Task{
		ID:         taskBID,
		PipelineID: &pipelineB,
		Status:     models.TaskStatusRunning,
		RetryConfig: models.RetryConfig{MaxRetries: 3},
	}
	tasks.tasks[taskBID] = taskB

	entry := &queue.PendingEntry{
		StreamID:   "102-1",
		ConsumerID: "worker-dead",
		IdleTime:   30 * time.Second,
		TaskID:     taskAID.String(),
	}

	m := NewMonitor(cfg, workers, tasks, chains, heartbeat, scanner, producer, broker)

	if err := m.deadLetterTask(context.Background(), entry, "demo"); err != nil {
		t.Fatalf("deadLetterTask: %v", err)
	}

	// At least one SSE task event must have been published for the cancelled downstream task.
	broker.mu.Lock()
	taskEventCount := len(broker.taskEvents)
	broker.mu.Unlock()

	if taskEventCount == 0 {
		t.Fatal("cascade: no SSE task events published for downstream cancellation")
	}

	// The published event must carry status "cancelled".
	broker.mu.Lock()
	defer broker.mu.Unlock()
	for _, evt := range broker.taskEvents {
		if evt.ID == taskBID && evt.Status != models.TaskStatusCancelled {
			t.Errorf("cascade: SSE event for task B has status=%q, want %q", evt.Status, models.TaskStatusCancelled)
		}
	}
}

// TestDeadLetterTask_CascadeOnlyDownstreamNotUpstream verifies that cascading
// cancellation affects only pipelines AFTER the failing pipeline in the chain,
// not the failing pipeline itself or any upstream pipelines.
// Chain: A -> B -> C; task B fails; only task C should be cancelled.
func TestDeadLetterTask_CascadeOnlyDownstreamNotUpstream(t *testing.T) {
	cfg := testCfg()
	workers := newFakeWorkerRepository()
	tasks := newFakeTaskRepository()
	heartbeat := newFakeHeartbeatStore()
	scanner := newFakePendingScanner()
	producer := newFakeProducer()
	broker := newFakeBroker()
	chains := newFakeChainRepository()

	pipelineA := uuid.New()
	pipelineB := uuid.New()
	pipelineC := uuid.New()

	chain := &models.Chain{
		ID:          uuid.New(),
		Name:        "middle-chain",
		UserID:      uuid.New(),
		PipelineIDs: []uuid.UUID{pipelineA, pipelineB, pipelineC},
	}
	chains.addChain(chain)

	// Task A is upstream and completed — must not be touched.
	taskAID := uuid.New()
	taskA := &models.Task{
		ID:          taskAID,
		PipelineID:  &pipelineA,
		Status:      models.TaskStatusCompleted,
		RetryConfig: models.RetryConfig{MaxRetries: 3},
	}
	tasks.tasks[taskAID] = taskA

	// Task B is the failing task (middle of chain).
	taskBID := uuid.New()
	taskB := &models.Task{
		ID:          taskBID,
		PipelineID:  &pipelineB,
		Status:      models.TaskStatusRunning,
		RetryConfig: models.RetryConfig{MaxRetries: 3},
		RetryCount:  3,
	}
	tasks.tasks[taskBID] = taskB

	// Task C is downstream and queued — must be cancelled.
	taskCID := uuid.New()
	taskC := &models.Task{
		ID:         taskCID,
		PipelineID: &pipelineC,
		Status:     models.TaskStatusQueued,
		RetryConfig: models.RetryConfig{MaxRetries: 3},
	}
	tasks.tasks[taskCID] = taskC

	entry := &queue.PendingEntry{
		StreamID:   "103-1",
		ConsumerID: "worker-dead",
		IdleTime:   30 * time.Second,
		TaskID:     taskBID.String(),
	}

	m := NewMonitor(cfg, workers, tasks, chains, heartbeat, scanner, producer, broker)

	if err := m.deadLetterTask(context.Background(), entry, "demo"); err != nil {
		t.Fatalf("deadLetterTask: %v", err)
	}

	// Task A (upstream, completed) must remain completed.
	if tasks.tasks[taskAID].Status != models.TaskStatusCompleted {
		t.Errorf("cascade: upstream task A was incorrectly modified: status=%q", tasks.tasks[taskAID].Status)
	}

	// Task C (downstream) must be cancelled.
	if tasks.tasks[taskCID].Status != models.TaskStatusCancelled {
		t.Errorf("cascade: downstream task C status=%q, want %q", tasks.tasks[taskCID].Status, models.TaskStatusCancelled)
	}
}

// TestDeadLetterTask_NilChainsIsNoop verifies that when chains is nil (standalone
// monitor configuration), deadLetterTask succeeds without panicking or performing
// any cascade operations.
func TestDeadLetterTask_NilChainsIsNoop(t *testing.T) {
	cfg := testCfg()
	workers := newFakeWorkerRepository()
	tasks := newFakeTaskRepository()
	heartbeat := newFakeHeartbeatStore()
	scanner := newFakePendingScanner()
	producer := newFakeProducer()
	broker := newFakeBroker()

	pipelineID := uuid.New()
	taskID := uuid.New()
	tasks.tasks[taskID] = &models.Task{
		ID:          taskID,
		PipelineID:  &pipelineID,
		Status:      models.TaskStatusRunning,
		RetryConfig: models.RetryConfig{MaxRetries: 3},
		RetryCount:  3,
	}

	entry := &queue.PendingEntry{
		StreamID:   "104-1",
		ConsumerID: "worker-dead",
		IdleTime:   30 * time.Second,
		TaskID:     taskID.String(),
	}

	// nil chains — cascadeCancelDownstream must be a no-op.
	m := NewMonitor(cfg, workers, tasks, nil, heartbeat, scanner, producer, broker)

	if err := m.deadLetterTask(context.Background(), entry, "demo"); err != nil {
		t.Fatalf("deadLetterTask with nil chains: unexpected error: %v", err)
	}

	// Task must still be dead-lettered normally.
	if len(producer.deadLettered) == 0 {
		t.Fatal("deadLetterTask: EnqueueDeadLetter not called when chains is nil")
	}
}
