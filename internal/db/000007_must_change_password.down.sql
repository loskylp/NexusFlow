-- Migration 000007 rollback: Remove must_change_password column from users table.
-- See: SEC-001

ALTER TABLE users
    DROP COLUMN IF EXISTS must_change_password;
