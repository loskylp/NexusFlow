// Package worker_test — unit tests for TASK-007: pipeline execution, schema mapping,
// state transitions, and event emission.
// Tests use in-memory fakes for all external dependencies so no live Redis or
// PostgreSQL instance is required.
// See: ADR-001, ADR-003, ADR-008, TASK-007
package worker_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nxlabs/nexusflow/internal/config"
	"github.com/nxlabs/nexusflow/internal/models"
	"github.com/nxlabs/nexusflow/internal/queue"
	"github.com/nxlabs/nexusflow/worker"
)

// --- fakes for TASK-007 ---

// fakeTaskRepo is an in-memory TaskRepository double that records all status transitions.
type fakeTaskRepo struct {
	mu           sync.Mutex
	tasks        map[uuid.UUID]*models.Task
	statusLog    []statusTransition
	statusErrors map[uuid.UUID]error // per-task errors to inject
}

type statusTransition struct {
	taskID    uuid.UUID
	newStatus models.TaskStatus
	reason    string
	workerID  *string
}

func newFakeTaskRepo(tasks ...*models.Task) *fakeTaskRepo {
	r := &fakeTaskRepo{
		tasks:        make(map[uuid.UUID]*models.Task),
		statusErrors: make(map[uuid.UUID]error),
	}
	for _, t := range tasks {
		cp := *t
		r.tasks[t.ID] = &cp
	}
	return r
}

func (r *fakeTaskRepo) Create(ctx context.Context, task *models.Task) (*models.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *task
	r.tasks[task.ID] = &cp
	return &cp, nil
}

func (r *fakeTaskRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.tasks[id]
	if !ok {
		return nil, nil
	}
	cp := *t
	return &cp, nil
}

func (r *fakeTaskRepo) ListByUser(ctx context.Context, userID uuid.UUID) ([]*models.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var result []*models.Task
	for _, t := range r.tasks {
		if t.UserID == userID {
			cp := *t
			result = append(result, &cp)
		}
	}
	return result, nil
}

func (r *fakeTaskRepo) List(ctx context.Context) ([]*models.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]*models.Task, 0, len(r.tasks))
	for _, t := range r.tasks {
		cp := *t
		result = append(result, &cp)
	}
	return result, nil
}

func (r *fakeTaskRepo) UpdateStatus(ctx context.Context, id uuid.UUID, newStatus models.TaskStatus, reason string, workerID *string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err, ok := r.statusErrors[id]; ok {
		return err
	}
	r.statusLog = append(r.statusLog, statusTransition{
		taskID:    id,
		newStatus: newStatus,
		reason:    reason,
		workerID:  workerID,
	})
	if t, ok := r.tasks[id]; ok {
		t.Status = newStatus
		if workerID != nil {
			t.WorkerID = workerID
		}
	}
	return nil
}

func (r *fakeTaskRepo) IncrementRetryCount(ctx context.Context, id uuid.UUID) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if t, ok := r.tasks[id]; ok {
		t.RetryCount++
		return t.RetryCount, nil
	}
	return 0, nil
}

func (r *fakeTaskRepo) Cancel(ctx context.Context, id uuid.UUID, reason string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if t, ok := r.tasks[id]; ok {
		t.Status = models.TaskStatusCancelled
	}
	return nil
}

func (r *fakeTaskRepo) GetStateLog(ctx context.Context, taskID uuid.UUID) ([]*models.TaskStateLog, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var log []*models.TaskStateLog
	for _, tr := range r.statusLog {
		if tr.taskID == taskID {
			log = append(log, &models.TaskStateLog{
				TaskID:  taskID,
				ToState: tr.newStatus,
				Reason:  tr.reason,
			})
		}
	}
	return log, nil
}

// getTransitions returns all recorded transitions for a task in order.
func (r *fakeTaskRepo) getTransitions(taskID uuid.UUID) []statusTransition {
	r.mu.Lock()
	defer r.mu.Unlock()
	var result []statusTransition
	for _, tr := range r.statusLog {
		if tr.taskID == taskID {
			result = append(result, tr)
		}
	}
	return result
}

// fakePipelineRepo is an in-memory PipelineRepository double.
type fakePipelineRepo struct {
	mu        sync.Mutex
	pipelines map[uuid.UUID]*models.Pipeline
}

