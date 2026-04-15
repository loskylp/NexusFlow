// Package db — PostgreSQL implementation of TaskRepository.
// Backed by sqlc-generated queries. Translates between sqlcdb.Task (generated) and
// models.Task (domain). State transitions are persisted to task_state_log in the same
// operation as status updates, satisfying the audit trail requirement (ADR-008).
// See: ADR-008, TASK-005, TASK-007
package db

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/nxlabs/nexusflow/internal/db/sqlc"
	"github.com/nxlabs/nexusflow/internal/models"
)

// PgTaskRepository implements TaskRepository backed by PostgreSQL via sqlc-generated queries.
// See: ADR-008, TASK-005
type PgTaskRepository struct {
	queries *sqlcdb.Queries
	pool    *Pool
}

// NewPgTaskRepository constructs a PgTaskRepository from the given connection pool.
// Panics if pool is nil (fail-fast: nil pool causes silent failures on every call).
//
// Args:
//
//	pool: A connected pgxpool.Pool. Must not be nil.
func NewPgTaskRepository(pool *Pool) *PgTaskRepository {
	if pool == nil {
		panic("db.NewPgTaskRepository: pool must not be nil")
	}
	return &PgTaskRepository{
		queries: sqlcdb.New(pool),
		pool:    pool,
	}
}

// Create implements TaskRepository.Create.
// Inserts a new Task with the status provided in the task struct (typically "submitted").
// The caller is responsible for calling UpdateStatus to advance to "queued" after enqueuing.
//
// Postconditions:
//   - On success: task is persisted; returned Task has database-populated fields.
func (r *PgTaskRepository) Create(ctx context.Context, task *models.Task) (*models.Task, error) {
	retryJSON, err := json.Marshal(task.RetryConfig)
	if err != nil {
		return nil, err
	}
	inputJSON, err := json.Marshal(task.Input)
	if err != nil {
		return nil, err
	}

	var pipelineID uuid.NullUUID
	if task.PipelineID != nil {
		pipelineID = uuid.NullUUID{UUID: *task.PipelineID, Valid: true}
	}

	var chainID uuid.NullUUID
	if task.ChainID != nil {
		chainID = uuid.NullUUID{UUID: *task.ChainID, Valid: true}
	}

	var workerID *string
	if task.WorkerID != nil {
		workerID = task.WorkerID
	}

	now := time.Now().UTC()
	row, err := r.queries.CreateTask(ctx, sqlcdb.CreateTaskParams{
		ID:          task.ID,
		PipelineID:  pipelineID,
		ChainID:     chainID,
		UserID:      task.UserID,
		Status:      string(task.Status),
		RetryConfig: json.RawMessage(retryJSON),
		RetryCount:  int32(task.RetryCount),
		ExecutionID: task.ExecutionID,
		WorkerID:    workerID,
		Input:       json.RawMessage(inputJSON),
		CreatedAt:   pgtype.Timestamptz{Time: now, Valid: true},
		UpdatedAt:   pgtype.Timestamptz{Time: now, Valid: true},
	})
	if err != nil {
		return nil, err
	}
	return toModelTask(row)
}

// GetByID implements TaskRepository.GetByID.
// Returns nil, nil if no task with the given ID exists.
func (r *PgTaskRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.Task, error) {
	row, err := r.queries.GetTaskByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return toModelTask(row)
}

