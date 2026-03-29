// Package monitor implements the NexusFlow Monitor service.
// The Monitor runs three periodic loops:
//  1. Heartbeat checker — detects workers that have stopped sending heartbeats
//     and marks them as "down" in PostgreSQL and Redis Pub/Sub.
//  2. Pending entry scanner — identifies tasks pending on downed workers
//     and reclaims them via XCLAIM; schedules deferred re-enqueue via backoff.
//  3. Retry-ready scanner — re-enqueues tasks whose backoff delay has elapsed.
//
// Infrastructure failure retry flow (TASK-010):
//  - reclaimTask: XCLAIM → IncrementRetryCount → SetRetryAfterAndTags → ACK pending entry
//  - scanRetryReady: when retry_after <= now, re-XADD to Redis and clear retry_after
//
// Process errors (connector failures) are XACK'd by the worker directly and never
// enter the Monitor reclaim path (Domain Invariant 2, ADR-003).
//
// Configuration (ADR-002):
//
//	HeartbeatTimeout:    15 seconds (3 missed heartbeats at 5s interval)
//	PendingScanInterval: 10 seconds
//
// See: ADR-002, TASK-009, TASK-010 (Cycle 2)
package monitor

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/nxlabs/nexusflow/internal/config"
	"github.com/nxlabs/nexusflow/internal/db"
	"github.com/nxlabs/nexusflow/internal/models"
	"github.com/nxlabs/nexusflow/internal/queue"
	"github.com/nxlabs/nexusflow/internal/sse"
)

// monitorConsumerID is the consumer name used when the Monitor claims entries via XCLAIM.
// Using a fixed name ensures that re-claimed entries appear under a single identity
// in the stream's pending list rather than accumulating per-scan-cycle names.
const monitorConsumerID = "monitor"

// Monitor is the main struct for the NexusFlow monitor service.
type Monitor struct {
	cfg       *config.Config
	workers   db.WorkerRepository
	tasks     db.TaskRepository
	chains    db.ChainRepository
	heartbeat queue.HeartbeatStore
	scanner   queue.PendingScanner
	producer  queue.Producer
	broker    sse.Broker
}

// NewMonitor constructs a Monitor with all required dependencies.
//
// Args:
//
//	cfg:       Runtime configuration (HeartbeatTimeout, PendingScanInterval).
//	workers:   WorkerRepository for marking workers down.
//	tasks:     TaskRepository for retry count checks and status transitions.
//	chains:    ChainRepository for pipeline chain lookup during cascading cancellation.
//	           May be nil when cascading cancellation is not required (e.g. unit tests
//	           that only exercise heartbeat or retry logic).
//	heartbeat: HeartbeatStore for ZRANGEBYSCORE queries on workers:active.
//	scanner:   PendingScanner for XPENDING and XCLAIM operations.
//	producer:  Producer for routing exhausted-retry tasks to queue:dead-letter.
//	broker:    SSE Broker for publishing worker and task events after failover.
//
// Preconditions:
//   - cfg, workers, tasks, heartbeat, scanner, producer must be non-nil.
//   - chains may be nil; when nil, cascadeCancelDownstream is a no-op.
func NewMonitor(
	cfg *config.Config,
	workers db.WorkerRepository,
	tasks db.TaskRepository,
	chains db.ChainRepository,
	heartbeat queue.HeartbeatStore,
	scanner queue.PendingScanner,
	producer queue.Producer,
	broker sse.Broker,
) *Monitor {
	return &Monitor{
		cfg:       cfg,
		workers:   workers,
		tasks:     tasks,
		chains:    chains,
		heartbeat: heartbeat,
		scanner:   scanner,
		producer:  producer,
		broker:    broker,
	}
}

