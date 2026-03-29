// Package api — unit tests for parseStreamEntry and parseStreamEntries.
// Verifies that Redis Stream messages are correctly decoded into models.TaskLog values.
// No live Redis required: these tests exercise the pure parsing logic only.
// See: ADR-008, TASK-016
package api

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// TestParseStreamEntry_ValidEntry verifies that a well-formed Redis Stream entry is
// decoded correctly into a models.TaskLog with all required fields.
func TestParseStreamEntry_ValidEntry(t *testing.T) {
	id := uuid.New()
	taskID := uuid.New()
	ts := time.Now().UTC().Truncate(time.Nanosecond)

	entry := redis.XMessage{
		ID: "1680000000000-0",
		Values: map[string]any{
			"id":        id.String(),
			"task_id":   taskID.String(),
			"level":     "INFO",
			"line":      "[datasource] fetching records",
			"timestamp": ts.Format(time.RFC3339Nano),
		},
	}

	got, err := parseStreamEntry(entry)
	if err != nil {
		t.Fatalf("parseStreamEntry: unexpected error: %v", err)
	}
	if got.ID != id {
		t.Errorf("ID: want %v got %v", id, got.ID)
	}
	if got.TaskID != taskID {
		t.Errorf("TaskID: want %v got %v", taskID, got.TaskID)
	}
	if got.Level != "INFO" {
		t.Errorf("Level: want INFO got %q", got.Level)
	}
	if got.Line != "[datasource] fetching records" {
		t.Errorf("Line: want %q got %q", "[datasource] fetching records", got.Line)
	}
	if !got.Timestamp.Equal(ts) {
		t.Errorf("Timestamp: want %v got %v", ts, got.Timestamp)
	}
}

// TestParseStreamEntry_MissingField verifies that entries missing required fields
// return an error instead of silently producing an incomplete TaskLog.
func TestParseStreamEntry_MissingField(t *testing.T) {
	requiredFields := []string{"id", "task_id", "level", "line", "timestamp"}

	for _, missing := range requiredFields {
		t.Run("missing_"+missing, func(t *testing.T) {
			values := map[string]any{
				"id":        uuid.New().String(),
				"task_id":   uuid.New().String(),
				"level":     "WARN",
				"line":      "[process] test",
				"timestamp": time.Now().Format(time.RFC3339Nano),
			}
			delete(values, missing)

			entry := redis.XMessage{ID: "1-0", Values: values}
			_, err := parseStreamEntry(entry)
			if err == nil {
				t.Errorf("expected error for missing field %q, got nil", missing)
			}
		})
	}
}

// TestParseStreamEntry_MalformedUUID verifies that a non-UUID value in the id field
// returns an error.
func TestParseStreamEntry_MalformedUUID(t *testing.T) {
	entry := redis.XMessage{
		ID: "1-0",
		Values: map[string]any{
			"id":        "not-a-uuid",
			"task_id":   uuid.New().String(),
			"level":     "INFO",
			"line":      "[sink] test",
			"timestamp": time.Now().Format(time.RFC3339Nano),
		},
	}
	_, err := parseStreamEntry(entry)
	if err == nil {
		t.Error("expected error for malformed UUID in id field")
	}
}

// TestParseStreamEntries_SkipsMalformedEntries verifies that parseStreamEntries
// skips individual bad entries and returns valid ones without propagating an error.
func TestParseStreamEntries_SkipsMalformedEntries(t *testing.T) {
	goodID := uuid.New()
	taskID := uuid.New()

	entries := []redis.XMessage{
		{
			ID: "1-0",
			Values: map[string]any{
				"id":        goodID.String(),
				"task_id":   taskID.String(),
				"level":     "INFO",
				"line":      "[datasource] ok",
				"timestamp": time.Now().Format(time.RFC3339Nano),
			},
		},
		{
			ID:     "2-0",
			Values: map[string]any{"id": "bad-uuid"}, // malformed
		},
	}

	got, err := parseStreamEntries(entries)
	if err != nil {
		t.Fatalf("parseStreamEntries: unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 valid entry, got %d", len(got))
	}
	if got[0].ID != goodID {
		t.Errorf("unexpected entry ID: %v", got[0].ID)
	}
}
