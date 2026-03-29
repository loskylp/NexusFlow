// Package worker_test — unit tests for Worker registration and heartbeat (TASK-006).
// Tests use in-memory fakes for all external dependencies so no live Redis or
// PostgreSQL instance is required.
// See: ADR-002, TASK-006
package worker_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/nxlabs/nexusflow/internal/config"
	"github.com/nxlabs/nexusflow/internal/models"
	"github.com/nxlabs/nexusflow/worker"
)

// --- fakes ---

// fakeWorkerRepo is an in-memory WorkerRepository double.
type fakeWorkerRepo struct {
	mu       sync.Mutex
	workers  map[string]*models.Worker
}

func newFakeWorkerRepo() *fakeWorkerRepo {
	return &fakeWorkerRepo{
		workers: make(map[string]*models.Worker),
	}
}

func (r *fakeWorkerRepo) Register(ctx context.Context, w *models.Worker) (*models.Worker, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	copy := *w
	r.workers[w.ID] = &copy
	return &copy, nil
}

func (r *fakeWorkerRepo) GetByID(ctx context.Context, id string) (*models.Worker, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	w, ok := r.workers[id]
	if !ok {
		return nil, nil
	}
	copy := *w
	return &copy, nil
}

func (r *fakeWorkerRepo) List(ctx context.Context) ([]*models.Worker, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]*models.Worker, 0, len(r.workers))
	for _, w := range r.workers {
		copy := *w
		result = append(result, &copy)
	}
	return result, nil
}

func (r *fakeWorkerRepo) UpdateStatus(ctx context.Context, id string, status models.WorkerStatus) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if w, ok := r.workers[id]; ok {
		w.Status = status
	}
	return nil
}

// fakeHeartbeatStore records RecordHeartbeat calls.
type fakeHeartbeatStore struct {
	mu    sync.Mutex
	beats map[string]int // workerID -> call count
}

func newFakeHeartbeatStore() *fakeHeartbeatStore {
	return &fakeHeartbeatStore{beats: make(map[string]int)}
}

func (s *fakeHeartbeatStore) RecordHeartbeat(ctx context.Context, workerID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.beats[workerID]++
	return nil
}

func (s *fakeHeartbeatStore) ListExpired(ctx context.Context, cutoff time.Time) ([]string, error) {
	return nil, nil
}

func (s *fakeHeartbeatStore) Remove(ctx context.Context, workerID string) error {
	return nil
}

func (s *fakeHeartbeatStore) count(workerID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.beats[workerID]
}

// --- helpers ---

// testConfig returns a minimal config for worker unit tests.
func testConfig(workerID string, tags []string) *config.Config {
	return &config.Config{
		WorkerID:          workerID,
		WorkerTags:        tags,
		HeartbeatInterval: 50 * time.Millisecond, // fast for tests
	}
}

// --- Tests ---

// TestNewWorker_ReturnsNonNil verifies that NewWorker returns a usable *Worker.
func TestNewWorker_ReturnsNonNil(t *testing.T) {
	cfg := testConfig("worker-unit-001", []string{"etl"})
	repo := newFakeWorkerRepo()
	hb := newFakeHeartbeatStore()

	w := worker.NewWorker(cfg, nil, repo, nil, hb, nil, nil, nil)
	if w == nil {
		t.Fatal("NewWorker returned nil")
	}
}

