// Package worker_test — unit tests for TASK-012: worker cancellation signal handling.
// Verifies that the Worker checks the cancel:{taskID} Redis flag between pipeline
// phases and halts execution when the flag is set.
//
// Tests use in-memory fakes so no live Redis or PostgreSQL instance is required.
// See: REQ-010, TASK-012
package worker_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nxlabs/nexusflow/internal/config"
	"github.com/nxlabs/nexusflow/internal/models"
	"github.com/nxlabs/nexusflow/internal/queue"
	"github.com/nxlabs/nexusflow/worker"
)

// fakeCancellationStore is an in-memory CancellationStore double.
// Pre-loaded flags trigger the cancellation path in the Worker.
type fakeCancellationStore struct {
	flags map[string]bool
}

func newFakeCancellationStore() *fakeCancellationStore {
	return &fakeCancellationStore{flags: make(map[string]bool)}
}

func (s *fakeCancellationStore) SetCancelFlag(_ context.Context, taskID string, _ time.Duration) error {
	s.flags[taskID] = true
	return nil
}

func (s *fakeCancellationStore) CheckCancelFlag(_ context.Context, taskID string) (bool, error) {
	return s.flags[taskID], nil
}

// Compile-time assertion that fakeCancellationStore satisfies queue.CancellationStore.
var _ queue.CancellationStore = (*fakeCancellationStore)(nil)

// buildCancellationWorker constructs a Worker wired with a pipeline, task repo,
// connector registry, and a pre-loaded cancellation store.
func buildCancellationWorker(
	taskRepo *fakeTaskRepo,
	pipelineRepo *fakePipelineRepo,
	reg worker.ConnectorRegistry,
	cs *fakeCancellationStore,
) *worker.Worker {
	cfg := &config.Config{
		WorkerID:   "cancel-worker-001",
		WorkerTags: []string{"demo"},
	}
	return worker.NewWorkerWithPipelines(
		cfg,
		taskRepo,
		newFakeWorkerRepo(),
		pipelineRepo,
		nil, // consumer — not needed for executeTask tests
		newFakeHeartbeatStore(),
		nil, // broker
		reg,
		cs,
	)
}

// TestWorker_CancellationFlagHaltsExecutionAfterDataSource verifies acceptance criterion 5:
// when cancel:{taskID} is set, the Worker does not proceed to the Process phase and
// transitions the task to "failed" (domain error path) rather than "completed".
//
// The test sets the cancel flag before execution begins so the check between
// DataSource and Process detects it and returns a domain error.
func TestWorker_CancellationFlagHaltsExecutionAfterDataSource(t *testing.T) {
	taskID := uuid.New()
	pipelineID := uuid.New()

	task := &models.Task{
		ID:          taskID,
		PipelineID:  &pipelineID,
		UserID:      uuid.New(),
		Status:      models.TaskStatusQueued,
		RetryConfig: models.DefaultRetryConfig(),
		ExecutionID: taskID.String() + ":0",
		Input:       map[string]any{},
	}

	pipeline := &models.Pipeline{
		ID:     pipelineID,
		UserID: task.UserID,
		DataSourceConfig: models.DataSourceConfig{
			ConnectorType: "demo",
			Config:        map[string]any{},
		},
		ProcessConfig: models.ProcessConfig{
			ConnectorType: "demo",
			Config:        map[string]any{},
		},
		SinkConfig: models.SinkConfig{
			ConnectorType: "demo",
			Config:        map[string]any{},
		},
	}

	taskRepo := newFakeTaskRepo(task)
	pipelineRepo := newFakePipelineRepo(pipeline)

	reg := worker.NewDefaultConnectorRegistry()
	worker.RegisterDemoConnectors(reg)

	cs := newFakeCancellationStore()
	// Pre-set the cancel flag so the worker detects it after DataSource.
	cs.flags[taskID.String()] = true

	w := buildCancellationWorker(taskRepo, pipelineRepo, reg, cs)

	msg := &queue.TaskMessage{
		TaskID:      taskID.String(),
		PipelineID:  pipelineID.String(),
		StreamID:    "1-0",
		ExecutionID: task.ExecutionID,
	}

	w.ExecuteTaskForTest(context.Background(), msg)

	// The task must not be "completed" — it was cancelled mid-execution.
	// The Worker marks it "failed" because the cancellation is detected as a domain error.
	transitions := taskRepo.getTransitions(taskID)

	var reachedCompleted bool
	var reachedFailed bool
	for _, tr := range transitions {
		if tr.newStatus == models.TaskStatusCompleted {
			reachedCompleted = true
		}
		if tr.newStatus == models.TaskStatusFailed {
			reachedFailed = true
		}
	}

	if reachedCompleted {
		t.Error("task reached 'completed' despite cancellation flag being set")
	}
	if !reachedFailed {
		t.Error("expected task to transition to 'failed' when cancellation flag is set")
	}
}

