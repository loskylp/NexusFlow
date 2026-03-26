-- sqlc query file: workers table
-- Generates Go code in internal/db/sqlc/ via `sqlc generate`
-- See: ADR-008, TASK-002, TASK-006

-- name: RegisterWorker :one
INSERT INTO workers (id, tags, status, last_heartbeat, registered_at)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (id) DO UPDATE
    SET tags           = EXCLUDED.tags,
        status         = EXCLUDED.status,
        last_heartbeat = EXCLUDED.last_heartbeat
RETURNING *;

-- name: GetWorkerByID :one
SELECT * FROM workers WHERE id = $1 LIMIT 1;

-- name: ListWorkers :many
-- current_task_id is the most recent non-terminal task assigned to this worker.
SELECT
    w.id,
    w.tags,
    w.status,
    w.last_heartbeat,
    w.registered_at,
    (
        SELECT t.id
        FROM tasks t
        WHERE t.worker_id = w.id
          AND t.status IN ('assigned', 'running')
        ORDER BY t.updated_at DESC
        LIMIT 1
    ) AS current_task_id
FROM workers w
ORDER BY w.registered_at ASC;

-- name: UpdateWorkerStatus :exec
UPDATE workers
SET status         = $2,
    last_heartbeat = NOW()
WHERE id = $1;
