// Package db — PostgreSQL implementation of WorkerRepository.
// Backed by sqlc-generated queries. Translates between sqlcdb.Worker (generated) and
// models.Worker (domain). The workers table uses TEXT as the primary key (not UUID)
// because worker IDs are hostname-or-UUID strings assigned at startup, not database-generated.
// See: ADR-008, TASK-006
package db

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/nxlabs/nexusflow/internal/db/sqlc"
	"github.com/nxlabs/nexusflow/internal/models"
)

// PgWorkerRepository implements WorkerRepository backed by PostgreSQL via sqlc-generated queries.
// See: ADR-008, TASK-006
type PgWorkerRepository struct {
	queries *sqlcdb.Queries
}

// NewPgWorkerRepository constructs a PgWorkerRepository from the given connection pool.
// Panics if pool is nil (fail-fast: nil pool causes silent failures on every call).
//
// Args:
//
//	pool: A connected pgxpool.Pool. Must not be nil.
func NewPgWorkerRepository(pool *Pool) *PgWorkerRepository {
	if pool == nil {
		panic("db.NewPgWorkerRepository: pool must not be nil")
	}
	return &PgWorkerRepository{queries: sqlcdb.New(pool)}
}

// Register implements WorkerRepository.Register.
// Upserts the worker record: inserts on first registration, updates tags, status,
// and last_heartbeat on subsequent registrations (e.g. worker restart).
//
// Postconditions:
//   - On success: worker exists in PostgreSQL with the provided ID, tags, and status.
func (r *PgWorkerRepository) Register(ctx context.Context, w *models.Worker) (*models.Worker, error) {
	row, err := r.queries.RegisterWorker(ctx, sqlcdb.RegisterWorkerParams{
		ID:            w.ID,
		Tags:          w.Tags,
		Status:        string(w.Status),
		LastHeartbeat: toTimestamptz(w.LastHeartbeat),
		RegisteredAt:  toTimestamptz(w.RegisteredAt),
	})
	if err != nil {
		return nil, err
	}
	return toModelWorker(row), nil
}

// GetByID implements WorkerRepository.GetByID.
// Returns nil, nil if no worker with the given ID exists.
func (r *PgWorkerRepository) GetByID(ctx context.Context, id string) (*models.Worker, error) {
	row, err := r.queries.GetWorkerByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return toModelWorker(row), nil
}

// List implements WorkerRepository.List.
// Returns all registered workers ordered by registration time (oldest first).
// CurrentTaskID is populated from a subquery on the tasks table.
// Returns an empty slice (not nil) when no workers are registered.
func (r *PgWorkerRepository) List(ctx context.Context) ([]*models.Worker, error) {
	rows, err := r.queries.ListWorkers(ctx)
	if err != nil {
		return nil, err
	}
	workers := make([]*models.Worker, 0, len(rows))
	for _, row := range rows {
		workers = append(workers, toModelWorkerFromListRow(row))
	}
	return workers, nil
}

// UpdateStatus implements WorkerRepository.UpdateStatus.
// Sets the worker's status and refreshes last_heartbeat to NOW() via the SQL query.
// Called by the Monitor when marking a worker down (ADR-002) and by the worker
// itself during graceful shutdown.
//
// Postconditions:
//   - On success: worker.Status = status in the database; last_heartbeat is refreshed.
func (r *PgWorkerRepository) UpdateStatus(ctx context.Context, id string, status models.WorkerStatus) error {
	return r.queries.UpdateWorkerStatus(ctx, sqlcdb.UpdateWorkerStatusParams{
		ID:     id,
		Status: string(status),
	})
}

// toTimestamptz converts a time.Time to a pgtype.Timestamptz for use in sqlc parameters.
// Always uses UTC to ensure consistent storage regardless of the server timezone.
func toTimestamptz(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t.UTC(), Valid: true}
}

// fromTimestamptz converts a pgtype.Timestamptz returned by sqlc to a time.Time in UTC.
// Returns the zero time.Time when the column value is NULL (Valid = false).
func fromTimestamptz(ts pgtype.Timestamptz) time.Time {
	if !ts.Valid {
		return time.Time{}
	}
	return ts.Time.UTC()
}

// toModelWorker converts a sqlcdb.Worker (generated) to a models.Worker (domain).
// pgtype.Timestamptz fields are converted to time.Time with UTC location.
func toModelWorker(row sqlcdb.Worker) *models.Worker {
	return &models.Worker{
		ID:            row.ID,
		Tags:          row.Tags,
		Status:        models.WorkerStatus(row.Status),
		LastHeartbeat: fromTimestamptz(row.LastHeartbeat),
		RegisteredAt:  fromTimestamptz(row.RegisteredAt),
	}
}

// toModelWorkerFromListRow converts a sqlcdb.ListWorkersRow to models.Worker.
// The CurrentTaskID field from the subquery is included only when non-zero (i.e.,
// when an active task is assigned or running on this worker).
func toModelWorkerFromListRow(row sqlcdb.ListWorkersRow) *models.Worker {
	w := &models.Worker{
		ID:            row.ID,
		Tags:          row.Tags,
		Status:        models.WorkerStatus(row.Status),
		LastHeartbeat: fromTimestamptz(row.LastHeartbeat),
		RegisteredAt:  fromTimestamptz(row.RegisteredAt),
	}
	// The subquery returns the nil UUID (all-zero bytes) when no active task exists.
	var nilUUID [16]byte
	if row.CurrentTaskID != nilUUID {
		id := row.CurrentTaskID
		w.CurrentTaskID = &id
	}
	return w
}
