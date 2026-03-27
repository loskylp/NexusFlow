// Package worker — Connector interfaces and registry.
// A Connector is one of three types: DataSource, Process, or Sink.
// Each Pipeline phase names a connector by ConnectorType; the registry resolves the name
// to its implementation at execution time.
// See: ADR-003, ADR-009, TASK-007, TASK-042
package worker

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/nxlabs/nexusflow/internal/models"
)

// DataSourceConnector ingests data from an external origin.
// Produces a slice of records conforming to the DataSource's declared output schema.
// See: process/analyst/brief.md (DataSource), TASK-007
type DataSourceConnector interface {
	// Type returns the connector type string that appears in DataSourceConfig.ConnectorType.
	// Used by ConnectorRegistry to resolve the connector by name.
	Type() string

	// Fetch retrieves data from the source and returns it as a slice of record maps.
	// Each record map key corresponds to a field in the DataSource's OutputSchema.
	//
	// Args:
	//   ctx:    Execution context. Cancellation aborts the fetch.
	//   config: The DataSourceConfig.Config from the pipeline definition.
	//   input:  The Task's input parameters (passed through from the submission).
	//
	// Returns:
	//   A slice of records. Each record is a map from field name to value.
	//   An error if the source is unreachable or returns an unexpected response.
	//
	// Postconditions:
	//   - On error: worker marks task "failed" (Process script error — no retry per ADR-003 domain invariant 2).
	Fetch(ctx context.Context, config map[string]any, input map[string]any) ([]map[string]any, error)
}

// ProcessConnector transforms records received from the DataSource phase.
// Applied after schema mapping from DataSource output to Process input.
// See: process/analyst/brief.md (Process), TASK-007
type ProcessConnector interface {
	// Type returns the connector type string that appears in ProcessConfig.ConnectorType.
	Type() string

	// Transform applies the process logic to each input record and returns output records.
	//
	// Args:
	//   ctx:     Execution context.
	//   config:  The ProcessConfig.Config from the pipeline definition.
	//   records: The records after DataSource->Process schema mapping has been applied.
	//
	// Returns:
	//   Transformed records conforming to the Process's OutputSchema.
	//   An error if transformation logic fails (Process script error — no retry).
	Transform(ctx context.Context, config map[string]any, records []map[string]any) ([]map[string]any, error)
}

// SinkConnector writes processed records to an external destination.
// Sink operations must be atomic: on failure, all partial writes are rolled back.
// Idempotency is enforced via ExecutionID (ADR-003, ADR-009).
// See: process/analyst/brief.md (Sink), ADR-003, ADR-009, TASK-007, TASK-018
type SinkConnector interface {
	// Type returns the connector type string that appears in SinkConfig.ConnectorType.
	Type() string

	// Snapshot captures the current state of the Sink destination within the output scope.
	// Called before Write to produce the "Before" snapshot for the Sink Inspector (ADR-009).
	//
	// Args:
	//   ctx:    Execution context.
	//   config: The SinkConfig.Config from the pipeline definition.
	//   taskID: The task being executed; used to scope the snapshot.
	//
	// Returns:
	//   A map representing the destination's current state relevant to this Sink's output scope.
	//   An error if the destination is unreadable (blocks Sink execution if snapshot required).
	Snapshot(ctx context.Context, config map[string]any, taskID string) (map[string]any, error)

	// Write atomically writes records to the destination using a Sink-type-specific transaction.
	// Checks whether executionID has already been applied before writing (idempotency guard).
	//
	// Args:
	//   ctx:         Execution context. Cancellation aborts the write and triggers rollback.
	//   config:      The SinkConfig.Config from the pipeline definition.
	//   records:     The records after Process->Sink schema mapping has been applied.
	//   executionID: Unique ID for this execution attempt (taskID:attemptNumber). Guards against duplicate writes.
	//
	// Returns:
	//   nil on successful atomic commit.
	//   ErrAlreadyApplied if this executionID was already committed (idempotent no-op).
	//   Any other error triggers rollback; partial writes must not persist.
	//
	// Postconditions:
	//   - On nil: records are durably written; no partial state exists.
	//   - On error: destination is in the same state as before Write was called.
	Write(ctx context.Context, config map[string]any, records []map[string]any, executionID string) error
}

