-- sqlc query file: task_logs table (cold log storage, partitioned by week)
-- Generates Go code in internal/db/sqlc/ via `sqlc generate`
-- See: ADR-008, TASK-002, TASK-016

-- name: BatchInsertLogs :exec
-- Inserts a single log line. Callers loop over slices; sqlc does not support
-- bulk-insert with variable-length input directly.
INSERT INTO task_logs (id, task_id, line, level, timestamp)
VALUES ($1, $2, $3, $4, $5);

-- name: ListLogsByTask :many
-- Returns log lines for a task ordered by timestamp.
-- afterID filters for Last-Event-ID replay: only rows with id > afterID are returned.
-- When afterID is the zero UUID ('00000000-0000-0000-0000-000000000000'), all rows are returned.
SELECT * FROM task_logs
WHERE task_id = $1
  AND id > $2
ORDER BY timestamp ASC, id ASC;
