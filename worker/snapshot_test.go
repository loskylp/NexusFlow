// Package worker_test — unit tests for TASK-033: Sink Before/After snapshot capture.
//
// All tests use an inMemoryPublisher (no live Redis) and the existing InMemoryDatabase /
// InMemoryS3 fakes (no live PostgreSQL or MinIO).
//
// AC coverage:
//   - AC1: Before snapshot is captured before Sink writes begin
//   - AC2: After snapshot is captured after Sink completion or rollback
//   - AC3: Snapshots are published to events:sink:{taskId} for SSE consumption
//   - AC4: For database sinks: snapshot queries the target table (row_count)
//   - AC5: For S3 sinks: snapshot lists objects in the target prefix (object_count)
//   - AC6: On rollback, After snapshot matches Before snapshot
//
// See: DEMO-003, ADR-009, TASK-033
package worker_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/nxlabs/nexusflow/worker"
	"github.com/redis/go-redis/v9"
)

// -----------------------------------------------------------------------
// inMemoryPublisher — fake snapshotPublisher for unit tests
// -----------------------------------------------------------------------

// inMemoryPublisher is a thread-safe fake that records every Publish call.
// It satisfies the snapshotPublisher interface via the exported PublisherForTest
// type returned by NewSnapshotCapturerForTest.
type inMemoryPublisher struct {
	mu       sync.Mutex
	messages []publishedMessage
}

type publishedMessage struct {
	Channel string
	Payload string
}

// Publish records the channel and message and returns a no-op *redis.IntCmd.
// The returned IntCmd always reports success (val=1, err=nil).
func (p *inMemoryPublisher) Publish(ctx context.Context, channel string, message any) *redis.IntCmd {
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
	p.messages = append(p.messages, publishedMessage{Channel: channel, Payload: payload})
	cmd := redis.NewIntCmd(ctx)
	cmd.SetVal(1)
	return cmd
}

// Messages returns a copy of all recorded messages.
func (p *inMemoryPublisher) Messages() []publishedMessage {
	p.mu.Lock()
	defer p.mu.Unlock()
	cp := make([]publishedMessage, len(p.messages))
	copy(cp, p.messages)
	return cp
}

// -----------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------

// newDatabaseCapturer returns a SnapshotCapturer wrapping a DatabaseSinkConnector
// backed by db, plus the inMemoryPublisher so tests can inspect published events.
func newDatabaseCapturer(db *worker.InMemoryDatabase) (*worker.SnapshotCapturer, *inMemoryPublisher) {
	pub := &inMemoryPublisher{}
	dedup := worker.NewInMemoryDedupStore()
	conn := worker.NewDatabaseSinkConnector(dedup)
	conn.UseDatabase(db)
	sc := worker.NewSnapshotCapturer(conn, pub)
	return sc, pub
}

// newS3Capturer returns a SnapshotCapturer wrapping an S3SinkConnector backed by
// s3, plus the publisher.
func newS3Capturer(s3 *worker.InMemoryS3) (*worker.SnapshotCapturer, *inMemoryPublisher) {
	pub := &inMemoryPublisher{}
	dedup := worker.NewInMemoryDedupStore()
	conn := worker.NewS3SinkConnector(s3, dedup)
	sc := worker.NewSnapshotCapturer(conn, pub)
	return sc, pub
}

// decodeEvent JSON-decodes the payload of the i-th published message.
// Panics when i is out of range or the payload is not valid JSON.
func decodeEvent(t *testing.T, msgs []publishedMessage, i int) map[string]any {
	t.Helper()
	if i >= len(msgs) {
		t.Fatalf("expected at least %d published message(s), got %d", i+1, len(msgs))
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(msgs[i].Payload), &out); err != nil {
		t.Fatalf("published message[%d] is not valid JSON: %v\npayload: %s", i, err, msgs[i].Payload)
	}
	return out
}

// -----------------------------------------------------------------------
// AC1, AC2, AC3 — basic capture-and-write cycle (database sink)
// -----------------------------------------------------------------------

