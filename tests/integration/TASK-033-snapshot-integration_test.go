// Package integration — TASK-033 integration tests: SnapshotCapturer component seam.
//
// Requirement: DEMO-003, ADR-009, TASK-033
//
// These tests verify the integration boundary between:
//   - Worker.runSink / WithSnapshotPublisher — the SnapshotCapturer wiring path
//   - SnapshotCapturer.CaptureAndWrite — event ordering and channel-name contract
//   - sinkChannelName — format events:sink:{taskID}
//
// All tests use the in-memory fakes (InMemoryDatabase, InMemoryS3, inMemoryPublisher)
// exported by the worker package and defined in tests/integration/helpers_033_test.go.
// No live Redis, PostgreSQL, or MinIO instance is required.
//
// Integration layer focus: these tests validate the assembly of SnapshotCapturer with
// the Worker and with each SinkConnector type. They do not duplicate unit-level tests
// of individual snapshot fields — that is the Builder's domain (worker/snapshot_test.go).
//
// Run:
//
//	go test ./tests/integration/... -v -run TASK033
//
// See: DEMO-003, ADR-009, TASK-033
package integration

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/redis/go-redis/v9"

	"github.com/nxlabs/nexusflow/worker"
)

// -----------------------------------------------------------------------
// inMemoryPublisher — Pub/Sub fake for integration tests (local to this file)
// -----------------------------------------------------------------------

// integrationPublisher is a thread-safe fake snapshotPublisher for integration tests.
// It records every Publish call so tests can inspect the emitted events.
type integrationPublisher struct {
	mu       sync.Mutex
	messages []intPubMessage
}

type intPubMessage struct {
	Channel string
	Payload string
}

// Publish records the channel and message, returns a no-error *redis.IntCmd.
func (p *integrationPublisher) Publish(ctx context.Context, channel string, message any) *redis.IntCmd {
	p.mu.Lock()
	defer p.mu.Unlock()
	var payload string
	switch m := message.(type) {
	case string:
		payload = m
	case []byte:
		payload = string(m)
	default:
		b, _ := json.Marshal(m)
		payload = string(b)
	}
	p.messages = append(p.messages, intPubMessage{Channel: channel, Payload: payload})
	cmd := redis.NewIntCmd(ctx)
	cmd.SetVal(1)
	return cmd
}

// Messages returns a snapshot of all recorded messages.
func (p *integrationPublisher) Messages() []intPubMessage {
	p.mu.Lock()
	defer p.mu.Unlock()
	cp := make([]intPubMessage, len(p.messages))
	copy(cp, p.messages)
	return cp
}

// decodeIntPubEvent JSON-decodes the payload of the i-th message.
func decodeIntPubEvent(t *testing.T, msgs []intPubMessage, i int) map[string]any {
	t.Helper()
	if i >= len(msgs) {
		t.Fatalf("expected at least %d published message(s), got %d", i+1, len(msgs))
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(msgs[i].Payload), &out); err != nil {
		t.Fatalf("message[%d] is not valid JSON: %v\npayload: %s", i, err, msgs[i].Payload)
	}
	return out
}

// -----------------------------------------------------------------------
// IT-1: Channel name format — events:sink:{taskID}
// -----------------------------------------------------------------------

// TestTASK033_IT1_ChannelNameFormat verifies that SnapshotCapturer publishes to the
// channel named "events:sink:{taskID}" exactly — the contract the Sink Inspector SSE
// endpoint subscribes to (ADR-007, TASK-032).
//
// DEMO-003, TASK-033
// Given:  a SnapshotCapturer wired with a DatabaseSinkConnector and an integrationPublisher
// When:   CaptureAndWrite is called for taskID "int-task-channel-001"
// Then:   all published messages use channel "events:sink:int-task-channel-001"
func TestTASK033_IT1_ChannelNameFormat(t *testing.T) {
	db := worker.NewInMemoryDatabase()
	pub := &integrationPublisher{}
	dedup := worker.NewInMemoryDedupStore()
	conn := worker.NewDatabaseSinkConnector(dedup)
	conn.UseDatabase(db)

	sc := worker.NewSnapshotCapturer(conn, pub)

	const taskID = "int-task-channel-001"
	expectedChannel := "events:sink:" + taskID

	cfg := map[string]any{"table": "orders"}
	records := []map[string]any{{"id": 1}}

	if err := sc.CaptureAndWrite(context.Background(), cfg, records, "exec-int-001", taskID); err != nil {
		t.Fatalf("CaptureAndWrite returned unexpected error: %v", err)
	}

	msgs := pub.Messages()
	if len(msgs) != 2 {
		t.Fatalf("expected exactly 2 published messages; got %d", len(msgs))
	}
	for i, msg := range msgs {
		if msg.Channel != expectedChannel {
			t.Errorf("message[%d] channel = %q; want %q", i, msg.Channel, expectedChannel)
		}
	}
}

