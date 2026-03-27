// Package worker implements the NexusFlow Worker process.
// A Worker self-registers with the system on startup, emits heartbeats every 5 seconds,
// and runs a blocking task consumption loop for each of its capability tags.
//
// Task execution flow (ADR-009, TASK-007):
//  1. XREADGROUP on queue:{tag} for each tag
//  2. Update task status: queued -> assigned
//  3. Update task status: assigned -> running
//  4. Execute DataSource phase
//  5. Apply schema mapping (DataSource output -> Process input)
//  6. Execute Process phase
//  7. Apply schema mapping (Process output -> Sink input)
//  8. Execute Sink phase (with atomicity wrapper and Before/After snapshot)
//  9. Update task status: running -> completed (or failed)
// 10. XACK the message
//
// See: ADR-001, ADR-002, ADR-003, ADR-009, TASK-006, TASK-007
package worker

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/nxlabs/nexusflow/internal/config"
	"github.com/nxlabs/nexusflow/internal/db"
	"github.com/nxlabs/nexusflow/internal/models"
	"github.com/nxlabs/nexusflow/internal/queue"
)


// Worker is the main struct for the NexusFlow worker process.
// A single Worker instance manages registration, heartbeat, and task execution.
type Worker struct {
	cfg        *config.Config
	tasks      db.TaskRepository
	workers    db.WorkerRepository
	pipelines  db.PipelineRepository
	consumer   queue.Consumer
	heartbeat  queue.HeartbeatStore
	broker     TaskEventBroker
	connectors ConnectorRegistry
}

// NewWorker constructs a Worker with all required dependencies.
// PipelineRepository is omitted (nil) — use NewWorkerWithPipelines when pipeline
// execution is required (TASK-007).
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
	broker TaskEventBroker,
	connectors ConnectorRegistry,
) *Worker {
	return &Worker{
		cfg:        cfg,
		tasks:      tasks,
		workers:    workers,
		pipelines:  nil,
		consumer:   consumer,
		heartbeat:  heartbeat,
		broker:     broker,
		connectors: connectors,
	}
}

// NewWorkerWithPipelines constructs a Worker with all dependencies including a
// PipelineRepository, which is required for pipeline execution (TASK-007).
//
// Args:
//
//	cfg:        Runtime configuration (WorkerTags and WorkerID are required).
//	tasks:      TaskRepository for status transitions.
//	workers:    WorkerRepository for registration and status updates.
//	pipelines:  PipelineRepository for loading pipeline definitions.
//	consumer:   Queue Consumer for XREADGROUP and XACK.
//	heartbeat:  HeartbeatStore for writing to workers:active.
//	broker:     SSE Broker for publishing task and log events. May be nil.
//	connectors: Registry of available DataSource, Process, and Sink connectors.
//
// Preconditions:
//   - cfg.WorkerTags is non-empty.
//   - cfg.WorkerID is a non-empty unique string.
func NewWorkerWithPipelines(
	cfg *config.Config,
	tasks db.TaskRepository,
	workers db.WorkerRepository,
	pipelines db.PipelineRepository,
	consumer queue.Consumer,
	heartbeat queue.HeartbeatStore,
	broker TaskEventBroker,
	connectors ConnectorRegistry,
) *Worker {
	return &Worker{
		cfg:        cfg,
		tasks:      tasks,
		workers:    workers,
		pipelines:  pipelines,
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
// The loop blocks for up to 1 second on each ReadTasks call, then re-checks the
// context. This ensures clean shutdown without hanging for an arbitrarily long time.
//
// See: TASK-007, ADR-001, ADR-003
func (w *Worker) runConsumptionLoop(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}

		messages, err := w.consumer.ReadTasks(ctx, w.cfg.WorkerID, w.cfg.WorkerTags, time.Second)
		if err != nil {
			log.Printf("worker: ReadTasks error: %v", err)
			// Brief pause before retrying so we don't spin-loop on persistent errors.
			select {
			case <-ctx.Done():
				return
			case <-time.After(500 * time.Millisecond):
			}
			continue
		}

		// ReadTasks returns nil when ctx is cancelled.
		if messages == nil {
			return
		}

		for _, msg := range messages {
			if ctx.Err() != nil {
				return
			}
			w.executeTask(ctx, msg)
		}
	}
}

