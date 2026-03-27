// Package worker_test — unit tests for TASK-042: demo connectors.
// Tests verify the DemoDataSource, DemoProcessConnector, and DemoSinkConnector
// implementations and their integration through the full pipeline execution path.
// All tests use in-memory fakes; no live Redis or PostgreSQL instance is required.
// See: TASK-042, ADR-003, ADR-009
package worker_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nxlabs/nexusflow/internal/config"
	"github.com/nxlabs/nexusflow/internal/models"
	"github.com/nxlabs/nexusflow/internal/queue"
	"github.com/nxlabs/nexusflow/worker"
)

// --- DemoDataSource tests ---

// TestDemoDataSource_Type_ReturnsDemoString verifies the connector type name used
// for registry lookup matches the string "demo".
func TestDemoDataSource_Type_ReturnsDemoString(t *testing.T) {
	src := &worker.DemoDataSource{}
	if got := src.Type(); got != "demo" {
		t.Errorf("DemoDataSource.Type() = %q; want %q", got, "demo")
	}
}

// TestDemoDataSource_Fetch_ReturnsDefaultRecords verifies that Fetch returns a
// non-empty slice of records when called with empty config and input maps.
func TestDemoDataSource_Fetch_ReturnsDefaultRecords(t *testing.T) {
	src := &worker.DemoDataSource{}
	records, err := src.Fetch(context.Background(), map[string]any{}, map[string]any{})
	if err != nil {
		t.Fatalf("DemoDataSource.Fetch: unexpected error: %v", err)
	}
	if len(records) == 0 {
		t.Fatal("DemoDataSource.Fetch: expected at least one record, got empty slice")
	}
}

// TestDemoDataSource_Fetch_IsDeterministic verifies that two calls with the same
// config produce identical records (determinism requirement, AC-1).
func TestDemoDataSource_Fetch_IsDeterministic(t *testing.T) {
	src := &worker.DemoDataSource{}
	cfg := map[string]any{}
	input := map[string]any{}

	first, err := src.Fetch(context.Background(), cfg, input)
	if err != nil {
		t.Fatalf("first Fetch error: %v", err)
	}
	second, err := src.Fetch(context.Background(), cfg, input)
	if err != nil {
		t.Fatalf("second Fetch error: %v", err)
	}

	if len(first) != len(second) {
		t.Fatalf("determinism: got %d records on first call, %d on second", len(first), len(second))
	}
	for i, r := range first {
		for k, v := range r {
			if second[i][k] != v {
				t.Errorf("record[%d][%q] not deterministic: first=%v second=%v", i, k, v, second[i][k])
			}
		}
	}
}

// TestDemoDataSource_Fetch_CountConfig verifies the "count" config key controls
// the number of records returned.
func TestDemoDataSource_Fetch_CountConfig(t *testing.T) {
	src := &worker.DemoDataSource{}
	cfg := map[string]any{"count": float64(7)}
	records, err := src.Fetch(context.Background(), cfg, map[string]any{})
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	if len(records) != 7 {
		t.Errorf("expected 7 records, got %d", len(records))
	}
}

// TestDemoDataSource_Fetch_RecordsHaveRequiredFields verifies that each record
// contains the fields expected by the demo pipeline schema ("id", "name", "value").
func TestDemoDataSource_Fetch_RecordsHaveRequiredFields(t *testing.T) {
	src := &worker.DemoDataSource{}
	records, err := src.Fetch(context.Background(), map[string]any{}, map[string]any{})
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	for i, r := range records {
		for _, field := range []string{"id", "name", "value"} {
			if _, ok := r[field]; !ok {
				t.Errorf("record[%d] missing field %q", i, field)
			}
		}
	}
}

// --- DemoProcessConnector tests ---

// TestDemoProcessConnector_Type_ReturnsDemoString verifies the connector type name.
func TestDemoProcessConnector_Type_ReturnsDemoString(t *testing.T) {
	proc := &worker.DemoProcessConnector{}
	if got := proc.Type(); got != "demo" {
		t.Errorf("DemoProcessConnector.Type() = %q; want %q", got, "demo")
	}
}

// TestDemoProcessConnector_Transform_PassesThroughUnknownFields verifies that
// records without a configured uppercase field are returned unchanged.
func TestDemoProcessConnector_Transform_PassesThroughUnknownFields(t *testing.T) {
	proc := &worker.DemoProcessConnector{}
	in := []map[string]any{{"x": "hello"}}
	out, err := proc.Transform(context.Background(), map[string]any{}, in)
	if err != nil {
		t.Fatalf("Transform error: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 record, got %d", len(out))
	}
	if out[0]["x"] != "hello" {
		t.Errorf("expected x=%q unchanged, got %v", "hello", out[0]["x"])
	}
}

