-- Migration 000002: Revert tasks.pipeline_id to NOT NULL with plain FK (down)
-- Reverses the up migration: restores the original non-nullable FK without ON DELETE.
-- WARNING: This will fail if any task rows currently have pipeline_id = NULL.
-- Ensure all NULL pipeline_ids are resolved before running this rollback.
-- See: TASK-013 fix iteration 2

-- Drop the ON DELETE SET NULL FK introduced by the up migration.
ALTER TABLE tasks DROP CONSTRAINT tasks_pipeline_id_fkey;

-- Restore NOT NULL constraint. Fails if any task has pipeline_id = NULL.
ALTER TABLE tasks ALTER COLUMN pipeline_id SET NOT NULL;

-- Re-create the original plain FK (RESTRICT is the PostgreSQL default).
ALTER TABLE tasks
    ADD CONSTRAINT tasks_pipeline_id_fkey
    FOREIGN KEY (pipeline_id) REFERENCES pipelines(id);
