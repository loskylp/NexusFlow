// Package queue_test contains unit tests for the Redis queue implementation.
// Tests that require a live Redis instance connect to localhost:6379 and are
// skipped automatically if the connection is unavailable, ensuring the test
// suite does not break in environments without Redis.
// See: TASK-004, ADR-001, ADR-003
package queue_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nxlabs/nexusflow/internal/models"
	"github.com/nxlabs/nexusflow/internal/queue"
	"github.com/redis/go-redis/v9"
)

// redisAddr returns the Redis address to use for tests.
// Falls back to localhost:6379 when REDIS_ADDR is not set.
func redisAddr() string {
	if addr := os.Getenv("REDIS_ADDR"); addr != "" {
		return addr
	}
	return "localhost:6379"
}

// --- Test helpers ---

// newClientOrSkip dials the Redis address (REDIS_ADDR env or localhost:6379) and
// skips the test if Redis is not available. The client is closed via t.Cleanup.
func newClientOrSkip(t *testing.T) *redis.Client {
	t.Helper()
	addr := redisAddr()
	client := redis.NewClient(&redis.Options{Addr: addr})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		t.Skipf("Redis unavailable at %s (%v) — skipping", addr, err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

// flushTestDB removes all keys so each test starts from a clean slate.
func flushTestDB(t *testing.T, client *redis.Client) {
	t.Helper()
	if err := client.FlushDB(context.Background()).Err(); err != nil {
		t.Fatalf("FlushDB: %v", err)
	}
}

// makeTask returns a minimal Task with a deterministic ID for test isolation.
func makeTask(id, pipelineID, userID string) *models.Task {
	return &models.Task{
		ID:          uuid.MustParse(id),
		PipelineID:  uuid.MustParse(pipelineID),
		UserID:      uuid.MustParse(userID),
		ExecutionID: id + ":0",
	}
}

const (
	testTaskID     = "00000000-0000-0000-0000-000000000001"
	testPipelineID = "00000000-0000-0000-0000-000000000002"
	testUserID     = "00000000-0000-0000-0000-000000000003"
)

// --- Construction ---

// TestNewRedisQueue_NilClientPanics verifies fail-fast precondition enforcement:
// a nil Redis client must panic immediately rather than produce a broken queue.
func TestNewRedisQueue_NilClientPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for nil client, got none")
		}
	}()
	queue.NewRedisQueue(nil)
}

// TestNewRedisQueue_NonNilResult verifies that a valid client produces a usable queue.
func TestNewRedisQueue_NonNilResult(t *testing.T) {
	client := redis.NewClient(&redis.Options{Addr: redisAddr()})
	defer func() { _ = client.Close() }()
	q := queue.NewRedisQueue(client)
	if q == nil {
		t.Fatal("NewRedisQueue returned nil")
	}
}

// --- Naming convention ---

// TestTaskQueueStream_Format verifies the stream key naming convention: queue:{tag} (ADR-001).
func TestTaskQueueStream_Format(t *testing.T) {
	cases := []struct{ tag, want string }{
		{"etl", "queue:etl"},
		{"report", "queue:report"},
		{"demo", "queue:demo"},
	}
	for _, tc := range cases {
		got := queue.TaskQueueStream(tc.tag)
		if got != tc.want {
			t.Errorf("TaskQueueStream(%q) = %q; want %q", tc.tag, got, tc.want)
		}
	}
}

// TestDeadLetterStream_Constant verifies the dead letter stream key value (ADR-001).
func TestDeadLetterStream_Constant(t *testing.T) {
	if queue.DeadLetterStream != "queue:dead-letter" {
		t.Errorf("DeadLetterStream = %q; want %q", queue.DeadLetterStream, "queue:dead-letter")
	}
}

// TestConsumerGroupName_Constant verifies the consumer group name (ADR-001: group named "workers").
func TestConsumerGroupName_Constant(t *testing.T) {
	if queue.ConsumerGroupName != "workers" {
		t.Errorf("ConsumerGroupName = %q; want %q", queue.ConsumerGroupName, "workers")
	}
}

// TestNewLogStream_Format verifies the log stream key naming convention: logs:{taskId}.
func TestNewLogStream_Format(t *testing.T) {
	got := queue.NewLogStream("abc-123")
	if got != "logs:abc-123" {
		t.Errorf("NewLogStream = %q; want %q", got, "logs:abc-123")
	}
}

// --- InitGroups ---