// -----------------------------------------------------------------------
// IT-2: Event sequence — before precedes after in the channel
// -----------------------------------------------------------------------

// TestTASK033_IT2_EventSequenceBeforeBeforeAfter verifies that when SnapshotCapturer
// publishes to the channel, the sink:before-snapshot event arrives before the
// sink:after-result event. This is the ordering contract the Sink Inspector depends on.
//
// DEMO-003, TASK-033
// Given:  a SnapshotCapturer wrapping a DatabaseSinkConnector
// When:   CaptureAndWrite completes successfully
// Then:   message[0].eventType == "sink:before-snapshot"
//
//	message[1].eventType == "sink:after-result"
func TestTASK033_IT2_EventSequenceBeforeBeforeAfter(t *testing.T) {
	db := worker.NewInMemoryDatabase()
	pub := &integrationPublisher{}
	dedup := worker.NewInMemoryDedupStore()
	conn := worker.NewDatabaseSinkConnector(dedup)
	conn.UseDatabase(db)
	sc := worker.NewSnapshotCapturer(conn, pub)

	cfg := map[string]any{"table": "seq_table"}
	if err := sc.CaptureAndWrite(context.Background(), cfg, []map[string]any{{"id": 1}}, "exec-int-seq", "task-int-seq"); err != nil {
		t.Fatalf("CaptureAndWrite error: %v", err)
	}

	msgs := pub.Messages()
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}

	evt0 := decodeIntPubEvent(t, msgs, 0)
	if evt0["eventType"] != worker.SinkEventBeforeSnapshot {
		t.Errorf("message[0].eventType = %q; want %q", evt0["eventType"], worker.SinkEventBeforeSnapshot)
	}
	evt1 := decodeIntPubEvent(t, msgs, 1)
	if evt1["eventType"] != worker.SinkEventAfterResult {
		t.Errorf("message[1].eventType = %q; want %q", evt1["eventType"], worker.SinkEventAfterResult)
	}
}

// -----------------------------------------------------------------------
// IT-3: Events published even on write failure (rollback path)
// -----------------------------------------------------------------------

// TestTASK033_IT3_BothEventsPublishedOnWriteFailure verifies that SnapshotCapturer
// publishes both events even when the underlying Write fails and rolls back.
// This is essential for the Sink Inspector to display rollback state.
//
// DEMO-003, ADR-009, TASK-033
// Given:  a DatabaseSinkConnector backed by a database configured to fail on first insert
// When:   CaptureAndWrite is called
// Then:   CaptureAndWrite returns a non-nil error
//
//	AND two events are published (before + after)
//	AND the after-result event has rolledBack=true
func TestTASK033_IT3_BothEventsPublishedOnWriteFailure(t *testing.T) {
	db := worker.NewInMemoryDatabase()
	db.FailAfterRow(0) // fail immediately
	pub := &integrationPublisher{}
	dedup := worker.NewInMemoryDedupStore()
	conn := worker.NewDatabaseSinkConnector(dedup)
	conn.UseDatabase(db)
	sc := worker.NewSnapshotCapturer(conn, pub)

	cfg := map[string]any{"table": "fail_table"}
	err := sc.CaptureAndWrite(context.Background(), cfg, []map[string]any{{"id": 1}}, "exec-int-fail", "task-int-fail")
	if err == nil {
		t.Fatal("expected CaptureAndWrite to return error on forced write failure")
	}

	msgs := pub.Messages()
	if len(msgs) != 2 {
		t.Fatalf("expected 2 published messages on failure; got %d", len(msgs))
	}

	afterEvt := decodeIntPubEvent(t, msgs, 1)
	if rolledBack, _ := afterEvt["rolledBack"].(bool); !rolledBack {
		t.Error("after-result event must have rolledBack=true on write failure")
	}
	if errMsg, _ := afterEvt["writeError"].(string); errMsg == "" {
		t.Error("after-result event must carry a non-empty writeError on failure")
	}
}

// -----------------------------------------------------------------------
// IT-4: S3 connector — channel name and event types identical
// -----------------------------------------------------------------------

