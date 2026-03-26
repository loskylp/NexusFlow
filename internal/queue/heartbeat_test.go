// Package queue_test — unit tests for HeartbeatStore (TASK-006).
// Tests connect to Redis at REDIS_ADDR (fallback: localhost:6379) and are skipped
// if the connection is unavailable.
// See: ADR-002, TASK-006
package queue_test

import (
	"context"
	"testing"
	"time"

	"github.com/nxlabs/nexusflow/internal/queue"
	"github.com/redis/go-redis/v9"
)

// TestRecordHeartbeat_AddsToSortedSet verifies that RecordHeartbeat executes
// ZADD workers:active with the current Unix timestamp as score (ADR-002).
func TestRecordHeartbeat_AddsToSortedSet(t *testing.T) {
	client := newClientOrSkip(t)
	flushTestDB(t, client)
	q := queue.NewRedisQueue(client)

	workerID := "worker-test-001"
	before := time.Now().Add(-time.Second) // allow 1s clock slack

	if err := q.RecordHeartbeat(context.Background(), workerID); err != nil {
		t.Fatalf("RecordHeartbeat: %v", err)
	}

	after := time.Now().Add(time.Second)

	// ZSCORE returns the score for the member — should be a Unix timestamp.
	score, err := client.ZScore(context.Background(), queue.WorkersActiveKey, workerID).Result()
	if err != nil {
		t.Fatalf("ZScore(%q, %q): %v", queue.WorkersActiveKey, workerID, err)
	}

	ts := time.Unix(int64(score), 0)
	if ts.Before(before) || ts.After(after) {
		t.Errorf("heartbeat timestamp %v is outside expected window [%v, %v]", ts, before, after)
	}
}

// TestRecordHeartbeat_UpdatesExistingEntry verifies that a second RecordHeartbeat call
// updates the score for an already-registered worker (ZADD is idempotent / overwrite).
func TestRecordHeartbeat_UpdatesExistingEntry(t *testing.T) {
	client := newClientOrSkip(t)
	flushTestDB(t, client)
	q := queue.NewRedisQueue(client)

	workerID := "worker-test-002"

	// Seed an old score.
	oldScore := float64(time.Now().Add(-30 * time.Second).Unix())
	if err := client.ZAdd(context.Background(), queue.WorkersActiveKey, redis.Z{
		Score:  oldScore,
		Member: workerID,
	}).Err(); err != nil {
		t.Fatalf("ZAdd seed: %v", err)
	}

	if err := q.RecordHeartbeat(context.Background(), workerID); err != nil {
		t.Fatalf("RecordHeartbeat: %v", err)
	}

	newScore, err := client.ZScore(context.Background(), queue.WorkersActiveKey, workerID).Result()
	if err != nil {
		t.Fatalf("ZScore after update: %v", err)
	}

	if newScore <= oldScore {
		t.Errorf("score was not updated: old=%v new=%v", oldScore, newScore)
	}
}

// TestRecordHeartbeat_MultipleWorkers verifies that multiple workers can record
// heartbeats simultaneously without overwriting each other's entries (AC-3).
func TestRecordHeartbeat_MultipleWorkers(t *testing.T) {
	client := newClientOrSkip(t)
	flushTestDB(t, client)
	q := queue.NewRedisQueue(client)

	workers := []string{"worker-A", "worker-B", "worker-C"}

	for _, id := range workers {
		if err := q.RecordHeartbeat(context.Background(), id); err != nil {
			t.Fatalf("RecordHeartbeat(%q): %v", id, err)
		}
	}

	// All three workers should appear in the sorted set.
	count, err := client.ZCard(context.Background(), queue.WorkersActiveKey).Result()
	if err != nil {
		t.Fatalf("ZCard: %v", err)
	}
	if int(count) != len(workers) {
		t.Errorf("expected %d members in %s, got %d", len(workers), queue.WorkersActiveKey, count)
	}

	for _, id := range workers {
		_, err := client.ZScore(context.Background(), queue.WorkersActiveKey, id).Result()
		if err != nil {
			t.Errorf("worker %q missing from %s: %v", id, queue.WorkersActiveKey, err)
		}
	}
}

// TestWorkersActiveKey_Constant verifies the sorted set key is workers:active (ADR-002).
func TestWorkersActiveKey_Constant(t *testing.T) {
	if queue.WorkersActiveKey != "workers:active" {
		t.Errorf("WorkersActiveKey = %q; want %q", queue.WorkersActiveKey, "workers:active")
	}
}
