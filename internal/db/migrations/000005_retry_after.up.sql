-- Migration 000005: Add retry_after and retry_tags columns to tasks (up)
-- retry_after gates re-enqueue until backoff delay elapses.
-- retry_tags records which stream(s) the task must be re-enqueued to when retry_after elapses.
-- Both columns are set together by the Monitor during XCLAIM reclamation (TASK-010).
-- The partial index makes the Monitor's ListRetryReady query efficient.
-- See: REQ-011, TASK-010

ALTER TABLE tasks
    ADD COLUMN IF NOT EXISTS retry_after TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS retry_tags  TEXT[] NOT NULL DEFAULT '{}';

CREATE INDEX IF NOT EXISTS idx_tasks_retry_ready
    ON tasks (retry_after)
    WHERE status = 'queued' AND retry_after IS NOT NULL;