// TestTASK033_IT4_S3ChannelNameAndEventTypes verifies that the S3SinkConnector path
// through SnapshotCapturer produces the same channel name and event type sequence as
// the database path — the channel name is not connector-type-specific.
//
// DEMO-003, TASK-033
// Given:  a SnapshotCapturer wrapping an S3SinkConnector
// When:   CaptureAndWrite completes successfully
// Then:   channel = "events:sink:task-int-s3-001"
//
//	message[0].eventType = "sink:before-snapshot"
//	message[1].eventType = "sink:after-result"
func TestTASK033_IT4_S3ChannelNameAndEventTypes(t *testing.T) {
	s3 := worker.NewInMemoryS3()
	pub := &integrationPublisher{}
	dedup := worker.NewInMemoryDedupStore()
	conn := worker.NewS3SinkConnector(s3, dedup)
	sc := worker.NewSnapshotCapturer(conn, pub)

	const taskID = "task-int-s3-001"
	expectedChannel := "events:sink:" + taskID

	cfg := map[string]any{"bucket": "test-bucket", "key": "output/data.json"}
	if err := sc.CaptureAndWrite(context.Background(), cfg, []map[string]any{{"id": 1}}, "exec-s3-int", taskID); err != nil {
		t.Fatalf("CaptureAndWrite error: %v", err)
	}

	msgs := pub.Messages()
	if len(msgs) != 2 {
		t.Fatalf("expected 2 published messages; got %d", len(msgs))
	}
	for i, msg := range msgs {
		if msg.Channel != expectedChannel {
			t.Errorf("message[%d] channel = %q; want %q", i, msg.Channel, expectedChannel)
		}
	}

	evt0 := decodeIntPubEvent(t, msgs, 0)
	if evt0["eventType"] != worker.SinkEventBeforeSnapshot {
		t.Errorf("message[0].eventType = %q; want %q", evt0["eventType"], worker.SinkEventBeforeSnapshot)
	}
	evt1 := decodeIntPubEvent(t, msgs, 1)
	if evt1["eventType"] != worker.SinkEventAfterResult {
		t.Errorf("message[1].eventType = %q; want %q", evt1["eventType"], worker.SinkEventAfterResult)
	}
}

// -----------------------------------------------------------------------
// IT-5: before snapshot taskID field matches the task being executed
// -----------------------------------------------------------------------

// TestTASK033_IT5_TaskIDFieldMatchesExecutingTask verifies that the taskId field in
// every published event matches the taskID passed to CaptureAndWrite. This is the
// field the Sink Inspector uses to route events to the correct subscription.
//
// DEMO-003, TASK-033
// Given:  a SnapshotCapturer wrapping a DatabaseSinkConnector
// When:   CaptureAndWrite is called with taskID "task-int-id-check"
// Then:   message[0].taskId == "task-int-id-check"
//
//	message[1].taskId == "task-int-id-check"
func TestTASK033_IT5_TaskIDFieldMatchesExecutingTask(t *testing.T) {
	db := worker.NewInMemoryDatabase()
	pub := &integrationPublisher{}
	dedup := worker.NewInMemoryDedupStore()
	conn := worker.NewDatabaseSinkConnector(dedup)
	conn.UseDatabase(db)
	sc := worker.NewSnapshotCapturer(conn, pub)

	const taskID = "task-int-id-check"
	cfg := map[string]any{"table": "check_table"}

	if err := sc.CaptureAndWrite(context.Background(), cfg, []map[string]any{{"id": 1}}, "exec-int-id", taskID); err != nil {
		t.Fatalf("CaptureAndWrite error: %v", err)
	}

	msgs := pub.Messages()
	for i, msg := range msgs {
		evt := decodeIntPubEvent(t, msgs, i)
		_ = msg
		if tid, _ := evt["taskId"].(string); tid != taskID {
			t.Errorf("message[%d].taskId = %q; want %q", i, tid, taskID)
		}
	}
}

// -----------------------------------------------------------------------
// IT-6 [VERIFIER-ADDED]: nil publisher wiring — WithSnapshotPublisher panics on nil
// -----------------------------------------------------------------------

// TestTASK033_IT6_NilPublisherWiringPanics verifies the fail-fast guard on
// NewSnapshotCapturer — passing a nil publisher panics immediately rather than
// silently failing at publish time. This guards against the nil-wiring gap
// documented in the project memory (feedback_nil_wiring.md).
//
// [VERIFIER-ADDED] — probes the wiring precondition at the integration boundary.
//
// DEMO-003, TASK-033
// Given:  a valid DatabaseSinkConnector
// When:   NewSnapshotCapturer is called with a nil publisher
// Then:   panic with a non-empty message (fail-fast; no silent no-op)
func TestTASK033_IT6_NilPublisherWiringPanics(t *testing.T) {
	conn := worker.NewDatabaseSinkConnector(worker.NewInMemoryDedupStore())
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic when publisher is nil — fail-fast guard not enforced")
		}
	}()
	_ = worker.NewSnapshotCapturer(conn, nil)
}
