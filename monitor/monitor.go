// Package monitor implements the NexusFlow Monitor service.
// The Monitor runs two periodic loops:
//  1. Heartbeat checker — detects workers that have stopped sending heartbeats
//     and marks them as "down" in PostgreSQL and Redis Pub/Sub.
//  2. Pending entry scanner — identifies tasks pending on downed workers
//     and reclaims them via XCLAIM for reassignment to healthy workers.
//
// Configuration (ADR-002):
//
//	HeartbeatTimeout:    15 seconds (3 missed heartbeats at 5s interval)
//	PendingScanInterval: 10 seconds
//
// See: ADR-002, TASK-009 (Cycle 2)
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
//	heartbeat: HeartbeatStore for ZRANGEBYSCORE queries on workers:active.
//	scanner:   PendingScanner for XPENDING and XCLAIM operations.
//	producer:  Producer for routing exhausted-retry tasks to queue:dead-letter.
//	broker:    SSE Broker for publishing worker and task events after failover.
//
// Preconditions:
//   - All arguments are non-nil and their underlying connections are open.
func NewMonitor(
	cfg *config.Config,
	workers db.WorkerRepository,
	tasks db.TaskRepository,
	heartbeat queue.HeartbeatStore,
	scanner queue.PendingScanner,
	producer queue.Producer,
	broker sse.Broker,
) *Monitor {
	return &Monitor{
		cfg:       cfg,
		workers:   workers,
		tasks:     tasks,
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
// Loads the task from PostgreSQL to check retry counts.
//
// If the task is not found in PostgreSQL (e.g. deleted after enqueue), the entry
// is skipped with a log message — we cannot reclaim a task that no longer exists.
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

	maxRetries := task.RetryConfig.MaxRetries
	if maxRetries <= 0 {
		maxRetries = models.DefaultRetryConfig().MaxRetries
	}

	if task.RetryCount >= maxRetries {
		return m.deadLetterTask(ctx, entry, tag)
	}
	return m.reclaimTask(ctx, entry, tag)
}

// reclaimTask reclaims a pending task from a downed worker and re-queues it for
// pickup by a healthy worker.
//
// Sequence:
//  1. XCLAIM: transfer the pending entry to the monitor consumer.
//  2. IncrementRetryCount: bump the retry counter in PostgreSQL.
//  3. UpdateStatus("queued"): mark the task available for consumption.
//  4. Enqueue: re-XADD the task message to its stream so healthy workers see it via XREADGROUP.
//  5. AcknowledgePending: XACK the monitor's claimed entry so the pending list stays clean.
//
// Steps 4 and 5 are necessary because XREADGROUP ">" only delivers entries that have
// never been delivered to any consumer; a message in a consumer's pending list is not
// re-delivered automatically. XACK + re-XADD is the correct XCLAIM + requeue pattern.
//
// Args:
//
//	ctx:   Request context.
//	entry: The pending entry from ListPendingOlderThan.
//	tag:   The stream tag the entry belongs to.
//
// Postconditions:
//   - On success: task is re-queued and will be picked up by the next healthy matching worker.
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

	// Step 2: Increment the retry counter before re-queuing so the updated count is
	// visible to the next worker that picks up the task.
	if _, err := m.tasks.IncrementRetryCount(ctx, taskID); err != nil {
		return fmt.Errorf("reclaimTask: IncrementRetryCount(%s): %w", taskID, err)
	}

	// Step 3: Transition status back to "queued".
	if err := m.tasks.UpdateStatus(ctx, taskID, models.TaskStatusQueued, "reclaimed by monitor after worker failure", nil); err != nil {
		return fmt.Errorf("reclaimTask: UpdateStatus(%s -> queued): %w", taskID, err)
	}

	// Step 4: Re-XADD the task to the stream so healthy workers see it via XREADGROUP ">".
	// Load the current task record to build the ProducerMessage envelope.
	task, err := m.tasks.GetByID(ctx, taskID)
	if err != nil {
		return fmt.Errorf("reclaimTask: GetByID(%s) for re-enqueue: %w", taskID, err)
	}
	if task == nil {
		return fmt.Errorf("reclaimTask: task %s not found for re-enqueue", taskID)
	}
	if _, err := m.producer.Enqueue(ctx, &queue.ProducerMessage{
		Task: task,
		Tags: []string{tag},
	}); err != nil {
		return fmt.Errorf("reclaimTask: Enqueue(%s, tag=%q): %w", taskID, tag, err)
	}

	// Step 5: ACK the monitor's claimed entry to keep the pending list clean.
	if err := m.scanner.AcknowledgePending(ctx, tag, entry.StreamID); err != nil {
		// Non-fatal: the task is already re-queued. Log and continue.
		log.Printf("monitor.reclaimTask: AcknowledgePending(%q, %q): %v — pending entry may linger", tag, entry.StreamID, err)
	}

	log.Printf("monitor: reclaimed task %s (stream=%q, entry=%q) — re-enqueued for retry", taskID, tag, entry.StreamID)
	return nil
}

// deadLetterTask moves a task with exhausted retries to the dead letter queue.
// Updates the task status to "failed" and enqueues a dead-letter entry.
// Cascading cancellation for PipelineChain tasks is implemented in TASK-011.
//
// Args:
//
//	ctx:   Request context.
//	entry: The pending entry from ListPendingOlderThan.
//	tag:   The stream tag the entry belongs to.
//
// Postconditions:
//   - On success: task is in queue:dead-letter; task.Status = "failed".
//   - Cascading chain cancellation: deferred to TASK-011 (Cycle 2).
func (m *Monitor) deadLetterTask(ctx context.Context, entry *queue.PendingEntry, tag string) error {
	taskID, err := uuid.Parse(entry.TaskID)
	if err != nil {
		return fmt.Errorf("deadLetterTask: parse taskID %q: %w", entry.TaskID, err)
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
	return nil
}