// executeTask runs the full three-phase pipeline for a single TaskMessage.
// Updates task status transitions in PostgreSQL and publishes SSE events after each transition.
//
// State machine: queued -> assigned -> running -> completed | failed
//
// Connector errors and schema mapping errors are terminal domain failures: the task is
// marked "failed" and the message is XACKed so it leaves the pending list without
// Monitor involvement (ADR-003, Domain Invariant 2: process errors do not retry).
//
// Infrastructure errors (DB, Redis) are logged; the message is NOT XACKed so the Monitor
// can XCLAIM and retry via a healthy worker (ADR-003 at-least-once guarantee).
//
// Args:
//
//	ctx:     Execution context; cancellation mid-execution results in task failure.
//	message: The task message read from XREADGROUP.
//
// Postconditions:
//   - On success: task.Status = "completed"; message is XACKed.
//   - On domain error (connector, schema): task.Status = "failed"; message is XACKed.
//   - On infrastructure error: task.Status = "failed" if reachable; message NOT XACKed.
func (w *Worker) executeTask(ctx context.Context, message *queue.TaskMessage) {
	taskID, err := uuid.Parse(message.TaskID)
	if err != nil {
		log.Printf("worker: malformed taskID %q in message %q: %v", message.TaskID, message.StreamID, err)
		return
	}

	// Transition 1: queued -> assigned
	workerID := w.cfg.WorkerID
	if transErr := w.transitionStatus(ctx, taskID, models.TaskStatusAssigned, "assigned to worker "+workerID, &workerID); transErr != nil {
		log.Printf("worker: task %s: transition to assigned failed: %v — leaving message pending for Monitor", taskID, transErr)
		return // Do NOT ack — leave for Monitor XCLAIM.
	}

	pipelineErr := w.runPipeline(ctx, message, taskID)

	if pipelineErr != nil {
		// Domain and infrastructure errors both result in "failed" status.
		// The message ack decision depends on error kind.
		failErr := w.transitionStatus(ctx, taskID, models.TaskStatusFailed, pipelineErr.Error(), &workerID)
		if failErr != nil {
			log.Printf("worker: task %s: could not mark failed: %v", taskID, failErr)
		}

		if isDomainError(pipelineErr) {
			// Domain errors (connector failure, schema mapping): ack so no retry.
			w.ackMessage(ctx, message)
		} else {
			// Infrastructure errors: leave unacked for Monitor XCLAIM + retry.
			log.Printf("worker: task %s: infrastructure error — leaving message pending for Monitor", taskID)
		}
		return
	}

	// Transition: running -> completed
	if transErr := w.transitionStatus(ctx, taskID, models.TaskStatusCompleted, "pipeline completed successfully", &workerID); transErr != nil {
		log.Printf("worker: task %s: transition to completed failed: %v", taskID, transErr)
		// Data was written; best-effort: do NOT ack so Monitor can verify.
		return
	}

	w.ackMessage(ctx, message)
}

// runPipeline loads the pipeline definition and executes all three phases.
// Returns a domainError for connector/schema failures, or an error for infrastructure failures.
//
// Transition 2: assigned -> running occurs at the start of this function.
//
// Returns:
//   - nil on full success (all three phases completed).
//   - *domainErrorWrapper for connector errors, schema errors, or missing pipeline.
//   - raw error for DB/Redis failures (infrastructure errors).
func (w *Worker) runPipeline(ctx context.Context, message *queue.TaskMessage, taskID uuid.UUID) error {
	workerID := w.cfg.WorkerID

	// Transition 2: assigned -> running
	if transErr := w.transitionStatus(ctx, taskID, models.TaskStatusRunning, "pipeline execution started", &workerID); transErr != nil {
		return transErr // infrastructure error
	}

	// Load pipeline definition.
	pipeline, err := w.loadPipeline(ctx, message)
	if err != nil {
		return &domainErrorWrapper{cause: err} // domain: pipeline missing or malformed
	}

	// Phase 1: DataSource
	dsRecords, err := w.runDataSource(ctx, pipeline, message)
	if err != nil {
		return &domainErrorWrapper{cause: err}
	}

	// Apply DataSource -> Process schema mapping.
	processRecords, err := w.applyMappingsToSlice(dsRecords, pipeline.ProcessConfig.InputMappings)
	if err != nil {
		return &domainErrorWrapper{cause: fmt.Errorf("DataSource->Process schema mapping: %w", err)}
	}

	// Phase 2: Process
	transformedRecords, err := w.runProcess(ctx, pipeline, processRecords)
	if err != nil {
		return &domainErrorWrapper{cause: err}
	}

	// Apply Process -> Sink schema mapping.
	sinkRecords, err := w.applyMappingsToSlice(transformedRecords, pipeline.SinkConfig.InputMappings)
	if err != nil {
		return &domainErrorWrapper{cause: fmt.Errorf("Process->Sink schema mapping: %w", err)}
	}

	// Phase 3: Sink
	if err := w.runSink(ctx, taskID, pipeline, sinkRecords, message.ExecutionID); err != nil {
		return &domainErrorWrapper{cause: err}
	}

	return nil
}