// ErrAlreadyApplied is returned by SinkConnector.Write when the executionID has already
// been committed. The caller treats this as a successful no-op (idempotent redelivery, ADR-003).
var ErrAlreadyApplied = &connectorError{"execution already applied"}

// ConnectorRegistry resolves connector type names to their implementations.
// Built at startup and injected into the Worker.
// See: TASK-007, TASK-042
type ConnectorRegistry interface {
	// DataSource returns the DataSourceConnector registered for the given type.
	// Returns ErrUnknownConnector if no connector is registered for that type.
	DataSource(connectorType string) (DataSourceConnector, error)

	// Process returns the ProcessConnector for the given type.
	Process(connectorType string) (ProcessConnector, error)

	// Sink returns the SinkConnector for the given type.
	Sink(connectorType string) (SinkConnector, error)

	// Register adds a connector to the registry.
	// connectorType must be unique; panics on duplicate registration.
	// connectorKind must be "datasource", "process", or "sink".
	Register(connectorKind string, connector any)
}

// DefaultConnectorRegistry is the standard ConnectorRegistry implementation.
// Connectors are registered at startup and resolved by type name at execution time.
// Panics on duplicate registration to surface misconfiguration at startup (fail-fast).
// See: TASK-007, TASK-042
type DefaultConnectorRegistry struct {
	sources map[string]DataSourceConnector
	procs   map[string]ProcessConnector
	sinks   map[string]SinkConnector
}

// NewDefaultConnectorRegistry constructs an empty DefaultConnectorRegistry.
// Call Register to add connectors before the Worker starts consuming tasks.
func NewDefaultConnectorRegistry() *DefaultConnectorRegistry {
	return &DefaultConnectorRegistry{
		sources: make(map[string]DataSourceConnector),
		procs:   make(map[string]ProcessConnector),
		sinks:   make(map[string]SinkConnector),
	}
}

// DataSource implements ConnectorRegistry.DataSource.
// Returns ErrUnknownConnector when the type has no registered implementation.
func (r *DefaultConnectorRegistry) DataSource(connectorType string) (DataSourceConnector, error) {
	c, ok := r.sources[connectorType]
	if !ok {
		return nil, ErrUnknownConnector
	}
	return c, nil
}

// Process implements ConnectorRegistry.Process.
// Returns ErrUnknownConnector when the type has no registered implementation.
func (r *DefaultConnectorRegistry) Process(connectorType string) (ProcessConnector, error) {
	c, ok := r.procs[connectorType]
	if !ok {
		return nil, ErrUnknownConnector
	}
	return c, nil
}

// Sink implements ConnectorRegistry.Sink.
// Returns ErrUnknownConnector when the type has no registered implementation.
func (r *DefaultConnectorRegistry) Sink(connectorType string) (SinkConnector, error) {
	c, ok := r.sinks[connectorType]
	if !ok {
		return nil, ErrUnknownConnector
	}
	return c, nil
}

// Register implements ConnectorRegistry.Register.
// connectorKind must be one of "datasource", "process", or "sink".
// Panics if connectorKind is invalid or if the type name is already registered (fail-fast).
func (r *DefaultConnectorRegistry) Register(connectorKind string, connector any) {
	switch connectorKind {
	case "datasource":
		c, ok := connector.(DataSourceConnector)
		if !ok {
			panic("DefaultConnectorRegistry.Register: connector does not implement DataSourceConnector")
		}
		if _, exists := r.sources[c.Type()]; exists {
			panic("DefaultConnectorRegistry.Register: duplicate DataSource connector type " + c.Type())
		}
		r.sources[c.Type()] = c
	case "process":
		c, ok := connector.(ProcessConnector)
		if !ok {
			panic("DefaultConnectorRegistry.Register: connector does not implement ProcessConnector")
		}
		if _, exists := r.procs[c.Type()]; exists {
			panic("DefaultConnectorRegistry.Register: duplicate Process connector type " + c.Type())
		}
		r.procs[c.Type()] = c
	case "sink":
		c, ok := connector.(SinkConnector)
		if !ok {
			panic("DefaultConnectorRegistry.Register: connector does not implement SinkConnector")
		}
		if _, exists := r.sinks[c.Type()]; exists {
			panic("DefaultConnectorRegistry.Register: duplicate Sink connector type " + c.Type())
		}
		r.sinks[c.Type()] = c
	default:
		panic("DefaultConnectorRegistry.Register: unknown connectorKind " + connectorKind + "; must be datasource, process, or sink")
	}
}

