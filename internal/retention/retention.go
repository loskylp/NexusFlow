// Package retention implements log retention management for NexusFlow (TASK-028).
//
// Two retention mechanisms are implemented:
//
//  1. PostgreSQL partition pruning: the task_logs table uses weekly partitioning
//     (ADR-008). Partitions older than 30 days are dropped by DropOldPartitions.
//     This runs as a weekly background job to avoid unbounded storage growth.
//
//  2. Redis Streams hot log trimming: each logs:{taskId} stream is trimmed to
//     enforce a 72-hour retention window via XTRIM MINID with approximate
//     trimming (~). The cutoff is now-72h expressed as a Redis Stream MINID
//     string: "<unix-ms>-0".
//
// Both jobs run as goroutines started by StartRetentionJobs. They run
// independently on their own tickers and log errors without panicking.
//
// See: ADR-008, TASK-028, FF-018
package retention

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
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

// partitionRetentionDays is the number of days after which a partition is dropped.
const partitionRetentionDays = 30

// redisScanBatchSize is the number of keys returned per SCAN iteration.
const redisScanBatchSize = 100

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
	// Partition pruner: runs weekly.
	go func() {
		ticker := time.NewTicker(partitionPruneInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				n, err := DropOldPartitions(ctx, pool)
				if err != nil {
					log.Printf("retention: partition pruner: %v", err)
				} else {
					log.Printf("retention: partition pruner: dropped %d partition(s)", n)
				}
			}
		}
	}()

	// Redis log trimmer: runs hourly.
	go func() {
		ticker := time.NewTicker(logTrimInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				n, err := TrimHotLogs(ctx, client)
				if err != nil {
					log.Printf("retention: redis trimmer: %v", err)
				} else {
					log.Printf("retention: redis trimmer: trimmed %d stream(s)", n)
				}
			}
		}
	}()
}

// DropOldPartitions drops PostgreSQL task_logs partitions older than 30 days.
//
// The task_logs table uses weekly range partitions named:
//
//	task_logs_YYYY_WW (e.g., task_logs_2026_14)
//
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
	// Query pg_inherits joined to pg_class to list child partitions of task_logs.
	// pg_inherits.inhparent = OID of the parent table; inhrelid = OID of the child.
	const listQuery = `
		SELECT c.relname
		FROM pg_inherits i
		JOIN pg_class c ON c.oid = i.inhrelid
		JOIN pg_class p ON p.oid = i.inhparent
		WHERE p.relname = 'task_logs'
		  AND c.relname LIKE 'task_logs_%_%'
		ORDER BY c.relname`

	rows, err := pool.Query(ctx, listQuery)
	if err != nil {
		return 0, fmt.Errorf("DropOldPartitions: list partitions: %w", err)
	}
	defer rows.Close()

	var partitions []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return 0, fmt.Errorf("DropOldPartitions: scan partition name: %w", err)
		}
		partitions = append(partitions, name)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("DropOldPartitions: iterate partitions: %w", err)
	}

	now := time.Now().UTC()
	dropped := 0

	for _, name := range partitions {
		year, week, err := parsePartitionDate(name)
		if err != nil {
			// Skip partitions with non-standard names (e.g., task_logs_default).
			log.Printf("retention: DropOldPartitions: skip %q: %v", name, err)
			continue
		}

		// Compute the end bound of this partition's week.
		// The end bound is the Monday that starts the *next* week (exclusive upper bound).
		_, end := weekBoundsFromYearWeek(year, week)

		if isPartitionOlderThan30Days(end, now) {
			dropSQL := fmt.Sprintf("DROP TABLE IF EXISTS %s", pgQuoteIdent(name))
			if _, err := pool.Exec(ctx, dropSQL); err != nil {
				// Non-fatal: log and continue — DROP will be retried next weekly cycle.
				log.Printf("retention: DropOldPartitions: DROP %s: %v", name, err)
				continue
			}
			log.Printf("retention: DropOldPartitions: dropped %s", name)
			dropped++
		}
	}

	return dropped, nil
}

