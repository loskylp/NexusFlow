// Package db — PostgreSQL implementation of TaskLogRepository.
// Backed by sqlc-generated queries. Translates between sqlcdb.TaskLog (generated) and
// models.TaskLog (domain). Cold log lines are stored in the partitioned task_logs table.
// See: ADR-008, OBS-007, TASK-016
package db

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	sqlcdb "github.com/nxlabs/nexusflow/internal/db/sqlc"
	"github.com/nxlabs/nexusflow/internal/models"
)

// PgTaskLogRepository implements TaskLogRepository backed by PostgreSQL via sqlc-generated queries.
// See: ADR-008, TASK-016
type PgTaskLogRepository struct {
	queries *sqlcdb.Queries
}

// NewPgTaskLogRepository constructs a PgTaskLogRepository from the given connection pool.
// Panics if pool is nil (fail-fast: nil pool causes silent failures on every call).
//
// Args:
//
//	pool: A connected pgxpool.Pool. Must not be nil.
func NewPgTaskLogRepository(pool *Pool) *PgTaskLogRepository {
	if pool == nil {
		panic("db.NewPgTaskLogRepository: pool must not be nil")
	}
	return &PgTaskLogRepository{
		queries: sqlcdb.New(pool),
	}
}

// BatchInsert implements TaskLogRepository.BatchInsert.
// Inserts each log line individually using the sqlc-generated BatchInsertLogs query.
// The sqlc query inserts one row per call; this method loops over the slice.
// Returns on the first error; partial inserts are not rolled back.
//
// Preconditions:
//   - logs must not be empty (caller guards).
//
// Postconditions:
//   - On success: all log lines are persisted in the partitioned task_logs table.
//   - On error: some lines may already be committed; callers must handle this.
func (r *PgTaskLogRepository) BatchInsert(ctx context.Context, logs []*models.TaskLog) error {
	for _, l := range logs {
		ts := pgtype.Timestamptz{Time: l.Timestamp, Valid: true}
		if err := r.queries.BatchInsertLogs(ctx, sqlcdb.BatchInsertLogsParams{
			ID:        l.ID,
			TaskID:    l.TaskID,
			Line:      l.Line,
			Level:     l.Level,
			Timestamp: ts,
		}); err != nil {
			return err
		}
	}
	return nil
}

// ListByTask implements TaskLogRepository.ListByTask.
// Returns cold-stored log lines for the given task ordered by timestamp.
// When afterID is empty or the zero UUID, all rows are returned.
// When afterID is a non-zero UUID, only rows with id > afterID are returned
// (exclusive lower bound for Last-Event-ID replay, ADR-007).
//
// Args:
//
//	ctx:    Request context.
//	taskID: The task whose logs are queried.
//	afterID: If non-empty, only log lines with ID > afterID are returned.
//
// Postconditions:
//   - On success: returns a non-nil slice (may be empty).
//   - Rows are ordered by timestamp ASC, id ASC.
func (r *PgTaskLogRepository) ListByTask(ctx context.Context, taskID uuid.UUID, afterID string) ([]*models.TaskLog, error) {
	// The sqlc query uses id > $2 as an exclusive lower bound.
	// Passing the zero UUID means "return all rows" because no row has id == zero UUID.
	var after uuid.UUID // zero value: all rows
	if afterID != "" && afterID != (uuid.UUID{}).String() {
		parsed, err := uuid.Parse(afterID)
		if err != nil {
			return nil, err
		}
		after = parsed
	}

	rows, err := r.queries.ListLogsByTask(ctx, sqlcdb.ListLogsByTaskParams{
		TaskID: taskID,
		ID:     after,
	})
	if err != nil {
		return nil, err
	}

	out := make([]*models.TaskLog, 0, len(rows))
	for _, row := range rows {
		out = append(out, &models.TaskLog{
			ID:        row.ID,
			TaskID:    row.TaskID,
			Line:      row.Line,
			Level:     row.Level,
			Timestamp: row.Timestamp.Time,
		})
	}
	return out, nil
}
