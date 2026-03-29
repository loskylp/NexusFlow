// Package worker — LogPublisher interface and constructor for log lines.
// The Worker produces structured log lines during pipeline phase execution and
// publishes them through the LogPublisher. Hot storage (Redis Streams) and SSE
// fan-out are handled by the RedisLogPublisher implementation.
// See: ADR-008, REQ-018, TASK-016
package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/nxlabs/nexusflow/internal/models"
	"github.com/redis/go-redis/v9"
)

// LogPublisher accepts a single log line and writes it to hot storage (Redis Stream)
// and publishes it for SSE consumption.
// Implementations must be safe for concurrent use.
//
// See: ADR-008, TASK-016
type LogPublisher interface {
	// Publish writes a single log line to hot storage and fires it for SSE.
	//
	// Args:
	//   ctx: Request context.
	//   log: The log line to publish. Must have a non-zero ID and TaskID.
	//
	// Postconditions:
	//   - On success: log line is in Redis Stream logs:{taskId} and published to events:logs:{taskId}.
	//   - On failure: error is returned; callers log and continue (fire-and-forget per ADR-007).
	Publish(ctx context.Context, log *models.TaskLog) error
}

// NewLogLine constructs a TaskLog with all required fields populated.
// The log line's text encodes the phase as a bracketed prefix: "[{phase}] {message}".
//
// Args:
//
//	taskID:  The task being executed.
//	level:   Severity level: INFO, WARN, or ERROR.
//	phase:   Pipeline phase: "datasource", "process", or "sink".
//	message: Human-readable description of the event.
//	ts:      The timestamp to record. Callers should pass time.Now().UTC().
//
// Postconditions:
//   - Returns a TaskLog with a fresh UUID, the given taskID, level, a phase-tagged
//     line, and the provided timestamp.
func NewLogLine(taskID uuid.UUID, level, phase, message string, ts time.Time) *models.TaskLog {
	return &models.TaskLog{
		ID:        uuid.New(),
		TaskID:    taskID,
		Level:     level,
		Line:      fmt.Sprintf("[%s] %s", phase, message),
		Timestamp: ts,
	}
}

// RedisLogPublisher implements LogPublisher using Redis Streams for hot storage and
// the SSE broker for real-time fan-out to connected log stream clients.
//
// Hot storage key: logs:{taskId} (Redis Stream via XADD).
// SSE fan-out:     events:logs:{taskId} (via broker.PublishLogLine).
//
// See: ADR-007, ADR-008, TASK-016
type RedisLogPublisher struct {
	client *redis.Client
	broker logLineBroker
}

// logLineBroker is the narrow interface from the SSE broker that the log publisher needs.
// The full sse.Broker satisfies this interface.
// See: SOLID Interface Segregation, ADR-007
type logLineBroker interface {
	PublishLogLine(ctx context.Context, log *models.TaskLog) error
}

// NewRedisLogPublisher constructs a RedisLogPublisher.
//
// Args:
//
//	client: A connected go-redis client for writing to Redis Streams.
//	broker: The SSE broker for publishing log line events. May be nil.
//
// Preconditions:
//   - client must not be nil.
func NewRedisLogPublisher(client *redis.Client, broker logLineBroker) *RedisLogPublisher {
	if client == nil {
		panic("worker.NewRedisLogPublisher: redis client must not be nil")
	}
	return &RedisLogPublisher{client: client, broker: broker}
}

// Publish implements LogPublisher.Publish.
// Writes the log line to Redis Stream logs:{taskId} via XADD, then publishes it
// to events:logs:{taskId} for SSE fan-out.
//
// Redis Stream entry fields:
//   - id:        log line UUID
//   - task_id:   task UUID
//   - level:     INFO/WARN/ERROR
//   - line:      phase-tagged message text
//   - timestamp: RFC3339Nano timestamp
//
// Postconditions:
//   - On success: entry is in Redis Stream and event is published to SSE channel.
//   - On Redis XADD failure: error returned; SSE publish is skipped.
//   - On SSE broker nil: XADD still runs; SSE step is skipped silently.
func (p *RedisLogPublisher) Publish(ctx context.Context, log *models.TaskLog) error {
	streamKey := "logs:" + log.TaskID.String()

	if err := p.client.XAdd(ctx, &redis.XAddArgs{
		Stream: streamKey,
		ID:     "*", // auto-generate Redis stream ID
		Values: map[string]any{
			"id":        log.ID.String(),
			"task_id":   log.TaskID.String(),
			"level":     log.Level,
			"line":      log.Line,
			"timestamp": log.Timestamp.Format(time.RFC3339Nano),
		},
	}).Err(); err != nil {
		return fmt.Errorf("log publisher: XADD logs:%s: %w", log.TaskID, err)
	}

	if p.broker != nil {
		if err := p.broker.PublishLogLine(ctx, log); err != nil {
			// Fire-and-forget: SSE publish failure does not affect hot storage.
			// Callers log this if needed.
			return fmt.Errorf("log publisher: PublishLogLine: %w", err)
		}
	}
	return nil
}
