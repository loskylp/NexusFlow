// Package worker — ChainTrigger fires downstream chain tasks on task completion.
// When a task for pipeline A in a chain completes, the trigger automatically
// submits a new task for the next pipeline (B) in the chain.
//
// Idempotency (ADR-003): the trigger is guarded by a Redis SET-NX keyed on
// "chain-trigger:{taskID}:{nextPipelineID}". Duplicate completion events for
// the same task do not create duplicate downstream tasks.
//
// See: REQ-014, ADR-003, TASK-014
package worker

import (
	"context"
	"fmt"
	"log"

	"github.com/google/uuid"
	"github.com/nxlabs/nexusflow/internal/db"
)

// ChainTaskEnqueuer submits a new task for the next pipeline in a chain.
// This interface isolates the chain trigger from the full task submission machinery,
// enabling clean unit testing of trigger logic.
// See: TASK-014
type ChainTaskEnqueuer interface {
	// SubmitChainTask creates and enqueues a new task targeting nextPipelineID.
	// The new task inherits the original task's userID and is associated with chainID.
	//
	// Args:
	//   ctx:            Request context.
	//   nextPipelineID: The pipeline that the new task will execute.
	//   userID:         The user who owns the chain (from the triggering task).
	//   chainID:        The chain the new task belongs to (for tracking).
	//
	// Postconditions:
	//   - On success: a new task is persisted in PostgreSQL and enqueued in Redis Streams.
	SubmitChainTask(ctx context.Context, nextPipelineID uuid.UUID, userID uuid.UUID, chainID uuid.UUID) error
}

// ChainIdempotencyStore provides SET-NX semantics for chain trigger deduplication.
// Each call attempts to acquire a unique key; the first call for a key succeeds,
// subsequent calls for the same key return false without side effects.
// See: ADR-003, TASK-014
type ChainIdempotencyStore interface {
	// SetNX atomically sets key if it does not already exist (Redis SET-NX).
	// Returns true when the key was set (first call), false when it already exists.
	//
	// Args:
	//   ctx: Request context.
	//   key: The unique idempotency key.
	//
	// Returns:
	//   true  — key was absent; caller should proceed with the side effect.
	//   false — key already exists; caller should skip the side effect (duplicate).
	//   error — Redis connectivity failure; caller must not proceed.
	SetNX(ctx context.Context, key string) (bool, error)
}

// ChainTrigger fires downstream pipeline tasks when a task completes.
// It is designed to be called from the worker's task completion path.
//
// Thread-safety: ChainTrigger is safe for concurrent use; it holds no mutable state.
// See: REQ-014, ADR-003, TASK-014
type ChainTrigger struct {
	chains      db.ChainRepository
	enqueuer    ChainTaskEnqueuer
	idempotency ChainIdempotencyStore
}

// NewChainTrigger constructs a ChainTrigger with all required dependencies.
//
// Args:
//
//	chains:      ChainRepository for chain and step lookup.
//	enqueuer:    ChainTaskEnqueuer for submitting the downstream task.
//	idempotency: ChainIdempotencyStore for SET-NX idempotency guard.
//
// Preconditions:
//   - All arguments must be non-nil.
func NewChainTrigger(chains db.ChainRepository, enqueuer ChainTaskEnqueuer, idempotency ChainIdempotencyStore) *ChainTrigger {
	if chains == nil {
		panic("worker.NewChainTrigger: chains must not be nil")
	}
	if enqueuer == nil {
		panic("worker.NewChainTrigger: enqueuer must not be nil")
	}
	if idempotency == nil {
		panic("worker.NewChainTrigger: idempotency must not be nil")
	}
	return &ChainTrigger{
		chains:      chains,
		enqueuer:    enqueuer,
		idempotency: idempotency,
	}
}

// OnTaskCompleted is called after a task transitions to "completed".
// It checks whether the task's pipeline is in a chain; if so, it submits
// a task for the next pipeline in the chain, guarded by an idempotency key.
//
// Args:
//
//	ctx:        Execution context.
//	taskID:     The ID of the just-completed task (used as the idempotency key component).
//	pipelineID: The pipeline that the completed task ran.
//	userID:     The user who owns the task (forwarded to the new task).
//
// Postconditions:
//   - When the pipeline is in a chain and is not the last step: a new task is submitted
//     for the next pipeline, provided the idempotency key has not been set before.
//   - When the pipeline is not in any chain, or is the last step: no side effect.
//   - When the idempotency key already exists: no side effect (duplicate suppressed).
func (t *ChainTrigger) OnTaskCompleted(ctx context.Context, taskID uuid.UUID, pipelineID uuid.UUID, userID uuid.UUID) error {
	// Look up the chain that contains this pipeline.
	chain, err := t.chains.FindByPipeline(ctx, pipelineID)
	if err != nil {
		return fmt.Errorf("ChainTrigger.OnTaskCompleted: FindByPipeline(%v): %w", pipelineID, err)
	}
	if chain == nil {
		return nil // pipeline is not in any chain — no action
	}

	// Determine the next pipeline in the chain.
	nextPipelineID, err := t.chains.GetNextPipeline(ctx, chain.ID, pipelineID)
	if err != nil {
		return fmt.Errorf("ChainTrigger.OnTaskCompleted: GetNextPipeline(chain=%v, pipeline=%v): %w", chain.ID, pipelineID, err)
	}
	if nextPipelineID == nil {
		return nil // pipeline is the last step — chain is complete
	}

	// Idempotency guard (ADR-003): SET-NX "chain-trigger:{taskID}:{nextPipelineID}".
	// If the key already exists, a previous execution already submitted this downstream task.
	key := chainTriggerKey(taskID, *nextPipelineID)
	acquired, err := t.idempotency.SetNX(ctx, key)
	if err != nil {
		return fmt.Errorf("ChainTrigger.OnTaskCompleted: idempotency SetNX(%q): %w", key, err)
	}
	if !acquired {
		log.Printf("worker: chain trigger: idempotency guard activated for key=%q — skipping duplicate submission", key)
		return nil
	}

	// Submit the downstream task.
	if err := t.enqueuer.SubmitChainTask(ctx, *nextPipelineID, userID, chain.ID); err != nil {
		return fmt.Errorf("ChainTrigger.OnTaskCompleted: SubmitChainTask(pipeline=%v): %w", *nextPipelineID, err)
	}

	log.Printf("worker: chain trigger: submitted downstream task for chain=%v pipeline=%v", chain.ID, *nextPipelineID)
	return nil
}

// chainTriggerKey builds the Redis idempotency key for a chain trigger event.
// Format: "chain-trigger:{taskID}:{nextPipelineID}"
// The key is unique per (task, next pipeline) pair, ensuring that re-delivery
// of the same completion event cannot submit a second downstream task.
func chainTriggerKey(taskID uuid.UUID, nextPipelineID uuid.UUID) string {
	return fmt.Sprintf("chain-trigger:%s:%s", taskID, nextPipelineID)
}