// TestSnapshotCapturer_CaptureAndWrite_PublishesBeforeEvent verifies that a
// sink:before-snapshot event is published to events:sink:{taskId} before any
// write occurs (AC1, AC3).
func TestSnapshotCapturer_CaptureAndWrite_PublishesBeforeEvent(t *testing.T) {
	db := worker.NewInMemoryDatabase()
	db.Seed("users", []map[string]any{{"id": 1}})
	sc, pub := newDatabaseCapturer(db)

	taskID := "task-before-001"
	cfg := map[string]any{"table": "users"}
	records := []map[string]any{{"id": 2, "name": "alice"}}

	if err := sc.CaptureAndWrite(context.Background(), cfg, records, "exec-001", taskID); err != nil {
		t.Fatalf("CaptureAndWrite returned unexpected error: %v", err)
	}

	msgs := pub.Messages()
	if len(msgs) < 1 {
		t.Fatalf("expected at least 1 published message, got 0")
	}

	// First message must be sink:before-snapshot on the correct channel.
	expectedChannel := "events:sink:" + taskID
	if msgs[0].Channel != expectedChannel {
		t.Errorf("before-snapshot channel = %q; want %q", msgs[0].Channel, expectedChannel)
	}

	evt := decodeEvent(t, msgs, 0)
	if evt["eventType"] != worker.SinkEventBeforeSnapshot {
		t.Errorf("before event type = %q; want %q", evt["eventType"], worker.SinkEventBeforeSnapshot)
	}
	if evt["taskId"] != taskID {
		t.Errorf("before event taskId = %q; want %q", evt["taskId"], taskID)
	}
	if evt["before"] == nil {
		t.Error("before event must carry a non-nil 'before' snapshot")
	}
}

// TestSnapshotCapturer_CaptureAndWrite_PublishesAfterEvent verifies that a
// sink:after-result event is published after the write completes (AC2, AC3).
func TestSnapshotCapturer_CaptureAndWrite_PublishesAfterEvent(t *testing.T) {
	db := worker.NewInMemoryDatabase()
	sc, pub := newDatabaseCapturer(db)

	taskID := "task-after-001"
	cfg := map[string]any{"table": "orders"}
	records := []map[string]any{{"id": 10}}

	if err := sc.CaptureAndWrite(context.Background(), cfg, records, "exec-after-001", taskID); err != nil {
		t.Fatalf("CaptureAndWrite returned unexpected error: %v", err)
	}

	msgs := pub.Messages()
	if len(msgs) < 2 {
		t.Fatalf("expected 2 published messages (before+after), got %d", len(msgs))
	}

	// Second message must be sink:after-result.
	expectedChannel := "events:sink:" + taskID
	if msgs[1].Channel != expectedChannel {
		t.Errorf("after-result channel = %q; want %q", msgs[1].Channel, expectedChannel)
	}

	evt := decodeEvent(t, msgs, 1)
	if evt["eventType"] != worker.SinkEventAfterResult {
		t.Errorf("after event type = %q; want %q", evt["eventType"], worker.SinkEventAfterResult)
	}
	if evt["taskId"] != taskID {
		t.Errorf("after event taskId = %q; want %q", evt["taskId"], taskID)
	}
	if evt["before"] == nil {
		t.Error("after event must carry a non-nil 'before' snapshot for comparison")
	}
	if evt["after"] == nil {
		t.Error("after event must carry a non-nil 'after' snapshot")
	}
}

// TestSnapshotCapturer_CaptureAndWrite_ReturnsWriteError verifies that when the
// underlying connector's Write fails, CaptureAndWrite returns the write error
// (not a snapshot error) after publishing both events (AC2).
func TestSnapshotCapturer_CaptureAndWrite_ReturnsWriteError(t *testing.T) {
	db := worker.NewInMemoryDatabase()
	db.FailAfterRow(0) // fail immediately on first insert
	sc, pub := newDatabaseCapturer(db)

	taskID := "task-write-fail"
	cfg := map[string]any{"table": "items"}
	records := []map[string]any{{"id": 1}}

	err := sc.CaptureAndWrite(context.Background(), cfg, records, "exec-fail", taskID)
	if err == nil {
		t.Fatal("expected CaptureAndWrite to return error on write failure, got nil")
	}

	// Both events must still be published even on failure.
	msgs := pub.Messages()
	if len(msgs) != 2 {
		t.Errorf("expected 2 published messages on failure, got %d", len(msgs))
	}
}

