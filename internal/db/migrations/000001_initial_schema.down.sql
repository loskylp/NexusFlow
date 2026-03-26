-- Migration 000001 rollback: Drop all initial schema tables.
-- See: ADR-008, TASK-002

-- TODO: Implement in TASK-002
-- DROP tables in reverse dependency order:
--   task_logs, task_state_log, tasks, pipeline_chains, pipelines, workers, users
