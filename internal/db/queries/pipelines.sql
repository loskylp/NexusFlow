-- sqlc query file: pipelines and pipeline_chains tables
-- Generates Go code in internal/db/sqlc/ via `sqlc generate`
-- See: ADR-008, TASK-002, TASK-013, TASK-014

-- name: CreatePipeline :one
INSERT INTO pipelines (id, name, user_id, data_source_config, process_config, sink_config, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetPipelineByID :one
SELECT * FROM pipelines WHERE id = $1 LIMIT 1;

-- name: ListPipelinesByUser :many
SELECT * FROM pipelines WHERE user_id = $1 ORDER BY created_at ASC;

-- name: ListAllPipelines :many
SELECT * FROM pipelines ORDER BY created_at ASC;

-- name: UpdatePipeline :one
UPDATE pipelines
SET name               = $2,
    data_source_config = $3,
    process_config     = $4,
    sink_config        = $5,
    updated_at         = $6
WHERE id = $1
RETURNING *;

-- name: DeletePipeline :exec
DELETE FROM pipelines WHERE id = $1;

-- name: PipelineHasActiveTasks :one
-- Returns TRUE if any non-terminal task references this pipeline.
SELECT EXISTS (
    SELECT 1 FROM tasks
    WHERE pipeline_id = $1
      AND status NOT IN ('completed', 'failed', 'cancelled')
) AS has_active_tasks;