// Run starts the monitor service. Blocks until ctx is cancelled.
// Runs checkHeartbeats and scanPendingEntries on a single shared ticker
// driven by PendingScanInterval. Both checks run on the same interval
// because the heartbeat timeout (15s) and scan interval (10s) are configured
// independently via config; running both on the scan interval ensures
// prompt detection without over-complicating the loop.
//
// Postconditions:
//   - On ctx cancellation: the ticker stops; no further Redis or PostgreSQL operations.
func (m *Monitor) Run(ctx context.Context) error {
	interval := m.cfg.PendingScanInterval
	if interval <= 0 {
		interval = 10 * time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	log.Printf("monitor: running — heartbeat-timeout=%v scan-interval=%v",
		m.cfg.HeartbeatTimeout, interval)

	for {
		select {
		case <-ctx.Done():
			log.Printf("monitor: context cancelled — stopping")
			return nil
		case <-ticker.C:
			if err := m.checkHeartbeats(ctx); err != nil {
				log.Printf("monitor: checkHeartbeats error: %v", err)
			}
			if err := m.scanPendingEntries(ctx); err != nil {
				log.Printf("monitor: scanPendingEntries error: %v", err)
			}
			if err := m.scanRetryReady(ctx); err != nil {
				log.Printf("monitor: scanRetryReady error: %v", err)
			}
		}
	}
}

// checkHeartbeats queries workers:active for entries older than HeartbeatTimeout.
// For each expired worker: marks it "down" in PostgreSQL, removes it from workers:active,
// and publishes a worker:down event via the SSE Broker.
//
// Errors marking a single worker down are logged and skipped; remaining workers
// in the expired list are still processed (fail-fast per worker, not per scan).
//
// Complexity: MEDIUM
//   - Single ZRANGEBYSCORE query then N PostgreSQL updates.
//   - N is bounded by fleet size (typical: < 10).
func (m *Monitor) checkHeartbeats(ctx context.Context) error {
	timeout := m.cfg.HeartbeatTimeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	cutoff := time.Now().Add(-timeout)

	expiredIDs, err := m.heartbeat.ListExpired(ctx, cutoff)
	if err != nil {
		return fmt.Errorf("monitor.checkHeartbeats: ListExpired: %w", err)
	}

	for _, workerID := range expiredIDs {
		if err := m.markWorkerDown(ctx, workerID); err != nil {
			log.Printf("monitor.checkHeartbeats: markWorkerDown(%q): %v", workerID, err)
		}
	}
	return nil
}

// markWorkerDown transitions a single worker to "down" status.
// Sequence: UpdateStatus in PostgreSQL → Remove from workers:active → PublishWorkerEvent.
// PublishWorkerEvent failure is non-fatal (fire-and-forget per ADR-007).
//
// Args:
//
//	ctx:      Request context.
//	workerID: The worker to mark down.
func (m *Monitor) markWorkerDown(ctx context.Context, workerID string) error {
	if err := m.workers.UpdateStatus(ctx, workerID, models.WorkerStatusDown); err != nil {
		return fmt.Errorf("UpdateStatus: %w", err)
	}

	if err := m.heartbeat.Remove(ctx, workerID); err != nil {
		return fmt.Errorf("remove from heartbeat store: %w", err)
	}

	// Retrieve the worker record to populate the SSE event payload.
	worker, err := m.workers.GetByID(ctx, workerID)
	if err != nil {
		log.Printf("monitor.markWorkerDown: GetByID(%q): %v — publishing minimal event", workerID, err)
		worker = &models.Worker{ID: workerID, Status: models.WorkerStatusDown}
	}
	if worker == nil {
		worker = &models.Worker{ID: workerID, Status: models.WorkerStatusDown}
	} else {
		worker.Status = models.WorkerStatusDown
	}

	// Fire-and-forget: SSE publish failure must not abort the failover sequence.
	if m.broker != nil {
		if pubErr := m.broker.PublishWorkerEvent(ctx, worker); pubErr != nil {
			log.Printf("monitor.markWorkerDown: PublishWorkerEvent(%q): %v", workerID, pubErr)
		}
	}

	log.Printf("monitor: worker %q marked down", workerID)
	return nil
}

// scanPendingEntries scans all known tag streams for messages pending on downed workers.
// "All known tag streams" is derived from the registered workers in PostgreSQL:
// the union of all unique tags across all registered workers.
//
// For each pending entry older than HeartbeatTimeout:
//   - If the task has retries remaining: calls reclaimTask.
//   - If retries are exhausted: calls deadLetterTask.
//
// Errors on individual entries are logged and skipped; the scan continues.
//
// Complexity: HIGH
//   - O(unique tags) XPENDING calls + O(pending entries) per tag.
func (m *Monitor) scanPendingEntries(ctx context.Context) error {
	tags, err := m.collectAllTags(ctx)
	if err != nil {
		return fmt.Errorf("monitor.scanPendingEntries: collectAllTags: %w", err)
	}

	idleThreshold := m.cfg.HeartbeatTimeout
	if idleThreshold <= 0 {
		idleThreshold = 15 * time.Second
	}

	for _, tag := range tags {
		entries, err := m.scanner.ListPendingOlderThan(ctx, tag, idleThreshold)
		if err != nil {
			log.Printf("monitor.scanPendingEntries: ListPendingOlderThan(tag=%q): %v", tag, err)
			continue
		}

		for _, entry := range entries {
			if err := m.processEntry(ctx, entry, tag); err != nil {
				log.Printf("monitor.scanPendingEntries: processEntry(stream=%q, entry=%q): %v",
					tag, entry.StreamID, err)
			}
		}
	}
	return nil
}

// collectAllTags returns the deduplicated union of all capability tags across all
// registered workers in PostgreSQL. This determines which streams to scan for
// pending entries during the failover check.
//
// Returns an empty slice when no workers are registered.
func (m *Monitor) collectAllTags(ctx context.Context) ([]string, error) {
	workers, err := m.workers.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("collectAllTags: List workers: %w", err)
	}

	seen := make(map[string]struct{})
	var tags []string
	for _, w := range workers {
		for _, tag := range w.Tags {
			if _, exists := seen[tag]; !exists {
				seen[tag] = struct{}{}
				tags = append(tags, tag)
			}
		}
	}
	return tags, nil
}