// -----------------------------------------------------------------------
// AC4 — database sink snapshots row_count
// -----------------------------------------------------------------------

// TestSnapshotCapturer_DatabaseSink_BeforeSnapshotReflectsRowCount verifies that
// the Before snapshot's data contains a "row_count" matching the table row count
// before the write (AC4).
func TestSnapshotCapturer_DatabaseSink_BeforeSnapshotReflectsRowCount(t *testing.T) {
	db := worker.NewInMemoryDatabase()
	db.Seed("products", []map[string]any{{"id": 1}, {"id": 2}, {"id": 3}})
	sc, pub := newDatabaseCapturer(db)

	cfg := map[string]any{"table": "products"}
	records := []map[string]any{{"id": 4}}

	if err := sc.CaptureAndWrite(context.Background(), cfg, records, "exec-rc", "task-rc"); err != nil {
		t.Fatalf("CaptureAndWrite error: %v", err)
	}

	msgs := pub.Messages()
	beforeEvt := decodeEvent(t, msgs, 0)
	before, ok := beforeEvt["before"].(map[string]any)
	if !ok {
		t.Fatalf("before snapshot is not a JSON object; got %T", beforeEvt["before"])
	}
	data, ok := before["data"].(map[string]any)
	if !ok {
		t.Fatalf("before.data is not a JSON object; got %T", before["data"])
	}
	// row_count is decoded as float64 from JSON.
	if rc, _ := data["row_count"].(float64); int(rc) != 3 {
		t.Errorf("before snapshot row_count = %v; want 3", data["row_count"])
	}
}

// TestSnapshotCapturer_DatabaseSink_AfterSnapshotReflectsNewRowCount verifies that
// the After snapshot's row_count is greater than the Before count after a successful
// write (AC4, AC2).
func TestSnapshotCapturer_DatabaseSink_AfterSnapshotReflectsNewRowCount(t *testing.T) {
	db := worker.NewInMemoryDatabase()
	db.Seed("products", []map[string]any{{"id": 1}})
	sc, pub := newDatabaseCapturer(db)

	cfg := map[string]any{"table": "products"}
	records := []map[string]any{{"id": 2}, {"id": 3}}

	if err := sc.CaptureAndWrite(context.Background(), cfg, records, "exec-after-rc", "task-after-rc"); err != nil {
		t.Fatalf("CaptureAndWrite error: %v", err)
	}

	msgs := pub.Messages()
	afterEvt := decodeEvent(t, msgs, 1)
	after, ok := afterEvt["after"].(map[string]any)
	if !ok {
		t.Fatalf("after snapshot is not a JSON object; got %T", afterEvt["after"])
	}
	data, ok := after["data"].(map[string]any)
	if !ok {
		t.Fatalf("after.data is not a JSON object; got %T", after["data"])
	}
	if rc, _ := data["row_count"].(float64); int(rc) != 3 {
		t.Errorf("after snapshot row_count = %v; want 3", data["row_count"])
	}
}

// -----------------------------------------------------------------------
// AC5 — S3 sink snapshots object_count
// -----------------------------------------------------------------------

