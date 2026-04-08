-- Migration 000006: Pre-create weekly partitions for task_logs (down)
--
-- Drops the helper function and all weekly partitions matching the task_logs_YYYY_WW
-- naming convention that were created by the up migration.
-- The default partition (task_logs_default) is retained — it was created in migration 000001.
--
-- See: ADR-008, TASK-028

-- Drop all weekly partitions (not the default partition).
DO $$
DECLARE
    r RECORD;
BEGIN
    FOR r IN
        SELECT c.relname
        FROM pg_inherits i
        JOIN pg_class c ON c.oid = i.inhrelid
        JOIN pg_class p ON p.oid = i.inhparent
        WHERE p.relname = 'task_logs'
          AND c.relname ~ '^task_logs_\d{4}_\d{2}$'
    LOOP
        EXECUTE format('DROP TABLE IF EXISTS %I', r.relname);
    END LOOP;
END;
$$;

-- Drop the helper function.
DROP FUNCTION IF EXISTS create_weekly_partition(INT, INT);