// processEntry decides whether to reclaim or dead-letter a single pending entry.
// Loads the task from PostgreSQL to check retry counts and retry_after gate.
//
// Skip conditions (entry is left in place):
//   - Task not found in PostgreSQL (deleted after enqueue).
//   - Task has retry_after set to a future time (backoff delay has not elapsed).
//
// Reclaim condition: RetryCount < MaxRetries (and retry_after is nil or in the past).
// Dead-letter condition: RetryCount >= MaxRetries.
func (m *Monitor) processEntry(ctx context.Context, entry *queue.PendingEntry, tag string) error {
	taskID, err := uuid.Parse(entry.TaskID)
	if err != nil {
		return fmt.Errorf("processEntry: malformed taskID %q: %w", entry.TaskID, err)
	}

	task, err := m.tasks.GetByID(ctx, taskID)
	if err != nil {
		return fmt.Errorf("processEntry: GetByID(%s): %w", taskID, err)
	}
	if task == nil {
		log.Printf("monitor.processEntry: task %s not found — skipping pending entry %q", taskID, entry.StreamID)
		return nil
	}

	// Skip tasks whose backoff delay has not yet elapsed. The Monitor already
	// claimed and ACKed this entry during a previous scan cycle; the task is
	// in "queued" status in the DB waiting for retry_after to pass.
	// scanRetryReady will re-enqueue it when the gate opens.
	if task.RetryAfter != nil && task.RetryAfter.After(time.Now()) {
		log.Printf("monitor.processEntry: task %s has retry_after=%v — skipping (backoff not elapsed)", taskID, *task.RetryAfter)
		return nil
	}

	maxRetries := task.RetryConfig.MaxRetries
	if maxRetries <= 0 {
		maxRetries = models.DefaultRetryConfig().MaxRetries
	}

	if task.RetryCount >= maxRetries {
		return m.deadLetterTask(ctx, entry, tag)
	}
	return m.reclaimTask(ctx, entry, tag)
}

