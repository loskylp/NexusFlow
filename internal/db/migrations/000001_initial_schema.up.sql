-- Migration 000001: Initial schema (up)
-- Creates the core data model tables for NexusFlow.
-- See: ADR-008, TASK-002, REQ-009, REQ-019, REQ-020

-- ============================================================
-- users
-- ============================================================
CREATE TABLE IF NOT EXISTS users (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    username      TEXT        NOT NULL UNIQUE,
    password_hash TEXT        NOT NULL,
    role          TEXT        NOT NULL CHECK (role IN ('admin', 'user')),
    active        BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============================================================
-- workers
-- ============================================================
CREATE TABLE IF NOT EXISTS workers (
    id              TEXT        PRIMARY KEY,
    tags            TEXT[]      NOT NULL DEFAULT '{}',
    status          TEXT        NOT NULL CHECK (status IN ('online', 'down')),
    last_heartbeat  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    registered_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============================================================
-- pipelines
-- Stores all three phase configs as JSONB so individual phase
-- fields can be queried and validated without separate tables.
-- ============================================================
CREATE TABLE IF NOT EXISTS pipelines (
    id                  UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name                TEXT        NOT NULL,
    user_id             UUID        NOT NULL REFERENCES users(id),
    data_source_config  JSONB       NOT NULL DEFAULT '{}',
    process_config      JSONB       NOT NULL DEFAULT '{}',
    sink_config         JSONB       NOT NULL DEFAULT '{}',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_pipelines_user_id ON pipelines(user_id);

-- ============================================================
-- pipeline_chains
-- Ordered list of pipeline IDs forming a linear chain.
-- ============================================================
CREATE TABLE IF NOT EXISTS pipeline_chains (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name         TEXT        NOT NULL,
    user_id      UUID        NOT NULL REFERENCES users(id),
    pipeline_ids UUID[]      NOT NULL DEFAULT '{}',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_pipeline_chains_user_id ON pipeline_chains(user_id);

-- ============================================================
-- tasks
-- retry_config is stored as JSONB: {maxRetries, backoff}.
-- input is opaque JSONB parameters provided at submission time.
-- ============================================================
CREATE TABLE IF NOT EXISTS tasks (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    pipeline_id  UUID        NOT NULL REFERENCES pipelines(id),
    chain_id     UUID        REFERENCES pipeline_chains(id),
    user_id      UUID        NOT NULL REFERENCES users(id),
    status       TEXT        NOT NULL CHECK (status IN ('submitted','queued','assigned','running','completed','failed','cancelled')),
    retry_config JSONB       NOT NULL DEFAULT '{"maxRetries":3,"backoff":"exponential"}',
    retry_count  INTEGER     NOT NULL DEFAULT 0,
    execution_id TEXT        NOT NULL,
    worker_id    TEXT        REFERENCES workers(id),
    input        JSONB       NOT NULL DEFAULT '{}',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_tasks_user_id    ON tasks(user_id);
CREATE INDEX IF NOT EXISTS idx_tasks_pipeline_id ON tasks(pipeline_id);
CREATE INDEX IF NOT EXISTS idx_tasks_status      ON tasks(status);
CREATE INDEX IF NOT EXISTS idx_tasks_worker_id   ON tasks(worker_id) WHERE worker_id IS NOT NULL;

-- ============================================================
-- task_state_log
-- Records every state transition for audit and replay.
-- The trigger below enforces valid (from_state, to_state) pairs.
-- ============================================================
CREATE TABLE IF NOT EXISTS task_state_log (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id     UUID        NOT NULL REFERENCES tasks(id),
    from_state  TEXT        NOT NULL,
    to_state    TEXT        NOT NULL,
    reason      TEXT        NOT NULL DEFAULT '',
    timestamp   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_task_state_log_task_id ON task_state_log(task_id);

-- ============================================================
-- Task state transition enforcement
-- Valid transitions (Domain Invariant 1, ADR-008):
--   submitted  -> queued
--   queued     -> assigned
--   queued     -> cancelled   (cancel before pickup)
--   assigned   -> running
--   assigned   -> queued      (failover reassignment)
--   assigned   -> cancelled
--   running    -> completed
--   running    -> failed
--   running    -> queued      (failover reassignment)
--   running    -> cancelled
--   failed     -> queued      (retry: monitor re-enqueues)
-- Terminal states (completed, cancelled) have no forward transitions.
-- ============================================================
CREATE OR REPLACE FUNCTION enforce_task_state_transition()
RETURNS TRIGGER AS $$
BEGIN
    IF NOT (
        (NEW.from_state = 'submitted'  AND NEW.to_state IN ('queued'))
        OR (NEW.from_state = 'queued'    AND NEW.to_state IN ('assigned', 'cancelled'))
        OR (NEW.from_state = 'assigned'  AND NEW.to_state IN ('running', 'queued', 'cancelled'))
        OR (NEW.from_state = 'running'   AND NEW.to_state IN ('completed', 'failed', 'queued', 'cancelled'))
        OR (NEW.from_state = 'failed'    AND NEW.to_state IN ('queued'))
    ) THEN
        RAISE EXCEPTION
            'Invalid task state transition: % -> % (task_id: %)',
            NEW.from_state, NEW.to_state, NEW.task_id
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_task_state_transition
    BEFORE INSERT ON task_state_log
    FOR EACH ROW EXECUTE FUNCTION enforce_task_state_transition();

-- ============================================================
-- task_logs
-- Partitioned parent table for cold log storage (ADR-008).
-- Weekly partitions are created dynamically by the application
-- and pruned after 30 days.
-- ============================================================
CREATE TABLE IF NOT EXISTS task_logs (
    id        UUID        NOT NULL DEFAULT gen_random_uuid(),
    task_id   UUID        NOT NULL REFERENCES tasks(id),
    line      TEXT        NOT NULL,
    level     TEXT        NOT NULL DEFAULT 'info',
    timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW()
) PARTITION BY RANGE (timestamp);

-- Default partition catches inserts that do not match any explicit weekly partition.
-- This prevents insert failures during the window between partition creation and pruning.
CREATE TABLE IF NOT EXISTS task_logs_default
    PARTITION OF task_logs DEFAULT;

CREATE INDEX IF NOT EXISTS idx_task_logs_task_id ON task_logs(task_id);
CREATE INDEX IF NOT EXISTS idx_task_logs_timestamp ON task_logs(timestamp);
