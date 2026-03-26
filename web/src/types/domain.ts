/**
 * Domain types for the NexusFlow frontend.
 * These mirror the Go models in internal/models/models.go.
 * Names follow the domain vocabulary from the Analyst Brief v2.
 *
 * In production these types should be generated from the OpenAPI spec (ADR-004).
 * For Cycle 1, they are hand-written and must be kept in sync with the Go backend.
 *
 * See: ADR-004, internal/models/models.go
 */

export type TaskStatus =
  | 'submitted'
  | 'queued'
  | 'assigned'
  | 'running'
  | 'completed'
  | 'failed'
  | 'cancelled'

export type WorkerStatus = 'online' | 'down'

export type Role = 'admin' | 'user'

export type BackoffStrategy = 'exponential' | 'linear' | 'fixed'

export interface RetryConfig {
  maxRetries: number
  backoff: BackoffStrategy
}

export interface SchemaMapping {
  sourceField: string
  targetField: string
}

export interface DataSourceConfig {
  connectorType: string
  config: Record<string, unknown>
  outputSchema: string[]
}

export interface ProcessConfig {
  connectorType: string
  config: Record<string, unknown>
  inputMappings: SchemaMapping[]
  outputSchema: string[]
}

export interface SinkConfig {
  connectorType: string
  config: Record<string, unknown>
  inputMappings: SchemaMapping[]
}

export interface User {
  id: string
  username: string
  role: Role
  active: boolean
  createdAt: string
}

export interface Pipeline {
  id: string
  name: string
  userId: string
  dataSourceConfig: DataSourceConfig
  processConfig: ProcessConfig
  sinkConfig: SinkConfig
  createdAt: string
  updatedAt: string
}

export interface Task {
  id: string
  pipelineId: string
  chainId?: string
  userId: string
  status: TaskStatus
  retryConfig: RetryConfig
  retryCount: number
  executionId: string
  workerId?: string
  input: Record<string, unknown>
  createdAt: string
  updatedAt: string
}

export interface TaskStateLog {
  id: string
  taskId: string
  fromState: TaskStatus
  toState: TaskStatus
  reason: string
  timestamp: string
}

export interface Worker {
  id: string
  tags: string[]
  status: WorkerStatus
  lastHeartbeat: string
  registeredAt: string
  currentTaskId?: string
}

export interface TaskLog {
  id: string
  taskId: string
  line: string
  level: string
  timestamp: string
}

/**
 * SSEEvent is the envelope received from SSE endpoint streams.
 * See: ADR-007, TASK-015
 */
export interface SSEEvent<T = unknown> {
  type: string
  payload: T
  id?: string
}

/**
 * AuthResponse is the response body from POST /api/auth/login.
 * See: ADR-006, TASK-003
 */
export interface AuthResponse {
  token: string
  user: User
}
