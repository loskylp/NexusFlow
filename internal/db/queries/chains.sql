-- sqlc query file: chains and chain_steps tables
-- Generates Go code in internal/db/sqlc/ via `sqlc generate`
-- See: ADR-008, TASK-014, REQ-014

-- name: CreateChain :one
INSERT INTO chains (id, name, user_id, created_at)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetChainByID :one
SELECT * FROM chains WHERE id = $1 LIMIT 1;

-- name: CreateChainStep :exec
INSERT INTO chain_steps (chain_id, pipeline_id, position)
VALUES ($1, $2, $3);

-- name: GetChainSteps :many
-- Returns all steps for a chain ordered by position (ascending).
SELECT * FROM chain_steps WHERE chain_id = $1 ORDER BY position ASC;

-- name: FindChainByPipeline :one
-- Returns the chain containing the given pipeline_id, if any.
-- Used by the chain trigger to determine whether a completed task's pipeline is in a chain.
SELECT c.* FROM chains c
JOIN chain_steps cs ON cs.chain_id = c.id
WHERE cs.pipeline_id = $1
LIMIT 1;

-- name: GetNextPipelineInChain :one
-- Returns the pipeline_id of the step immediately after the given pipeline in the chain.
-- Returns no rows when the given pipeline is the last step.
SELECT cs_next.pipeline_id
FROM chain_steps cs_current
JOIN chain_steps cs_next
  ON cs_next.chain_id = cs_current.chain_id
 AND cs_next.position = cs_current.position + 1
WHERE cs_current.chain_id    = $1
  AND cs_current.pipeline_id = $2
LIMIT 1;
