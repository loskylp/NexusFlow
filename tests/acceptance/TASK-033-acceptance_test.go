// Package acceptance — TASK-033 acceptance tests: Sink Before/After snapshot capture.
//
// Requirement: DEMO-003, ADR-009, TASK-033
//
// Acceptance criteria:
//   AC-1: Before snapshot is captured and stored as JSON before Sink writes begin.
//   AC-2: After snapshot is captured after Sink completion or rollback.
//   AC-3: Snapshots are published to events:sink:{taskId} for SSE consumption.
//   AC-4: For database sinks: snapshot queries the target table within the Sink's output scope.
//   AC-5: For S3 sinks: snapshot lists objects in the target prefix.
//   AC-6: On rollback, After snapshot matches Before snapshot.
//
// Each AC has at least one positive case (criterion satisfied) and one negative case
// (a condition that would not satisfy the criterion, confirming the test is not trivially permissive).
//
// Run:
//
//	go test ./tests/acceptance/... -v -run TASK033
//
// See: DEMO-003, ADR-009, TASK-033
package acceptance

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/redis/go-redis/v9"

	"github.com/nxlabs/nexusflow/worker"
)

// -----------------------------------------------------------------------
// acceptancePublisher — Pub/Sub fake (local to this file)
// -----------------------------------------------------------------------

type acceptancePublisher struct {
	mu       sync.Mutex
	messages []accPubMessage
}

type accPubMessage struct {
	Channel string
	Payload string
}

func (p *acceptancePublisher) Publish(ctx context.Context, channel string, message any) *redis.IntCmd {
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
	p.messages = append(p.messages, accPubMessage{Channel: channel, Payload: payload})
	cmd := redis.NewIntCmd(ctx)
	cmd.SetVal(1)
	return cmd
}

func (p *acceptancePublisher) Messages() []accPubMessage {
	p.mu.Lock()
	defer p.mu.Unlock()
	cp := make([]accPubMessage, len(p.messages))
	copy(cp, p.messages)
	return cp
}