// reclaimTask reclaims a pending task from a downed worker and schedules it for
// retry after the configured backoff delay.
//
// Sequence:
//  1. XCLAIM: transfer the pending entry to the monitor consumer.
//  2. IncrementRetryCount: bump the retry counter in PostgreSQL (pre-increment).
//     The backoff delay is computed from the pre-increment RetryCount so that:
//     retry 1 → 1s, retry 2 → 2s, retry 3 → 4s (exponential).
//  3. SetRetryAfterAndTags: record when the task may next be dispatched and which
//     stream(s) to dispatch it to. The tag from the current pending entry is stored
//     so scanRetryReady can re-enqueue to the correct stream(s).
//  4. UpdateStatus("queued"): mark the task available for eventual re-dispatch.
//  5. AcknowledgePending: XACK the monitor's claimed entry so the pending list stays clean.
//     The actual re-XADD is deferred to scanRetryReady (called on each scan tick)
//     once retry_after has elapsed.
//
// This design separates the XCLAIM/claim step from the re-enqueue step, inserting a
// configurable delay between them. It satisfies Domain Invariant 2: infrastructure
// failures (worker death) trigger this path; process errors are XACK'd by the worker
// directly and never reach the Monitor reclaim path.
//
// Args:
//
//	ctx:   Request context.
//	entry: The pending entry from ListPendingOlderThan.
//	tag:   The stream tag the entry belongs to.
//
// Postconditions:
//   - On success: task.RetryCount is incremented; task.RetryAfter = now + backoffDelay;
//     task.RetryTags = [tag]; task.Status = "queued".
//   - The task will be re-enqueued to Redis by scanRetryReady once retry_after elapses.
func (m *Monitor) reclaimTask(ctx context.Context, entry *queue.PendingEntry, tag string) error {
	idleThreshold := m.cfg.HeartbeatTimeout
	if idleThreshold <= 0 {
		idleThreshold = 15 * time.Second
	}

	// Step 1: XCLAIM — transfer the entry to the monitor consumer.
	if err := m.scanner.Claim(ctx, tag, entry.StreamID, monitorConsumerID, idleThreshold); err != nil {
		return fmt.Errorf("reclaimTask: Claim(tag=%q, id=%q): %w", tag, entry.StreamID, err)
	}

	taskID, err := uuid.Parse(entry.TaskID)
	if err != nil {
		return fmt.Errorf("reclaimTask: parse taskID %q: %w", entry.TaskID, err)
	}

	// Load the task to read its current RetryCount (pre-increment) for backoff computation.
	task, err := m.tasks.GetByID(ctx, taskID)
	if err != nil {
		return fmt.Errorf("reclaimTask: GetByID(%s) for backoff: %w", taskID, err)
	}
	if task == nil {
		return fmt.Errorf("reclaimTask: task %s not found", taskID)
	}

	// Compute the backoff delay from the pre-increment retry count so the first
	// retry uses 1s, the second uses 2s (exponential), etc.
	preIncrementCount := task.RetryCount
	backoffStrategy := task.RetryConfig.Backoff
	if backoffStrategy == "" {
		backoffStrategy = models.DefaultRetryConfig().Backoff
	}
	delay := computeBackoffDelay(backoffStrategy, preIncrementCount)
	retryAfter := time.Now().Add(delay)

	// Step 2: Increment the retry counter so the updated count is visible to
	// the next worker that picks up the task after the backoff elapses.
	if _, err := m.tasks.IncrementRetryCount(ctx, taskID); err != nil {
		return fmt.Errorf("reclaimTask: IncrementRetryCount(%s): %w", taskID, err)
	}

	// Step 3: Record retry_after and the stream tag(s) for deferred re-enqueue.
	if err := m.tasks.SetRetryAfterAndTags(ctx, taskID, &retryAfter, []string{tag}); err != nil {
		return fmt.Errorf("reclaimTask: SetRetryAfterAndTags(%s): %w", taskID, err)
	}

	// Step 4: Transition status to "queued" so the task is visible as pending retry.
	if err := m.tasks.UpdateStatus(ctx, taskID, models.TaskStatusQueued, "reclaimed by monitor after worker failure — waiting for backoff delay", nil); err != nil {
		return fmt.Errorf("reclaimTask: UpdateStatus(%s -> queued): %w", taskID, err)
	}

	// Step 5: ACK the monitor's claimed entry to keep the pending list clean.
	// The actual re-XADD to Redis is deferred to scanRetryReady (once retry_after elapses).
	if err := m.scanner.AcknowledgePending(ctx, tag, entry.StreamID); err != nil {
		// Non-fatal: the task is already gated by retry_after. Log and continue.
		log.Printf("monitor.reclaimTask: AcknowledgePending(%q, %q): %v — pending entry may linger", tag, entry.StreamID, err)
	}

	log.Printf("monitor: reclaimed task %s (stream=%q, entry=%q) — retry scheduled after %v (delay=%v)",
		taskID, tag, entry.StreamID, retryAfter.Format(time.RFC3339), delay)
	return nil
}