// TestSnapshotCapturer_S3Sink_BeforeSnapshotReflectsObjectCount verifies that
// the Before snapshot carries an "object_count" for the prefix in scope (AC5).
func TestSnapshotCapturer_S3Sink_BeforeSnapshotReflectsObjectCount(t *testing.T) {
	s3 := worker.NewInMemoryS3()
	s3.Put("my-bucket", "output/a.json", []byte(`{}`))
	s3.Put("my-bucket", "output/b.json", []byte(`{}`))
	sc, pub := newS3Capturer(s3)

	cfg := map[string]any{"bucket": "my-bucket", "key": "output/data.json"}
	records := []map[string]any{{"id": 1}}

	if err := sc.CaptureAndWrite(context.Background(), cfg, records, "exec-s3", "task-s3"); err != nil {
		t.Fatalf("CaptureAndWrite error: %v", err)
	}

	msgs := pub.Messages()
	beforeEvt := decodeEvent(t, msgs, 0)
	before, ok := beforeEvt["before"].(map[string]any)
	if !ok {
		t.Fatalf("before snapshot not a JSON object; got %T", beforeEvt["before"])
	}
	data, ok := before["data"].(map[string]any)
	if !ok {
		t.Fatalf("before.data not a JSON object; got %T", before["data"])
	}
	if oc, _ := data["object_count"].(float64); int(oc) != 2 {
		t.Errorf("before snapshot object_count = %v; want 2", data["object_count"])
	}
}

// TestSnapshotCapturer_S3Sink_AfterSnapshotReflectsNewObjectCount verifies that
// the After snapshot object_count increments after a successful write (AC5).
func TestSnapshotCapturer_S3Sink_AfterSnapshotReflectsNewObjectCount(t *testing.T) {
	s3 := worker.NewInMemoryS3()
	s3.Put("my-bucket", "output/existing.json", []byte(`{}`))
	sc, pub := newS3Capturer(s3)

	cfg := map[string]any{"bucket": "my-bucket", "key": "output/new.json"}
	records := []map[string]any{{"id": 1}}

	if err := sc.CaptureAndWrite(context.Background(), cfg, records, "exec-s3-after", "task-s3-after"); err != nil {
		t.Fatalf("CaptureAndWrite error: %v", err)
	}

	msgs := pub.Messages()
	afterEvt := decodeEvent(t, msgs, 1)
	after, ok := afterEvt["after"].(map[string]any)
	if !ok {
		t.Fatalf("after snapshot not a JSON object; got %T", afterEvt["after"])
	}
	data, ok := after["data"].(map[string]any)
	if !ok {
		t.Fatalf("after.data not a JSON object; got %T", after["data"])
	}
	if oc, _ := data["object_count"].(float64); int(oc) != 2 {
		t.Errorf("after snapshot object_count = %v; want 2", data["object_count"])
	}
}

// -----------------------------------------------------------------------
// AC6 — After snapshot matches Before on rollback
// -----------------------------------------------------------------------

// TestSnapshotCapturer_DatabaseSink_AfterMatchesBeforeOnRollback verifies that
// when a Write fails and the connector rolls back, the After snapshot's row_count
// equals the Before snapshot's row_count, confirming rollback (AC6).
func TestSnapshotCapturer_DatabaseSink_AfterMatchesBeforeOnRollback(t *testing.T) {
	db := worker.NewInMemoryDatabase()
	db.Seed("items", []map[string]any{{"id": 1}, {"id": 2}})
	db.FailAfterRow(1) // write two records; fail after first insert
	sc, pub := newDatabaseCapturer(db)

	cfg := map[string]any{"table": "items"}
	records := []map[string]any{{"id": 3}, {"id": 4}}

	err := sc.CaptureAndWrite(context.Background(), cfg, records, "exec-rollback", "task-rollback")
	if err == nil {
		t.Fatal("expected CaptureAndWrite to return an error on forced rollback")
	}

	msgs := pub.Messages()
	if len(msgs) != 2 {
		t.Fatalf("expected 2 published messages; got %d", len(msgs))
	}

	beforeEvt := decodeEvent(t, msgs, 0)
	afterEvt := decodeEvent(t, msgs, 1)

	before, _ := beforeEvt["before"].(map[string]any)
	afterSnap, _ := afterEvt["after"].(map[string]any)

	beforeData, _ := before["data"].(map[string]any)
	afterData, _ := afterSnap["data"].(map[string]any)

	beforeRC, _ := beforeData["row_count"].(float64)
	afterRC, _ := afterData["row_count"].(float64)

	if beforeRC != afterRC {
		t.Errorf("After row_count (%v) must equal Before row_count (%v) after rollback (AC6)", afterRC, beforeRC)
	}

	// The after-result event must have rolledBack = true.
	if rolledBack, _ := afterEvt["rolledBack"].(bool); !rolledBack {
		t.Error("after-result event must have rolledBack=true on write failure")
	}

	// The after-result event must carry the write error message.
	if errMsg, _ := afterEvt["writeError"].(string); errMsg == "" {
		t.Error("after-result event must have a non-empty writeError on write failure")
	}
}