// loadPipeline retrieves the Pipeline definition referenced by the TaskMessage.
// Returns a domain error when the pipeline does not exist (e.g. deleted after task submission).
//
// Preconditions:
//   - w.pipelines must be non-nil; panics otherwise.
func (w *Worker) loadPipeline(ctx context.Context, message *queue.TaskMessage) (*models.Pipeline, error) {
	if w.pipelines == nil {
		return nil, fmt.Errorf("pipeline repository not configured")
	}

	pipelineID, err := uuid.Parse(message.PipelineID)
	if err != nil {
		return nil, fmt.Errorf("malformed pipelineID %q: %w", message.PipelineID, err)
	}

	pipeline, err := w.pipelines.GetByID(ctx, pipelineID)
	if err != nil {
		return nil, fmt.Errorf("load pipeline %s: %w", pipelineID, err)
	}
	if pipeline == nil {
		return nil, fmt.Errorf("pipeline %s not found (deleted after task submission)", pipelineID)
	}
	return pipeline, nil
}

// runDataSource executes Phase 1: fetches data from the DataSource connector.
func (w *Worker) runDataSource(ctx context.Context, pipeline *models.Pipeline, message *queue.TaskMessage) ([]map[string]any, error) {
	connector, err := w.resolveDataSource(pipeline.DataSourceConfig.ConnectorType)
	if err != nil {
		return nil, err
	}

	// Load the task's input parameters for the DataSource.
	input, err := w.loadTaskInput(ctx, message)
	if err != nil {
		return nil, err
	}

	records, err := connector.Fetch(ctx, pipeline.DataSourceConfig.Config, input)
	if err != nil {
		return nil, fmt.Errorf("DataSource(%s).Fetch: %w", pipeline.DataSourceConfig.ConnectorType, err)
	}
	return records, nil
}

// runProcess executes Phase 2: transforms records using the Process connector.
func (w *Worker) runProcess(ctx context.Context, pipeline *models.Pipeline, records []map[string]any) ([]map[string]any, error) {
	connector, err := w.resolveProcess(pipeline.ProcessConfig.ConnectorType)
	if err != nil {
		return nil, err
	}

	transformed, err := connector.Transform(ctx, pipeline.ProcessConfig.Config, records)
	if err != nil {
		return nil, fmt.Errorf("Process(%s).Transform: %w", pipeline.ProcessConfig.ConnectorType, err)
	}
	return transformed, nil
}

// runSink executes Phase 3: writes records to the Sink connector with idempotency guard.
// ErrAlreadyApplied is treated as a successful no-op (ADR-003: idempotent redelivery).
func (w *Worker) runSink(ctx context.Context, taskID uuid.UUID, pipeline *models.Pipeline, records []map[string]any, executionID string) error {
	connector, err := w.resolveSink(pipeline.SinkConfig.ConnectorType)
	if err != nil {
		return err
	}

	if writeErr := connector.Write(ctx, pipeline.SinkConfig.Config, records, executionID); writeErr != nil {
		if errors.Is(writeErr, ErrAlreadyApplied) {
			// Idempotent redelivery: this executionID was already committed. Treat as success (ADR-003).
			log.Printf("worker: task %s: Sink.Write: ErrAlreadyApplied for executionID=%q — treating as no-op", taskID, executionID)
			return nil
		}
		return fmt.Errorf("Sink(%s).Write: %w", pipeline.SinkConfig.ConnectorType, writeErr)
	}
	return nil
}