// TestInitGroups_CreatesGroupForEachTag verifies AC-2: consumer groups are created
// automatically on startup for each tag stream via XGROUP CREATE MKSTREAM.
func TestInitGroups_CreatesGroupForEachTag(t *testing.T) {
	client := newClientOrSkip(t)
	flushTestDB(t, client)
	q := queue.NewRedisQueue(client)

	tags := []string{"etl", "report"}
	if err := q.InitGroups(context.Background(), tags); err != nil {
		t.Fatalf("InitGroups: %v", err)
	}

	for _, tag := range tags {
		stream := queue.TaskQueueStream(tag)
		groups, err := client.XInfoGroups(context.Background(), stream).Result()
		if err != nil {
			t.Fatalf("XInfoGroups(%q): %v", stream, err)
		}
		if !hasGroup(groups, queue.ConsumerGroupName) {
			t.Errorf("consumer group %q not found on stream %q", queue.ConsumerGroupName, stream)
		}
	}
}

// TestInitGroups_Idempotent verifies that calling InitGroups twice does not return an error.
// Redis returns BUSYGROUP when the group already exists; the implementation swallows it.
func TestInitGroups_Idempotent(t *testing.T) {
	client := newClientOrSkip(t)
	flushTestDB(t, client)
	q := queue.NewRedisQueue(client)

	tags := []string{"etl"}
	if err := q.InitGroups(context.Background(), tags); err != nil {
		t.Fatalf("first InitGroups: %v", err)
	}
	if err := q.InitGroups(context.Background(), tags); err != nil {
		t.Errorf("second InitGroups returned error (expected nil): %v", err)
	}
}

// TestInitGroups_EmptyTagsNoOp verifies that an empty tag slice is a no-op.
func TestInitGroups_EmptyTagsNoOp(t *testing.T) {
	client := newClientOrSkip(t)
	flushTestDB(t, client)
	q := queue.NewRedisQueue(client)

	if err := q.InitGroups(context.Background(), []string{}); err != nil {
		t.Errorf("InitGroups with empty tags: %v", err)
	}
}

// --- Enqueue ---

// TestEnqueue_AddsToTagStream verifies AC-1: a task enqueued with tag "etl"
// is added to stream queue:etl via XADD.
func TestEnqueue_AddsToTagStream(t *testing.T) {
	client := newClientOrSkip(t)
	flushTestDB(t, client)
	q := queue.NewRedisQueue(client)

	task := makeTask(testTaskID, testPipelineID, testUserID)
	ids, err := q.Enqueue(context.Background(), &queue.ProducerMessage{Task: task, Tags: []string{"etl"}})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if len(ids) != 1 {
		t.Fatalf("expected 1 stream ID, got %d", len(ids))
	}

	entries, err := client.XRange(context.Background(), "queue:etl", "-", "+").Result()
	if err != nil {
		t.Fatalf("XRange: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry in queue:etl, got %d", len(entries))
	}
	if entries[0].ID != ids[0] {
		t.Errorf("entry ID = %q; want %q", entries[0].ID, ids[0])
	}
}

// TestEnqueue_MultipleTagsAddsToEachStream verifies that a task with multiple tags
// is written to all corresponding streams, one entry per tag.
func TestEnqueue_MultipleTagsAddsToEachStream(t *testing.T) {
	client := newClientOrSkip(t)
	flushTestDB(t, client)
	q := queue.NewRedisQueue(client)

	tags := []string{"etl", "report", "demo"}
	task := makeTask(testTaskID, testPipelineID, testUserID)
	ids, err := q.Enqueue(context.Background(), &queue.ProducerMessage{Task: task, Tags: tags})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if len(ids) != len(tags) {
		t.Fatalf("expected %d stream IDs, got %d", len(tags), len(ids))
	}

	for _, tag := range tags {
		entries, err := client.XRange(context.Background(), queue.TaskQueueStream(tag), "-", "+").Result()
		if err != nil {
			t.Fatalf("XRange(%q): %v", tag, err)
		}
		if len(entries) != 1 {
			t.Errorf("expected 1 entry in queue:%s, got %d", tag, len(entries))
		}
	}
}