func newFakePipelineRepo(pipelines ...*models.Pipeline) *fakePipelineRepo {
	r := &fakePipelineRepo{pipelines: make(map[uuid.UUID]*models.Pipeline)}
	for _, p := range pipelines {
		cp := *p
		r.pipelines[p.ID] = &cp
	}
	return r
}

func (r *fakePipelineRepo) Create(ctx context.Context, p *models.Pipeline) (*models.Pipeline, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *p
	r.pipelines[p.ID] = &cp
	return &cp, nil
}

func (r *fakePipelineRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.Pipeline, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.pipelines[id]
	if !ok {
		return nil, nil
	}
	cp := *p
	return &cp, nil
}

func (r *fakePipelineRepo) ListByUser(ctx context.Context, userID uuid.UUID) ([]*models.Pipeline, error) {
	return nil, nil
}

func (r *fakePipelineRepo) List(ctx context.Context) ([]*models.Pipeline, error) {
	return nil, nil
}

func (r *fakePipelineRepo) Update(ctx context.Context, p *models.Pipeline) (*models.Pipeline, error) {
	return nil, nil
}

func (r *fakePipelineRepo) Delete(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (r *fakePipelineRepo) HasActiveTasks(ctx context.Context, pipelineID uuid.UUID) (bool, error) {
	return false, nil
}

// fakeConsumer delivers a pre-loaded set of messages then blocks briefly on subsequent calls.
type fakeConsumer struct {
	mu       sync.Mutex
	messages []*queue.TaskMessage
	acked    []string // stream IDs that were acknowledged
}

func newFakeConsumer(messages ...*queue.TaskMessage) *fakeConsumer {
	return &fakeConsumer{messages: messages}
}

func (c *fakeConsumer) ReadTasks(ctx context.Context, consumerID string, tags []string, blockFor time.Duration) ([]*queue.TaskMessage, error) {
	if ctx.Err() != nil {
		return nil, nil
	}
	c.mu.Lock()
	if len(c.messages) > 0 {
		batch := c.messages
		c.messages = nil
		c.mu.Unlock()
		return batch, nil
	}
	c.mu.Unlock()

	// No messages: block briefly then return empty so the loop can check ctx.
	select {
	case <-ctx.Done():
		return nil, nil
	case <-time.After(10 * time.Millisecond):
		return []*queue.TaskMessage{}, nil
	}
}

func (c *fakeConsumer) Acknowledge(ctx context.Context, tag string, streamID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.acked = append(c.acked, streamID)
	return nil
}

func (c *fakeConsumer) InitGroups(ctx context.Context, tags []string) error {
	return nil
}

func (c *fakeConsumer) wasAcked(streamID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, id := range c.acked {
		if id == streamID {
			return true
		}
	}
	return false
}

// fakeBroker records PublishTaskEvent calls. Implements worker.TaskEventBroker.
type fakeBroker struct {
	mu     sync.Mutex
	events []*models.Task
}

func (b *fakeBroker) PublishTaskEvent(ctx context.Context, task *models.Task, reason string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	cp := *task
	b.events = append(b.events, &cp)
	return nil
}

func (b *fakeBroker) publishCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.events)
}

func (b *fakeBroker) lastStatus() models.TaskStatus {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.events) == 0 {
		return ""
	}
	return b.events[len(b.events)-1].Status
}

// fakeDataSource is a DataSourceConnector that returns fixed records.
type fakeDataSource struct {
	connType string
	records  []map[string]any
	err      error
}

func (d *fakeDataSource) Type() string { return d.connType }
func (d *fakeDataSource) Fetch(ctx context.Context, config map[string]any, input map[string]any) ([]map[string]any, error) {
	if d.err != nil {
		return nil, d.err
	}
	return d.records, nil
}

// fakeProcess is a ProcessConnector that returns records unchanged unless configured to error.
type fakeProcess struct {
	connType string
	err      error
}

func (p *fakeProcess) Type() string { return p.connType }
func (p *fakeProcess) Transform(ctx context.Context, config map[string]any, records []map[string]any) ([]map[string]any, error) {
	if p.err != nil {
		return nil, p.err
	}
	return records, nil
}

// fakeSink is a SinkConnector that stores writes in memory.
type fakeSink struct {
	connType  string
	mu        sync.Mutex
	written   map[string][]map[string]any // executionID -> records
	writeErr  error
	snapshots []map[string]any
}