// TestSnapshotCapturer_S3Sink_AfterMatchesBeforeOnRollback verifies the same
// rollback semantics for S3 sinks (AC6).
func TestSnapshotCapturer_S3Sink_AfterMatchesBeforeOnRollback(t *testing.T) {
	s3 := worker.NewInMemoryS3()
	s3.Put("bucket", "dir/existing.json", []byte(`{}`))
	s3.FailUploadAfterPart(0) // abort immediately
	sc, pub := newS3Capturer(s3)

	cfg := map[string]any{"bucket": "bucket", "key": "dir/new.json"}
	records := []map[string]any{{"id": 1}}

	err := sc.CaptureAndWrite(context.Background(), cfg, records, "exec-s3-rb", "task-s3-rb")
	if err == nil {
		t.Fatal("expected CaptureAndWrite to return an error on forced S3 abort")
	}

	msgs := pub.Messages()
	if len(msgs) != 2 {
		t.Fatalf("expected 2 published messages; got %d", len(msgs))
	}

	beforeEvt := decodeEvent(t, msgs, 0)
	afterEvt := decodeEvent(t, msgs, 1)

	before, _ := beforeEvt["before"].(map[string]any)
	afterSnap, _ := afterEvt["after"].(map[string]any)

	beforeData, _ := before["data"].(map[string]any)
	afterData, _ := afterSnap["data"].(map[string]any)

	beforeOC, _ := beforeData["object_count"].(float64)
	afterOC, _ := afterData["object_count"].(float64)

	if beforeOC != afterOC {
		t.Errorf("After object_count (%v) must equal Before object_count (%v) after S3 abort (AC6)", afterOC, beforeOC)
	}
	if rolledBack, _ := afterEvt["rolledBack"].(bool); !rolledBack {
		t.Error("after-result event must have rolledBack=true on S3 write failure")
	}
}

// -----------------------------------------------------------------------
// Ordering guarantee
// -----------------------------------------------------------------------

// TestSnapshotCapturer_CaptureAndWrite_BeforePublishedBeforeWrite verifies that
// the before-snapshot event is published before the write occurs (i.e. the database
// has only the seeded rows when the Before snapshot is captured, not the new rows).
//
// This is verified by checking that the Before snapshot row_count reflects the
// state before the write, not after (ordering guarantee for AC1).
func TestSnapshotCapturer_CaptureAndWrite_BeforePublishedBeforeWrite(t *testing.T) {
	db := worker.NewInMemoryDatabase()
	// Seed with zero rows so Before snapshot must show 0.
	sc, pub := newDatabaseCapturer(db)

	cfg := map[string]any{"table": "audit"}
	records := []map[string]any{{"id": 1}, {"id": 2}}

	if err := sc.CaptureAndWrite(context.Background(), cfg, records, "exec-order", "task-order"); err != nil {
		t.Fatalf("CaptureAndWrite error: %v", err)
	}

	msgs := pub.Messages()
	beforeEvt := decodeEvent(t, msgs, 0)
	before, _ := beforeEvt["before"].(map[string]any)
	data, _ := before["data"].(map[string]any)
	if rc, _ := data["row_count"].(float64); int(rc) != 0 {
		t.Errorf("Before snapshot row_count = %v; want 0 (snapshot must precede write)", data["row_count"])
	}
}

// -----------------------------------------------------------------------
// Snapshot phase labelling
// -----------------------------------------------------------------------

