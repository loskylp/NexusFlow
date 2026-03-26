-- sqlc query file: workers table
-- See: ADR-008, TASK-002, TASK-006

-- TODO: Write SQL queries in TASK-002 after migration is complete.
-- Required named queries (sqlc format):
--   -- name: RegisterWorker :one  (upsert)
--   -- name: GetWorkerByID :one
--   -- name: ListWorkers :many    (JOIN tasks to populate current_task_id)
--   -- name: UpdateWorkerStatus :exec
