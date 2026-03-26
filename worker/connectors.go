// Package worker — Connector interfaces and registry.
// A Connector is one of three types: DataSource, Process, or Sink.
// Each Pipeline phase names a connector by ConnectorType; the registry resolves the name
// to its implementation at execution time.
// See: ADR-003, ADR-009, TASK-007, TASK-042
package worker

import (
	"context"

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

// --- Demo connectors (implemented in TASK-042) ---

// DemoDataSource is an in-memory DataSource that returns deterministic sample data.
// Enables end-to-end walking skeleton demonstration without external infrastructure.
// See: TASK-042
type DemoDataSource struct{}

// Type implements DataSourceConnector.Type.
func (d *DemoDataSource) Type() string { return "demo" }

// Fetch implements DataSourceConnector.Fetch.
// Returns a fixed set of sample records regardless of config or input.
func (d *DemoDataSource) Fetch(ctx context.Context, config map[string]any, input map[string]any) ([]map[string]any, error) {
	// TODO: Implement in TASK-042
	panic("not implemented")
}

// DemoProcessConnector is a pass-through Process connector for the walking skeleton.
// Applies no transformation; returns records unchanged.
// See: TASK-042
type DemoProcessConnector struct{}

// Type implements ProcessConnector.Type.
func (d *DemoProcessConnector) Type() string { return "demo" }

// Transform implements ProcessConnector.Transform.
// Pass-through: returns records unchanged.
func (d *DemoProcessConnector) Transform(ctx context.Context, config map[string]any, records []map[string]any) ([]map[string]any, error) {
	// TODO: Implement in TASK-042
	panic("not implemented")
}

// DemoSinkConnector logs records to stdout and records them in an in-memory store.
// Satisfies atomicity by writing to the in-memory store only on success.
// See: TASK-042
type DemoSinkConnector struct {
	// store is the in-memory record of committed writes, keyed by executionID.
	store map[string][]map[string]any //lint:ignore U1000 scaffold stub — wired in TASK-042
}

// Type implements SinkConnector.Type.
func (d *DemoSinkConnector) Type() string { return "demo" }

// Snapshot implements SinkConnector.Snapshot.
// Returns the current contents of the in-memory store for the given taskID scope.
func (d *DemoSinkConnector) Snapshot(ctx context.Context, config map[string]any, taskID string) (map[string]any, error) {
	// TODO: Implement in TASK-042
	panic("not implemented")
}

// Write implements SinkConnector.Write.
// Atomically appends records to the in-memory store. Returns ErrAlreadyApplied if
// the executionID is already present (idempotency guard, ADR-003).
func (d *DemoSinkConnector) Write(ctx context.Context, config map[string]any, records []map[string]any, executionID string) error {
	// TODO: Implement in TASK-042
	panic("not implemented")
}

// LogEntry is what the DemoSinkConnector records for each committed write,
// viewable via the Sink Inspector.
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
