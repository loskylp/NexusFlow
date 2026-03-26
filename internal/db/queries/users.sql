-- sqlc query file: users table
-- Generates Go code in internal/db/sqlc/ via `sqlc generate`
-- See: ADR-008, TASK-002, TASK-017

-- name: CreateUser :one
INSERT INTO users (id, username, password_hash, role, active, created_at)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1 LIMIT 1;

-- name: GetUserByUsername :one
SELECT * FROM users WHERE username = $1 LIMIT 1;

-- name: ListUsers :many
SELECT * FROM users ORDER BY created_at ASC;

-- name: DeactivateUser :exec
UPDATE users SET active = FALSE WHERE id = $1;
