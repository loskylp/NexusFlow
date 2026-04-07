// Package retention implements log retention management for NexusFlow (TASK-028).
//
// Two retention mechanisms are implemented:
//
//  1. PostgreSQL partition pruning: the task_logs table uses weekly partitioning
//     (ADR-008). Partitions older than 30 days are dropped by DropOldPartitions.
//     This runs as a weekly background job to avoid unbounded storage growth.
//
//  2. Redis Streams hot log trimming: each logs:{taskId} stream is trimmed to
//     enforce a 72-hour retention window via XTRIM MAXLEN with approximate
//     trimming (~). The exact entry count is derived from the stream's message
//     arrival rate at the time of trimming — see TrimHotLogs for the strategy.
//
// Both jobs run as goroutines started by StartRetentionJobs. They run
// independently on their own tickers and log errors without panicking.
//
// See: ADR-008, TASK-028, FF-018
package retention

import (
	"context"
	"time"

	"github.com/nxlabs/nexusflow/internal/db"
	"github.com/redis/go-redis/v9"
)

// partitionPruneInterval is how often the partition pruner runs.
// Weekly is sufficient — partitions are weekly and the 30-day window keeps ~4.
const partitionPruneInterval = 7 * 24 * time.Hour

// logTrimInterval is how often the Redis hot log streams are trimmed.
// Hourly keeps the trim lag within the 72-hour window at reasonable overhead.
const logTrimInterval = 1 * time.Hour

// hotLogMaxAgeHours is the maximum age for entries in Redis log streams.
// ADR-008: "72-hour hot retention window".
const hotLogMaxAgeHours = 72

// Stub: reference constants to satisfy staticcheck U1000 until implementation.
var (
	_ = partitionPruneInterval
	_ = logTrimInterval
	_ = hotLogMaxAgeHours
)

// StartRetentionJobs launches the partition pruner and Redis log trimmer
// as background goroutines. Both run until ctx is cancelled.
//
// Args:
//
//	ctx:     A context whose cancellation stops both goroutines.
//	pool:    PostgreSQL pool used by DropOldPartitions.
//	client:  Redis client used by TrimHotLogs.
//
// Preconditions:
//   - pool and client are non-nil and their connections are open.
//
// Postconditions:
//   - On ctx cancellation: both goroutines exit after the current operation completes.
func StartRetentionJobs(ctx context.Context, pool *db.Pool, client *redis.Client) {
	// TODO: implement
	panic("not implemented")
}

// DropOldPartitions drops PostgreSQL task_logs partitions older than 30 days.
//
// The task_logs table uses weekly range partitions named:
//   task_logs_YYYY_WW (e.g., task_logs_2026_14)
// Partitions whose end boundary is older than 30 days from now are dropped via
// DROP TABLE IF EXISTS task_logs_YYYY_WW CASCADE.
//
// Args:
//
//	ctx:  Context for the database operation.
//	pool: PostgreSQL connection pool.
//
// Returns:
//   - The number of partitions dropped.
//   - An error if the partition listing query fails (individual DROP failures
//     are logged but do not return an error — a failed DROP is retried next cycle).
//
// Preconditions:
//   - task_logs is partitioned by week with naming convention task_logs_YYYY_WW.
//
// Postconditions:
//   - All partitions whose end boundary < now - 30 days are dropped.
//   - Partitions within the 30-day window are not touched.
//   - Log insertion for current/recent weeks continues unaffected.
func DropOldPartitions(ctx context.Context, pool *db.Pool) (int, error) {
	// TODO: implement
	panic("not implemented")
}

// TrimHotLogs scans all logs:{taskId} streams in Redis and trims entries
// older than hotLogMaxAgeHours using XTRIM MAXLEN ~ with approximate trimming.
//
// Scan strategy: SCAN for keys matching "logs:*" in batches of 100.
// Trim strategy: XTRIM logs:{taskId} MINID ~ <cutoff-timestamp-ms>-0
// where cutoff-timestamp-ms is (now - 72h) in Unix milliseconds.
//
// Args:
//
//	ctx:    Context for all Redis operations.
//	client: Redis client connected to the NexusFlow Redis instance.
//
// Returns:
//   - The number of log stream keys trimmed (at least 1 entry removed).
//   - An error if the SCAN command fails. Individual XTRIM failures are
//     logged but do not return an error.
//
// Preconditions:
//   - Log stream keys follow the naming convention logs:{taskId}.
//
// Postconditions:
//   - No entries older than 72 hours remain in any logs:{taskId} stream.
//   - Streams for recently completed tasks (< 72 hours) are not trimmed.
func TrimHotLogs(ctx context.Context, client *redis.Client) (int, error) {
	// TODO: implement
	panic("not implemented")
}
