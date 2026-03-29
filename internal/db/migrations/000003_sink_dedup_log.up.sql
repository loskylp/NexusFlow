-- Migration 000003: Sink deduplication log (up)
-- Creates the sink_dedup_log table used by the production DatabaseSinkConnector
-- to enforce at-least-once delivery with idempotent Sink writes.
-- Each row records an executionID that has been successfully committed at a Sink destination.
-- Before writing, the Sink checks for the executionID here; if found, it skips the write.
-- After a successful COMMIT, it inserts the executionID here.
--
-- The executionID format is "{taskID}:{attemptNumber}" (ADR-003, ADR-009, TASK-018).
-- See: ADR-003, ADR-009, REQ-008, TASK-018

CREATE TABLE IF NOT EXISTS sink_dedup_log (
    execution_id TEXT        PRIMARY KEY,
    connector_type TEXT      NOT NULL,
    applied_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Index is redundant for a PRIMARY KEY (already has an implicit unique index) but
-- included explicitly for documentation and query plan visibility.
-- Dropped in favour of the implicit PRIMARY KEY index.
-- CREATE UNIQUE INDEX IF NOT EXISTS idx_sink_dedup_log_execution_id ON sink_dedup_log(execution_id);

COMMENT ON TABLE sink_dedup_log IS
    'Records execution IDs that have been atomically committed at a Sink destination. '
    'Used to enforce idempotency: a second write attempt with the same execution_id is '
    'rejected before any rows are written. See ADR-003, ADR-009.';