// scanRetryReady finds tasks in "queued" status whose retry_after has elapsed and
// re-enqueues them to Redis so healthy workers can pick them up via XREADGROUP.
//
// This is the second half of the deferred-retry pattern: reclaimTask sets retry_after
// and ACKs the pending entry; scanRetryReady fires the re-XADD once the gate opens.
//
// For each retry-ready task:
//  1. Re-XADD the task to each of its recorded RetryTags streams via producer.Enqueue.
//  2. Clear retry_after (set to nil) so the task is not double-dispatched on the next scan.
//
// Errors on individual tasks are logged and skipped; the scan continues.
//
// Complexity: O(retry-ready tasks) PostgreSQL reads + O(tags) Redis XADD per task.
func (m *Monitor) scanRetryReady(ctx context.Context) error {
	tasks, err := m.tasks.ListRetryReady(ctx)
	if err != nil {
		return fmt.Errorf("monitor.scanRetryReady: ListRetryReady: %w", err)
	}

	for _, task := range tasks {
		if err := m.dispatchRetryReadyTask(ctx, task); err != nil {
			log.Printf("monitor.scanRetryReady: dispatchRetryReadyTask(task=%s): %v", task.ID, err)
		}
	}
	return nil
}

// dispatchRetryReadyTask re-enqueues a single retry-ready task to Redis and clears
// its retry_after gate to prevent duplicate dispatch on the next scan tick.
//
// Args:
//
//	ctx:  Request context.
//	task: A task whose RetryAfter has elapsed and RetryTags is populated.
//
// Postconditions:
//   - On success: task is re-XADD'd to each RetryTag stream; task.RetryAfter = nil.
//   - On Enqueue failure: retry_after is NOT cleared (will be retried next scan).
func (m *Monitor) dispatchRetryReadyTask(ctx context.Context, task *models.Task) error {
	if len(task.RetryTags) == 0 {
		log.Printf("monitor.dispatchRetryReadyTask: task %s has no retry_tags — cannot re-enqueue; clearing retry_after", task.ID)
		// Clear the gate even without tags to avoid the task being stuck indefinitely.
		_ = m.tasks.SetRetryAfterAndTags(ctx, task.ID, nil, nil)
		return fmt.Errorf("task %s has no retry_tags", task.ID)
	}

	// Re-XADD the task to its recorded stream(s).
	if _, err := m.producer.Enqueue(ctx, &queue.ProducerMessage{
		Task: task,
		Tags: task.RetryTags,
	}); err != nil {
		return fmt.Errorf("Enqueue(task=%s, tags=%v): %w", task.ID, task.RetryTags, err)
	}

	// Clear retry_after so the task is not dispatched again on the next scan tick.
	if err := m.tasks.SetRetryAfterAndTags(ctx, task.ID, nil, nil); err != nil {
		// Non-fatal: the task is already re-enqueued. Log and continue.
		// The next scan will find it again, but producer.Enqueue with the same tags
		// is idempotent at the stream level (duplicate XADD produces a unique message ID).
		log.Printf("monitor.dispatchRetryReadyTask: SetRetryAfterAndTags(nil) for task %s: %v", task.ID, err)
	}

	log.Printf("monitor: dispatched retry-ready task %s to streams %v", task.ID, task.RetryTags)
	return nil
}