func newFakeSink(connType string) *fakeSink {
	return &fakeSink{connType: connType, written: make(map[string][]map[string]any)}
}

func (s *fakeSink) Type() string { return s.connType }

func (s *fakeSink) Snapshot(ctx context.Context, config map[string]any, taskID string) (map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	snap := map[string]any{"taskId": taskID}
	s.snapshots = append(s.snapshots, snap)
	return snap, nil
}

func (s *fakeSink) Write(ctx context.Context, config map[string]any, records []map[string]any, executionID string) error {
	if s.writeErr != nil {
		return s.writeErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.written[executionID]; exists {
		return worker.ErrAlreadyApplied
	}
	s.written[executionID] = records
	return nil
}

func (s *fakeSink) recordCount(executionID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.written[executionID])
}

// fakeRegistry is a ConnectorRegistry backed by maps.
type fakeRegistry struct {
	sources map[string]worker.DataSourceConnector
	procs   map[string]worker.ProcessConnector
	sinks   map[string]worker.SinkConnector
}

func newFakeRegistry() *fakeRegistry {
	return &fakeRegistry{
		sources: make(map[string]worker.DataSourceConnector),
		procs:   make(map[string]worker.ProcessConnector),
		sinks:   make(map[string]worker.SinkConnector),
	}
}

func (r *fakeRegistry) DataSource(t string) (worker.DataSourceConnector, error) {
	c, ok := r.sources[t]
	if !ok {
		return nil, worker.ErrUnknownConnector
	}
	return c, nil
}

func (r *fakeRegistry) Process(t string) (worker.ProcessConnector, error) {
	c, ok := r.procs[t]
	if !ok {
		return nil, worker.ErrUnknownConnector
	}
	return c, nil
}

func (r *fakeRegistry) Sink(t string) (worker.SinkConnector, error) {
	c, ok := r.sinks[t]
	if !ok {
		return nil, worker.ErrUnknownConnector
	}
	return c, nil
}

func (r *fakeRegistry) Register(kind string, connector any) {
	switch kind {
	case "datasource":
		c := connector.(worker.DataSourceConnector)
		r.sources[c.Type()] = c
	case "process":
		c := connector.(worker.ProcessConnector)
		r.procs[c.Type()] = c
	case "sink":
		c := connector.(worker.SinkConnector)
		r.sinks[c.Type()] = c
	}
}

// --- helpers ---

// makePipeline constructs a simple Pipeline for testing.
func makePipeline(id uuid.UUID, dsType, procType, sinkType string) *models.Pipeline {
	return &models.Pipeline{
		ID:     id,
		Name:   "test-pipeline",
		UserID: uuid.New(),
		DataSourceConfig: models.DataSourceConfig{
			ConnectorType: dsType,
			Config:        map[string]any{},
			OutputSchema:  []string{"id", "name"},
		},
		ProcessConfig: models.ProcessConfig{
			ConnectorType: procType,
			Config:        map[string]any{},
			InputMappings: []models.SchemaMapping{},
			OutputSchema:  []string{"id", "name"},
		},
		SinkConfig: models.SinkConfig{
			ConnectorType: sinkType,
			Config:        map[string]any{},
			InputMappings: []models.SchemaMapping{},
		},
	}
}

// makeTask constructs a minimal queued Task for testing.
func makeTask(id, pipelineID uuid.UUID) *models.Task {
	return &models.Task{
		ID:          id,
		PipelineID:  &pipelineID,
		UserID:      uuid.New(),
		Status:      models.TaskStatusQueued,
		RetryConfig: models.DefaultRetryConfig(),
		ExecutionID: id.String() + ":1",
		Input:       map[string]any{},
	}
}

// testExecutorConfig returns a minimal Worker config.
func testExecutorConfig(workerID string) *config.Config {
	return &config.Config{
		WorkerID:   workerID,
		WorkerTags: []string{"etl"},
	}
}

// buildWorkerForExecution constructs a Worker wired with task/pipeline repos, a consumer,
// and a broker for integration-style unit tests of executeTask.
// The fakeBroker satisfies worker.TaskEventBroker directly.
func buildWorkerForExecution(
	workerID string,
	taskRepo *fakeTaskRepo,
	pipelineRepo *fakePipelineRepo,
	consumer *fakeConsumer,
	broker *fakeBroker,
	reg worker.ConnectorRegistry,
) *worker.Worker {
	return worker.NewWorkerWithPipelines(
		testExecutorConfig(workerID),
		taskRepo,
		newFakeWorkerRepo(),
		pipelineRepo,
		consumer,
		newFakeHeartbeatStore(),
		broker,
		reg,
		nil, // cancellations — not needed for executor tests
	)
}

