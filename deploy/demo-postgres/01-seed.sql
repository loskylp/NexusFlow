-- demo-postgres seed script (TASK-031, DEMO-002)
-- Mounted at /docker-entrypoint-initdb.d/ in the demo-postgres container.
-- Executed exactly once on first container startup.
--
-- Creates the sample_data table and inserts 10 000 deterministic rows.
-- Row content is stable across restarts because the data is generated once
-- and persisted in the demo-pg-data Docker volume.
--
-- See: DEMO-002, TASK-031

CREATE TABLE IF NOT EXISTS sample_data (
    id         SERIAL PRIMARY KEY,
    name       TEXT        NOT NULL,
    category   TEXT        NOT NULL,
    value      INTEGER     NOT NULL,
    score      NUMERIC(6,2) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Insert 10 000 rows using generate_series for determinism.
-- Row values are derived from the series index so they are reproducible.
INSERT INTO sample_data (name, category, value, score)
SELECT
    'record-' || i::TEXT                               AS name,
    CASE (i % 5)
        WHEN 0 THEN 'alpha'
        WHEN 1 THEN 'beta'
        WHEN 2 THEN 'gamma'
        WHEN 3 THEN 'delta'
        ELSE        'epsilon'
    END                                                AS category,
    (i % 1000) + 1                                     AS value,
    ROUND(((i % 100) + (i % 7) * 0.14)::NUMERIC, 2)   AS score
FROM generate_series(1, 10000) AS s(i);

-- Create an output table for sink connector tests.
-- Pipelines writing to demo-postgres use this table as their destination.
CREATE TABLE IF NOT EXISTS demo_output (
    id         SERIAL PRIMARY KEY,
    data       JSONB       NOT NULL,
    written_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Grant explicit privileges to the demo user (idempotent).
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO demo;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO demo;