// deadLetterTask moves a task with exhausted retries to the dead letter queue.
// Updates the task status to "failed", enqueues a dead-letter entry, and triggers
// cascading cancellation for all downstream tasks in the same pipeline chain (TASK-011).
//
// Args:
//
//	ctx:   Request context.
//	entry: The pending entry from ListPendingOlderThan.
//	tag:   The stream tag the entry belongs to.
//
// Postconditions:
//   - On success: task is in queue:dead-letter; task.Status = "failed".
//   - If the task belongs to a pipeline chain: all downstream non-terminal tasks
//     in the chain are cancelled with reason "upstream task failed" (REQ-012).
//   - Cascading cancellation errors are logged but do not abort the dead-letter flow.
func (m *Monitor) deadLetterTask(ctx context.Context, entry *queue.PendingEntry, tag string) error {
	taskID, err := uuid.Parse(entry.TaskID)
	if err != nil {
		return fmt.Errorf("deadLetterTask: parse taskID %q: %w", entry.TaskID, err)
	}

	// Load the task before marking it failed so we can read its PipelineID for
	// cascade lookup — UpdateStatus does not return the updated task.
	task, err := m.tasks.GetByID(ctx, taskID)
	if err != nil {
		return fmt.Errorf("deadLetterTask: GetByID(%s): %w", taskID, err)
	}
	if task == nil {
		return fmt.Errorf("deadLetterTask: task %s not found", taskID)
	}

	reason := fmt.Sprintf("retries exhausted after worker failure (stream=%q, entry=%q)", tag, entry.StreamID)

	// Mark the task failed in PostgreSQL first so status is consistent even if
	// the dead-letter enqueue is interrupted.
	if err := m.tasks.UpdateStatus(ctx, taskID, models.TaskStatusFailed, reason, nil); err != nil {
		return fmt.Errorf("deadLetterTask: UpdateStatus(%s -> failed): %w", taskID, err)
	}

	if err := m.producer.EnqueueDeadLetter(ctx, taskID.String(), reason); err != nil {
		return fmt.Errorf("deadLetterTask: EnqueueDeadLetter(%s): %w", taskID, err)
	}

	log.Printf("monitor: task %s dead-lettered (stream=%q, entry=%q)", taskID, tag, entry.StreamID)

	// Trigger cascading cancellation. Errors here are non-fatal: the task is already
	// dead-lettered. Log and continue so a cascade lookup failure does not mask the
	// primary dead-letter event.
	if err := m.cascadeCancelDownstream(ctx, task); err != nil {
		log.Printf("monitor: cascadeCancelDownstream(task=%s): %v — dead-letter succeeded; cascade partial", taskID, err)
	}

	return nil
}