// --- applySchemaMapping tests ---

// TestApplySchemaMapping_RenamesFields verifies that ApplySchemaMapping renames
// fields from SourceField to TargetField for each SchemaMapping.
// AC-7: Schema mapping renames fields between phases.
func TestApplySchemaMapping_RenamesFields(t *testing.T) {
	w := worker.NewWorker(
		testExecutorConfig("map-test-001"),
		nil, nil, nil, nil, nil, nil, nil,
	)

	input := map[string]any{"id": "123", "name": "Alice"}
	mappings := []models.SchemaMapping{
		{SourceField: "id", TargetField: "user_id"},
		{SourceField: "name", TargetField: "full_name"},
	}

	got, err := w.ApplySchemaMapping(input, mappings)
	if err != nil {
		t.Fatalf("ApplySchemaMapping: unexpected error: %v", err)
	}

	if got["user_id"] != "123" {
		t.Errorf("user_id = %v; want %q", got["user_id"], "123")
	}
	if got["full_name"] != "Alice" {
		t.Errorf("full_name = %v; want %q", got["full_name"], "Alice")
	}
}

// TestApplySchemaMapping_ErrorOnMissingSourceField verifies that ApplySchemaMapping
// returns an error when a mapping references a SourceField absent from the input.
// AC-7: Schema mapping error causes task failure.
func TestApplySchemaMapping_ErrorOnMissingSourceField(t *testing.T) {
	w := worker.NewWorker(
		testExecutorConfig("map-test-002"),
		nil, nil, nil, nil, nil, nil, nil,
	)

	input := map[string]any{"id": "123"}
	mappings := []models.SchemaMapping{
		{SourceField: "id", TargetField: "user_id"},
		{SourceField: "missing_field", TargetField: "something"},
	}

	_, err := w.ApplySchemaMapping(input, mappings)
	if err == nil {
		t.Fatal("ApplySchemaMapping: expected error for missing source field, got nil")
	}
}

// TestApplySchemaMapping_EmptyMappings verifies that empty mappings returns an empty map.
func TestApplySchemaMapping_EmptyMappings(t *testing.T) {
	w := worker.NewWorker(
		testExecutorConfig("map-test-003"),
		nil, nil, nil, nil, nil, nil, nil,
	)

	input := map[string]any{"id": "123", "name": "Alice"}
	got, err := w.ApplySchemaMapping(input, []models.SchemaMapping{})
	if err != nil {
		t.Fatalf("ApplySchemaMapping with empty mappings: unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty map with empty mappings, got %d entries", len(got))
	}
}

// TestApplySchemaMapping_DoesNotMutateInput verifies that ApplySchemaMapping returns
// a new map and does not modify the input.
func TestApplySchemaMapping_DoesNotMutateInput(t *testing.T) {
	w := worker.NewWorker(
		testExecutorConfig("map-test-004"),
		nil, nil, nil, nil, nil, nil, nil,
	)

	input := map[string]any{"id": "123"}
	mappings := []models.SchemaMapping{{SourceField: "id", TargetField: "user_id"}}

	_, err := w.ApplySchemaMapping(input, mappings)
	if err != nil {
		t.Fatalf("ApplySchemaMapping: unexpected error: %v", err)
	}

	if _, ok := input["user_id"]; ok {
		t.Error("ApplySchemaMapping mutated the input map (user_id found in original)")
	}
	if _, ok := input["id"]; !ok {
		t.Error("ApplySchemaMapping removed id from the original input map")
	}
}

// --- ConnectorRegistry tests ---

