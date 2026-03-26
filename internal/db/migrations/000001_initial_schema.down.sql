-- Migration 000001 rollback: Drop all initial schema objects.
-- Drops in reverse dependency order to avoid FK constraint violations.
-- See: ADR-008, TASK-002

DROP TRIGGER  IF EXISTS trg_task_state_transition   ON task_state_log;
DROP FUNCTION IF EXISTS enforce_task_state_transition();

-- task_logs is a partitioned table; dropping the parent cascades to all partitions.
DROP TABLE IF EXISTS task_logs;
DROP TABLE IF EXISTS task_state_log;
DROP TABLE IF EXISTS tasks;
DROP TABLE IF EXISTS pipeline_chains;
DROP TABLE IF EXISTS pipelines;
DROP TABLE IF EXISTS workers;
DROP TABLE IF EXISTS users;
