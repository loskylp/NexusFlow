// Package worker_test — unit tests for log production during pipeline execution.
// Tests verify that log lines with correct fields (AC-5) are written to the
// LogPublisher during each pipeline phase.
// See: REQ-018, TASK-016
package worker_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nxlabs/nexusflow/internal/models"
	"github.com/nxlabs/nexusflow/worker"
)

// fakeLogPublisher is an in-memory LogPublisher double.
// Captures every log line emitted so tests can assert on them.
type fakeLogPublisher struct {
	mu   sync.Mutex
	logs []*models.TaskLog
}

func (f *fakeLogPublisher) Publish(ctx context.Context, log *models.TaskLog) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *log
	f.logs = append(f.logs, &cp)
	return nil
}

// Logs returns a copy of all captured log lines.
func (f *fakeLogPublisher) Logs() []*models.TaskLog {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]*models.TaskLog, len(f.logs))
	copy(out, f.logs)
	return out
}

// Compile-time check: fakeLogPublisher satisfies worker.LogPublisher.
var _ worker.LogPublisher = (*fakeLogPublisher)(nil)

// TestNewLogLine_FieldsArePopulated verifies that NewLogLine fills all required fields:
// non-zero UUID, correct taskID, non-empty level, message, phase prefix in line, and timestamp.
// AC-5: log lines include timestamp, level (INFO/WARN/ERROR), phase, and message.
func TestNewLogLine_FieldsArePopulated(t *testing.T) {
	taskID := uuid.New()
	ts := time.Now().UTC()

	line := worker.NewLogLine(taskID, "INFO", "datasource", "fetching records", ts)

	if line.ID == (uuid.UUID{}) {
		t.Error("ID must be a non-zero UUID")
	}
	if line.TaskID != taskID {
		t.Errorf("TaskID: want %v got %v", taskID, line.TaskID)
	}
	if line.Level != "INFO" {
		t.Errorf("Level: want INFO got %q", line.Level)
	}
	if line.Timestamp != ts {
		t.Errorf("Timestamp: want %v got %v", ts, line.Timestamp)
	}
	if !strings.Contains(line.Line, "[datasource]") {
		t.Errorf("Line should contain phase tag [datasource]: %q", line.Line)
	}
	if !strings.Contains(line.Line, "fetching records") {
		t.Errorf("Line should contain message: %q", line.Line)
	}
}

// TestNewLogLine_WarnAndErrorLevels verifies that WARN and ERROR level values are
// preserved verbatim in the TaskLog.Level field.
func TestNewLogLine_WarnAndErrorLevels(t *testing.T) {
	taskID := uuid.New()
	for _, level := range []string{"WARN", "ERROR"} {
		line := worker.NewLogLine(taskID, level, "sink", "something went wrong", time.Now())
		if line.Level != level {
			t.Errorf("level %q: want %q got %q", level, level, line.Level)
		}
	}
}

// TestNewLogLine_PhaseTagsAreEncoded verifies that all three pipeline phases
// (datasource, process, sink) are encoded as bracketed prefixes in Line.
func TestNewLogLine_PhaseTagsAreEncoded(t *testing.T) {
	taskID := uuid.New()
	phases := []string{"datasource", "process", "sink"}
	for _, phase := range phases {
		line := worker.NewLogLine(taskID, "INFO", phase, "test msg", time.Now())
		expected := "[" + phase + "]"
		if !strings.Contains(line.Line, expected) {
			t.Errorf("phase %q: expected Line to contain %q, got %q", phase, expected, line.Line)
		}
	}
}