// loadTaskInput retrieves the Task's input map from PostgreSQL.
// Returns an empty map when tasks repository is nil (test mode).
func (w *Worker) loadTaskInput(ctx context.Context, message *queue.TaskMessage) (map[string]any, error) {
	if w.tasks == nil {
		return map[string]any{}, nil
	}

	taskID, err := uuid.Parse(message.TaskID)
	if err != nil {
		return nil, fmt.Errorf("malformed taskID %q: %w", message.TaskID, err)
	}

	task, err := w.tasks.GetByID(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("load task %s for input: %w", taskID, err)
	}
	if task == nil {
		return map[string]any{}, nil
	}
	return task.Input, nil
}

// resolveDataSource looks up the named DataSource connector from the registry.
// Returns a domain error if the connector type is not registered.
func (w *Worker) resolveDataSource(connectorType string) (DataSourceConnector, error) {
	if w.connectors == nil {
		return nil, fmt.Errorf("connector registry not configured")
	}
	c, err := w.connectors.DataSource(connectorType)
	if err != nil {
		return nil, fmt.Errorf("unknown DataSource connector %q: %w", connectorType, err)
	}
	return c, nil
}

// resolveProcess looks up the named Process connector from the registry.
func (w *Worker) resolveProcess(connectorType string) (ProcessConnector, error) {
	if w.connectors == nil {
		return nil, fmt.Errorf("connector registry not configured")
	}
	c, err := w.connectors.Process(connectorType)
	if err != nil {
		return nil, fmt.Errorf("unknown Process connector %q: %w", connectorType, err)
	}
	return c, nil
}

// resolveSink looks up the named Sink connector from the registry.
func (w *Worker) resolveSink(connectorType string) (SinkConnector, error) {
	if w.connectors == nil {
		return nil, fmt.Errorf("connector registry not configured")
	}
	c, err := w.connectors.Sink(connectorType)
	if err != nil {
		return nil, fmt.Errorf("unknown Sink connector %q: %w", connectorType, err)
	}
	return c, nil
}

// transitionStatus updates the task's status in PostgreSQL and publishes an SSE event.
// Logs an error if SSE publication fails (fire-and-forget per ADR-007).
//
// Args:
//
//	ctx:      Request context.
//	taskID:   The task to update.
//	status:   The target state.
//	reason:   Human-readable reason recorded in task_state_log.
//	workerID: Non-nil for transitions that assign work to this worker.
//
// Postconditions:
//   - On success: task.Status = status in PostgreSQL; task_state_log has a new entry.
func (w *Worker) transitionStatus(ctx context.Context, taskID uuid.UUID, status models.TaskStatus, reason string, workerID *string) error {
	if w.tasks == nil {
		return nil
	}

	if err := w.tasks.UpdateStatus(ctx, taskID, status, reason, workerID); err != nil {
		return fmt.Errorf("UpdateStatus(%s -> %s): %w", taskID, status, err)
	}

	w.publishTaskEvent(ctx, taskID, status, reason)
	return nil
}

// publishTaskEvent fires an SSE task event for the given task state.
// Retrieves the current task from the repository to populate the event payload.
// Errors are logged and discarded (fire-and-forget per ADR-007).
func (w *Worker) publishTaskEvent(ctx context.Context, taskID uuid.UUID, status models.TaskStatus, reason string) {
	if w.broker == nil || w.tasks == nil {
		return
	}

	task, err := w.tasks.GetByID(ctx, taskID)
	if err != nil || task == nil {
		log.Printf("worker: publishTaskEvent: could not load task %s: %v", taskID, err)
		return
	}

	if pubErr := w.broker.PublishTaskEvent(ctx, task, reason); pubErr != nil {
		log.Printf("worker: publishTaskEvent: %v", pubErr)
	}
}

