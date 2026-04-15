-- sqlc query file: tasks and task_state_log tables
-- Generates Go code in internal/db/sqlc/ via `sqlc generate`
-- See: ADR-008, TASK-002, TASK-005, TASK-007

-- name: CreateTask :one
INSERT INTO tasks (id, pipeline_id, chain_id, user_id, status, retry_config, retry_count, execution_id, worker_id, input, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
RETURNING *;

-- name: GetTaskByID :one
SELECT * FROM tasks WHERE id = $1 LIMIT 1;

-- name: ListTasksByUser :many
SELECT * FROM tasks WHERE user_id = $1 ORDER BY created_at DESC;

-- name: ListAllTasks :many
SELECT * FROM tasks ORDER BY created_at DESC;

-- name: UpdateTaskStatus :exec
UPDATE tasks
SET status     = $2,
    worker_id  = $3,
    updated_at = NOW()
WHERE id = $1;

-- name: IncrementTaskRetryCount :one
UPDATE tasks
SET retry_count = retry_count + 1,
    updated_at  = NOW()
WHERE id = $1
RETURNING retry_count;

-- name: CancelTask :exec
UPDATE tasks
SET status     = 'cancelled',
    updated_at = NOW()
WHERE id = $1;

-- name: GetTaskStateLog :many
SELECT * FROM task_state_log WHERE task_id = $1 ORDER BY timestamp ASC;

-- name: InsertTaskStateLog :exec
INSERT INTO task_state_log (id, task_id, from_state, to_state, reason, timestamp)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: ListTasksByPipelineAndStatuses :many
SELECT * FROM tasks
WHERE pipeline_id = $1
  AND status = ANY($2::text[])
ORDER BY created_at DESC;

-- name: SetTaskRetryAfterAndTags :exec
-- TASK-010: set retry_after and retry_tags atomically for a task after XCLAIM reclamation.
UPDATE tasks
SET retry_after = $2,
    retry_tags  = $3,
    updated_at  = NOW()
WHERE id = $1;

-- name: ListRetryReadyTasks :many
-- TASK-010: return tasks in "queued" status whose retry_after has elapsed.
SELECT * FROM tasks
WHERE status = 'queued'
  AND retry_after IS NOT NULL
  AND retry_after <= NOW()
ORDER BY retry_after ASC;