// TestWorker_NoCancellationFlagCompletesNormally verifies that when no cancel flag
// is set, the Worker completes the task successfully.
func TestWorker_NoCancellationFlagCompletesNormally(t *testing.T) {
	taskID := uuid.New()
	pipelineID := uuid.New()

	task := &models.Task{
		ID:          taskID,
		PipelineID:  &pipelineID,
		UserID:      uuid.New(),
		Status:      models.TaskStatusQueued,
		RetryConfig: models.DefaultRetryConfig(),
		ExecutionID: taskID.String() + ":0",
		Input:       map[string]any{},
	}

	pipeline := &models.Pipeline{
		ID:     pipelineID,
		UserID: task.UserID,
		DataSourceConfig: models.DataSourceConfig{
			ConnectorType: "demo",
			Config:        map[string]any{},
		},
		ProcessConfig: models.ProcessConfig{
			ConnectorType: "demo",
			Config:        map[string]any{},
		},
		SinkConfig: models.SinkConfig{
			ConnectorType: "demo",
			Config:        map[string]any{},
		},
	}

	taskRepo := newFakeTaskRepo(task)
	pipelineRepo := newFakePipelineRepo(pipeline)

	reg := worker.NewDefaultConnectorRegistry()
	worker.RegisterDemoConnectors(reg)

	cs := newFakeCancellationStore() // no flags set

	w := buildCancellationWorker(taskRepo, pipelineRepo, reg, cs)

	msg := &queue.TaskMessage{
		TaskID:      taskID.String(),
		PipelineID:  pipelineID.String(),
		StreamID:    "1-0",
		ExecutionID: task.ExecutionID,
	}

	w.ExecuteTaskForTest(context.Background(), msg)

	transitions := taskRepo.getTransitions(taskID)

	var reachedCompleted bool
	for _, tr := range transitions {
		if tr.newStatus == models.TaskStatusCompleted {
			reachedCompleted = true
		}
	}

	if !reachedCompleted {
		t.Error("expected task to reach 'completed' when no cancellation flag is set")
	}
}

// TestWorker_NilCancellationStoreCompletesNormally verifies that when cancellations
// is nil (e.g. in environments without Redis), the Worker completes tasks normally
// and does not panic.
func TestWorker_NilCancellationStoreCompletesNormally(t *testing.T) {
	taskID := uuid.New()
	pipelineID := uuid.New()

	task := &models.Task{
		ID:          taskID,
		PipelineID:  &pipelineID,
		UserID:      uuid.New(),
		Status:      models.TaskStatusQueued,
		RetryConfig: models.DefaultRetryConfig(),
		ExecutionID: taskID.String() + ":0",
		Input:       map[string]any{},
	}

	pipeline := &models.Pipeline{
		ID:     pipelineID,
		UserID: task.UserID,
		DataSourceConfig: models.DataSourceConfig{
			ConnectorType: "demo",
			Config:        map[string]any{},
		},
		ProcessConfig: models.ProcessConfig{
			ConnectorType: "demo",
			Config:        map[string]any{},
		},
		SinkConfig: models.SinkConfig{
			ConnectorType: "demo",
			Config:        map[string]any{},
		},
	}

	taskRepo := newFakeTaskRepo(task)
	pipelineRepo := newFakePipelineRepo(pipeline)

	reg := worker.NewDefaultConnectorRegistry()
	worker.RegisterDemoConnectors(reg)

	cfg := &config.Config{
		WorkerID:   "cancel-worker-nil",
		WorkerTags: []string{"demo"},
	}
	w := worker.NewWorkerWithPipelines(
		cfg,
		taskRepo,
		newFakeWorkerRepo(),
		pipelineRepo,
		nil,
		newFakeHeartbeatStore(),
		nil,
		reg,
		nil, // nil CancellationStore — pass-through
	)

	msg := &queue.TaskMessage{
		TaskID:      taskID.String(),
		PipelineID:  pipelineID.String(),
		StreamID:    "1-0",
		ExecutionID: task.ExecutionID,
	}

	w.ExecuteTaskForTest(context.Background(), msg)

	transitions := taskRepo.getTransitions(taskID)
	var reachedCompleted bool
	for _, tr := range transitions {
		if tr.newStatus == models.TaskStatusCompleted {
			reachedCompleted = true
		}
	}

	if !reachedCompleted {
		t.Error("expected task to reach 'completed' with nil CancellationStore")
	}
}