// ackMessage sends XACK to remove the message from the pending entry list.
// Logs errors but does not return them — the message state cannot be rolled back.
//
// Args:
//
//	ctx:     Request context.
//	message: The task message to acknowledge.
func (w *Worker) ackMessage(ctx context.Context, message *queue.TaskMessage) {
	// Determine the tag from the stream ID is not possible; the tag is embedded in the
	// stream name. We use the first worker tag as the tag because each message is read
	// from a single tag stream and the TaskMessage does not carry its origin stream tag.
	// For multi-tag workers, XACK targets the correct stream via TaskQueueStream(tag).
	// Since Acknowledge requires the tag to form "queue:{tag}", and TaskMessage does not
	// carry the tag it was read from, we pass an empty-string tag: the RedisQueue.Acknowledge
	// implementation prepends "queue:" — this works when tag is the stream suffix.
	// Resolution: store the tag in TaskMessage (StreamTag field). For now we use the
	// stream ID approach: parse the tag from StreamID is not possible. We call Acknowledge
	// with each tag and let Redis XACK be a no-op if the message is not in that stream.
	if w.consumer == nil {
		return
	}

	// Try to ack against each of the worker's tags. XACK is idempotent and a no-op
	// if the message ID does not belong to that stream's pending list.
	for _, tag := range w.cfg.WorkerTags {
		ackCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := w.consumer.Acknowledge(ackCtx, tag, message.StreamID)
		cancel()
		if err == nil {
			return // Successfully acked.
		}
		// Log but continue trying other tags.
		log.Printf("worker: XACK error on stream queue:%s for message %q: %v", tag, message.StreamID, err)
	}
}

// applyMappingsToSlice applies ApplySchemaMapping to each record in a slice.
// Returns a new slice of renamed records. Fails fast on the first mapping error.
// When mappings is empty, the original slice is returned unchanged so that all
// fields from the preceding phase are passed through to the next phase without
// any renaming.
//
// Args:
//
//	records:  The output records from the preceding phase.
//	mappings: The schema mappings to apply. Empty means pass-through.
//
// Returns:
//   - The original records slice unchanged when mappings is empty (pass-through).
//   - A new slice of mapped records (each containing only the mapped fields) when mappings is non-empty.
//   - An error if any record has a missing source field.
func (w *Worker) applyMappingsToSlice(records []map[string]any, mappings []models.SchemaMapping) ([]map[string]any, error) {
	if len(mappings) == 0 {
		// No mappings: pass records through unchanged to preserve all fields.
		return records, nil
	}

	result := make([]map[string]any, 0, len(records))
	for i, record := range records {
		mapped, err := w.ApplySchemaMapping(record, mappings)
		if err != nil {
			return nil, fmt.Errorf("record[%d]: %w", i, err)
		}
		result = append(result, mapped)
	}
	return result, nil
}

// ApplySchemaMapping transforms a data record from one phase's output schema
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
//   - The original data map is not mutated.
//   - On error: the task must be marked "failed" by the caller; the error names the missing field.
func (w *Worker) ApplySchemaMapping(data map[string]any, mappings []models.SchemaMapping) (map[string]any, error) {
	result := make(map[string]any, len(mappings))
	for _, m := range mappings {
		val, ok := data[m.SourceField]
		if !ok {
			return nil, fmt.Errorf("schema mapping: source field %q not found in record", m.SourceField)
		}
		result[m.TargetField] = val
	}
	return result, nil
}

// isDomainError returns true when err is a domainErrorWrapper, meaning the failure is
// due to connector or mapping logic rather than infrastructure unreachability.
// Domain errors result in terminal task failure without XCLAIM retry (ADR-003).
func isDomainError(err error) bool {
	var de *domainErrorWrapper
	return errors.As(err, &de)
}

// domainErrorWrapper distinguishes domain errors (connector, schema, missing pipeline)
// from infrastructure errors (DB, Redis). Domain errors do not retry via XCLAIM.
// See: ADR-003, Domain Invariant 2.
type domainErrorWrapper struct {
	cause error
}

func (e *domainErrorWrapper) Error() string {
	if e.cause != nil {
		return e.cause.Error()
	}
	return "domain error"
}

func (e *domainErrorWrapper) Unwrap() error { return e.cause }
