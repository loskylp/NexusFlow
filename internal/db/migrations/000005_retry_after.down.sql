-- Migration 000005: Add retry_after and retry_tags columns to tasks (down)
-- See: TASK-010

DROP INDEX IF EXISTS idx_tasks_retry_ready;

ALTER TABLE tasks
    DROP COLUMN IF EXISTS retry_after,
    DROP COLUMN IF EXISTS retry_tags;