// TestDemoProcessConnector_Transform_UppercasesConfiguredField verifies that when
// "uppercase_field" is set in config, string values in that field are uppercased.
func TestDemoProcessConnector_Transform_UppercasesConfiguredField(t *testing.T) {
	proc := &worker.DemoProcessConnector{}
	cfg := map[string]any{"uppercase_field": "name"}
	in := []map[string]any{
		{"name": "alice", "value": 1},
		{"name": "bob", "value": 2},
	}
	out, err := proc.Transform(context.Background(), cfg, in)
	if err != nil {
		t.Fatalf("Transform error: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 records, got %d", len(out))
	}
	if out[0]["name"] != "ALICE" {
		t.Errorf("expected name=ALICE, got %v", out[0]["name"])
	}
	if out[1]["name"] != "BOB" {
		t.Errorf("expected name=BOB, got %v", out[1]["name"])
	}
}

// TestDemoProcessConnector_Transform_AddsProcessedFlag verifies that a "processed"
// boolean field is added to each output record.
func TestDemoProcessConnector_Transform_AddsProcessedFlag(t *testing.T) {
	proc := &worker.DemoProcessConnector{}
	in := []map[string]any{{"name": "alice"}}
	out, err := proc.Transform(context.Background(), map[string]any{}, in)
	if err != nil {
		t.Fatalf("Transform error: %v", err)
	}
	if processed, ok := out[0]["processed"]; !ok || processed != true {
		t.Errorf("expected processed=true, got %v (ok=%v)", processed, ok)
	}
}

