// Package worker implements the NexusFlow Worker process.
// A Worker self-registers with the system on startup, emits heartbeats every 5 seconds,
// and runs a blocking task consumption loop for each of its capability tags.
//
// Task execution flow (ADR-009, TASK-007):
//  1. XREADGROUP on queue:{tag} for each tag
//  2. Update task status: assigned -> running
//  3. Execute DataSource phase
//  4. Apply schema mapping (DataSource output -> Process input)
//  5. Execute Process phase
//  6. Apply schema mapping (Process output -> Sink input)
//  7. Execute Sink phase (with atomicity wrapper and Before/After snapshot)
//  8. Update task status: running -> completed (or failed)
//  9. XACK the message
//
// See: ADR-001, ADR-002, ADR-003, ADR-009, TASK-006, TASK-007
package worker

import (
	"context"
	"log"
	"time"

	"github.com/nxlabs/nexusflow/internal/config"
	"github.com/nxlabs/nexusflow/internal/db"
	"github.com/nxlabs/nexusflow/internal/models"
	"github.com/nxlabs/nexusflow/internal/queue"
	"github.com/nxlabs/nexusflow/internal/sse"
)

// Worker is the main struct for the NexusFlow worker process.
// A single Worker instance manages registration, heartbeat, and task execution.
type Worker struct {
	cfg       *config.Config
	tasks     db.TaskRepository
	workers   db.WorkerRepository
	consumer  queue.Consumer
	heartbeat queue.HeartbeatStore
	broker    sse.Broker
	connectors ConnectorRegistry
}

// NewWorker constructs a Worker with all required dependencies.
//
// Args:
//
//	cfg:        Runtime configuration (WorkerTags and WorkerID are required).
//	tasks:      TaskRepository for status transitions.
//	workers:    WorkerRepository for registration and status updates.
//	consumer:   Queue Consumer for XREADGROUP and XACK.
//	heartbeat:  HeartbeatStore for writing to workers:active.
//	broker:     SSE Broker for publishing task and log events.
//	connectors: Registry of available DataSource, Process, and Sink connectors.
//
// Preconditions:
//   - cfg.WorkerTags is non-empty.
//   - cfg.WorkerID is a non-empty unique string.
func NewWorker(
	cfg *config.Config,
	tasks db.TaskRepository,
	workers db.WorkerRepository,
	consumer queue.Consumer,
	heartbeat queue.HeartbeatStore,
	broker sse.Broker,
	connectors ConnectorRegistry,
) *Worker {
	return &Worker{
		cfg:        cfg,
		tasks:      tasks,
		workers:    workers,
		consumer:   consumer,
		heartbeat:  heartbeat,
		broker:     broker,
		connectors: connectors,
	}
}

// Run starts the worker process. Blocks until ctx is cancelled.
// Performs in order:
//  1. Self-registration (Register)
//  2. Consumer group initialization (Consumer.InitGroups) — when consumer is non-nil
//  3. Heartbeat goroutine (emitHeartbeats)
//  4. Task consumption loop (runConsumptionLoop) — when consumer is non-nil
//
// On ctx cancellation: heartbeat goroutine stops; the worker is marked "down" in the
// repository and removed from workers:active before Run returns.
//
// Postconditions:
//   - On exit: worker status is "down" in PostgreSQL.
func (w *Worker) Run(ctx context.Context) error {
	if err := w.Register(ctx); err != nil {
		return err
	}

	// Initialize consumer groups for all tags so the worker can start reading.
	if w.consumer != nil && len(w.cfg.WorkerTags) > 0 {
		if err := w.consumer.InitGroups(ctx, w.cfg.WorkerTags); err != nil {
			return err
		}
	}

	// Heartbeat loop runs in its own goroutine.
	go w.emitHeartbeats(ctx)

	// Task consumption loop (TASK-007). Blocks until ctx is cancelled.
	if w.consumer != nil {
		w.runConsumptionLoop(ctx)
	} else {
		// No consumer wired (e.g. unit tests): wait for cancellation.
		<-ctx.Done()
	}

	// Graceful shutdown: mark the worker offline and stop emitting heartbeats.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	w.markOffline(shutdownCtx)

	return nil
}

