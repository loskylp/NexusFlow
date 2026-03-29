// Package worker — unit tests for ChainTrigger.
// Tests verify: trigger submits next pipeline task, idempotency guard prevents
// duplicate submissions, and no-op when pipeline is last in chain.
// See: REQ-014, ADR-003, TASK-014
package worker

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/nxlabs/nexusflow/internal/db"
	"github.com/nxlabs/nexusflow/internal/models"
)

// --- stubs ---

// stubChainRepository is an in-memory ChainRepository for testing.
type stubChainRepository struct {
	chains map[uuid.UUID]*models.Chain
}

func newStubChainRepository() *stubChainRepository {
	return &stubChainRepository{chains: make(map[uuid.UUID]*models.Chain)}
}

func (r *stubChainRepository) Create(_ context.Context, chain *models.Chain) (*models.Chain, error) {
	r.chains[chain.ID] = chain
	return chain, nil
}

func (r *stubChainRepository) GetByID(_ context.Context, id uuid.UUID) (*models.Chain, error) {
	c, ok := r.chains[id]
	if !ok {
		return nil, nil
	}
	return c, nil
}

func (r *stubChainRepository) FindByPipeline(_ context.Context, pipelineID uuid.UUID) (*models.Chain, error) {
	for _, c := range r.chains {
		for _, pid := range c.PipelineIDs {
			if pid == pipelineID {
				return c, nil
			}
		}
	}
	return nil, nil
}

func (r *stubChainRepository) GetNextPipeline(_ context.Context, chainID uuid.UUID, pipelineID uuid.UUID) (*uuid.UUID, error) {
	c, ok := r.chains[chainID]
	if !ok {
		return nil, nil
	}
	for i, pid := range c.PipelineIDs {
		if pid == pipelineID && i+1 < len(c.PipelineIDs) {
			next := c.PipelineIDs[i+1]
			return &next, nil
		}
	}
	return nil, nil
}

// stubChainTaskEnqueuer tracks task submissions made by the chain trigger.
type stubChainTaskEnqueuer struct {
	mu         sync.Mutex
	submitted  []submittedTask
	enqueueErr error
}

type submittedTask struct {
	pipelineID uuid.UUID
	userID     uuid.UUID
}

func (e *stubChainTaskEnqueuer) SubmitChainTask(_ context.Context, pipelineID uuid.UUID, userID uuid.UUID, _ uuid.UUID) error {
	if e.enqueueErr != nil {
		return e.enqueueErr
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.submitted = append(e.submitted, submittedTask{pipelineID: pipelineID, userID: userID})
	return nil
}

func (e *stubChainTaskEnqueuer) count() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.submitted)
}

// stubIdempotencyStore is an in-memory idempotency guard for testing.
// SetNX returns true (acquired) on the first call for a key, false on subsequent calls.
type stubIdempotencyStore struct {
	mu   sync.Mutex
	keys map[string]bool
}

func newStubIdempotencyStore() *stubIdempotencyStore {
	return &stubIdempotencyStore{keys: make(map[string]bool)}
}

func (s *stubIdempotencyStore) SetNX(_ context.Context, key string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.keys[key] {
		return false, nil // already set — duplicate
	}
	s.keys[key] = true
	return true, nil
}

// --- tests ---

// TestChainTrigger_submits_next_pipeline verifies that when a task for pipeline A
// in a chain completes, a task for pipeline B is automatically submitted.
func TestChainTrigger_submits_next_pipeline(t *testing.T) {
	// Arrange
	pipelineA := uuid.New()
	pipelineB := uuid.New()
	chainID := uuid.New()
	userID := uuid.New()
	taskID := uuid.New()

	chainRepo := newStubChainRepository()
	chainRepo.chains[chainID] = &models.Chain{
		ID:          chainID,
		Name:        "test-chain",
		UserID:      userID,
		PipelineIDs: []uuid.UUID{pipelineA, pipelineB},
	}

	enqueuer := &stubChainTaskEnqueuer{}
	idempotency := newStubIdempotencyStore()

	trigger := &ChainTrigger{
		chains:      chainRepo,
		enqueuer:    enqueuer,
		idempotency: idempotency,
	}

	// Act
	err := trigger.OnTaskCompleted(context.Background(), taskID, pipelineA, userID)

	// Assert
	if err != nil {
		t.Fatalf("OnTaskCompleted returned error: %v", err)
	}
	if enqueuer.count() != 1 {
		t.Fatalf("expected 1 submitted task, got %d", enqueuer.count())
	}
	if enqueuer.submitted[0].pipelineID != pipelineB {
		t.Errorf("expected next pipeline %v, got %v", pipelineB, enqueuer.submitted[0].pipelineID)
	}
}