// ListByUser implements TaskRepository.ListByUser.
// Returns all tasks submitted by the given user, ordered by creation time (newest first).
// Returns an empty slice (not nil) when no tasks exist for this user.
func (r *PgTaskRepository) ListByUser(ctx context.Context, userID uuid.UUID) ([]*models.Task, error) {
	rows, err := r.queries.ListTasksByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	tasks := make([]*models.Task, 0, len(rows))
	for _, row := range rows {
		t, err := toModelTask(row)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, nil
}

// List implements TaskRepository.List.
// Returns all tasks across all users, ordered by creation time (newest first).
// Admin-only at the service layer. Returns an empty slice (not nil) when no tasks exist.
func (r *PgTaskRepository) List(ctx context.Context) ([]*models.Task, error) {
	rows, err := r.queries.ListAllTasks(ctx)
	if err != nil {
		return nil, err
	}
	tasks := make([]*models.Task, 0, len(rows))
	for _, row := range rows {
		t, err := toModelTask(row)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, nil
}

// UpdateStatus implements TaskRepository.UpdateStatus.
// Transitions the task to newStatus and records the transition in task_state_log.
// Both operations run in a single database transaction so the audit trail is never
// partially written.
//
// Preconditions:
//   - The task with the given ID must exist.
//   - The database trigger on task_state_log enforces valid (fromState, toState) pairs.
//
// Postconditions:
//   - On success: task.Status = newStatus; a new task_state_log entry exists.
//   - On invalid transition: the database trigger rejects and an error is returned.
func (r *PgTaskRepository) UpdateStatus(ctx context.Context, id uuid.UUID, newStatus models.TaskStatus, reason string, workerID *string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := r.queries.WithTx(tx)

	// Fetch the current status so the state log can record the fromState.
	current, err := qtx.GetTaskByID(ctx, id)
	if err != nil {
		return err
	}

	if err := qtx.UpdateTaskStatus(ctx, sqlcdb.UpdateTaskStatusParams{
		ID:       id,
		Status:   string(newStatus),
		WorkerID: workerID,
	}); err != nil {
		return err
	}

	if err := qtx.InsertTaskStateLog(ctx, sqlcdb.InsertTaskStateLogParams{
		ID:        uuid.New(),
		TaskID:    id,
		FromState: current.Status,
		ToState:   string(newStatus),
		Reason:    reason,
		Timestamp: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// IncrementRetryCount implements TaskRepository.IncrementRetryCount.
// Atomically increments the retry_count and returns the new value.
// Called by the Monitor on task reclamation via XCLAIM (ADR-002).
func (r *PgTaskRepository) IncrementRetryCount(ctx context.Context, id uuid.UUID) (int, error) {
	count, err := r.queries.IncrementTaskRetryCount(ctx, id)
	if err != nil {
		return 0, err
	}
	return int(count), nil
}

// SetRetryAfterAndTags implements TaskRepository.SetRetryAfterAndTags.
// Updates the retry_after and retry_tags columns atomically for the task identified by id.
// When retryAfter is nil, sets retry_after to NULL (task is immediately retryable).
// Called by the Monitor after XCLAIM to enforce backoff delay (TASK-010).
//
// Postconditions:
//   - On success: task.retry_after = retryAfter and task.retry_tags = retryTags in the database.
func (r *PgTaskRepository) SetRetryAfterAndTags(ctx context.Context, id uuid.UUID, retryAfter *time.Time, retryTags []string) error {
	var ts pgtype.Timestamptz
	if retryAfter != nil {
		ts = pgtype.Timestamptz{Time: retryAfter.UTC(), Valid: true}
	}
	if retryTags == nil {
		retryTags = []string{}
	}
	return r.queries.SetTaskRetryAfterAndTags(ctx, sqlcdb.SetTaskRetryAfterAndTagsParams{
		ID:         id,
		RetryAfter: ts,
		RetryTags:  retryTags,
	})
}

// ListRetryReady implements TaskRepository.ListRetryReady.
// Returns all tasks in "queued" status whose retry_after has elapsed.
// Called by the Monitor scan loop to find tasks ready for re-dispatch (TASK-010).
//
// Postconditions:
//   - Returns an empty (non-nil) slice when no tasks are ready.
func (r *PgTaskRepository) ListRetryReady(ctx context.Context) ([]*models.Task, error) {
	rows, err := r.queries.ListRetryReadyTasks(ctx)
	if err != nil {
		return nil, err
	}
	tasks := make([]*models.Task, 0, len(rows))
	for _, row := range rows {
		t, err := toModelTask(row)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, nil
}

// ListByPipelineAndStatuses implements TaskRepository.ListByPipelineAndStatuses.
// Returns all tasks for the given pipeline whose status is in the provided set.
// Used by the Monitor to find non-terminal tasks in downstream pipelines for cascading
// cancellation (TASK-011, REQ-012).
//
// Returns an empty (non-nil) slice when no matching tasks exist.
func (r *PgTaskRepository) ListByPipelineAndStatuses(ctx context.Context, pipelineID uuid.UUID, statuses []models.TaskStatus) ([]*models.Task, error) {
	strStatuses := make([]string, len(statuses))
	for i, s := range statuses {
		strStatuses[i] = string(s)
	}
	rows, err := r.queries.ListTasksByPipelineAndStatuses(ctx, sqlcdb.ListTasksByPipelineAndStatusesParams{
		PipelineID: uuid.NullUUID{UUID: pipelineID, Valid: true},
		Column2:    strStatuses,
	})
	if err != nil {
		return nil, err
	}
	tasks := make([]*models.Task, 0, len(rows))
	for _, row := range rows {
		t, err := toModelTask(row)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, nil
}

// Cancel implements TaskRepository.Cancel.
// Sets the task status to "cancelled". Does not verify cancel authority — that is
// the service layer's responsibility (Domain Invariant 8).
//
// Preconditions:
//   - Caller has verified the requesting user is the task owner or Admin.
//   - Task is in a cancellable state (not already terminal).
func (r *PgTaskRepository) Cancel(ctx context.Context, id uuid.UUID, reason string) error {
	return r.UpdateStatus(ctx, id, models.TaskStatusCancelled, reason, nil)
}

// GetStateLog implements TaskRepository.GetStateLog.
// Returns all state transition log entries for a task in chronological order.
func (r *PgTaskRepository) GetStateLog(ctx context.Context, taskID uuid.UUID) ([]*models.TaskStateLog, error) {
	rows, err := r.queries.GetTaskStateLog(ctx, taskID)
	if err != nil {
		return nil, err
	}
	entries := make([]*models.TaskStateLog, 0, len(rows))
	for _, row := range rows {
		entries = append(entries, toModelTaskStateLog(row))
	}
	return entries, nil
}

// toModelTask converts a sqlcdb.Task (generated) to a models.Task (domain).
// Unmarshals the JSON-encoded retry_config and input fields.
// PipelineID is mapped from uuid.NullUUID to *uuid.UUID: nil when the
// referenced pipeline has been deleted (ON DELETE SET NULL).
func toModelTask(row sqlcdb.Task) (*models.Task, error) {
	var retryConfig models.RetryConfig
	if err := json.Unmarshal(row.RetryConfig, &retryConfig); err != nil {
		return nil, err
	}
	var input map[string]any
	if err := json.Unmarshal(row.Input, &input); err != nil {
		return nil, err
	}

	var pipelineID *uuid.UUID
	if row.PipelineID.Valid {
		id := row.PipelineID.UUID
		pipelineID = &id
	}

	var chainID *uuid.UUID
	if row.ChainID.Valid {
		id := row.ChainID.UUID
		chainID = &id
	}

	var retryAfter *time.Time
	if row.RetryAfter.Valid {
		t := row.RetryAfter.Time
		retryAfter = &t
	}

	retryTags := row.RetryTags
	if retryTags == nil {
		retryTags = []string{}
	}

	return &models.Task{
		ID:          row.ID,
		PipelineID:  pipelineID,
		ChainID:     chainID,
		UserID:      row.UserID,
		Status:      models.TaskStatus(row.Status),
		RetryConfig: retryConfig,
		RetryCount:  int(row.RetryCount),
		RetryAfter:  retryAfter,
		RetryTags:   retryTags,
		ExecutionID: row.ExecutionID,
		WorkerID:    row.WorkerID,
		Input:       input,
		CreatedAt:   fromTimestamptz(row.CreatedAt),
		UpdatedAt:   fromTimestamptz(row.UpdatedAt),
	}, nil
}

// toModelTaskStateLog converts a sqlcdb.TaskStateLog (generated) to a models.TaskStateLog (domain).
func toModelTaskStateLog(row sqlcdb.TaskStateLog) *models.TaskStateLog {
	return &models.TaskStateLog{
		ID:        row.ID,
		TaskID:    row.TaskID,
		FromState: models.TaskStatus(row.FromState),
		ToState:   models.TaskStatus(row.ToState),
		Reason:    row.Reason,
		Timestamp: fromTimestamptz(row.Timestamp),
	}
}
