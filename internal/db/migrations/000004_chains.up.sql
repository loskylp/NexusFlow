-- Migration 000004: Normalised chain tables (up)
-- Creates the chains and chain_steps tables for pipeline chain definition (TASK-014).
-- Chains are strictly linear: each pipeline_id appears at most once per chain.
-- chain_steps.position enforces ordering; UNIQUE(chain_id, position) prevents branching.
-- See: REQ-014, ADR-003, ADR-008, TASK-014

-- ============================================================
-- chains
-- Top-level chain record. Owns a name and belongs to a user.
-- ============================================================
CREATE TABLE IF NOT EXISTS chains (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT        NOT NULL,
    user_id    UUID        NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_chains_user_id ON chains(user_id);

-- ============================================================
-- chain_steps
-- Each row is one step in a chain: (chain_id, position) -> pipeline_id.
-- UNIQUE(chain_id, position) ensures no two steps share the same position,
-- enforcing strict linearity (no branching).
-- UNIQUE(chain_id, pipeline_id) ensures a pipeline appears at most once per chain.
-- ============================================================
CREATE TABLE IF NOT EXISTS chain_steps (
    chain_id    UUID    NOT NULL REFERENCES chains(id) ON DELETE CASCADE,
    pipeline_id UUID    NOT NULL REFERENCES pipelines(id),
    position    INTEGER NOT NULL,
    PRIMARY KEY (chain_id, pipeline_id),
    UNIQUE (chain_id, position)
);

CREATE INDEX IF NOT EXISTS idx_chain_steps_chain_id ON chain_steps(chain_id);