// TestRegister_InsertsWorkerWithOnlineStatus verifies AC-1:
// after Register, the worker record exists in the repository with status "online".
func TestRegister_InsertsWorkerWithOnlineStatus(t *testing.T) {
	cfg := testConfig("worker-unit-002", []string{"etl", "report"})
	repo := newFakeWorkerRepo()
	hb := newFakeHeartbeatStore()

	w := worker.NewWorker(cfg, nil, repo, nil, hb, nil, nil, nil)

	if err := w.Register(context.Background()); err != nil {
		t.Fatalf("Register: %v", err)
	}

	stored, err := repo.GetByID(context.Background(), cfg.WorkerID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if stored == nil {
		t.Fatal("worker record not found after Register")
	}
	if stored.Status != models.WorkerStatusOnline {
		t.Errorf("Status = %q; want %q", stored.Status, models.WorkerStatusOnline)
	}
}

// TestRegister_RecordsInitialHeartbeat verifies AC-2:
// Register calls RecordHeartbeat so the worker appears in workers:active immediately.
func TestRegister_RecordsInitialHeartbeat(t *testing.T) {
	cfg := testConfig("worker-unit-003", []string{"etl"})
	repo := newFakeWorkerRepo()
	hb := newFakeHeartbeatStore()

	w := worker.NewWorker(cfg, nil, repo, nil, hb, nil, nil, nil)

	if err := w.Register(context.Background()); err != nil {
		t.Fatalf("Register: %v", err)
	}

	if hb.count(cfg.WorkerID) < 1 {
		t.Error("RecordHeartbeat not called during Register")
	}
}

// TestRegister_SetsCorrectTags verifies AC-4:
// the persisted worker record carries the tags from the config.
func TestRegister_SetsCorrectTags(t *testing.T) {
	tags := []string{"etl", "report"}
	cfg := testConfig("worker-unit-004", tags)
	repo := newFakeWorkerRepo()
	hb := newFakeHeartbeatStore()

	w := worker.NewWorker(cfg, nil, repo, nil, hb, nil, nil, nil)

	if err := w.Register(context.Background()); err != nil {
		t.Fatalf("Register: %v", err)
	}

	stored, _ := repo.GetByID(context.Background(), cfg.WorkerID)
	if stored == nil {
		t.Fatal("worker not found")
	}

	if len(stored.Tags) != len(tags) {
		t.Fatalf("Tags len = %d; want %d", len(stored.Tags), len(tags))
	}
	for i, tag := range tags {
		if stored.Tags[i] != tag {
			t.Errorf("Tags[%d] = %q; want %q", i, stored.Tags[i], tag)
		}
	}
}

// TestRegister_SetsRegisteredAt verifies AC-4:
// the worker record includes a non-zero registration timestamp.
func TestRegister_SetsRegisteredAt(t *testing.T) {
	cfg := testConfig("worker-unit-005", []string{"etl"})
	repo := newFakeWorkerRepo()
	hb := newFakeHeartbeatStore()

	before := time.Now().Add(-time.Second)
	w := worker.NewWorker(cfg, nil, repo, nil, hb, nil, nil, nil)
	if err := w.Register(context.Background()); err != nil {
		t.Fatalf("Register: %v", err)
	}
	after := time.Now().Add(time.Second)

	stored, _ := repo.GetByID(context.Background(), cfg.WorkerID)
	if stored == nil {
		t.Fatal("worker not found")
	}
	if stored.RegisteredAt.IsZero() {
		t.Error("RegisteredAt is zero")
	}
	if stored.RegisteredAt.Before(before) || stored.RegisteredAt.After(after) {
		t.Errorf("RegisteredAt %v is outside window [%v, %v]", stored.RegisteredAt, before, after)
	}
}

// TestEmitHeartbeats_CallsRecordHeartbeatPeriodically verifies AC-2:
// the heartbeat loop calls RecordHeartbeat at roughly the configured interval.
func TestEmitHeartbeats_CallsRecordHeartbeatPeriodically(t *testing.T) {
	cfg := testConfig("worker-unit-006", []string{"etl"})
	cfg.HeartbeatInterval = 40 * time.Millisecond
	repo := newFakeWorkerRepo()
	hb := newFakeHeartbeatStore()

	w := worker.NewWorker(cfg, nil, repo, nil, hb, nil, nil, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()

	// Run blocks until ctx expires; we measure heartbeat calls afterwards.
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()

	<-ctx.Done()
	<-done // drain

	// At 40ms interval over ~250ms: 1 from Register + at least 3 from ticker = at least 4.
	got := hb.count(cfg.WorkerID)
	if got < 4 {
		t.Errorf("expected at least 4 RecordHeartbeat calls in 250ms at 40ms interval, got %d", got)
	}
}

// TestRun_MarksWorkerDownOnShutdown verifies graceful shutdown:
// when ctx is cancelled, Run marks the worker status as "down".
func TestRun_MarksWorkerDownOnShutdown(t *testing.T) {
	cfg := testConfig("worker-unit-007", []string{"etl"})
	cfg.HeartbeatInterval = 100 * time.Millisecond
	repo := newFakeWorkerRepo()
	hb := newFakeHeartbeatStore()

	w := worker.NewWorker(cfg, nil, repo, nil, hb, nil, nil, nil)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()

	// Let it start then cancel.
	time.Sleep(60 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Errorf("Run returned unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after context cancellation")
	}

	stored, _ := repo.GetByID(context.Background(), cfg.WorkerID)
	if stored == nil {
		t.Fatal("worker record missing after shutdown")
	}
	if stored.Status != models.WorkerStatusDown {
		t.Errorf("Status after shutdown = %q; want %q", stored.Status, models.WorkerStatusDown)
	}
}

// TestRun_MultipleWorkersDifferentIDs verifies AC-3:
// two workers with distinct IDs can register simultaneously without interfering.
func TestRun_MultipleWorkersDifferentIDs(t *testing.T) {
	repo := newFakeWorkerRepo() // shared repo to simulate concurrent registration
	hb := newFakeHeartbeatStore()

	configs := []*config.Config{
		testConfig("worker-multi-A", []string{"etl"}),
		testConfig("worker-multi-B", []string{"report"}),
	}

	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(context.Background())

	for _, cfg := range configs {
		cfg := cfg
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := worker.NewWorker(cfg, nil, repo, nil, hb, nil, nil, nil)
			_ = w.Run(ctx)
		}()
	}

	time.Sleep(100 * time.Millisecond)
	cancel()
	wg.Wait()

	list, err := repo.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("expected 2 registered workers, got %d", len(list))
	}
}
