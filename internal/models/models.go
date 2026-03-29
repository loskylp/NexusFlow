// Package models defines the domain types for NexusFlow.
// All names follow the domain vocabulary established in the Analyst Brief v2.
// See: process/analyst/brief.md (Domain Model section)
package models

import (
	"time"

	"github.com/google/uuid"
)

// TaskStatus represents the lifecycle state of a Task.
// Domain Invariant 1: Task lifecycle is monotonically forward.
// The only backward exception is failover reassignment (assigned/running -> queued).
// Valid transitions are enforced by a database trigger on task_state_log.
// See: ADR-008, process/analyst/brief.md (Domain Invariants)
type TaskStatus string

const (
	TaskStatusSubmitted TaskStatus = "submitted"
	TaskStatusQueued    TaskStatus = "queued"
	TaskStatusAssigned  TaskStatus = "assigned"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusCancelled TaskStatus = "cancelled"
)

// WorkerStatus represents whether a Worker is reachable and processing tasks.
// See: ADR-002, process/analyst/brief.md (Domain Model — Worker)
type WorkerStatus string

const (
	WorkerStatusOnline WorkerStatus = "online"
	WorkerStatusDown   WorkerStatus = "down"
)

// Role defines the access level of a User within the system.
// See: process/analyst/brief.md (User Roles)
type Role string

const (
	RoleAdmin Role = "admin"
	RoleUser  Role = "user"
)

// BackoffStrategy describes how retry delays grow between attempts.
// See: ADR-002, TASK-010
type BackoffStrategy string

const (
	BackoffExponential BackoffStrategy = "exponential"
	BackoffLinear      BackoffStrategy = "linear"
	BackoffFixed       BackoffStrategy = "fixed"
)

// RetryConfig holds per-task retry parameters.
// Domain Invariant 2: Retry is infrastructure-only — Process script errors do not retry.
// Safe defaults: MaxRetries=3, Backoff=exponential.
// See: ADR-002, ADR-003, TASK-005, TASK-010
type RetryConfig struct {
	MaxRetries int             `json:"maxRetries" db:"max_retries"`
	Backoff    BackoffStrategy `json:"backoff"    db:"backoff"`
}

// DefaultRetryConfig returns the safe default retry configuration applied when
// a task is submitted without explicit retry settings.
// See: TASK-005
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{MaxRetries: 3, Backoff: BackoffExponential}
}

// SchemaMapping defines how data fields from one pipeline phase map to the
// input fields of the next phase. Validated at design-time (pipeline save) and
// at runtime (execution). See: ADR-008, TASK-026, TASK-007
type SchemaMapping struct {
	// SourceField is the field name in the preceding phase's output schema.
	SourceField string `json:"sourceField" db:"source_field"`
	// TargetField is the field name expected by the next phase's input schema.
	TargetField string `json:"targetField" db:"target_field"`
}

// DataSourceConfig describes the first phase of a Pipeline.
// ConnectorType identifies which DataSource implementation to use (e.g. "demo", "s3", "postgres").
// Config holds connector-specific parameters as opaque JSON.
// OutputSchema declares the field names this phase produces; used for design-time SchemaMapping validation.
// See: process/analyst/brief.md (DataSource), ADR-008, ADR-009
type DataSourceConfig struct {
	ConnectorType string            `json:"connectorType" db:"connector_type"`
	Config        map[string]any    `json:"config"        db:"config"`
	OutputSchema  []string          `json:"outputSchema"  db:"output_schema"`
}

// ProcessConfig describes the second phase of a Pipeline.
// ConnectorType identifies the transformation implementation.
// InputMappings maps DataSource output fields to Process input fields.
// OutputSchema declares the field names this phase produces.
// See: process/analyst/brief.md (Process), ADR-008
type ProcessConfig struct {
	ConnectorType string            `json:"connectorType"  db:"connector_type"`
	Config        map[string]any    `json:"config"         db:"config"`
	InputMappings []SchemaMapping   `json:"inputMappings"  db:"input_mappings"`
	OutputSchema  []string          `json:"outputSchema"   db:"output_schema"`
}

// SinkConfig describes the third phase of a Pipeline.
// ConnectorType identifies the Sink implementation (e.g. "demo", "s3", "postgres").
// InputMappings maps Process output fields to Sink input fields.
// Operations are atomic: on failure, partial writes are rolled back (ADR-009).
// See: process/analyst/brief.md (Sink), ADR-003, ADR-009
type SinkConfig struct {
	ConnectorType string          `json:"connectorType"  db:"connector_type"`
	Config        map[string]any  `json:"config"         db:"config"`
	InputMappings []SchemaMapping `json:"inputMappings"  db:"input_mappings"`
}

