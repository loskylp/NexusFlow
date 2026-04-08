-- Migration 000006: Pre-create weekly partitions for task_logs (up)
--
-- The task_logs table was created as PARTITION BY RANGE (timestamp) in migration 000001.
-- This migration pre-creates the partition for the current ISO week plus 4 future weeks
-- so that log inserts go to named partitions immediately (not the default catch-all).
--
-- Partition naming: task_logs_YYYY_WW (e.g., task_logs_2026_04) using ISO 8601 week numbers.
-- Range: [Monday 00:00:00 UTC, following Monday 00:00:00 UTC)
--
-- The application (retention.StartRetentionJobs) maintains the partition table going forward:
--   - DropOldPartitions drops partitions older than 30 days (weekly job)
--   - EnsureUpcomingPartitions (not yet wired) pre-creates the next N weeks on startup
--     to guarantee inserts always land in a named partition.
--
-- For now, this migration creates the initial 5 partitions relative to migration run time.
-- This covers the immediate deployment window.
--
-- See: ADR-008, TASK-028, FF-018

-- Helper function to create a weekly partition if it does not already exist.
-- Idempotent: CREATE TABLE IF NOT EXISTS prevents errors on repeated migration.
--
-- Args:
--   p_year : ISO year
--   p_week : ISO week number (1..53)
CREATE OR REPLACE FUNCTION create_weekly_partition(p_year INT, p_week INT)
RETURNS VOID AS $$
DECLARE
    v_name      TEXT;
    v_start     TIMESTAMPTZ;
    v_end       TIMESTAMPTZ;
    v_jan4      DATE;
    v_jan4_dow  INT;   -- day of week of Jan 4 (1=Mon, 7=Sun in ISO)
    v_week1_mon DATE;
    v_week_mon  DATE;
BEGIN
    -- ISO 8601: Jan 4 is always in week 1. Find week 1 Monday.
    v_jan4      := make_date(p_year, 1, 4);
    -- ISODOW: 1=Monday, 7=Sunday
    v_jan4_dow  := EXTRACT(ISODOW FROM v_jan4)::INT;
    v_week1_mon := v_jan4 - (v_jan4_dow - 1) * INTERVAL '1 day';
    v_week_mon  := v_week1_mon + (p_week - 1) * INTERVAL '7 days';

    v_start := v_week_mon::TIMESTAMPTZ AT TIME ZONE 'UTC';
    v_end   := (v_week_mon + INTERVAL '7 days')::TIMESTAMPTZ AT TIME ZONE 'UTC';

    v_name  := format('task_logs_%s_%s', lpad(p_year::TEXT, 4, '0'), lpad(p_week::TEXT, 2, '0'));

    EXECUTE format(
        'CREATE TABLE IF NOT EXISTS %I PARTITION OF task_logs FOR VALUES FROM (%L) TO (%L)',
        v_name, v_start, v_end
    );
END;
$$ LANGUAGE plpgsql;

-- Pre-create the current week and 4 future weeks.
-- EXTRACT(ISOYEAR/WEEK) gives ISO year and week of the current transaction timestamp.
DO $$
DECLARE
    base_date  DATE := CURRENT_DATE;
    i          INT;
    y          INT;
    w          INT;
BEGIN
    FOR i IN 0..4 LOOP
        -- Advance by i weeks from today.
        y := EXTRACT(ISOYEAR FROM (base_date + i * INTERVAL '7 days'))::INT;
        w := EXTRACT(WEEK    FROM (base_date + i * INTERVAL '7 days'))::INT;
        PERFORM create_weekly_partition(y, w);
    END LOOP;
END;
$$;
