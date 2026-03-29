// Package db — PostgreSQL implementation of ChainRepository.
// Backed by sqlc-generated queries for the chains and chain_steps tables (migration 000004).
// Chain creation is transactional: the chain header and all steps are inserted atomically.
// See: REQ-014, ADR-003, ADR-008, TASK-014
package db

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	sqlcdb "github.com/nxlabs/nexusflow/internal/db/sqlc"
	"github.com/nxlabs/nexusflow/internal/models"
)

// PgChainRepository implements ChainRepository backed by PostgreSQL via sqlc-generated queries.
// See: ADR-008, TASK-014
type PgChainRepository struct {
	queries *sqlcdb.Queries
	pool    *Pool
}

// NewPgChainRepository constructs a PgChainRepository from the given connection pool.
// Panics if pool is nil (fail-fast: nil pool causes silent failures on every call).
//
// Args:
//
//	pool: A connected pgxpool.Pool. Must not be nil.
func NewPgChainRepository(pool *Pool) *PgChainRepository {
	if pool == nil {
		panic("db.NewPgChainRepository: pool must not be nil")
	}
	return &PgChainRepository{
		queries: sqlcdb.New(pool),
		pool:    pool,
	}
}

// Create implements ChainRepository.Create.
// Inserts the chain header and all chain_steps in a single database transaction
// so the chain is never partially visible.
//
// Preconditions:
//   - chain.PipelineIDs has at least two entries (enforced by the handler layer).
//   - chain.ID is a new UUID set by the caller.
//
// Postconditions:
//   - On success: the chain and all steps are persisted; returned Chain reflects the database state.
//   - On failure: the transaction is rolled back; no partial state exists.
func (r *PgChainRepository) Create(ctx context.Context, chain *models.Chain) (*models.Chain, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := r.queries.WithTx(tx)
	now := time.Now().UTC()

	row, err := qtx.CreateChain(ctx, sqlcdb.CreateChainParams{
		ID:        chain.ID,
		Name:      chain.Name,
		UserID:    chain.UserID,
		CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
	})
	if err != nil {
		return nil, err
	}

	for i, pipelineID := range chain.PipelineIDs {
		if err := qtx.CreateChainStep(ctx, sqlcdb.CreateChainStepParams{
			ChainID:    chain.ID,
			PipelineID: pipelineID,
			Position:   int32(i),
		}); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	result := &models.Chain{
		ID:          row.ID,
		Name:        row.Name,
		UserID:      row.UserID,
		PipelineIDs: chain.PipelineIDs,
		CreatedAt:   fromTimestamptz(row.CreatedAt),
	}
	return result, nil
}

// GetByID implements ChainRepository.GetByID.
// Fetches the chain header and assembles PipelineIDs from the chain_steps table.
// Returns nil, nil if no chain with the given ID exists.
func (r *PgChainRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.Chain, error) {
	row, err := r.queries.GetChainByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	steps, err := r.queries.GetChainSteps(ctx, id)
	if err != nil {
		return nil, err
	}

	pipelineIDs := make([]uuid.UUID, len(steps))
	for i, s := range steps {
		pipelineIDs[i] = s.PipelineID
	}

	return &models.Chain{
		ID:          row.ID,
		Name:        row.Name,
		UserID:      row.UserID,
		PipelineIDs: pipelineIDs,
		CreatedAt:   fromTimestamptz(row.CreatedAt),
	}, nil
}

// FindByPipeline implements ChainRepository.FindByPipeline.
// Returns the Chain whose chain_steps table contains the given pipeline_id.
// Returns nil, nil when the pipeline is not part of any chain.
func (r *PgChainRepository) FindByPipeline(ctx context.Context, pipelineID uuid.UUID) (*models.Chain, error) {
	row, err := r.queries.FindChainByPipeline(ctx, pipelineID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	// Assemble the full PipelineIDs list for the discovered chain.
	steps, err := r.queries.GetChainSteps(ctx, row.ID)
	if err != nil {
		return nil, err
	}

	pipelineIDs := make([]uuid.UUID, len(steps))
	for i, s := range steps {
		pipelineIDs[i] = s.PipelineID
	}

	return &models.Chain{
		ID:          row.ID,
		Name:        row.Name,
		UserID:      row.UserID,
		PipelineIDs: pipelineIDs,
		CreatedAt:   fromTimestamptz(row.CreatedAt),
	}, nil
}

// GetNextPipeline implements ChainRepository.GetNextPipeline.
// Returns a pointer to the next pipeline_id in chainID after pipelineID.
// Returns nil when pipelineID is the last step in the chain.
func (r *PgChainRepository) GetNextPipeline(ctx context.Context, chainID uuid.UUID, pipelineID uuid.UUID) (*uuid.UUID, error) {
	nextID, err := r.queries.GetNextPipelineInChain(ctx, sqlcdb.GetNextPipelineInChainParams{
		ChainID:    chainID,
		PipelineID: pipelineID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil // pipelineID is the last step
	}
	if err != nil {
		return nil, err
	}
	return &nextID, nil
}