// TrimHotLogs scans all logs:{taskId} streams in Redis and trims entries
// older than hotLogMaxAgeHours using XTRIM MINID with approximate trimming (~).
//
// Scan strategy: SCAN for keys matching "logs:*" in batches of redisScanBatchSize.
// Trim strategy: XTRIM logs:{taskId} MINID ~ <cutoff-timestamp-ms>-0
// where cutoff-timestamp-ms is (now - 72h) in Unix milliseconds.
//
// Args:
//
//	ctx:    Context for all Redis operations.
//	client: Redis client connected to the NexusFlow Redis instance.
//
// Returns:
//   - The number of log stream keys trimmed (at least 1 entry removed per key counted).
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
	cutoffID := hotLogCutoffID(time.Now().UTC())
	trimmed := 0

	var cursor uint64
	for {
		keys, nextCursor, err := client.Scan(ctx, cursor, "logs:*", redisScanBatchSize).Result()
		if err != nil {
			return trimmed, fmt.Errorf("TrimHotLogs: SCAN logs:*: %w", err)
		}

		for _, key := range keys {
			// XTRIM key MINID ~ <cutoff-id>
			// MINID removes entries with IDs less than cutoff-id.
			// The ~ flag allows approximate trimming for performance.
			removed, err := client.XTrimMinID(ctx, key, cutoffID).Result()
			if err != nil {
				log.Printf("retention: TrimHotLogs: XTRIM %s: %v", key, err)
				continue
			}
			if removed > 0 {
				trimmed++
			}
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	return trimmed, nil
}

// ---------------------------------------------------------------------------
// Package-internal helpers (exported for testing within the package)
// ---------------------------------------------------------------------------

// partitionNameFromBounds constructs the partition table name from year and ISO week number.
// The naming convention is task_logs_YYYY_WW with zero-padded week (e.g., task_logs_2026_04).
func partitionNameFromBounds(year, week int) string {
	return fmt.Sprintf("task_logs_%04d_%02d", year, week)
}

// weekBounds returns the UTC start (Monday 00:00:00) and exclusive end (following Monday 00:00:00)
// of the ISO week containing t.
//
// Go's time.Weekday uses Sunday=0, Monday=1. ISO 8601 weeks start on Monday.
// This function converts to ISO 8601 convention by treating Sunday as day 7 (end of week).
func weekBounds(t time.Time) (start, end time.Time) {
	t = t.UTC()
	wd := int(t.Weekday()) // Sunday=0, Monday=1, ..., Saturday=6
	if wd == 0 {
		wd = 7 // treat Sunday as day 7 (ISO: Sunday is last day of week)
	}
	// daysFromMonday: 0 for Monday, 1 for Tuesday, ..., 6 for Sunday
	daysFromMonday := wd - 1

	start = time.Date(t.Year(), t.Month(), t.Day()-daysFromMonday, 0, 0, 0, 0, time.UTC)
	end = start.AddDate(0, 0, 7)
	return start, end
}

// weekBoundsFromYearWeek returns the UTC start and exclusive end of the given ISO year and week.
// It derives the bounds by finding the Monday of the given week using ISO 8601 week date arithmetic.
func weekBoundsFromYearWeek(year, week int) (start, end time.Time) {
	// Jan 4 is always in ISO week 1 of its year (ISO 8601 rule).
	jan4 := time.Date(year, 1, 4, 0, 0, 0, 0, time.UTC)
	// Find Monday of week 1.
	jan4WD := int(jan4.Weekday())
	if jan4WD == 0 {
		jan4WD = 7
	}
	// monday of week 1 = jan4 - (jan4WD - 1) days
	week1Monday := jan4.AddDate(0, 0, -(jan4WD - 1))
	// Monday of target week
	start = week1Monday.AddDate(0, 0, (week-1)*7)
	end = start.AddDate(0, 0, 7)
	return start, end
}

// isPartitionOlderThan30Days reports whether the partition's exclusive end bound
// is more than partitionRetentionDays days before now.
//
// Args:
//
//	partitionEnd: The exclusive upper bound of the partition's range.
//	now:          The current time, used as the reference point.
func isPartitionOlderThan30Days(partitionEnd, now time.Time) bool {
	cutoff := now.Add(-partitionRetentionDays * 24 * time.Hour)
	return partitionEnd.Before(cutoff)
}

// parsePartitionDate parses the year and ISO week from a partition name
// in the format task_logs_YYYY_WW.
//
// Returns an error if the name does not match the expected format.
func parsePartitionDate(name string) (year, week int, err error) {
	const prefix = "task_logs_"
	if !strings.HasPrefix(name, prefix) {
		return 0, 0, fmt.Errorf("parsePartitionDate: %q: wrong prefix (want %q)", name, prefix)
	}
	rest := name[len(prefix):]
	parts := strings.SplitN(rest, "_", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("parsePartitionDate: %q: expected YYYY_WW suffix", name)
	}

	year, err = strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("parsePartitionDate: %q: year: %w", name, err)
	}

	week, err = strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, fmt.Errorf("parsePartitionDate: %q: week: %w", name, err)
	}

	return year, week, nil
}

// hotLogCutoffID returns the Redis Stream MINID string for the 72-hour retention cutoff.
// The MINID format is "<unix-ms>-0", which trims entries with IDs less than this value.
//
// Args:
//
//	now: The current time; the cutoff is now - hotLogMaxAgeHours.
func hotLogCutoffID(now time.Time) string {
	cutoffMs := now.Add(-hotLogMaxAgeHours * time.Hour).UnixMilli()
	return fmt.Sprintf("%d-0", cutoffMs)
}

// pgQuoteIdent quotes a PostgreSQL identifier to prevent SQL injection.
// Replaces double-quotes in the identifier with paired double-quotes.
// This is appropriate for table names produced by the internal partition naming logic.
func pgQuoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
