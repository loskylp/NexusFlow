-- Migration 000004: Normalised chain tables (down)
-- Drops chain_steps first (FK references chains), then chains.
-- See: TASK-014

DROP TABLE IF EXISTS chain_steps;
DROP TABLE IF EXISTS chains;