// User is a person who interacts with NexusFlow. Either an Admin or a regular User.
// Admins have full visibility; Users see only their own Tasks.
// See: process/analyst/brief.md (User), ADR-006, REQ-019, REQ-020
type User struct {
	ID           uuid.UUID `json:"id"           db:"id"`
	Username     string    `json:"username"     db:"username"`
	PasswordHash string    `json:"-"            db:"password_hash"`
	Role         Role      `json:"role"         db:"role"`
	Active       bool      `json:"active"       db:"active"`
	CreatedAt    time.Time `json:"createdAt"    db:"created_at"`
}

// Pipeline is a linear sequence of three phases — DataSource, Process, Sink —
// that defines how a Task's data flows from ingestion through transformation to output.
// Domain Invariant 6: Pipelines are strictly linear (one DataSource, one Process, one Sink).
// Owned by the creating User; Users manage their own Pipelines; Admins manage all.
// See: process/analyst/brief.md (Pipeline), REQ-022, ADR-008
type Pipeline struct {
	ID               uuid.UUID        `json:"id"               db:"id"`
	Name             string           `json:"name"             db:"name"`
	UserID           uuid.UUID        `json:"userId"           db:"user_id"`
	DataSourceConfig DataSourceConfig `json:"dataSourceConfig" db:"data_source_config"`
	ProcessConfig    ProcessConfig    `json:"processConfig"    db:"process_config"`
	SinkConfig       SinkConfig       `json:"sinkConfig"       db:"sink_config"`
	CreatedAt        time.Time        `json:"createdAt"        db:"created_at"`
	UpdatedAt        time.Time        `json:"updatedAt"        db:"updated_at"`
}

// PipelineChain is a linear sequence of Pipelines where the completion of one
// triggers the next (A -> B -> C). Failure in any Pipeline cascades cancellation
// to all downstream Pipelines in the chain.
// Domain Invariant 4: Cascading cancellation fires on dead-letter routing.
// See: process/analyst/brief.md (PipelineChain), REQ-014, TASK-014
type PipelineChain struct {
	ID          uuid.UUID   `json:"id"          db:"id"`
	Name        string      `json:"name"        db:"name"`
	UserID      uuid.UUID   `json:"userId"      db:"user_id"`
	PipelineIDs []uuid.UUID `json:"pipelineIds" db:"pipeline_ids"`
	CreatedAt   time.Time   `json:"createdAt"   db:"created_at"`
}

// Chain is the normalised pipeline chain stored in the chains + chain_steps tables
// (migration 000004). It supersedes PipelineChain for the TASK-014 implementation.
// PipelineIDs is ordered by position: index 0 is the first pipeline to execute.
// Chains are strictly linear: no pipeline appears more than once; no branching.
// See: REQ-014, ADR-003, TASK-014
type Chain struct {
	ID          uuid.UUID   `json:"id"          db:"id"`
	Name        string      `json:"name"        db:"name"`
	UserID      uuid.UUID   `json:"userId"      db:"user_id"`
	PipelineIDs []uuid.UUID `json:"pipelineIds" db:"-"` // assembled from chain_steps rows
	CreatedAt   time.Time   `json:"createdAt"   db:"created_at"`
}