// TestDefaultConnectorRegistry_RegisterAndResolve verifies that registered connectors
// can be retrieved by type name.
func TestDefaultConnectorRegistry_RegisterAndResolve(t *testing.T) {
	reg := worker.NewDefaultConnectorRegistry()
	ds := &fakeDataSource{connType: "test-ds"}
	proc := &fakeProcess{connType: "test-proc"}
	sink := newFakeSink("test-sink")

	reg.Register("datasource", ds)
	reg.Register("process", proc)
	reg.Register("sink", sink)

	gotDS, err := reg.DataSource("test-ds")
	if err != nil || gotDS == nil {
		t.Fatalf("DataSource lookup failed: %v", err)
	}

	gotProc, err := reg.Process("test-proc")
	if err != nil || gotProc == nil {
		t.Fatalf("Process lookup failed: %v", err)
	}

	gotSink, err := reg.Sink("test-sink")
	if err != nil || gotSink == nil {
		t.Fatalf("Sink lookup failed: %v", err)
	}
}

// TestDefaultConnectorRegistry_UnknownType verifies ErrUnknownConnector is returned
// for unregistered connector types.
func TestDefaultConnectorRegistry_UnknownType(t *testing.T) {
	reg := worker.NewDefaultConnectorRegistry()

	_, err := reg.DataSource("no-such-connector")
	if !errors.Is(err, worker.ErrUnknownConnector) {
		t.Errorf("DataSource unknown type: got %v; want ErrUnknownConnector", err)
	}

	_, err = reg.Process("no-such-connector")
	if !errors.Is(err, worker.ErrUnknownConnector) {
		t.Errorf("Process unknown type: got %v; want ErrUnknownConnector", err)
	}

	_, err = reg.Sink("no-such-connector")
	if !errors.Is(err, worker.ErrUnknownConnector) {
		t.Errorf("Sink unknown type: got %v; want ErrUnknownConnector", err)
	}
}

// --- executeTask state transition tests ---

