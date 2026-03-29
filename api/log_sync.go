// Package api — background log sync goroutine.
// Periodically reads log entries from Redis Streams (logs:{taskId}) and batch-inserts
// them into the PostgreSQL task_logs cold store. Processed entries are trimmed from
// the stream to prevent unbounded growth.
//
// Sync interval: 60 seconds (ADR-008: "background log sync goroutine every 60 seconds").
// Scan strategy: SCAN for keys matching "logs:*", then XRANGE each stream.
//
// See: ADR-008, TASK-016
package api

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/nxlabs/nexusflow/internal/db"
	"github.com/nxlabs/nexusflow/internal/models"
	"github.com/redis/go-redis/v9"
)

// logSyncInterval is how often the background sync scans Redis Streams for unsynced entries.
// ADR-008 specifies 60 seconds.
const logSyncInterval = 60 * time.Second

// logSyncBatchSize is the maximum number of stream entries read per XRANGE call per task.
// Limits memory usage for tasks with many log lines accumulated between sync cycles.
const logSyncBatchSize = 1000

// StartLogSync launches the background log sync goroutine. It runs until ctx is cancelled.
// Call this once at API server startup.
//
// Args:
//
//	ctx:      A context whose cancellation stops the sync loop gracefully.
//	client:   go-redis client connected to the same Redis instance as the workers.
//	taskLogs: TaskLogRepository for persisting log lines to PostgreSQL cold storage.
//
// Postconditions:
//   - On ctx cancellation: the goroutine exits after the current sync cycle completes.
//   - One sync cycle: scans all logs:* streams, inserts new entries, trims processed IDs.
func StartLogSync(ctx context.Context, client *redis.Client, taskLogs db.TaskLogRepository) {
	go func() {
		ticker := time.NewTicker(logSyncInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := syncLogs(ctx, client, taskLogs); err != nil {
					log.Printf("log-sync: cycle error: %v", err)
				}
			}
		}
	}()
}

// syncLogs performs one scan-and-insert cycle across all Redis Streams matching logs:*.
// For each stream it reads up to logSyncBatchSize entries from the beginning, inserts them
// into PostgreSQL, then trims those entries from the stream.
//
// Errors on individual streams are logged and the cycle continues to the next stream.
// This prevents a single bad stream from blocking the entire sync cycle.
func syncLogs(ctx context.Context, client *redis.Client, taskLogs db.TaskLogRepository) error {
	// SCAN for all keys matching the log stream pattern.
	var cursor uint64
	var streamKeys []string

	for {
		keys, nextCursor, err := client.Scan(ctx, cursor, "logs:*", 100).Result()
		if err != nil {
			return fmt.Errorf("log-sync: SCAN logs:*: %w", err)
		}
		streamKeys = append(streamKeys, keys...)
		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	for _, streamKey := range streamKeys {
		if err := syncStream(ctx, client, taskLogs, streamKey); err != nil {
			log.Printf("log-sync: stream %q: %v", streamKey, err)
			// Continue processing other streams.
		}
	}
	return nil
}

// syncStream reads entries from a single Redis Stream, inserts them into PostgreSQL,
// and trims the processed entries from the stream.
//
// Args:
//
//	ctx:       Request context.
//	client:    go-redis client.
//	taskLogs:  TaskLogRepository for cold storage inserts.
//	streamKey: The Redis stream key (format: "logs:{taskId}").
func syncStream(ctx context.Context, client *redis.Client, taskLogs db.TaskLogRepository, streamKey string) error {
	// XRANGE reads up to logSyncBatchSize entries from the start of the stream.
	entries, err := client.XRangeN(ctx, streamKey, "-", "+", int64(logSyncBatchSize)).Result()
	if err != nil {
		return fmt.Errorf("XRANGE %s: %w", streamKey, err)
	}
	if len(entries) == 0 {
		return nil
	}

	logs, err := parseStreamEntries(entries)
	if err != nil {
		return fmt.Errorf("parseStreamEntries %s: %w", streamKey, err)
	}

	if len(logs) == 0 {
		return nil
	}

	if err := taskLogs.BatchInsert(ctx, logs); err != nil {
		return fmt.Errorf("BatchInsert: %w", err)
	}

	// Trim all entries we just inserted. XDEL removes entries by their stream ID.
	// XTRIM by MINID is not used here because XDEL is precise about which entries
	// were successfully processed.
	ids := make([]string, len(entries))
	for i, e := range entries {
		ids[i] = e.ID
	}
	if err := client.XDel(ctx, streamKey, ids...).Err(); err != nil {
		// Non-fatal: entries remain in the stream and will be re-processed next cycle.
		// Duplicate inserts into the partitioned table are silently skipped by PostgreSQL
		// (no unique constraint on task_logs per OBS-007, so re-insertion creates duplicates).
		// TASK-028 will add XTRIM-based retention to address this.
		log.Printf("log-sync: XDEL %s: %v (entries will be re-processed next cycle)", streamKey, err)
	}

	return nil
}

// parseStreamEntries converts Redis Stream entries to models.TaskLog values.
// Entries with missing or malformed fields are skipped with a warning.
//
// Expected stream entry fields (set by RedisLogPublisher):
//
//	id        — log line UUID
//	task_id   — task UUID
//	level     — INFO/WARN/ERROR
//	line      — phase-tagged message text
//	timestamp — RFC3339Nano
func parseStreamEntries(entries []redis.XMessage) ([]*models.TaskLog, error) {
	out := make([]*models.TaskLog, 0, len(entries))
	for _, e := range entries {
		l, err := parseStreamEntry(e)
		if err != nil {
			log.Printf("log-sync: skip malformed entry %q: %v", e.ID, err)
			continue
		}
		out = append(out, l)
	}
	return out, nil
}

// parseStreamEntry converts a single Redis Stream entry to a models.TaskLog.
// Returns an error if any required field is missing or cannot be parsed.
func parseStreamEntry(e redis.XMessage) (*models.TaskLog, error) {
	getString := func(key string) (string, error) {
		v, ok := e.Values[key]
		if !ok {
			return "", fmt.Errorf("missing field %q", key)
		}
		s, ok := v.(string)
		if !ok {
			return "", fmt.Errorf("field %q is not a string", key)
		}
		return s, nil
	}

	idStr, err := getString("id")
	if err != nil {
		return nil, err
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		return nil, fmt.Errorf("field id: %w", err)
	}

	taskIDStr, err := getString("task_id")
	if err != nil {
		return nil, err
	}
	taskID, err := uuid.Parse(taskIDStr)
	if err != nil {
		return nil, fmt.Errorf("field task_id: %w", err)
	}

	level, err := getString("level")
	if err != nil {
		return nil, err
	}

	line, err := getString("line")
	if err != nil {
		return nil, err
	}

	tsStr, err := getString("timestamp")
	if err != nil {
		return nil, err
	}
	ts, err := time.Parse(time.RFC3339Nano, tsStr)
	if err != nil {
		return nil, fmt.Errorf("field timestamp: %w", err)
	}

	return &models.TaskLog{
		ID:        id,
		TaskID:    taskID,
		Line:      line,
		Level:     level,
		Timestamp: ts,
	}, nil
}