func decodeAccEvent(t *testing.T, msgs []accPubMessage, i int) map[string]any {
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

// newDBCapturer builds a SnapshotCapturer wrapping a DatabaseSinkConnector backed by db.
func newDBCapturer(db *worker.InMemoryDatabase) (*worker.SnapshotCapturer, *acceptancePublisher) {
	pub := &acceptancePublisher{}
	dedup := worker.NewInMemoryDedupStore()
	conn := worker.NewDatabaseSinkConnector(dedup)
	conn.UseDatabase(db)
	return worker.NewSnapshotCapturer(conn, pub), pub
}

// newS3Capturer builds a SnapshotCapturer wrapping an S3SinkConnector backed by s3.
func newS3Capturer(s3 *worker.InMemoryS3) (*worker.SnapshotCapturer, *acceptancePublisher) {
	pub := &acceptancePublisher{}
	dedup := worker.NewInMemoryDedupStore()
	conn := worker.NewS3SinkConnector(s3, dedup)
	return worker.NewSnapshotCapturer(conn, pub), pub
}

// -----------------------------------------------------------------------
// AC-1: Before snapshot is captured and stored as JSON before Sink writes begin
// -----------------------------------------------------------------------

// TestTASK033_AC1_BeforeSnapshotCapturedBeforeWrite — positive case.
// DEMO-003, TASK-033 / AC-1
// Given:  a DatabaseSinkConnector backed by a seeded InMemoryDatabase (1 existing row)
// When:   CaptureAndWrite is called with one new record
// Then:   the before-snapshot event carries a "before" object with row_count=1
//         (i.e. the pre-write count, not the post-write count)
func TestTASK033_AC1_BeforeSnapshotCapturedBeforeWrite(t *testing.T) {
	db := worker.NewInMemoryDatabase()
	db.Seed("customers", []map[string]any{{"id": 1}})
	sc, pub := newDBCapturer(db)

	cfg := map[string]any{"table": "customers"}
	if err := sc.CaptureAndWrite(context.Background(), cfg, []map[string]any{{"id": 2}}, "exec-ac1-pos", "task-ac1"); err != nil {
		t.Fatalf("CaptureAndWrite returned unexpected error: %v", err)
	}

	msgs := pub.Messages()
	beforeEvt := decodeAccEvent(t, msgs, 0)
	before, ok := beforeEvt["before"].(map[string]any)
	if !ok {
		t.Fatalf("before-snapshot event missing 'before' field; got: %v", beforeEvt)
	}
	data, ok := before["data"].(map[string]any)
	if !ok {
		t.Fatalf("before.data is not a map; got: %T", before["data"])
	}
	rc, _ := data["row_count"].(float64)
	if int(rc) != 1 {
		t.Errorf("AC-1 FAIL: Before snapshot row_count = %v; want 1 (pre-write state)", data["row_count"])
	}
}

// TestTASK033_AC1_NegativeCase_AfterSnapshotIsNotBefore — negative case.
// Confirms the Before snapshot does NOT reflect the post-write row count.
// If Before and After had the same row_count on a successful write, AC-1 would be violated.
//
// DEMO-003, TASK-033 / AC-1
// Given:  an empty database
// When:   CaptureAndWrite inserts 2 new records successfully
// Then:   Before snapshot row_count == 0 (pre-write)
//         After snapshot row_count == 2 (post-write)
//         Before row_count != After row_count
func TestTASK033_AC1_NegativeCase_AfterSnapshotIsNotBefore(t *testing.T) {
	db := worker.NewInMemoryDatabase()
	sc, pub := newDBCapturer(db)

	cfg := map[string]any{"table": "items"}
	records := []map[string]any{{"id": 1}, {"id": 2}}
	if err := sc.CaptureAndWrite(context.Background(), cfg, records, "exec-ac1-neg", "task-ac1-neg"); err != nil {
		t.Fatalf("CaptureAndWrite error: %v", err)
	}

	msgs := pub.Messages()
	beforeEvt := decodeAccEvent(t, msgs, 0)
	afterEvt := decodeAccEvent(t, msgs, 1)

	before, _ := beforeEvt["before"].(map[string]any)
	after, _ := afterEvt["after"].(map[string]any)
	beforeData, _ := before["data"].(map[string]any)
	afterData, _ := after["data"].(map[string]any)

	beforeRC, _ := beforeData["row_count"].(float64)
	afterRC, _ := afterData["row_count"].(float64)

	if beforeRC == afterRC {
		t.Errorf("AC-1 negative case FAIL: Before row_count (%v) must differ from After row_count (%v) after a successful insert", beforeRC, afterRC)
	}
	if int(beforeRC) != 0 {
		t.Errorf("AC-1 negative case FAIL: Before row_count = %v; want 0 (pre-write)", beforeRC)
	}
}

// -----------------------------------------------------------------------
// AC-2: After snapshot is captured after Sink completion or rollback
// -----------------------------------------------------------------------

// TestTASK033_AC2_AfterSnapshotCapturedOnSuccess — positive case (completion).
// DEMO-003, TASK-033 / AC-2
// Given:  an InMemoryDatabase with 2 seeded rows
// When:   CaptureAndWrite writes 1 new record successfully
// Then:   the after-result event carries an "after" object with row_count=3
func TestTASK033_AC2_AfterSnapshotCapturedOnSuccess(t *testing.T) {
	db := worker.NewInMemoryDatabase()
	db.Seed("logs", []map[string]any{{"id": 1}, {"id": 2}})
	sc, pub := newDBCapturer(db)

	cfg := map[string]any{"table": "logs"}
	if err := sc.CaptureAndWrite(context.Background(), cfg, []map[string]any{{"id": 3}}, "exec-ac2-pos", "task-ac2-pos"); err != nil {
		t.Fatalf("CaptureAndWrite error: %v", err)
	}

	msgs := pub.Messages()
	afterEvt := decodeAccEvent(t, msgs, 1)
	after, ok := afterEvt["after"].(map[string]any)
	if !ok {
		t.Fatalf("after-result event missing 'after' field")
	}
	data, _ := after["data"].(map[string]any)
	rc, _ := data["row_count"].(float64)
	if int(rc) != 3 {
		t.Errorf("AC-2 FAIL: After snapshot row_count = %v; want 3 (post-write)", data["row_count"])
	}
}

// TestTASK033_AC2_AfterSnapshotCapturedOnRollback — positive case (rollback).
// DEMO-003, TASK-033 / AC-2
// Given:  a DatabaseSinkConnector configured to fail on first insert
// When:   CaptureAndWrite is called
// Then:   after-result event is published even though Write failed
//         AND after-result.rolledBack == true
func TestTASK033_AC2_AfterSnapshotCapturedOnRollback(t *testing.T) {
	db := worker.NewInMemoryDatabase()
	db.FailAfterRow(0)
	sc, pub := newDBCapturer(db)

	cfg := map[string]any{"table": "events"}
	err := sc.CaptureAndWrite(context.Background(), cfg, []map[string]any{{"id": 1}}, "exec-ac2-rb", "task-ac2-rb")
	if err == nil {
		t.Fatal("expected error from CaptureAndWrite on forced rollback")
	}

	msgs := pub.Messages()
	if len(msgs) != 2 {
		t.Fatalf("AC-2 FAIL: expected 2 published messages on rollback; got %d", len(msgs))
	}
	afterEvt := decodeAccEvent(t, msgs, 1)
	if afterEvt["after"] == nil {
		t.Error("AC-2 FAIL: after-result event must carry 'after' snapshot even on rollback")
	}
	if rb, _ := afterEvt["rolledBack"].(bool); !rb {
		t.Error("AC-2 FAIL: after-result.rolledBack must be true on rollback")
	}
}

// TestTASK033_AC2_NegativeCase_NoAfterEventWithoutCapture — negative case.
// Verifies the test would catch an implementation that skips after-snapshot on success.
// If no after-result event is published, the AC-2 test above catches it.
// This test demonstrates the anti-pattern explicitly by verifying that if
// CaptureAndWrite runs, the after event must be present.
//
// [VERIFIER-ADDED]
// Given:  a normal successful CaptureAndWrite run
// When:   only 1 message is published (simulating a missing after event)
// Then:   the AC-2 check catches the deficit
func TestTASK033_AC2_NegativeCase_ExactlyTwoEventsRequired(t *testing.T) {
	db := worker.NewInMemoryDatabase()
	sc, pub := newDBCapturer(db)

	cfg := map[string]any{"table": "neg_table"}
	if err := sc.CaptureAndWrite(context.Background(), cfg, []map[string]any{{"id": 1}}, "exec-ac2-neg", "task-ac2-neg"); err != nil {
		t.Fatalf("CaptureAndWrite error: %v", err)
	}

	// Confirm the implementation publishes exactly 2 events (before + after).
	msgs := pub.Messages()
	if len(msgs) != 2 {
		t.Errorf("AC-2 negative-case FAIL: expected exactly 2 events; got %d — after event is missing or extra events produced", len(msgs))
	}
}

// -----------------------------------------------------------------------
// AC-3: Snapshots published to events:sink:{taskId} for SSE consumption
// -----------------------------------------------------------------------

// TestTASK033_AC3_SnapshotsPublishedToCorrectChannel — positive case.
// DEMO-003, ADR-007, TASK-033 / AC-3
// Given:  a SnapshotCapturer running for task "acc-task-sse-001"
// When:   CaptureAndWrite completes
// Then:   all published messages are on channel "events:sink:acc-task-sse-001"
func TestTASK033_AC3_SnapshotsPublishedToCorrectChannel(t *testing.T) {
	db := worker.NewInMemoryDatabase()
	sc, pub := newDBCapturer(db)

	const taskID = "acc-task-sse-001"
	expectedChannel := "events:sink:" + taskID

	cfg := map[string]any{"table": "sse_table"}
	if err := sc.CaptureAndWrite(context.Background(), cfg, []map[string]any{{"id": 1}}, "exec-ac3", taskID); err != nil {
		t.Fatalf("CaptureAndWrite error: %v", err)
	}

	msgs := pub.Messages()
	if len(msgs) == 0 {
		t.Fatal("AC-3 FAIL: no messages published")
	}
	for i, msg := range msgs {
		if msg.Channel != expectedChannel {
			t.Errorf("AC-3 FAIL: message[%d] channel = %q; want %q", i, msg.Channel, expectedChannel)
		}
	}
}

// TestTASK033_AC3_NegativeCase_WrongChannelWouldFail — negative case.
// Confirms that if messages were published to a different channel, the test would catch it.
// Verifies the channel name is dynamically scoped to taskID (not a static channel).
//
// DEMO-003, TASK-033 / AC-3
// Given:  two runs with different taskIDs
// When:   each completes
// Then:   the channels are distinct (events:sink:task-A != events:sink:task-B)
func TestTASK033_AC3_NegativeCase_ChannelIsScopedToTaskID(t *testing.T) {
	db := worker.NewInMemoryDatabase()
	pubA := &acceptancePublisher{}
	pubB := &acceptancePublisher{}

	dedupA := worker.NewInMemoryDedupStore()
	connA := worker.NewDatabaseSinkConnector(dedupA)
	connA.UseDatabase(db)
	scA := worker.NewSnapshotCapturer(connA, pubA)

	dedupB := worker.NewInMemoryDedupStore()
	connB := worker.NewDatabaseSinkConnector(dedupB)
	connB.UseDatabase(db)
	scB := worker.NewSnapshotCapturer(connB, pubB)

	cfg := map[string]any{"table": "scope_test"}
	_ = scA.CaptureAndWrite(context.Background(), cfg, []map[string]any{{"id": 1}}, "exec-scope-A", "task-scope-A")
	_ = scB.CaptureAndWrite(context.Background(), cfg, []map[string]any{{"id": 2}}, "exec-scope-B", "task-scope-B")

	msgsA := pubA.Messages()
	msgsB := pubB.Messages()

	if len(msgsA) == 0 || len(msgsB) == 0 {
		t.Fatalf("expected messages from both captures")
	}

	if msgsA[0].Channel == msgsB[0].Channel {
		t.Errorf("AC-3 negative-case FAIL: different tasks produced same channel %q — channel must be task-scoped", msgsA[0].Channel)
	}
}

// -----------------------------------------------------------------------
// AC-4: Database sinks snapshot the target table (row_count)
// -----------------------------------------------------------------------

// TestTASK033_AC4_DatabaseSnapshot_RowCountReflectsTargetTable — positive case.
// DEMO-003, ADR-009, TASK-033 / AC-4
// Given:  a DatabaseSinkConnector backed by a table "report_lines" with 5 seeded rows
// When:   CaptureAndWrite is called
// Then:   before-snapshot.data.row_count == 5
//         after-result.data.row_count == 6 (one new record written)
func TestTASK033_AC4_DatabaseSnapshot_RowCountReflectsTargetTable(t *testing.T) {
	db := worker.NewInMemoryDatabase()
	db.Seed("report_lines", []map[string]any{
		{"id": 1}, {"id": 2}, {"id": 3}, {"id": 4}, {"id": 5},
	})
	sc, pub := newDBCapturer(db)

	cfg := map[string]any{"table": "report_lines"}
	if err := sc.CaptureAndWrite(context.Background(), cfg, []map[string]any{{"id": 6}}, "exec-ac4", "task-ac4"); err != nil {
		t.Fatalf("CaptureAndWrite error: %v", err)
	}

	msgs := pub.Messages()
	beforeEvt := decodeAccEvent(t, msgs, 0)
	afterEvt := decodeAccEvent(t, msgs, 1)

	before, _ := beforeEvt["before"].(map[string]any)
	after, _ := afterEvt["after"].(map[string]any)
	beforeData, _ := before["data"].(map[string]any)
	afterData, _ := after["data"].(map[string]any)

	beforeRC, _ := beforeData["row_count"].(float64)
	afterRC, _ := afterData["row_count"].(float64)

	if int(beforeRC) != 5 {
		t.Errorf("AC-4 FAIL: before row_count = %v; want 5", beforeRC)
	}
	if int(afterRC) != 6 {
		t.Errorf("AC-4 FAIL: after row_count = %v; want 6", afterRC)
	}
}

// TestTASK033_AC4_NegativeCase_DatabaseSnapshotContainsRowCountKey — negative case.
// Verifies the snapshot data map contains the "row_count" key specifically (not
// some other shape). An implementation that returns an empty map would fail this check.
//
// DEMO-003, TASK-033 / AC-4
// Given:  a DatabaseSinkConnector
// When:   CaptureAndWrite runs
// Then:   before.data contains the key "row_count"
func TestTASK033_AC4_NegativeCase_DatabaseSnapshotContainsRowCountKey(t *testing.T) {
	db := worker.NewInMemoryDatabase()
	sc, pub := newDBCapturer(db)

	cfg := map[string]any{"table": "key_check"}
	if err := sc.CaptureAndWrite(context.Background(), cfg, []map[string]any{{"id": 1}}, "exec-ac4-neg", "task-ac4-neg"); err != nil {
		t.Fatalf("CaptureAndWrite error: %v", err)
	}

	msgs := pub.Messages()
	beforeEvt := decodeAccEvent(t, msgs, 0)
	before, _ := beforeEvt["before"].(map[string]any)
	data, _ := before["data"].(map[string]any)
	if _, ok := data["row_count"]; !ok {
		t.Error("AC-4 negative-case FAIL: before.data must contain 'row_count' key for database sinks")
	}
}

// -----------------------------------------------------------------------
// AC-5: S3 sinks snapshot objects in the target prefix (object_count)
// -----------------------------------------------------------------------

// TestTASK033_AC5_S3Snapshot_ObjectCountReflectsTargetPrefix — positive case.
// DEMO-003, ADR-009, TASK-033 / AC-5
// Given:  an S3SinkConnector backed by an InMemoryS3 with 3 objects in the target prefix
// When:   CaptureAndWrite writes a new object to the same prefix
// Then:   before-snapshot.data.object_count == 3
//         after-result.data.object_count == 4
func TestTASK033_AC5_S3Snapshot_ObjectCountReflectsTargetPrefix(t *testing.T) {
	s3 := worker.NewInMemoryS3()
	s3.Put("data-bucket", "exports/2026/a.json", []byte(`{}`))
	s3.Put("data-bucket", "exports/2026/b.json", []byte(`{}`))
	s3.Put("data-bucket", "exports/2026/c.json", []byte(`{}`))
	sc, pub := newS3Capturer(s3)

	cfg := map[string]any{"bucket": "data-bucket", "key": "exports/2026/d.json"}
	if err := sc.CaptureAndWrite(context.Background(), cfg, []map[string]any{{"id": 1}}, "exec-ac5", "task-ac5"); err != nil {
		t.Fatalf("CaptureAndWrite error: %v", err)
	}

	msgs := pub.Messages()
	beforeEvt := decodeAccEvent(t, msgs, 0)
	afterEvt := decodeAccEvent(t, msgs, 1)

	before, _ := beforeEvt["before"].(map[string]any)
	after, _ := afterEvt["after"].(map[string]any)
	beforeData, _ := before["data"].(map[string]any)
	afterData, _ := after["data"].(map[string]any)

	beforeOC, _ := beforeData["object_count"].(float64)
	afterOC, _ := afterData["object_count"].(float64)

	if int(beforeOC) != 3 {
		t.Errorf("AC-5 FAIL: before object_count = %v; want 3", beforeOC)
	}
	if int(afterOC) != 4 {
		t.Errorf("AC-5 FAIL: after object_count = %v; want 4", afterOC)
	}
}

// TestTASK033_AC5_NegativeCase_S3SnapshotContainsObjectCountKey — negative case.
// Verifies S3 snapshot data uses "object_count" (not "row_count" or some other key).
//
// DEMO-003, TASK-033 / AC-5
// Given:  an S3SinkConnector
// When:   CaptureAndWrite runs
// Then:   before.data contains "object_count" key (not absent, not "row_count")
func TestTASK033_AC5_NegativeCase_S3SnapshotContainsObjectCountKey(t *testing.T) {
	s3 := worker.NewInMemoryS3()
	sc, pub := newS3Capturer(s3)

	cfg := map[string]any{"bucket": "test-bkt", "key": "dir/out.json"}
	if err := sc.CaptureAndWrite(context.Background(), cfg, []map[string]any{{"id": 1}}, "exec-ac5-neg", "task-ac5-neg"); err != nil {
		t.Fatalf("CaptureAndWrite error: %v", err)
	}

	msgs := pub.Messages()
	beforeEvt := decodeAccEvent(t, msgs, 0)
	before, _ := beforeEvt["before"].(map[string]any)
	data, _ := before["data"].(map[string]any)
	if _, ok := data["object_count"]; !ok {
		t.Error("AC-5 negative-case FAIL: S3 before.data must contain 'object_count' key")
	}
	if _, ok := data["row_count"]; ok {
		t.Error("AC-5 negative-case FAIL: S3 before.data must not contain 'row_count' — wrong connector type")
	}
}

// -----------------------------------------------------------------------
// AC-6: On rollback, After snapshot matches Before snapshot
// -----------------------------------------------------------------------

// TestTASK033_AC6_DatabaseRollback_AfterMatchesBefore — positive case (database).
// DEMO-003, ADR-009, TASK-033 / AC-6
// Given:  a DatabaseSinkConnector with 2 seeded rows, configured to fail immediately
// When:   CaptureAndWrite is called with 2 new records
// Then:   Write returns an error (rollback occurred)
//         AND after.data.row_count == before.data.row_count == 2
//         AND after-result.rolledBack == true
func TestTASK033_AC6_DatabaseRollback_AfterMatchesBefore(t *testing.T) {
	db := worker.NewInMemoryDatabase()
	db.Seed("transactions", []map[string]any{{"id": 1}, {"id": 2}})
	db.FailAfterRow(0) // fail immediately
	sc, pub := newDBCapturer(db)

	cfg := map[string]any{"table": "transactions"}
	err := sc.CaptureAndWrite(context.Background(), cfg, []map[string]any{{"id": 3}, {"id": 4}}, "exec-ac6-db", "task-ac6-db")
	if err == nil {
		t.Fatal("AC-6 FAIL: expected CaptureAndWrite to return error on forced rollback")
	}

	msgs := pub.Messages()
	if len(msgs) != 2 {
		t.Fatalf("AC-6 FAIL: expected 2 published messages; got %d", len(msgs))
	}

	beforeEvt := decodeAccEvent(t, msgs, 0)
	afterEvt := decodeAccEvent(t, msgs, 1)

	before, _ := beforeEvt["before"].(map[string]any)
	after, _ := afterEvt["after"].(map[string]any)
	beforeData, _ := before["data"].(map[string]any)
	afterData, _ := after["data"].(map[string]any)

	beforeRC, _ := beforeData["row_count"].(float64)
	afterRC, _ := afterData["row_count"].(float64)

	if beforeRC != afterRC {
		t.Errorf("AC-6 FAIL: after row_count (%v) must equal before row_count (%v) after rollback", afterRC, beforeRC)
	}
	if int(beforeRC) != 2 {
		t.Errorf("AC-6 FAIL: before row_count = %v; want 2 (original seed count)", beforeRC)
	}
	if rb, _ := afterEvt["rolledBack"].(bool); !rb {
		t.Error("AC-6 FAIL: after-result.rolledBack must be true on rollback")
	}
}

// TestTASK033_AC6_S3Rollback_AfterMatchesBefore — positive case (S3).
// DEMO-003, ADR-009, TASK-033 / AC-6
// Given:  an InMemoryS3 with 1 existing object, configured to fail on first upload part
// When:   CaptureAndWrite is called
// Then:   Write returns an error (multipart aborted)
//         AND after.data.object_count == before.data.object_count == 1
//         AND after-result.rolledBack == true
func TestTASK033_AC6_S3Rollback_AfterMatchesBefore(t *testing.T) {
	s3 := worker.NewInMemoryS3()
	s3.Put("rollback-bucket", "archive/existing.json", []byte(`{}`))
	s3.FailUploadAfterPart(0) // abort immediately
	sc, pub := newS3Capturer(s3)

	cfg := map[string]any{"bucket": "rollback-bucket", "key": "archive/new.json"}
	err := sc.CaptureAndWrite(context.Background(), cfg, []map[string]any{{"id": 1}}, "exec-ac6-s3", "task-ac6-s3")
	if err == nil {
		t.Fatal("AC-6 FAIL: expected CaptureAndWrite to return error on S3 upload abort")
	}

	msgs := pub.Messages()
	if len(msgs) != 2 {
		t.Fatalf("AC-6 FAIL: expected 2 published messages on S3 abort; got %d", len(msgs))
	}

	beforeEvt := decodeAccEvent(t, msgs, 0)
	afterEvt := decodeAccEvent(t, msgs, 1)

	before, _ := beforeEvt["before"].(map[string]any)
	after, _ := afterEvt["after"].(map[string]any)
	beforeData, _ := before["data"].(map[string]any)
	afterData, _ := after["data"].(map[string]any)

	beforeOC, _ := beforeData["object_count"].(float64)
	afterOC, _ := afterData["object_count"].(float64)

	if beforeOC != afterOC {
		t.Errorf("AC-6 FAIL: after object_count (%v) must equal before object_count (%v) after S3 abort", afterOC, beforeOC)
	}
	if rb, _ := afterEvt["rolledBack"].(bool); !rb {
		t.Error("AC-6 FAIL: after-result.rolledBack must be true on S3 abort")
	}
}

// TestTASK033_AC6_NegativeCase_SuccessfulWriteAfterDiffersFromBefore — negative case.
// Confirms the AC-6 equality check is specific to rollback — on success the after snapshot
// differs from before. An implementation that always returns the same before/after would
// not satisfy AC-6 (which requires equality only on rollback).
//
// DEMO-003, TASK-033 / AC-6
// Given:  a normal successful write with 1 seeded row + 1 new record
// When:   CaptureAndWrite completes without error
// Then:   after.data.row_count > before.data.row_count (snapshots differ on success)
func TestTASK033_AC6_NegativeCase_SuccessfulWriteAfterDiffersFromBefore(t *testing.T) {
	db := worker.NewInMemoryDatabase()
	db.Seed("delta_check", []map[string]any{{"id": 1}})
	sc, pub := newDBCapturer(db)

	cfg := map[string]any{"table": "delta_check"}
	if err := sc.CaptureAndWrite(context.Background(), cfg, []map[string]any{{"id": 2}}, "exec-ac6-neg", "task-ac6-neg"); err != nil {
		t.Fatalf("CaptureAndWrite error: %v", err)
	}

	msgs := pub.Messages()
	beforeEvt := decodeAccEvent(t, msgs, 0)
	afterEvt := decodeAccEvent(t, msgs, 1)

	before, _ := beforeEvt["before"].(map[string]any)
	after, _ := afterEvt["after"].(map[string]any)
	beforeData, _ := before["data"].(map[string]any)
	afterData, _ := after["data"].(map[string]any)

	beforeRC, _ := beforeData["row_count"].(float64)
	afterRC, _ := afterData["row_count"].(float64)

	if beforeRC == afterRC {
		t.Errorf("AC-6 negative-case FAIL: on successful write, after row_count (%v) must differ from before (%v)", afterRC, beforeRC)
	}
}