// TestExecuteTask_SuccessfulPipeline_CompletesTask verifies:
// AC-1: Worker picks up task from the queue
// AC-2: State transitions queued -> assigned -> running -> completed
// AC-9: Task state change events emitted
func TestExecuteTask_SuccessfulPipeline_CompletesTask(t *testing.T) {
	pipelineID := uuid.New()
	taskID := uuid.New()

	pipeline := makePipeline(pipelineID, "demo", "demo", "demo")
	task := makeTask(taskID, pipelineID)

	taskRepo := newFakeTaskRepo(task)
	pipelineRepo := newFakePipelineRepo(pipeline)

	msg := &queue.TaskMessage{
		TaskID:      taskID.String(),
		PipelineID:  pipelineID.String(),
		UserID:      task.UserID.String(),
		ExecutionID: task.ExecutionID,
		StreamID:    "1234-0",
	}

	consumer := newFakeConsumer(msg)
	broker := &fakeBroker{}
	sink := newFakeSink("demo")

	reg := newFakeRegistry()
	reg.Register("datasource", &fakeDataSource{
		connType: "demo",
		records:  []map[string]any{{"id": "1", "name": "Alice"}},
	})
	reg.Register("process", &fakeProcess{connType: "demo"})
	reg.Register("sink", sink)

	w := buildWorkerForExecution("exec-test-001", taskRepo, pipelineRepo, consumer, broker, reg)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_ = w.Run(ctx)

	// Verify final task status.
	got, err := taskRepo.GetByID(context.Background(), taskID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got == nil {
		t.Fatal("task not found after execution")
	}
	if got.Status != models.TaskStatusCompleted {
		t.Errorf("task Status = %q; want %q", got.Status, models.TaskStatusCompleted)
	}

	// Verify state transitions were all recorded: assigned, running, completed.
	transitions := taskRepo.getTransitions(taskID)
	if len(transitions) < 3 {
		t.Errorf("expected at least 3 state transitions, got %d: %+v", len(transitions), transitions)
	}

	// Verify sink received the data.
	if sink.recordCount(task.ExecutionID) < 1 {
		t.Error("sink received no records")
	}

	// Verify the message was acknowledged.
	if !consumer.wasAcked(msg.StreamID) {
		t.Error("message was not acknowledged after successful execution")
	}

	// Verify SSE events were published.
	if broker.publishCount() < 1 {
		t.Error("expected at least one SSE event published")
	}
}

// TestExecuteTask_ProcessError_SetsFailedStatus verifies:
// AC-8: Failed pipeline execution sets task status to "failed".
// Domain Invariant 2 (ADR-003): Process errors do not retry.
func TestExecuteTask_ProcessError_SetsFailedStatus(t *testing.T) {
	pipelineID := uuid.New()
	taskID := uuid.New()

	pipeline := makePipeline(pipelineID, "demo", "demo", "demo")
	task := makeTask(taskID, pipelineID)

	taskRepo := newFakeTaskRepo(task)
	pipelineRepo := newFakePipelineRepo(pipeline)

	msg := &queue.TaskMessage{
		TaskID:      taskID.String(),
		PipelineID:  pipelineID.String(),
		UserID:      task.UserID.String(),
		ExecutionID: task.ExecutionID,
		StreamID:    "9999-0",
	}

	consumer := newFakeConsumer(msg)
	broker := &fakeBroker{}
	sink := newFakeSink("demo")

	reg := newFakeRegistry()
	reg.Register("datasource", &fakeDataSource{
		connType: "demo",
		records:  []map[string]any{{"id": "1"}},
	})
	reg.Register("process", &fakeProcess{
		connType: "demo",
		err:      errors.New("script error: division by zero"),
	})
	reg.Register("sink", sink)

	w := buildWorkerForExecution("exec-test-002", taskRepo, pipelineRepo, consumer, broker, reg)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_ = w.Run(ctx)

	got, err := taskRepo.GetByID(context.Background(), taskID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got == nil {
		t.Fatal("task not found after failed execution")
	}
	if got.Status != models.TaskStatusFailed {
		t.Errorf("task Status after process error = %q; want %q", got.Status, models.TaskStatusFailed)
	}

	// Message should be acknowledged so it leaves the pending list (domain error, no XCLAIM needed).
	if !consumer.wasAcked(msg.StreamID) {
		t.Error("message was not acknowledged after process error (process errors should be terminal — no XCLAIM)")
	}
}

// TestExecuteTask_DataSourceError_SetsFailedStatus verifies that a DataSource error
// also marks the task as failed and acknowledges the message.
func TestExecuteTask_DataSourceError_SetsFailedStatus(t *testing.T) {
	pipelineID := uuid.New()
	taskID := uuid.New()

	pipeline := makePipeline(pipelineID, "demo", "demo", "demo")
	task := makeTask(taskID, pipelineID)

	taskRepo := newFakeTaskRepo(task)
	pipelineRepo := newFakePipelineRepo(pipeline)

	msg := &queue.TaskMessage{
		TaskID:      taskID.String(),
		PipelineID:  pipelineID.String(),
		UserID:      task.UserID.String(),
		ExecutionID: task.ExecutionID,
		StreamID:    "8888-0",
	}

	consumer := newFakeConsumer(msg)
	broker := &fakeBroker{}
	sink := newFakeSink("demo")

	reg := newFakeRegistry()
	reg.Register("datasource", &fakeDataSource{
		connType: "demo",
		err:      errors.New("source unreachable"),
	})
	reg.Register("process", &fakeProcess{connType: "demo"})
	reg.Register("sink", sink)

	w := buildWorkerForExecution("exec-test-003", taskRepo, pipelineRepo, consumer, broker, reg)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_ = w.Run(ctx)

	got, _ := taskRepo.GetByID(context.Background(), taskID)
	if got == nil || got.Status != models.TaskStatusFailed {
		t.Errorf("task Status after datasource error = %v; want %q", got, models.TaskStatusFailed)
	}
}

// TestExecuteTask_SchemaMapping_Applied verifies AC-7:
// schema mappings rename fields between DataSource output and Process input.
func TestExecuteTask_SchemaMapping_Applied(t *testing.T) {
	pipelineID := uuid.New()
	taskID := uuid.New()

	pipeline := makePipeline(pipelineID, "demo", "demo", "demo")
	// Add input mappings to the Process phase: rename "id" -> "user_id"
	pipeline.ProcessConfig.InputMappings = []models.SchemaMapping{
		{SourceField: "id", TargetField: "user_id"},
		{SourceField: "name", TargetField: "full_name"},
	}
	// Sink receives Process output directly (no extra mapping)
	pipeline.SinkConfig.InputMappings = []models.SchemaMapping{}

	task := makeTask(taskID, pipelineID)

	taskRepo := newFakeTaskRepo(task)
	pipelineRepo := newFakePipelineRepo(pipeline)

	msg := &queue.TaskMessage{
		TaskID:      taskID.String(),
		PipelineID:  pipelineID.String(),
		UserID:      task.UserID.String(),
		ExecutionID: task.ExecutionID,
		StreamID:    "7777-0",
	}

	// capturingProcess records what records it received so we can inspect.
	receivedRecords := make([]map[string]any, 0)
	var mu sync.Mutex
	capturingProcess := &capturingProcessConnector{
		connType: "demo",
		capture: func(records []map[string]any) {
			mu.Lock()
			defer mu.Unlock()
			receivedRecords = append(receivedRecords, records...)
		},
	}

	consumer := newFakeConsumer(msg)
	broker := &fakeBroker{}
	sink := newFakeSink("demo")

	reg := newFakeRegistry()
	reg.Register("datasource", &fakeDataSource{
		connType: "demo",
		records:  []map[string]any{{"id": "42", "name": "Bob"}},
	})
	reg.Register("process", capturingProcess)
	reg.Register("sink", sink)

	w := buildWorkerForExecution("exec-test-004", taskRepo, pipelineRepo, consumer, broker, reg)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_ = w.Run(ctx)

	mu.Lock()
	defer mu.Unlock()

	if len(receivedRecords) == 0 {
		t.Fatal("Process connector received no records")
	}

	rec := receivedRecords[0]
	if rec["user_id"] != "42" {
		t.Errorf("process received user_id = %v; want %q (schema mapping failed)", rec["user_id"], "42")
	}
	if rec["full_name"] != "Bob" {
		t.Errorf("process received full_name = %v; want %q (schema mapping failed)", rec["full_name"], "Bob")
	}
	if _, hasOldID := rec["id"]; hasOldID {
		t.Error("process received 'id' field; schema mapping should have renamed it to 'user_id'")
	}
}

// capturingProcessConnector calls a capture function with each record batch it receives.
type capturingProcessConnector struct {
	connType string
	capture  func([]map[string]any)
}

func (c *capturingProcessConnector) Type() string { return c.connType }
func (c *capturingProcessConnector) Transform(ctx context.Context, config map[string]any, records []map[string]any) ([]map[string]any, error) {
	c.capture(records)
	return records, nil
}

// TestExecuteTask_IdempotentSink_ErrAlreadyApplied_CompletesSuccessfully verifies
// that when the Sink returns ErrAlreadyApplied, the task is still marked completed
// (idempotent redelivery — ADR-003).
func TestExecuteTask_IdempotentSink_ErrAlreadyApplied_CompletesSuccessfully(t *testing.T) {
	pipelineID := uuid.New()
	taskID := uuid.New()

	pipeline := makePipeline(pipelineID, "demo", "demo", "demo")
	task := makeTask(taskID, pipelineID)

	taskRepo := newFakeTaskRepo(task)
	pipelineRepo := newFakePipelineRepo(pipeline)

	msg := &queue.TaskMessage{
		TaskID:      taskID.String(),
		PipelineID:  pipelineID.String(),
		UserID:      task.UserID.String(),
		ExecutionID: task.ExecutionID,
		StreamID:    "6666-0",
	}

	consumer := newFakeConsumer(msg)
	broker := &fakeBroker{}

	// Pre-populate the sink so Write returns ErrAlreadyApplied.
	sink := newFakeSink("demo")
	sink.written[task.ExecutionID] = []map[string]any{{"id": "pre-written"}}

	reg := newFakeRegistry()
	reg.Register("datasource", &fakeDataSource{
		connType: "demo",
		records:  []map[string]any{{"id": "1"}},
	})
	reg.Register("process", &fakeProcess{connType: "demo"})
	reg.Register("sink", sink)

	w := buildWorkerForExecution("exec-test-005", taskRepo, pipelineRepo, consumer, broker, reg)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_ = w.Run(ctx)

	got, _ := taskRepo.GetByID(context.Background(), taskID)
	if got == nil || got.Status != models.TaskStatusCompleted {
		t.Errorf("ErrAlreadyApplied should still complete the task, got status=%v", got)
	}
}

// TestExecuteTask_MissingPipeline_SetsFailedStatus verifies that when a pipeline
// cannot be loaded (e.g. deleted), the task is marked failed.
func TestExecuteTask_MissingPipeline_SetsFailedStatus(t *testing.T) {
	pipelineID := uuid.New()
	taskID := uuid.New()
	task := makeTask(taskID, pipelineID)

	taskRepo := newFakeTaskRepo(task)
	pipelineRepo := newFakePipelineRepo() // empty — pipeline not found

	msg := &queue.TaskMessage{
		TaskID:      taskID.String(),
		PipelineID:  pipelineID.String(),
		UserID:      task.UserID.String(),
		ExecutionID: task.ExecutionID,
		StreamID:    "5555-0",
	}

	consumer := newFakeConsumer(msg)
	broker := &fakeBroker{}
	reg := newFakeRegistry()

	w := buildWorkerForExecution("exec-test-006", taskRepo, pipelineRepo, consumer, broker, reg)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_ = w.Run(ctx)

	got, _ := taskRepo.GetByID(context.Background(), taskID)
	if got == nil || got.Status != models.TaskStatusFailed {
		t.Errorf("missing pipeline should set task to failed, got status=%v", got)
	}
}

// TestExecuteTask_SSEEventsEmitted verifies AC-9:
// task state change events are emitted to the broker on each transition.
func TestExecuteTask_SSEEventsEmitted(t *testing.T) {
	pipelineID := uuid.New()
	taskID := uuid.New()

	pipeline := makePipeline(pipelineID, "demo", "demo", "demo")
	task := makeTask(taskID, pipelineID)

	taskRepo := newFakeTaskRepo(task)
	pipelineRepo := newFakePipelineRepo(pipeline)

	msg := &queue.TaskMessage{
		TaskID:      taskID.String(),
		PipelineID:  pipelineID.String(),
		UserID:      task.UserID.String(),
		ExecutionID: task.ExecutionID,
		StreamID:    "4444-0",
	}

	consumer := newFakeConsumer(msg)
	broker := &fakeBroker{}
	sink := newFakeSink("demo")

	reg := newFakeRegistry()
	reg.Register("datasource", &fakeDataSource{
		connType: "demo",
		records:  []map[string]any{{"id": "1"}},
	})
	reg.Register("process", &fakeProcess{connType: "demo"})
	reg.Register("sink", sink)

	w := buildWorkerForExecution("exec-test-007", taskRepo, pipelineRepo, consumer, broker, reg)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_ = w.Run(ctx)

	// Expect events for: assigned, running, completed = at least 3 events.
	if broker.publishCount() < 3 {
		t.Errorf("expected at least 3 SSE events (assigned, running, completed), got %d", broker.publishCount())
	}

	// Last event should have status "completed".
	if broker.lastStatus() != models.TaskStatusCompleted {
		t.Errorf("last SSE event status = %q; want %q", broker.lastStatus(), models.TaskStatusCompleted)
	}
}

// TestRunConsumptionLoop_TagFiltering verifies AC-1:
// a worker with tag "etl" passes its tags to ReadTasks, filtering to queue:etl.
func TestRunConsumptionLoop_TagFiltering(t *testing.T) {
	cfg := &config.Config{
		WorkerID:   "tag-test-001",
		WorkerTags: []string{"etl"},
	}

	calledWithTags := make(chan []string, 1)
	consumer := &recordingConsumer{tags: calledWithTags}

	w := worker.NewWorkerWithPipelines(
		cfg,
		newFakeTaskRepo(),
		newFakeWorkerRepo(),
		newFakePipelineRepo(),
		consumer,
		newFakeHeartbeatStore(),
		nil, // broker
		newFakeRegistry(),
		nil, // cancellations
	)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_ = w.Run(ctx)

	select {
	case tags := <-calledWithTags:
		if len(tags) != 1 || tags[0] != "etl" {
			t.Errorf("ReadTasks called with tags=%v; want [etl]", tags)
		}
	default:
		t.Error("ReadTasks was not called — consumption loop did not start")
	}
}

// recordingConsumer records the tags passed to ReadTasks.
type recordingConsumer struct {
	tags chan []string
}

func (c *recordingConsumer) ReadTasks(ctx context.Context, consumerID string, tags []string, blockFor time.Duration) ([]*queue.TaskMessage, error) {
	select {
	case c.tags <- tags:
	default:
	}
	select {
	case <-ctx.Done():
		return nil, nil
	case <-time.After(10 * time.Millisecond):
		return []*queue.TaskMessage{}, nil
	}
}

func (c *recordingConsumer) Acknowledge(ctx context.Context, tag string, streamID string) error {
	return nil
}

func (c *recordingConsumer) InitGroups(ctx context.Context, tags []string) error {
	return nil
}