// TestSnapshotCapturer_SnapshotPhaseLabels verifies that Before snapshots carry
// phase="before" and After snapshots carry phase="after".
func TestSnapshotCapturer_SnapshotPhaseLabels(t *testing.T) {
	db := worker.NewInMemoryDatabase()
	sc, pub := newDatabaseCapturer(db)

	cfg := map[string]any{"table": "labels"}
	if err := sc.CaptureAndWrite(context.Background(), cfg, []map[string]any{{"id": 1}}, "exec-labels", "task-labels"); err != nil {
		t.Fatalf("CaptureAndWrite error: %v", err)
	}

	msgs := pub.Messages()
	beforeEvt := decodeEvent(t, msgs, 0)
	afterEvt := decodeEvent(t, msgs, 1)

	before, _ := beforeEvt["before"].(map[string]any)
	afterSnap, _ := afterEvt["after"].(map[string]any)

	if phase, _ := before["phase"].(string); phase != "before" {
		t.Errorf("Before snapshot phase = %q; want \"before\"", phase)
	}
	if phase, _ := afterSnap["phase"].(string); phase != "after" {
		t.Errorf("After snapshot phase = %q; want \"after\"", phase)
	}
}

// -----------------------------------------------------------------------
// CapturedAt field
// -----------------------------------------------------------------------

// TestSnapshotCapturer_SnapshotsCapturedAtIsSet verifies that both snapshots in
// the published events have a non-zero CapturedAt timestamp.
func TestSnapshotCapturer_SnapshotsCapturedAtIsSet(t *testing.T) {
	db := worker.NewInMemoryDatabase()
	sc, pub := newDatabaseCapturer(db)

	cfg := map[string]any{"table": "ts"}
	if err := sc.CaptureAndWrite(context.Background(), cfg, []map[string]any{{"id": 1}}, "exec-ts", "task-ts"); err != nil {
		t.Fatalf("CaptureAndWrite error: %v", err)
	}

	msgs := pub.Messages()
	beforeEvt := decodeEvent(t, msgs, 0)
	afterEvt := decodeEvent(t, msgs, 1)

	before, _ := beforeEvt["before"].(map[string]any)
	afterSnap, _ := afterEvt["after"].(map[string]any)

	parseCapturedAt := func(name string, snap map[string]any) {
		t.Helper()
		raw, ok := snap["capturedAt"]
		if !ok || raw == nil {
			t.Errorf("%s snapshot missing capturedAt field", name)
			return
		}
		s, ok := raw.(string)
		if !ok || s == "" {
			t.Errorf("%s snapshot capturedAt is not a non-empty string; got %v", name, raw)
			return
		}
		ts, err := time.Parse(time.RFC3339, s)
		if err != nil {
			t.Errorf("%s snapshot capturedAt %q is not RFC3339: %v", name, s, err)
			return
		}
		if ts.IsZero() {
			t.Errorf("%s snapshot capturedAt is zero", name)
		}
	}

	parseCapturedAt("before", before)
	parseCapturedAt("after", afterSnap)
}

// -----------------------------------------------------------------------
// Precondition guard — NewSnapshotCapturer
// -----------------------------------------------------------------------

// TestNewSnapshotCapturer_NilConnectorPanics verifies that NewSnapshotCapturer
// panics when connector is nil (fail-fast precondition).
func TestNewSnapshotCapturer_NilConnectorPanics(t *testing.T) {
	pub := &inMemoryPublisher{}
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for nil connector, got none")
		}
	}()
	_ = worker.NewSnapshotCapturer(nil, pub)
}

// TestNewSnapshotCapturer_NilPublisherPanics verifies that NewSnapshotCapturer
// panics when publisher is nil (fail-fast precondition).
func TestNewSnapshotCapturer_NilPublisherPanics(t *testing.T) {
	conn := worker.NewDatabaseSinkConnector(worker.NewInMemoryDedupStore())
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for nil publisher, got none")
		}
	}()
	_ = worker.NewSnapshotCapturer(conn, nil)
}