// Task is the primary unit of work in NexusFlow.
// A Task references a Pipeline and carries input parameters. It transitions through
// the lifecycle states defined by TaskStatus.
// ExecutionID encodes task ID + attempt number for Sink idempotency (ADR-003).
// PipelineID is nullable: when the referenced pipeline is deleted, PipelineID
// is set to nil by the database (ON DELETE SET NULL). Historical tasks are
// preserved with a nil PipelineID rather than being cascade-deleted.
// RetryAfter is set by the Monitor when a task is reclaimed after an infrastructure
// failure. When non-nil, the task must not be re-enqueued until RetryAfter has elapsed
// (exponential/linear/fixed backoff per RetryConfig.Backoff). A nil RetryAfter means
// the task has never been retried or is immediately ready for dispatch.
// RetryTags records the capability tag streams the task must be re-enqueued to when
// RetryAfter elapses. Populated alongside RetryAfter during XCLAIM reclamation so
// the Monitor's retry-ready scan knows which stream(s) to target.
// See: process/analyst/brief.md (Task), REQ-001, REQ-009, ADR-003, TASK-010
type Task struct {
	ID          uuid.UUID    `json:"id"          db:"id"`
	PipelineID  *uuid.UUID   `json:"pipelineId"  db:"pipeline_id"`
	ChainID     *uuid.UUID   `json:"chainId"     db:"chain_id"`
	UserID      uuid.UUID    `json:"userId"      db:"user_id"`
	Status      TaskStatus   `json:"status"      db:"status"`
	RetryConfig RetryConfig  `json:"retryConfig" db:"retry_config"`
	RetryCount  int          `json:"retryCount"  db:"retry_count"`
	RetryAfter  *time.Time   `json:"retryAfter"  db:"retry_after"`
	RetryTags   []string     `json:"retryTags"   db:"retry_tags"`
	ExecutionID string       `json:"executionId" db:"execution_id"`
	WorkerID    *string      `json:"workerId"    db:"worker_id"`
	Input       map[string]any `json:"input"     db:"input"`
	CreatedAt   time.Time    `json:"createdAt"   db:"created_at"`
	UpdatedAt   time.Time    `json:"updatedAt"   db:"updated_at"`
}

// TaskStateLog records each state transition for a Task.
// Provides a full audit trail of the Task lifecycle.
// See: ADR-008, REQ-009
type TaskStateLog struct {
	ID        uuid.UUID  `json:"id"        db:"id"`
	TaskID    uuid.UUID  `json:"taskId"    db:"task_id"`
	FromState TaskStatus `json:"fromState" db:"from_state"`
	ToState   TaskStatus `json:"toState"   db:"to_state"`
	Reason    string     `json:"reason"    db:"reason"`
	Timestamp time.Time  `json:"timestamp" db:"timestamp"`
}

// Worker is a compute node that pulls Tasks from the queue and executes Pipelines.
// Self-registers with the system on startup; emits heartbeats every 5 seconds (ADR-002).
// Capability Tags determine which task queue streams the Worker consumes.
// See: process/analyst/brief.md (Worker), REQ-004, ADR-001, ADR-002, TASK-006
type Worker struct {
	ID            string       `json:"id"            db:"id"`
	Tags          []string     `json:"tags"          db:"tags"`
	Status        WorkerStatus `json:"status"        db:"status"`
	LastHeartbeat time.Time    `json:"lastHeartbeat" db:"last_heartbeat"`
	RegisteredAt  time.Time    `json:"registeredAt"  db:"registered_at"`
	// CurrentTaskID is populated at query time from the task table; not stored in workers table.
	CurrentTaskID *uuid.UUID `json:"currentTaskId,omitempty" db:"-"`
}

// TaskLog is a single log line produced during Task execution.
// Hot logs (0-72h) live in Redis Streams (logs:{taskId}); cold logs in PostgreSQL.
// See: ADR-008, REQ-018, TASK-016
type TaskLog struct {
	ID        uuid.UUID `json:"id"        db:"id"`
	TaskID    uuid.UUID `json:"taskId"    db:"task_id"`
	Line      string    `json:"line"      db:"line"`
	Level     string    `json:"level"     db:"level"`
	Timestamp time.Time `json:"timestamp" db:"timestamp"`
}

// Session holds the server-side session data stored in Redis at key session:{token}.
// See: ADR-006, TASK-003
type Session struct {
	UserID    uuid.UUID `json:"userId"`
	Role      Role      `json:"role"`
	CreatedAt time.Time `json:"createdAt"`
}

// SSEEvent is the envelope for events published to SSE channels via Redis Pub/Sub.
// See: ADR-007, TASK-015
type SSEEvent struct {
	// Channel is the Redis Pub/Sub channel this event was received on.
	Channel string `json:"channel"`
	// Type is the event type (e.g. "task:state-changed", "worker:down", "log:line").
	Type    string `json:"type"`
	// Payload is the event-specific data.
	Payload any    `json:"payload"`
	// ID is a monotonically increasing identifier, used for Last-Event-ID on log streams.
	ID      string `json:"id,omitempty"`
}

// SinkSnapshot holds the captured state of a Sink destination before and after
// a Sink phase execution. Used by the Sink Inspector (DEMO-003).
// See: ADR-009, TASK-033
type SinkSnapshot struct {
	TaskID    uuid.UUID      `json:"taskId"`
	Phase     string         `json:"phase"` // "before" or "after"
	Data      map[string]any `json:"data"`
	CapturedAt time.Time     `json:"capturedAt"`
}
