-- sqlc query file: task_logs table (cold log storage, partitioned by week)
-- See: ADR-008, TASK-002, TASK-016

-- TODO: Write SQL queries in TASK-002 after migration is complete.
-- Required named queries (sqlc format):
--   -- name: BatchInsertLogs :exec
--   -- name: ListLogsByTask :many  (supports afterID for Last-Event-ID replay, ADR-007)