// TestDemoProcessConnector_Transform_EmptyInput verifies that an empty record slice
// produces an empty output slice without error.
func TestDemoProcessConnector_Transform_EmptyInput(t *testing.T) {
	proc := &worker.DemoProcessConnector{}
	out, err := proc.Transform(context.Background(), map[string]any{}, []map[string]any{})
	if err != nil {
		t.Fatalf("Transform error: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("expected empty output, got %d records", len(out))
	}
}

// TestDemoProcessConnector_Transform_DoesNotMutateInput verifies that Transform
// returns new records without mutating the input slice entries.
func TestDemoProcessConnector_Transform_DoesNotMutateInput(t *testing.T) {
	proc := &worker.DemoProcessConnector{}
	cfg := map[string]any{"uppercase_field": "name"}
	in := []map[string]any{{"name": "alice"}}
	originalName := in[0]["name"]

	_, err := proc.Transform(context.Background(), cfg, in)
	if err != nil {
		t.Fatalf("Transform error: %v", err)
	}
	if in[0]["name"] != originalName {
		t.Errorf("input mutated: name was %q, now %v", originalName, in[0]["name"])
	}
}

// --- DemoSinkConnector tests ---

// TestDemoSinkConnector_Type_ReturnsDemoString verifies the connector type name.
func TestDemoSinkConnector_Type_ReturnsDemoString(t *testing.T) {
	sink := worker.NewDemoSinkConnector()
	if got := sink.Type(); got != "demo" {
		t.Errorf("DemoSinkConnector.Type() = %q; want %q", got, "demo")
	}
}

// TestDemoSinkConnector_Write_StoresRecords verifies that Write commits records
// to the in-memory store and returns nil on success.
func TestDemoSinkConnector_Write_StoresRecords(t *testing.T) {
	sink := worker.NewDemoSinkConnector()
	records := []map[string]any{{"id": 1, "name": "alice"}}
	err := sink.Write(context.Background(), map[string]any{}, records, "exec-001")
	if err != nil {
		t.Fatalf("Write error: %v", err)
	}
}

// TestDemoSinkConnector_Write_IdempotentOnDuplicateExecutionID verifies that a
// second Write with the same executionID returns ErrAlreadyApplied (ADR-003).
func TestDemoSinkConnector_Write_IdempotentOnDuplicateExecutionID(t *testing.T) {
	sink := worker.NewDemoSinkConnector()
	records := []map[string]any{{"id": 1}}
	execID := "exec-idempotent-001"

	// First write succeeds.
	if err := sink.Write(context.Background(), map[string]any{}, records, execID); err != nil {
		t.Fatalf("first Write error: %v", err)
	}
	// Second write with same executionID must return ErrAlreadyApplied.
	if err := sink.Write(context.Background(), map[string]any{}, records, execID); err != worker.ErrAlreadyApplied {
		t.Errorf("second Write: expected ErrAlreadyApplied, got %v", err)
	}
}

// TestDemoSinkConnector_Snapshot_ReturnsEmptyBeforeFirstWrite verifies that
// Snapshot returns a non-nil map with a "record_count" key of 0 before any write.
func TestDemoSinkConnector_Snapshot_ReturnsEmptyBeforeFirstWrite(t *testing.T) {
	sink := worker.NewDemoSinkConnector()
	snap, err := sink.Snapshot(context.Background(), map[string]any{}, "task-001")
	if err != nil {
		t.Fatalf("Snapshot error: %v", err)
	}
	if snap == nil {
		t.Fatal("Snapshot returned nil map")
	}
	count, ok := snap["record_count"]
	if !ok {
		t.Fatal("Snapshot missing 'record_count' key")
	}
	if count != 0 {
		t.Errorf("expected record_count=0 before any write, got %v", count)
	}
}

// TestDemoSinkConnector_Snapshot_ReflectsCommittedWrites verifies that Snapshot
// reflects the total count of records committed across all executionIDs.
func TestDemoSinkConnector_Snapshot_ReflectsCommittedWrites(t *testing.T) {
	sink := worker.NewDemoSinkConnector()
	r1 := []map[string]any{{"id": 1}, {"id": 2}}
	r2 := []map[string]any{{"id": 3}}

	if err := sink.Write(context.Background(), map[string]any{}, r1, "exec-a"); err != nil {
		t.Fatalf("first Write error: %v", err)
	}
	if err := sink.Write(context.Background(), map[string]any{}, r2, "exec-b"); err != nil {
		t.Fatalf("second Write error: %v", err)
	}

	snap, err := sink.Snapshot(context.Background(), map[string]any{}, "task-001")
	if err != nil {
		t.Fatalf("Snapshot error: %v", err)
	}
	if snap["record_count"] != 3 {
		t.Errorf("expected record_count=3, got %v", snap["record_count"])
	}
}

// --- Registry integration test ---

// TestDemoConnectors_RegisteredInDefaultRegistry verifies that after registering
// the three demo connectors, the registry resolves them by type "demo".
func TestDemoConnectors_RegisteredInDefaultRegistry(t *testing.T) {
	reg := worker.NewDefaultConnectorRegistry()
	worker.RegisterDemoConnectors(reg)

	if _, err := reg.DataSource("demo"); err != nil {
		t.Errorf("registry.DataSource(%q) error: %v", "demo", err)
	}
	if _, err := reg.Process("demo"); err != nil {
		t.Errorf("registry.Process(%q) error: %v", "demo", err)
	}
	if _, err := reg.Sink("demo"); err != nil {
		t.Errorf("registry.Sink(%q) error: %v", "demo", err)
	}
}

// --- End-to-end: demo connectors through full pipeline execution ---

// TestDemoConnectors_EndToEnd_TaskCompletes verifies that a task referencing demo
// connectors in all three phases executes to completion (AC-4 end-to-end criterion).
// This test uses the Worker's executeTask path with in-memory fakes for DB and queue.
func TestDemoConnectors_EndToEnd_TaskCompletes(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	pipelineID := uuid.New()
	taskID := uuid.New()
	userID := uuid.New()

	pipeline := &models.Pipeline{
		ID:     pipelineID,
		Name:   "demo-e2e",
		UserID: userID,
		DataSourceConfig: models.DataSourceConfig{
			ConnectorType: "demo",
			Config:        map[string]any{"count": float64(3)},
		},
		ProcessConfig: models.ProcessConfig{
			ConnectorType: "demo",
			Config:        map[string]any{"uppercase_field": "name"},
		},
		SinkConfig: models.SinkConfig{
			ConnectorType: "demo",
			Config:        map[string]any{},
		},
	}

	task := &models.Task{
		ID:          taskID,
		PipelineID:  &pipelineID,
		UserID:      userID,
		Status:      models.TaskStatusQueued,
		RetryConfig: models.DefaultRetryConfig(),
		ExecutionID: fmt.Sprintf("%s:1", taskID),
		Input:       map[string]any{},
	}

	taskRepo := newFakeTaskRepo(task)
	pipelineRepo := newFakePipelineRepo(pipeline)

	reg := worker.NewDefaultConnectorRegistry()
	worker.RegisterDemoConnectors(reg)

	cfg := &config.Config{
		WorkerID:   "test-worker",
		WorkerTags: []string{"demo"},
	}
	w := worker.NewWorkerWithPipelines(cfg, taskRepo, nil, pipelineRepo, nil, nil, nil, reg)

	msg := &queue.TaskMessage{
		TaskID:      taskID.String(),
		PipelineID:  pipelineID.String(),
		StreamID:    "1-0",
		ExecutionID: task.ExecutionID,
	}

	// Access executeTask via the exported test helper.
	w.ExecuteTaskForTest(ctx, msg)

	transitions := taskRepo.getTransitions(taskID)
	if len(transitions) < 3 {
		t.Fatalf("expected at least 3 status transitions, got %d: %v", len(transitions), transitions)
	}
	last := transitions[len(transitions)-1]
	if last.newStatus != models.TaskStatusCompleted {
		t.Errorf("expected final status=completed, got %s", last.newStatus)
	}
}

// TestDemoDataSource_Fetch_NameFieldsContainRecordIndex verifies that each record's
// "name" field encodes the record index, giving consistent test-observable values.
func TestDemoDataSource_Fetch_NameFieldsContainRecordIndex(t *testing.T) {
	src := &worker.DemoDataSource{}
	records, err := src.Fetch(context.Background(), map[string]any{}, map[string]any{})
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	// name field should contain the index (0-based) as part of its value.
	for i, r := range records {
		name, ok := r["name"].(string)
		if !ok {
			t.Errorf("record[%d] name is not a string: %T", i, r["name"])
			continue
		}
		idx := fmt.Sprintf("%d", i)
		if !strings.Contains(name, idx) {
			t.Errorf("record[%d] name=%q does not contain index %q", i, name, idx)
		}
	}
}
