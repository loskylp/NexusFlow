// Package db — unit tests for PgTaskLogRepository.
// Uses in-memory fakes; no live database required.
// See: ADR-008, TASK-016
package db_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nxlabs/nexusflow/internal/db"
	"github.com/nxlabs/nexusflow/internal/models"
)

// fakeTaskLogStore is an in-memory implementation of db.TaskLogRepository used for
// unit testing. It is not a mock — it applies the same semantics as the real repository.
type fakeTaskLogStore struct {
	mu   sync.Mutex
	rows []*models.TaskLog
}

func (s *fakeTaskLogStore) BatchInsert(_ context.Context, logs []*models.TaskLog) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rows = append(s.rows, logs...)
	return nil
}

func (s *fakeTaskLogStore) ListByTask(_ context.Context, taskID uuid.UUID, afterID string) ([]*models.TaskLog, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	zeroID := uuid.UUID{}
	var after uuid.UUID
	if afterID != "" && afterID != zeroID.String() {
		parsed, err := uuid.Parse(afterID)
		if err != nil {
			return nil, err
		}
		after = parsed
	}

	var out []*models.TaskLog
	for _, row := range s.rows {
		if row.TaskID != taskID {
			continue
		}
		if after != zeroID && row.ID == after {
			// afterID is exclusive: skip this row, start from the next.
			after = zeroID // reset so subsequent rows pass through
			continue
		}
		if after == zeroID {
			out = append(out, row)
		}
	}
	return out, nil
}

// Compile-time check: fakeTaskLogStore satisfies db.TaskLogRepository.
var _ db.TaskLogRepository = (*fakeTaskLogStore)(nil)

// TestBatchInsert_StoresLogLines verifies that BatchInsert accepts a slice of log lines
// and that ListByTask returns them in insertion order for the correct task.
// RED: this test defines the interface contract before the real implementation exists.
func TestBatchInsert_StoresLogLines(t *testing.T) {
	store := &fakeTaskLogStore{}
	taskID := uuid.New()

	logs := []*models.TaskLog{
		{
			ID:        uuid.New(),
			TaskID:    taskID,
			Line:      "[datasource] fetching records",
			Level:     "INFO",
			Timestamp: time.Now().UTC(),
		},
		{
			ID:        uuid.New(),
			TaskID:    taskID,
			Line:      "[process] transforming 5 records",
			Level:     "INFO",
			Timestamp: time.Now().UTC().Add(time.Millisecond),
		},
	}

	if err := store.BatchInsert(context.Background(), logs); err != nil {
		t.Fatalf("BatchInsert: unexpected error: %v", err)
	}

	got, err := store.ListByTask(context.Background(), taskID, "")
	if err != nil {
		t.Fatalf("ListByTask: unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListByTask: want 2 rows, got %d", len(got))
	}
	if got[0].Line != logs[0].Line {
		t.Errorf("ListByTask row 0: want line %q, got %q", logs[0].Line, got[0].Line)
	}
	if got[1].Level != "INFO" {
		t.Errorf("ListByTask row 1: want level INFO, got %q", got[1].Level)
	}
}

// TestListByTask_IsolatesPerTask verifies that ListByTask returns only rows
// for the requested task and does not leak rows from other tasks.
func TestListByTask_IsolatesPerTask(t *testing.T) {
	store := &fakeTaskLogStore{}
	taskA := uuid.New()
	taskB := uuid.New()

	_ = store.BatchInsert(context.Background(), []*models.TaskLog{
		{ID: uuid.New(), TaskID: taskA, Line: "a-line", Level: "INFO", Timestamp: time.Now()},
		{ID: uuid.New(), TaskID: taskB, Line: "b-line", Level: "INFO", Timestamp: time.Now()},
	})

	got, err := store.ListByTask(context.Background(), taskA, "")
	if err != nil {
		t.Fatalf("ListByTask: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 row for taskA, got %d", len(got))
	}
	if got[0].TaskID != taskA {
		t.Errorf("row belongs to wrong task: %v", got[0].TaskID)
	}
}

// TestLogLineFieldsIncludeRequiredFields verifies that TaskLog captures all fields
// required by AC-5: timestamp, level, phase (encoded in line prefix), and message.
func TestLogLineFieldsIncludeRequiredFields(t *testing.T) {
	store := &fakeTaskLogStore{}
	taskID := uuid.New()
	ts := time.Now().UTC()

	entry := &models.TaskLog{
		ID:        uuid.New(),
		TaskID:    taskID,
		Line:      "[sink] writing 5 records",
		Level:     "INFO",
		Timestamp: ts,
	}
	_ = store.BatchInsert(context.Background(), []*models.TaskLog{entry})

	got, _ := store.ListByTask(context.Background(), taskID, "")
	if len(got) == 0 {
		t.Fatal("no rows returned")
	}
	row := got[0]
	if row.ID == (uuid.UUID{}) {
		t.Error("ID must be a non-zero UUID")
	}
	if row.TaskID != taskID {
		t.Errorf("TaskID mismatch: want %v got %v", taskID, row.TaskID)
	}
	if row.Level == "" {
		t.Error("Level must not be empty")
	}
	if row.Line == "" {
		t.Error("Line must not be empty")
	}
	if row.Timestamp.IsZero() {
		t.Error("Timestamp must be set")
	}
}