// TestChainTrigger_idempotent verifies that duplicate completion events for the same
// task do not create duplicate downstream tasks (ADR-003, TASK-014 AC-4).
func TestChainTrigger_idempotent(t *testing.T) {
	// Arrange
	pipelineA := uuid.New()
	pipelineB := uuid.New()
	chainID := uuid.New()
	userID := uuid.New()
	taskID := uuid.New()

	chainRepo := newStubChainRepository()
	chainRepo.chains[chainID] = &models.Chain{
		ID:          chainID,
		Name:        "test-chain",
		UserID:      userID,
		PipelineIDs: []uuid.UUID{pipelineA, pipelineB},
	}

	enqueuer := &stubChainTaskEnqueuer{}
	idempotency := newStubIdempotencyStore()

	trigger := &ChainTrigger{
		chains:      chainRepo,
		enqueuer:    enqueuer,
		idempotency: idempotency,
	}

	// Act: fire twice with the same taskID
	_ = trigger.OnTaskCompleted(context.Background(), taskID, pipelineA, userID)
	_ = trigger.OnTaskCompleted(context.Background(), taskID, pipelineA, userID)

	// Assert: only one task submitted despite two completion events
	if enqueuer.count() != 1 {
		t.Fatalf("expected 1 submitted task (idempotent), got %d", enqueuer.count())
	}
}

// TestChainTrigger_noop_for_last_pipeline verifies that when a task for the final
// pipeline in a chain completes, no downstream task is submitted.
func TestChainTrigger_noop_for_last_pipeline(t *testing.T) {
	// Arrange
	pipelineA := uuid.New()
	pipelineB := uuid.New()
	chainID := uuid.New()
	userID := uuid.New()
	taskID := uuid.New()

	chainRepo := newStubChainRepository()
	chainRepo.chains[chainID] = &models.Chain{
		ID:          chainID,
		Name:        "test-chain",
		UserID:      userID,
		PipelineIDs: []uuid.UUID{pipelineA, pipelineB},
	}

	enqueuer := &stubChainTaskEnqueuer{}
	idempotency := newStubIdempotencyStore()

	trigger := &ChainTrigger{
		chains:      chainRepo,
		enqueuer:    enqueuer,
		idempotency: idempotency,
	}

	// Act: pipeline B is the last in the chain — no downstream task
	err := trigger.OnTaskCompleted(context.Background(), taskID, pipelineB, userID)

	// Assert
	if err != nil {
		t.Fatalf("OnTaskCompleted returned error: %v", err)
	}
	if enqueuer.count() != 0 {
		t.Fatalf("expected 0 submitted tasks for last pipeline, got %d", enqueuer.count())
	}
}

// TestChainTrigger_noop_when_not_in_chain verifies that when a task's pipeline
// is not part of any chain, no downstream task is submitted.
func TestChainTrigger_noop_when_not_in_chain(t *testing.T) {
	// Arrange
	pipelineNotInChain := uuid.New()
	userID := uuid.New()
	taskID := uuid.New()

	chainRepo := newStubChainRepository() // no chains
	enqueuer := &stubChainTaskEnqueuer{}
	idempotency := newStubIdempotencyStore()

	trigger := &ChainTrigger{
		chains:      chainRepo,
		enqueuer:    enqueuer,
		idempotency: idempotency,
	}

	// Act
	err := trigger.OnTaskCompleted(context.Background(), taskID, pipelineNotInChain, userID)

	// Assert
	if err != nil {
		t.Fatalf("OnTaskCompleted returned error: %v", err)
	}
	if enqueuer.count() != 0 {
		t.Fatalf("expected 0 submitted tasks for pipeline not in chain, got %d", enqueuer.count())
	}
}

// Compile-time interface satisfaction check.
var _ db.ChainRepository = (*stubChainRepository)(nil)
