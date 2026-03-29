// Package worker — WorkerChainEnqueuer implements ChainTaskEnqueuer.
// Creates and enqueues a new task targeting the next pipeline in a chain,
// using the same task submission flow as the API task handler.
// See: REQ-014, TASK-014
package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/nxlabs/nexusflow/internal/db"
	"github.com/nxlabs/nexusflow/internal/models"
	"github.com/nxlabs/nexusflow/internal/queue"
)

// WorkerChainEnqueuer implements ChainTaskEnqueuer.
// It creates a Task record in PostgreSQL and enqueues it on the Redis Streams queue,
// mirroring the task submission logic in the API layer.
//
// Thread-safety: WorkerChainEnqueuer is safe for concurrent use; it holds no mutable state.
// See: TASK-014
type WorkerChainEnqueuer struct {
	tasks    db.TaskRepository
	producer queue.Producer
	// pipelineDefaultTags specifies the stream tags used when submitting chain tasks.
	// Chain tasks must be routable to a worker that can handle them.
	// Using the single "demo" tag matches the existing walking-skeleton worker tags.
	pipelineDefaultTags []string
}

// NewWorkerChainEnqueuer constructs a WorkerChainEnqueuer.
//
// Args:
//
//	tasks:    TaskRepository for persisting the new task record.
//	producer: Producer for enqueuing the task onto Redis Streams.
//	tags:     The capability tags to route the chain task to. Must be non-empty.
//
// Preconditions:
//   - tasks and producer must be non-nil.
//   - tags must be non-empty.
func NewWorkerChainEnqueuer(tasks db.TaskRepository, producer queue.Producer, tags []string) *WorkerChainEnqueuer {
	if tasks == nil {
		panic("worker.NewWorkerChainEnqueuer: tasks must not be nil")
	}
	if producer == nil {
		panic("worker.NewWorkerChainEnqueuer: producer must not be nil")
	}
	if len(tags) == 0 {
		panic("worker.NewWorkerChainEnqueuer: tags must not be empty")
	}
	return &WorkerChainEnqueuer{
		tasks:               tasks,
		producer:            producer,
		pipelineDefaultTags: tags,
	}
}

// SubmitChainTask implements ChainTaskEnqueuer.SubmitChainTask.
// Creates a Task in PostgreSQL with status "submitted", advances it to "queued"
// via UpdateStatus, then enqueues it on Redis Streams.
//
// The new task carries:
//   - pipelineID: the next pipeline in the chain
//   - userID:     inherited from the triggering task (chain ownership)
//   - chainID:    the chain this task belongs to (for tracking)
//   - executionID: taskID + ":0" (first attempt, no retry yet)
//
// Postconditions:
//   - On success: task exists in PostgreSQL with status "queued"; message is on queue:{tag}.
//   - On failure: error returned; task may be partially created (best-effort cleanup not performed).
func (e *WorkerChainEnqueuer) SubmitChainTask(ctx context.Context, nextPipelineID uuid.UUID, userID uuid.UUID, chainID uuid.UUID) error {
	taskID := uuid.New()
	executionID := fmt.Sprintf("%s:0", taskID)

	task := &models.Task{
		ID:          taskID,
		PipelineID:  &nextPipelineID,
		UserID:      userID,
		Status:      models.TaskStatusSubmitted,
		RetryConfig: models.DefaultRetryConfig(),
		ExecutionID: executionID,
		Input:       map[string]any{},
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}

	created, err := e.tasks.Create(ctx, task)
	if err != nil {
		return fmt.Errorf("WorkerChainEnqueuer.SubmitChainTask: Create task: %w", err)
	}

	_, err = e.producer.Enqueue(ctx, &queue.ProducerMessage{
		Task: created,
		Tags: e.pipelineDefaultTags,
	})
	if err != nil {
		return fmt.Errorf("WorkerChainEnqueuer.SubmitChainTask: Enqueue task %s: %w", created.ID, err)
	}

	if err := e.tasks.UpdateStatus(ctx, created.ID, models.TaskStatusQueued, "chain trigger: enqueued for next pipeline", nil); err != nil {
		return fmt.Errorf("WorkerChainEnqueuer.SubmitChainTask: UpdateStatus to queued: %w", err)
	}

	return nil
}