// cascadeCancelDownstream cancels all non-terminal tasks in pipelines that follow
// the given task's pipeline in the same chain. Called after a task is dead-lettered
// so that downstream work that depends on failed upstream output is halted promptly.
//
// If the task has no PipelineID, or the pipeline is not part of any chain, or the
// pipeline is the last step in the chain, this is a no-op.
//
// For each downstream pipeline in the chain (ordered by position after the failing
// pipeline): all tasks in submitted/queued/assigned/running states are cancelled with
// reason "upstream task failed", and a task SSE event is published for each.
//
// Args:
//
//	ctx:  Request context.
//	task: The failed task whose downstream chain members should be cancelled.
//
// Postconditions:
//   - On success: all non-terminal tasks in downstream pipelines are cancelled.
//   - SSE events are published for each cancelled task (fire-and-forget; publish errors
//     are logged but do not abort the cancellation loop).
func (m *Monitor) cascadeCancelDownstream(ctx context.Context, task *models.Task) error {
	// No chains repository wired — standalone configuration; nothing to do.
	if m.chains == nil {
		return nil
	}

	// Task has no pipeline reference (e.g. pipeline was deleted); cannot look up chain.
	if task.PipelineID == nil {
		return nil
	}

	// Find the chain containing this pipeline. Returns nil when the pipeline is standalone.
	chain, err := m.chains.FindByPipeline(ctx, *task.PipelineID)
	if err != nil {
		return fmt.Errorf("cascadeCancelDownstream: FindByPipeline(%s): %w", *task.PipelineID, err)
	}
	if chain == nil {
		// Standalone pipeline — no cascade required (REQ-012 AC-2).
		return nil
	}

	// Walk the chain from the failing pipeline's position to the end, cancelling tasks
	// in each downstream pipeline. chain.PipelineIDs is ordered by position (0-based).
	failingPipelineID := *task.PipelineID
	foundFailing := false
	for _, pipelineID := range chain.PipelineIDs {
		if !foundFailing {
			if pipelineID == failingPipelineID {
				foundFailing = true
			}
			continue // skip pipelines up to and including the failing one
		}

		// pipelineID is downstream of the failing pipeline; cancel its non-terminal tasks.
		if err := m.cancelNonTerminalTasksForPipeline(ctx, pipelineID); err != nil {
			// Log but continue: best-effort cancellation of remaining downstream pipelines.
			log.Printf("monitor: cascadeCancelDownstream: cancelNonTerminalTasksForPipeline(pipeline=%s): %v", pipelineID, err)
		}
	}

	return nil
}

// nonTerminalStatuses are the task states that are eligible for cascading cancellation.
// Terminal states (completed, failed, cancelled) are already in a final state and
// must not be transitioned.
var nonTerminalStatuses = []models.TaskStatus{
	models.TaskStatusSubmitted,
	models.TaskStatusQueued,
	models.TaskStatusAssigned,
	models.TaskStatusRunning,
}

// cancelNonTerminalTasksForPipeline cancels all tasks in non-terminal states for the
// given pipeline. Each cancellation is recorded in the task_state_log with reason
// "upstream task failed". A task SSE event is published for each cancelled task.
//
// Args:
//
//	ctx:        Request context.
//	pipelineID: The pipeline whose non-terminal tasks are to be cancelled.
//
// Postconditions:
//   - On success: all matched tasks are in status "cancelled".
//   - SSE publish errors are non-fatal and are logged (fire-and-forget per ADR-007).
func (m *Monitor) cancelNonTerminalTasksForPipeline(ctx context.Context, pipelineID uuid.UUID) error {
	tasks, err := m.tasks.ListByPipelineAndStatuses(ctx, pipelineID, nonTerminalStatuses)
	if err != nil {
		return fmt.Errorf("ListByPipelineAndStatuses(pipeline=%s): %w", pipelineID, err)
	}

	const cascadeReason = "upstream task failed"

	for _, t := range tasks {
		if err := m.tasks.Cancel(ctx, t.ID, cascadeReason); err != nil {
			log.Printf("monitor: cancelNonTerminalTasksForPipeline: Cancel(task=%s): %v — skipping", t.ID, err)
			continue
		}

		// Publish SSE event so the GUI reflects the cancellation in real time (NFR-003).
		t.Status = models.TaskStatusCancelled
		if m.broker != nil {
			if pubErr := m.broker.PublishTaskEvent(ctx, t, cascadeReason); pubErr != nil {
				log.Printf("monitor: cancelNonTerminalTasksForPipeline: PublishTaskEvent(task=%s): %v", t.ID, pubErr)
			}
		}

		log.Printf("monitor: cascade-cancelled task %s (pipeline=%s)", t.ID, pipelineID)
	}

	return nil
}