// ErrUnknownConnector is returned by ConnectorRegistry methods when the requested type
// has no registered implementation.
var ErrUnknownConnector = &connectorError{"unknown connector type"}

type connectorError struct{ msg string }

func (e *connectorError) Error() string { return e.msg }

// --- Demo connectors (TASK-042) ---

// defaultDemoRecordCount is the number of sample records DemoDataSource produces
// when the "count" config key is absent or zero.
const defaultDemoRecordCount = 5

// DemoDataSource is an in-memory DataSource that produces deterministic sample data.
// It does not contact any external infrastructure; all records are synthesised from
// a fixed pattern indexed by record position.
//
// Config keys (all optional):
//   - "count" (float64): number of records to produce. Defaults to 5.
//
// Output schema: each record has three fields — "id" (int), "name" (string), "value" (int).
// The field values are deterministic: name = "record-{i}", value = i*10, id = i+1.
//
// See: TASK-042
type DemoDataSource struct{}

// Type implements DataSourceConnector.Type.
// Returns "demo" — the connector type string used in pipeline DataSourceConfig.
func (d *DemoDataSource) Type() string { return "demo" }

// Fetch produces a slice of deterministic sample records.
// Record count is controlled by config["count"] (float64). Defaults to 5.
// The same config always produces identical output (deterministic).
//
// Preconditions:
//   - ctx must not be cancelled before Fetch is called.
//
// Postconditions:
//   - On success: returns a non-nil slice of length == count.
//   - Records are not shared; callers may mutate them freely.
func (d *DemoDataSource) Fetch(ctx context.Context, config map[string]any, input map[string]any) ([]map[string]any, error) {
	count := defaultDemoRecordCount
	if v, ok := config["count"]; ok {
		if n, ok := v.(float64); ok && n > 0 {
			count = int(n)
		}
	}

	records := make([]map[string]any, count)
	for i := 0; i < count; i++ {
		records[i] = map[string]any{
			"id":    i + 1,
			"name":  fmt.Sprintf("record-%d", i),
			"value": i * 10,
		}
	}
	return records, nil
}

// DemoProcessConnector is the walking skeleton Process connector.
// It adds a "processed" boolean flag to every record and, when configured,
// uppercases the string value of a named field.
//
// Config keys (all optional):
//   - "uppercase_field" (string): name of the field whose string value is uppercased.
//
// The connector does not mutate input records; it produces new record maps.
//
// See: TASK-042
type DemoProcessConnector struct{}

// Type implements ProcessConnector.Type.
// Returns "demo" — the connector type string used in pipeline ProcessConfig.
func (d *DemoProcessConnector) Type() string { return "demo" }

// Transform applies the demo process logic to each record.
// Adds "processed": true to every output record.
// When config["uppercase_field"] is set, uppercases the named field's string value.
// Returns an empty slice when records is empty.
// Does not mutate input records.
//
// Preconditions:
//   - records is non-nil (may be empty).
//
// Postconditions:
//   - On success: len(output) == len(records).
//   - Each output record contains all fields from the input plus "processed": true.
func (d *DemoProcessConnector) Transform(ctx context.Context, config map[string]any, records []map[string]any) ([]map[string]any, error) {
	uppercaseField, _ := config["uppercase_field"].(string)

	result := make([]map[string]any, len(records))
	for i, rec := range records {
		// Build a new map so the input is not mutated.
		out := make(map[string]any, len(rec)+1)
		for k, v := range rec {
			out[k] = v
		}
		out["processed"] = true
		if uppercaseField != "" {
			if s, ok := out[uppercaseField].(string); ok {
				out[uppercaseField] = strings.ToUpper(s)
			}
		}
		result[i] = out
	}
	return result, nil
}