// TestEnqueue_PayloadContainsTaskID verifies that the stream entry payload encodes
// the TaskID so consumers can cross-reference with PostgreSQL.
func TestEnqueue_PayloadContainsTaskID(t *testing.T) {
	client := newClientOrSkip(t)
	flushTestDB(t, client)
	q := queue.NewRedisQueue(client)

	task := makeTask(testTaskID, testPipelineID, testUserID)
	if _, err := q.Enqueue(context.Background(), &queue.ProducerMessage{Task: task, Tags: []string{"etl"}}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	entries, _ := client.XRange(context.Background(), "queue:etl", "-", "+").Result()
	if len(entries) == 0 {
		t.Fatal("no entries in queue:etl")
	}

	rawPayload, ok := entries[0].Values["payload"]
	if !ok {
		t.Fatal("stream entry missing 'payload' field")
	}
	var tm queue.TaskMessage
	if err := json.Unmarshal([]byte(fmt.Sprintf("%v", rawPayload)), &tm); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if tm.TaskID != testTaskID {
		t.Errorf("TaskMessage.TaskID = %q; want %q", tm.TaskID, testTaskID)
	}
}

// TestEnqueue_CreatesConsumerGroupOnFirstUse verifies that Enqueue creates the consumer
// group for a stream that does not yet exist, satisfying AC-2 for the enqueue path.
func TestEnqueue_CreatesConsumerGroupOnFirstUse(t *testing.T) {
	client := newClientOrSkip(t)
	flushTestDB(t, client)
	q := queue.NewRedisQueue(client)

	task := makeTask(testTaskID, testPipelineID, testUserID)
	if _, err := q.Enqueue(context.Background(), &queue.ProducerMessage{Task: task, Tags: []string{"newstream"}}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	groups, err := client.XInfoGroups(context.Background(), "queue:newstream").Result()
	if err != nil {
		t.Fatalf("XInfoGroups: %v", err)
	}
	if !hasGroup(groups, queue.ConsumerGroupName) {
		t.Errorf("consumer group %q not created by Enqueue", queue.ConsumerGroupName)
	}
}

// TestEnqueue_EmptyTagsReturnsError verifies the fail-fast precondition:
// an empty Tags slice is rejected before any Redis call.
func TestEnqueue_EmptyTagsReturnsError(t *testing.T) {
	client := redis.NewClient(&redis.Options{Addr: redisAddr()})
	defer func() { _ = client.Close() }()
	q := queue.NewRedisQueue(client)

	task := makeTask(testTaskID, testPipelineID, testUserID)
	_, err := q.Enqueue(context.Background(), &queue.ProducerMessage{Task: task, Tags: []string{}})
	if err == nil {
		t.Fatal("expected error for empty tags, got nil")
	}
}

// TestEnqueue_NilTaskReturnsError verifies the fail-fast precondition:
// a nil Task is rejected immediately.
func TestEnqueue_NilTaskReturnsError(t *testing.T) {
	client := redis.NewClient(&redis.Options{Addr: redisAddr()})
	defer func() { _ = client.Close() }()
	q := queue.NewRedisQueue(client)

	_, err := q.Enqueue(context.Background(), &queue.ProducerMessage{Task: nil, Tags: []string{"etl"}})
	if err == nil {
		t.Fatal("expected error for nil task, got nil")
	}
}

// --- ReadTasks ---

// TestReadTasks_ReturnsEnqueuedTask verifies AC-3: XREADGROUP blocking read returns
// tasks to the appropriate consumer.
func TestReadTasks_ReturnsEnqueuedTask(t *testing.T) {
	client := newClientOrSkip(t)
	flushTestDB(t, client)
	q := queue.NewRedisQueue(client)

	tags := []string{"etl"}
	if err := q.InitGroups(context.Background(), tags); err != nil {
		t.Fatalf("InitGroups: %v", err)
	}

	task := makeTask(testTaskID, testPipelineID, testUserID)
	if _, err := q.Enqueue(context.Background(), &queue.ProducerMessage{Task: task, Tags: tags}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	messages, err := q.ReadTasks(context.Background(), "worker-1", tags, 200*time.Millisecond)
	if err != nil {
		t.Fatalf("ReadTasks: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}
	if messages[0].TaskID != testTaskID {
		t.Errorf("TaskMessage.TaskID = %q; want %q", messages[0].TaskID, testTaskID)
	}
}

// TestReadTasks_EmptyOnTimeout verifies that ReadTasks returns an empty slice (not an error)
// when the block window expires with no messages available.
func TestReadTasks_EmptyOnTimeout(t *testing.T) {
	client := newClientOrSkip(t)
	flushTestDB(t, client)
	q := queue.NewRedisQueue(client)

	tags := []string{"etl"}
	if err := q.InitGroups(context.Background(), tags); err != nil {
		t.Fatalf("InitGroups: %v", err)
	}

	messages, err := q.ReadTasks(context.Background(), "worker-1", tags, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("ReadTasks: %v", err)
	}
	if len(messages) != 0 {
		t.Errorf("expected 0 messages on timeout, got %d", len(messages))
	}
}

// TestReadTasks_ContextCancelReturnsNil verifies that a cancelled context causes
// ReadTasks to return nil, nil — not an error — so the caller loop can exit cleanly.
func TestReadTasks_ContextCancelReturnsNil(t *testing.T) {
	client := newClientOrSkip(t)
	flushTestDB(t, client)
	q := queue.NewRedisQueue(client)

	tags := []string{"etl"}
	if err := q.InitGroups(context.Background(), tags); err != nil {
		t.Fatalf("InitGroups: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(30 * time.Millisecond)
		cancel()
	}()

	messages, err := q.ReadTasks(ctx, "worker-1", tags, 5*time.Second)
	if err != nil {
		t.Errorf("ReadTasks with cancelled context returned error: %v", err)
	}
	if messages != nil {
		t.Errorf("expected nil messages on context cancel, got %v", messages)
	}
}

// TestReadTasks_PopulatesStreamID verifies that TaskMessage.StreamID is set from the
// XREADGROUP response, enabling Acknowledge to call XACK with the correct message ID.
func TestReadTasks_PopulatesStreamID(t *testing.T) {
	client := newClientOrSkip(t)
	flushTestDB(t, client)
	q := queue.NewRedisQueue(client)

	tags := []string{"etl"}
	if err := q.InitGroups(context.Background(), tags); err != nil {
		t.Fatalf("InitGroups: %v", err)
	}

	task := makeTask(testTaskID, testPipelineID, testUserID)
	enqueueIDs, err := q.Enqueue(context.Background(), &queue.ProducerMessage{Task: task, Tags: tags})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	messages, err := q.ReadTasks(context.Background(), "worker-1", tags, 200*time.Millisecond)
	if err != nil || len(messages) == 0 {
		t.Fatalf("ReadTasks: err=%v, len=%d", err, len(messages))
	}

	if messages[0].StreamID == "" {
		t.Fatal("TaskMessage.StreamID is empty after ReadTasks")
	}
	if messages[0].StreamID != enqueueIDs[0] {
		t.Errorf("StreamID = %q; want %q", messages[0].StreamID, enqueueIDs[0])
	}
}

// --- Acknowledge ---

// TestAcknowledge_RemovesFromPendingList verifies AC-4: XACK removes the task from
// the pending entry list so the Monitor does not attempt to reclaim it.
func TestAcknowledge_RemovesFromPendingList(t *testing.T) {
	client := newClientOrSkip(t)
	flushTestDB(t, client)
	q := queue.NewRedisQueue(client)

	tags := []string{"etl"}
	if err := q.InitGroups(context.Background(), tags); err != nil {
		t.Fatalf("InitGroups: %v", err)
	}

	task := makeTask(testTaskID, testPipelineID, testUserID)
	if _, err := q.Enqueue(context.Background(), &queue.ProducerMessage{Task: task, Tags: tags}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	messages, err := q.ReadTasks(context.Background(), "worker-1", tags, 200*time.Millisecond)
	if err != nil || len(messages) == 0 {
		t.Fatalf("ReadTasks: err=%v, len=%d", err, len(messages))
	}

	// Verify the message is pending before ACK.
	pending, err := client.XPending(context.Background(), "queue:etl", queue.ConsumerGroupName).Result()
	if err != nil {
		t.Fatalf("XPending (before ACK): %v", err)
	}
	if pending.Count != 1 {
		t.Fatalf("expected 1 pending entry before ACK, got %d", pending.Count)
	}

	if err := q.Acknowledge(context.Background(), "etl", messages[0].StreamID); err != nil {
		t.Fatalf("Acknowledge: %v", err)
	}

	// Verify the message is no longer pending after ACK.
	pending, err = client.XPending(context.Background(), "queue:etl", queue.ConsumerGroupName).Result()
	if err != nil {
		t.Fatalf("XPending (after ACK): %v", err)
	}
	if pending.Count != 0 {
		t.Errorf("expected 0 pending entries after ACK, got %d", pending.Count)
	}
}

// --- EnqueueDeadLetter ---

// TestEnqueueDeadLetter_AddsToDeadLetterStream verifies that EnqueueDeadLetter writes
// to queue:dead-letter with taskId and reason fields (ADR-001, dead letter stream setup).
func TestEnqueueDeadLetter_AddsToDeadLetterStream(t *testing.T) {
	client := newClientOrSkip(t)
	flushTestDB(t, client)
	q := queue.NewRedisQueue(client)

	reason := "max retries exhausted"
	if err := q.EnqueueDeadLetter(context.Background(), testTaskID, reason); err != nil {
		t.Fatalf("EnqueueDeadLetter: %v", err)
	}

	entries, err := client.XRange(context.Background(), queue.DeadLetterStream, "-", "+").Result()
	if err != nil {
		t.Fatalf("XRange(dead-letter): %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry in dead letter stream, got %d", len(entries))
	}

	if got := fmt.Sprintf("%v", entries[0].Values["taskId"]); got != testTaskID {
		t.Errorf("taskId = %q; want %q", got, testTaskID)
	}
	if got := fmt.Sprintf("%v", entries[0].Values["reason"]); got != reason {
		t.Errorf("reason = %q; want %q", got, reason)
	}
}

// TestEnqueueDeadLetter_MultipleEntriesDistinct verifies that multiple dead-letter
// enqueues each produce a distinct stream entry (streams allow duplicate content).
func TestEnqueueDeadLetter_MultipleEntriesDistinct(t *testing.T) {
	client := newClientOrSkip(t)
	flushTestDB(t, client)
	q := queue.NewRedisQueue(client)

	for i := 0; i < 2; i++ {
		if err := q.EnqueueDeadLetter(context.Background(), testTaskID, "reason"); err != nil {
			t.Fatalf("EnqueueDeadLetter call %d: %v", i, err)
		}
	}

	entries, err := client.XRange(context.Background(), queue.DeadLetterStream, "-", "+").Result()
	if err != nil {
		t.Fatalf("XRange: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 entries in dead letter stream, got %d", len(entries))
	}
	if entries[0].ID == entries[1].ID {
		t.Errorf("duplicate stream IDs — entries must be distinct: %q", entries[0].ID)
	}
}

// --- Benchmark: AC-5 ---

// BenchmarkEnqueue_1000Sequential measures per-operation XADD latency over 1,000
// sequential enqueues and asserts that the p95 is under 50ms (AC-5, ADR-001 FF-001).
//
// Run: go test -bench=BenchmarkEnqueue_1000Sequential -benchtime=1x -run=^$ ./internal/queue/
func BenchmarkEnqueue_1000Sequential(b *testing.B) {
	addr := redisAddr()
	client := redis.NewClient(&redis.Options{Addr: addr})
	defer func() { _ = client.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	if err := client.Ping(ctx).Err(); err != nil {
		cancel()
		b.Skipf("Redis unavailable at %s: %v", addr, err)
	}
	cancel()
	_ = client.FlushDB(context.Background()).Err()

	q := queue.NewRedisQueue(client)

	const n = 1000
	latencies := make([]time.Duration, 0, n)

	b.ResetTimer()
	for i := 0; i < n; i++ {
		taskID := fmt.Sprintf("00000000-0000-0000-0000-%012d", i+1)
		task := makeTask(taskID, testPipelineID, testUserID)
		start := time.Now()
		_, err := q.Enqueue(context.Background(), &queue.ProducerMessage{Task: task, Tags: []string{"etl"}})
		latencies = append(latencies, time.Since(start))
		if err != nil {
			b.Fatalf("Enqueue %d: %v", i, err)
		}
	}
	b.StopTimer()

	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	p95idx := int(float64(n)*0.95) - 1
	if p95idx < 0 {
		p95idx = 0
	}
	p95 := latencies[p95idx]
	b.ReportMetric(float64(p95.Milliseconds()), "p95_ms")
	b.Logf("p95 latency = %v (limit: 50ms)", p95)

	if p95 >= 50*time.Millisecond {
		b.Errorf("AC-5 VIOLATED: p95 latency %v >= 50ms", p95)
	}
}

// --- helpers ---

// hasGroup returns true if the named consumer group is present in the XINFO GROUPS result.
func hasGroup(groups []redis.XInfoGroup, name string) bool {
	for _, g := range groups {
		if g.Name == name {
			return true
		}
	}
	return false
}
