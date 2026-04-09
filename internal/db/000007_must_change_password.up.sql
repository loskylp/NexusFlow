-- Migration 000007: Add must_change_password column to users table (SEC-001).
--
-- The must_change_password flag forces first-time and admin-seeded users to change
-- their password before accessing the application. Until the password is changed,
-- all API requests (except POST /api/auth/change-password) return 403 with the
-- reason "password_change_required".
--
-- The seeded admin user (admin/admin) is created with must_change_password = TRUE
-- so that first startup forces immediate credential rotation. In non-dev environments
-- the ADMIN_SEED_PASSWORD env var must be set; on first login the admin must change it.
--
-- See: SEC-001, TASK-003, ADR-006

ALTER TABLE users
    ADD COLUMN must_change_password BOOLEAN NOT NULL DEFAULT FALSE;

-- All existing users are treated as having already set their password.
-- The seeded admin user is handled by seedAdminIfEmpty at startup, not here.
-- This migration sets the default to FALSE so existing users are unaffected.