// DemoSinkConnector writes processed records to an in-memory store and logs each
// committed write to stdout. It satisfies the Sink atomicity requirement (ADR-009)
// by committing to the in-memory map only after all preconditions pass.
//
// Idempotency (ADR-003): a second Write call with the same executionID returns
// ErrAlreadyApplied without modifying the store.
//
// The store is safe for concurrent access.
//
// See: TASK-042, ADR-003, ADR-009
type DemoSinkConnector struct {
	mu    sync.Mutex
	store map[string]logEntry // keyed by executionID
}

// NewDemoSinkConnector constructs a DemoSinkConnector with an initialised store.
// The caller is responsible for registering the returned connector in the ConnectorRegistry.
func NewDemoSinkConnector() *DemoSinkConnector {
	return &DemoSinkConnector{
		store: make(map[string]logEntry),
	}
}

// Type implements SinkConnector.Type.
// Returns "demo" — the connector type string used in pipeline SinkConfig.
func (d *DemoSinkConnector) Type() string { return "demo" }

// Snapshot returns the current state of the in-memory store as a summary map.
// The summary contains "record_count" (total records across all executionIDs) and
// "execution_count" (number of distinct executionIDs committed).
// taskID is accepted for interface compliance but not used to scope the snapshot,
// since the demo store is not partitioned by task.
//
// Postconditions:
//   - Returns a non-nil map. Never returns an error.
func (d *DemoSinkConnector) Snapshot(ctx context.Context, config map[string]any, taskID string) (map[string]any, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	total := 0
	for _, e := range d.store {
		total += len(e.Records)
	}
	return map[string]any{
		"record_count":    total,
		"execution_count": len(d.store),
	}, nil
}

// Write commits records to the in-memory store under the given executionID.
// Returns ErrAlreadyApplied without modifying the store when executionID is already
// present (idempotency guard, ADR-003).
// Logs each commit to stdout for demo observability.
//
// Preconditions:
//   - executionID is non-empty.
//
// Postconditions:
//   - On nil: records are in the store under executionID.
//   - On ErrAlreadyApplied: store is unchanged.
func (d *DemoSinkConnector) Write(ctx context.Context, config map[string]any, records []map[string]any, executionID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if _, exists := d.store[executionID]; exists {
		return ErrAlreadyApplied
	}

	// Deep-copy records so the store is not affected by subsequent mutations.
	stored := make([]map[string]any, len(records))
	for i, r := range records {
		cp := make(map[string]any, len(r))
		for k, v := range r {
			cp[k] = v
		}
		stored[i] = cp
	}

	d.store[executionID] = logEntry{
		ExecutionID: executionID,
		Records:     stored,
		CommittedAt: time.Now().UTC().Format(time.RFC3339),
	}

	log.Printf("demo-sink: committed %d record(s) for executionID=%q", len(records), executionID)
	return nil
}

// logEntry is the in-memory record of a single committed Sink write.
// Stored by DemoSinkConnector keyed by executionID.
type logEntry struct {
	ExecutionID string
	Records     []map[string]any
	CommittedAt string
}

// RegisterDemoConnectors registers the three demo connector implementations
// (DemoDataSource, DemoProcessConnector, DemoSinkConnector) in the given registry.
// Called at worker startup to enable pipelines that use connector type "demo".
// Panics on duplicate registration (fail-fast: calling this twice is a startup bug).
//
// See: TASK-042, cmd/worker/main.go
func RegisterDemoConnectors(reg *DefaultConnectorRegistry) {
	reg.Register("datasource", &DemoDataSource{})
	reg.Register("process", &DemoProcessConnector{})
	reg.Register("sink", NewDemoSinkConnector())
}

// LogEntry is the public representation of a committed DemoSinkConnector write,
// exposed for the Sink Inspector (ADR-009, TASK-033).
type LogEntry struct {
	ExecutionID string
	Records     []map[string]any
	CommittedAt string
}

// SinkInspectorData carries the before/after snapshot for a Sink execution.
// Populated by the Worker and published as an SSEEvent to events:sink:{taskId}.
// See: ADR-009, TASK-033
type SinkInspectorData struct {
	Before *models.SinkSnapshot
	After  *models.SinkSnapshot
}