// Register inserts or updates the worker record in PostgreSQL and records an initial
// heartbeat in Redis (workers:active). Called once on startup inside Run.
//
// Postconditions:
//   - On success: worker exists in PostgreSQL with status "online" and correct tags.
//   - On success: workers:active contains the worker's entry with the current timestamp.
func (w *Worker) Register(ctx context.Context) error {
	now := time.Now().UTC()
	worker := &models.Worker{
		ID:            w.cfg.WorkerID,
		Tags:          w.cfg.WorkerTags,
		Status:        models.WorkerStatusOnline,
		LastHeartbeat: now,
		RegisteredAt:  now,
	}

	if w.workers != nil {
		if _, err := w.workers.Register(ctx, worker); err != nil {
			return err
		}
	}

	// Record the initial heartbeat so the worker appears in workers:active immediately.
	if w.heartbeat != nil {
		if err := w.heartbeat.RecordHeartbeat(ctx, w.cfg.WorkerID); err != nil {
			return err
		}
	}

	log.Printf("worker: registered id=%q tags=%v", w.cfg.WorkerID, w.cfg.WorkerTags)
	return nil
}

// emitHeartbeats writes the worker's liveness signal to workers:active on the configured
// HeartbeatInterval (default 5 seconds). Runs in its own goroutine. Stops on ctx cancellation.
//
// Postconditions:
//   - On each tick: workers:active entry for this worker has score = current Unix timestamp.
//   - On ctx done: last heartbeat may be stale; Monitor will detect and mark down after timeout.
func (w *Worker) emitHeartbeats(ctx context.Context) {
	interval := w.cfg.HeartbeatInterval
	if interval <= 0 {
		interval = 5 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if w.heartbeat != nil {
				if err := w.heartbeat.RecordHeartbeat(ctx, w.cfg.WorkerID); err != nil {
					log.Printf("worker: heartbeat error: %v", err)
				}
			}
		}
	}
}

// markOffline sets the worker's status to "down" in PostgreSQL.
// Called during graceful shutdown so the Monitor does not attempt to reclaim tasks
// from a worker that has stopped intentionally.
//
// Args:
//
//	ctx: A short-lived context (typically 5 seconds); the DB call must complete quickly.
func (w *Worker) markOffline(ctx context.Context) {
	if w.workers == nil {
		return
	}
	if err := w.workers.UpdateStatus(ctx, w.cfg.WorkerID, models.WorkerStatusDown); err != nil {
		log.Printf("worker: failed to mark offline: %v", err)
	}
}

// runConsumptionLoop performs a blocking XREADGROUP on all tag streams and dispatches
// each received TaskMessage to executeTask. Runs indefinitely until ctx is cancelled.
//
// Complexity note: This is the hot path of the worker. Goroutine management,
// XREADGROUP blocking timeout, and error handling must all be correct here.
//
// See: TASK-007
func (w *Worker) runConsumptionLoop(ctx context.Context) {
	// TODO: Implement in TASK-007
	<-ctx.Done()
}

// executeTask runs the full three-phase pipeline for a single TaskMessage.
// Updates task status transitions in PostgreSQL and publishes SSE events after each transition.
//
// Args:
//
//	ctx:     Request context; cancellation mid-execution results in task failure.
//	message: The task message read from XREADGROUP.
//
// Complexity signal: HIGH
// This is the most complex single method in the system. It integrates:
//   - Two PostgreSQL status transitions (assigned -> running)
//   - DataSource, Process, and Sink phase execution with schema mapping at each boundary
//   - Sink atomicity wrapper and Before/After snapshot capture (ADR-009)
//   - Execution ID idempotency check at the Sink boundary (ADR-003)
//   - XACK on success or terminal failure
//   - SSE event publication after each state transition
//   - Log line production via broker.PublishLogLine
//
// Postconditions:
//   - On success: task.Status = "completed"; message is XACKed.
//   - On Process error: task.Status = "failed"; message is XACKed; no retry.
//   - On infrastructure error: task.Status = "failed"; message NOT XACKed (eligible for XCLAIM by Monitor).
//
//lint:ignore U1000 scaffold stub — wired in TASK-007
func (w *Worker) executeTask(ctx context.Context, message *queue.TaskMessage) {
	// TODO: Implement in TASK-007
	panic("not implemented")
}

// applySchemaMapping transforms a data record from one phase's output schema
// to the next phase's input schema according to the provided SchemaMapping list.
//
// Args:
//
//	data:     The output record from the preceding phase (DataSource or Process).
//	mappings: The schema mappings defined in the Pipeline.
//
// Returns:
//
//	A new map with fields renamed according to mappings.
//	An error if any mapping references a SourceField that does not exist in data.
//
// Preconditions:
//   - data is a non-nil map.
//   - mappings is the slice from Pipeline.ProcessConfig.InputMappings or SinkConfig.InputMappings.
//
// Postconditions:
//   - On success: returned map contains exactly the fields specified in mappings (by TargetField).
//   - On error: the task must be marked "failed" by the caller; the error reason is logged.
//
//lint:ignore U1000 scaffold stub — wired in TASK-007
func (w *Worker) applySchemaMapping(data map[string]any, mappings []models.SchemaMapping) (map[string]any, error) {
	// TODO: Implement in TASK-007
	panic("not implemented")
}
