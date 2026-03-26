// Package db — PostgreSQL implementation of PipelineRepository.
// Backed by sqlc-generated queries. Translates between sqlcdb.Pipeline (generated) and
// models.Pipeline (domain). Ownership enforcement is at the service layer; this repository
// provides raw data access without role checks.
// See: ADR-008, TASK-013
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

// PgPipelineRepository implements PipelineRepository backed by PostgreSQL via sqlc-generated queries.
// See: ADR-008, TASK-013
type PgPipelineRepository struct {
	queries *sqlcdb.Queries
}

// NewPgPipelineRepository constructs a PgPipelineRepository from the given connection pool.
// Panics if pool is nil (fail-fast: nil pool causes silent failures on every call).
//
// Args:
//
//	pool: A connected pgxpool.Pool. Must not be nil.
func NewPgPipelineRepository(pool *Pool) *PgPipelineRepository {
	if pool == nil {
		panic("db.NewPgPipelineRepository: pool must not be nil")
	}
	return &PgPipelineRepository{queries: sqlcdb.New(pool)}
}

// Create implements PipelineRepository.Create.
// Inserts a new Pipeline. Returns the persisted record with ID and timestamps.
//
// Postconditions:
//   - On success: pipeline is persisted with database-populated timestamps.
func (r *PgPipelineRepository) Create(ctx context.Context, pipeline *models.Pipeline) (*models.Pipeline, error) {
	dsJSON, err := json.Marshal(pipeline.DataSourceConfig)
	if err != nil {
		return nil, err
	}
	processJSON, err := json.Marshal(pipeline.ProcessConfig)
	if err != nil {
		return nil, err
	}
	sinkJSON, err := json.Marshal(pipeline.SinkConfig)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	row, err := r.queries.CreatePipeline(ctx, sqlcdb.CreatePipelineParams{
		ID:               pipeline.ID,
		Name:             pipeline.Name,
		UserID:           pipeline.UserID,
		DataSourceConfig: json.RawMessage(dsJSON),
		ProcessConfig:    json.RawMessage(processJSON),
		SinkConfig:       json.RawMessage(sinkJSON),
		CreatedAt:        pgtype.Timestamptz{Time: now, Valid: true},
		UpdatedAt:        pgtype.Timestamptz{Time: now, Valid: true},
	})
	if err != nil {
		return nil, err
	}
	return toModelPipeline(row)
}

// GetByID implements PipelineRepository.GetByID.
// Returns nil, nil if no pipeline with the given ID exists.
func (r *PgPipelineRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.Pipeline, error) {
	row, err := r.queries.GetPipelineByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return toModelPipeline(row)
}

// ListByUser implements PipelineRepository.ListByUser.
// Returns all pipelines owned by the given user, ordered by creation time.
// Returns an empty slice (not nil) when no pipelines exist for this user.
func (r *PgPipelineRepository) ListByUser(ctx context.Context, userID uuid.UUID) ([]*models.Pipeline, error) {
	rows, err := r.queries.ListPipelinesByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	pipelines := make([]*models.Pipeline, 0, len(rows))
	for _, row := range rows {
		p, err := toModelPipeline(row)
		if err != nil {
			return nil, err
		}
		pipelines = append(pipelines, p)
	}
	return pipelines, nil
}

// List implements PipelineRepository.List.
// Returns all pipelines across all users, ordered by creation time.
// Admin-only at the service layer. Returns an empty slice (not nil) when no pipelines exist.
func (r *PgPipelineRepository) List(ctx context.Context) ([]*models.Pipeline, error) {
	rows, err := r.queries.ListAllPipelines(ctx)
	if err != nil {
		return nil, err
	}
	pipelines := make([]*models.Pipeline, 0, len(rows))
	for _, row := range rows {
		p, err := toModelPipeline(row)
		if err != nil {
			return nil, err
		}
		pipelines = append(pipelines, p)
	}
	return pipelines, nil
}

// Update implements PipelineRepository.Update.
// Replaces the mutable fields of a Pipeline (name, phase configs).
// Returns ErrNotFound if the pipeline does not exist.
//
// Preconditions:
//   - Caller has verified ownership or admin role before calling.
//
// Postconditions:
//   - On success: pipeline is updated; returned Pipeline reflects the new state.
func (r *PgPipelineRepository) Update(ctx context.Context, pipeline *models.Pipeline) (*models.Pipeline, error) {
	dsJSON, err := json.Marshal(pipeline.DataSourceConfig)
	if err != nil {
		return nil, err
	}
	processJSON, err := json.Marshal(pipeline.ProcessConfig)
	if err != nil {
		return nil, err
	}
	sinkJSON, err := json.Marshal(pipeline.SinkConfig)
	if err != nil {
		return nil, err
	}

	row, err := r.queries.UpdatePipeline(ctx, sqlcdb.UpdatePipelineParams{
		ID:               pipeline.ID,
		Name:             pipeline.Name,
		DataSourceConfig: json.RawMessage(dsJSON),
		ProcessConfig:    json.RawMessage(processJSON),
		SinkConfig:       json.RawMessage(sinkJSON),
		UpdatedAt:        pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return toModelPipeline(row)
}

// Delete implements PipelineRepository.Delete.
// Removes the pipeline from PostgreSQL. Returns ErrActiveTasks if any non-terminal Task
// references this pipeline, preventing deletion of pipelines with running work.
//
// Preconditions:
//   - Caller has verified ownership or admin role.
//
// Postconditions:
//   - On success: pipeline is deleted; all references from tasks remain (tasks are not deleted).
//   - On ErrActiveTasks: pipeline is unchanged; caller maps to 409.
func (r *PgPipelineRepository) Delete(ctx context.Context, id uuid.UUID) error {
	hasActive, err := r.queries.PipelineHasActiveTasks(ctx, uuid.NullUUID{UUID: id, Valid: true})
	if err != nil {
		return err
	}
	if hasActive {
		return ErrActiveTasks
	}
	return r.queries.DeletePipeline(ctx, id)
}

// HasActiveTasks implements PipelineRepository.HasActiveTasks.
// Returns true if any non-terminal Task references this pipeline.
// Used by Delete to enforce the 409 guard before attempting deletion.
func (r *PgPipelineRepository) HasActiveTasks(ctx context.Context, pipelineID uuid.UUID) (bool, error) {
	return r.queries.PipelineHasActiveTasks(ctx, uuid.NullUUID{UUID: pipelineID, Valid: true})
}

// toModelPipeline converts a sqlcdb.Pipeline (generated) to a models.Pipeline (domain).
// Unmarshals the JSON-encoded phase config fields.
func toModelPipeline(row sqlcdb.Pipeline) (*models.Pipeline, error) {
	var ds models.DataSourceConfig
	if err := json.Unmarshal(row.DataSourceConfig, &ds); err != nil {
		return nil, err
	}
	var proc models.ProcessConfig
	if err := json.Unmarshal(row.ProcessConfig, &proc); err != nil {
		return nil, err
	}
	var sink models.SinkConfig
	if err := json.Unmarshal(row.SinkConfig, &sink); err != nil {
		return nil, err
	}

	return &models.Pipeline{
		ID:               row.ID,
		Name:             row.Name,
		UserID:           row.UserID,
		DataSourceConfig: ds,
		ProcessConfig:    proc,
		SinkConfig:       sink,
		CreatedAt:        fromTimestamptz(row.CreatedAt),
		UpdatedAt:        fromTimestamptz(row.UpdatedAt),
	}, nil
}
