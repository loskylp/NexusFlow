-- Migration 000003: Sink deduplication log (down)
-- Drops the sink_dedup_log table.
-- WARNING: dropping this table removes the idempotency guarantee for all Sink types.
-- Only apply this rollback in a development/test environment.
-- See: ADR-003, ADR-009, TASK-018

DROP TABLE IF EXISTS sink_dedup_log;
