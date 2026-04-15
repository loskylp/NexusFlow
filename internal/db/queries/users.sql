-- sqlc query file: users table
-- Generates Go code in internal/db/sqlc/ via `sqlc generate`
-- See: ADR-008, TASK-002, TASK-017

-- name: CreateUser :one
-- SEC-001: must_change_password is included so seed and admin-created users can be
-- flagged for forced rotation on first login.
INSERT INTO users (id, username, password_hash, role, active, must_change_password, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1 LIMIT 1;

-- name: GetUserByUsername :one
SELECT * FROM users WHERE username = $1 LIMIT 1;

-- name: ListUsers :many
SELECT * FROM users ORDER BY created_at ASC;

-- name: DeactivateUser :exec
UPDATE users SET active = FALSE WHERE id = $1;

-- name: UpdateUserPassword :exec
-- SEC-001: update password_hash and clear must_change_password in one atomic statement.
-- Called by PasswordChangeHandler after verifying the current password.
UPDATE users
SET password_hash = $2, must_change_password = FALSE
WHERE id = $1;
