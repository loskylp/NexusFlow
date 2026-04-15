// Package db — PostgreSQL implementation of UserRepository.
// Backed by sqlc-generated queries. Translates between sqlcdb.User (generated) and models.User (domain).
// See: ADR-008, TASK-003
package db

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/nxlabs/nexusflow/internal/db/sqlc"
	"github.com/nxlabs/nexusflow/internal/models"
)

// PgUserRepository implements UserRepository backed by PostgreSQL via sqlc-generated queries.
// See: ADR-008, TASK-003
type PgUserRepository struct {
	queries *sqlcdb.Queries
}

// NewPgUserRepository constructs a PgUserRepository from the given connection pool.
// Panics if pool is nil (fail-fast: nil pool causes silent failures on every call).
//
// Args:
//
//	pool: A connected pgxpool.Pool. Must not be nil.
func NewPgUserRepository(pool *Pool) *PgUserRepository {
	if pool == nil {
		panic("db.NewPgUserRepository: pool must not be nil")
	}
	return &PgUserRepository{queries: sqlcdb.New(pool)}
}

// Create implements UserRepository.Create.
// Inserts a new user record. The PasswordHash must already be bcrypt-hashed.
// Returns ErrConflict if the username is already taken.
//
// SEC-001: MustChangePassword is passed through so seed users and admin-created users
// can be flagged for forced password rotation on first login.
//
// Postconditions:
//   - On success: user is persisted; returned User has database-populated timestamps.
//   - On ErrConflict: no user is created; caller maps to 409.
func (r *PgUserRepository) Create(ctx context.Context, user *models.User) (*models.User, error) {
	row, err := r.queries.CreateUser(ctx, sqlcdb.CreateUserParams{
		ID:                 user.ID,
		Username:           user.Username,
		PasswordHash:       user.PasswordHash,
		Role:               string(user.Role),
		Active:             user.Active,
		MustChangePassword: user.MustChangePassword,
		CreatedAt:          pgtype.Timestamptz{Time: user.CreatedAt, Valid: true},
	})
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrConflict
		}
		return nil, err
	}
	return toModelUser(row), nil
}

// GetByID implements UserRepository.GetByID.
// Returns nil, nil if no user with the given ID exists.
func (r *PgUserRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	row, err := r.queries.GetUserByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return toModelUser(row), nil
}

// GetByUsername implements UserRepository.GetByUsername.
// Returns nil, nil if no user with the given username exists.
func (r *PgUserRepository) GetByUsername(ctx context.Context, username string) (*models.User, error) {
	row, err := r.queries.GetUserByUsername(ctx, username)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return toModelUser(row), nil
}

// List implements UserRepository.List.
// Returns all users ordered by creation time. Admin-only at the service layer.
// Returns an empty slice (not nil) when no users exist.
func (r *PgUserRepository) List(ctx context.Context) ([]*models.User, error) {
	rows, err := r.queries.ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	users := make([]*models.User, 0, len(rows))
	for _, row := range rows {
		users = append(users, toModelUser(row))
	}
	return users, nil
}

// Deactivate implements UserRepository.Deactivate.
// Sets the user's active flag to false. Session invalidation is the caller's responsibility.
//
// Postconditions:
//   - On success: user.Active = false in the database.
func (r *PgUserRepository) Deactivate(ctx context.Context, id uuid.UUID) error {
	return r.queries.DeactivateUser(ctx, id)
}

// ChangePassword implements UserRepository.ChangePassword.
// Updates the user's password_hash to passwordHash and sets must_change_password = false.
// The caller must verify the current password and hash the new one before calling.
//
// Preconditions:
//   - id references an existing active user.
//   - passwordHash is a valid bcrypt hash.
//
// Postconditions:
//   - On success: user.PasswordHash and must_change_password are updated atomically.
//   - On error: no change to the user record.
func (r *PgUserRepository) ChangePassword(ctx context.Context, id uuid.UUID, passwordHash string) error {
	return r.queries.UpdateUserPassword(ctx, sqlcdb.UpdateUserPasswordParams{
		ID:           id,
		PasswordHash: passwordHash,
	})
}

// toModelUser converts a sqlcdb.User (generated) to a models.User (domain).
// The pgtype.Timestamptz is converted to time.Time with UTC location.
// MustChangePassword is included so the auth middleware can enforce the
// mandatory first-login password change (SEC-001).
func toModelUser(row sqlcdb.User) *models.User {
	createdAt := time.Time{}
	if row.CreatedAt.Valid {
		createdAt = row.CreatedAt.Time.UTC()
	}
	return &models.User{
		ID:                 row.ID,
		Username:           row.Username,
		PasswordHash:       row.PasswordHash,
		Role:               models.Role(row.Role),
		Active:             row.Active,
		MustChangePassword: row.MustChangePassword,
		CreatedAt:          createdAt,
	}
}

// isUniqueViolation returns true when the error is a PostgreSQL unique constraint violation.
// Unique violation SQLSTATE: 23505.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "23505") || strings.Contains(msg, "unique_violation")
}
