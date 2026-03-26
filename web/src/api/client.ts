/**
 * API client — typed wrappers around the NexusFlow REST API.
 * All requests are relative to the current origin; Vite's dev proxy routes /api to the Go server.
 * Credentials: 'include' is set on all requests so the browser sends the HTTP-only session cookie.
 *
 * See: ADR-004, ADR-006, TASK-019
 */

import type {
  AuthResponse,
  Pipeline,
  Task,
  Worker,
} from '@/types/domain'

/** Base fetch wrapper that adds credentials and Content-Type. */
async function apiFetch<T>(
  path: string,
  init?: RequestInit
): Promise<T> {
  // TODO: Implement in TASK-019
  throw new Error('Not implemented: apiFetch ' + path + JSON.stringify(init))
}

// --- Auth ---

/**
 * Login with username and password.
 * The server sets an HTTP-only session cookie on success.
 *
 * @throws On 401: invalid credentials. On 5xx: server error.
 * See: ADR-006, TASK-003
 */
export async function login(username: string, password: string): Promise<AuthResponse> {
  // TODO: Implement in TASK-019
  return apiFetch<AuthResponse>('/api/auth/login', {
    method: 'POST',
    body: JSON.stringify({ username, password }),
  })
}

/**
 * Logout the current user.
 * Invalidates the server-side session. The HTTP-only cookie is cleared by the server.
 * See: ADR-006, TASK-003
 */
export async function logout(): Promise<void> {
  // TODO: Implement in TASK-019
  await apiFetch<void>('/api/auth/logout', { method: 'POST' })
}

// --- Workers ---

/**
 * List all registered workers.
 * All authenticated users can view all workers (Domain Invariant 5).
 * See: REQ-016, TASK-025
 */
export async function listWorkers(): Promise<Worker[]> {
  // TODO: Implement in TASK-019
  return apiFetch<Worker[]>('/api/workers')
}

// --- Pipelines ---

/**
 * List pipelines visible to the current user.
 * User role: own pipelines only. Admin: all pipelines.
 * See: REQ-022, TASK-013
 */
export async function listPipelines(): Promise<Pipeline[]> {
  // TODO: Implement in TASK-019
  return apiFetch<Pipeline[]>('/api/pipelines')
}

/**
 * Create a new pipeline.
 * See: REQ-022, TASK-013
 */
export async function createPipeline(
  pipeline: Omit<Pipeline, 'id' | 'userId' | 'createdAt' | 'updatedAt'>
): Promise<Pipeline> {
  // TODO: Implement in TASK-019
  return apiFetch<Pipeline>('/api/pipelines', {
    method: 'POST',
    body: JSON.stringify(pipeline),
  })
}

// --- Tasks ---

/**
 * Submit a new task.
 * See: REQ-001, TASK-005
 */
export async function submitTask(payload: {
  pipelineId: string
  input: Record<string, unknown>
  retryConfig?: { maxRetries: number; backoff: string }
}): Promise<{ taskId: string; status: string }> {
  // TODO: Implement in TASK-019
  return apiFetch('/api/tasks', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

/**
 * List tasks visible to the current user.
 * See: REQ-009, TASK-008 (Cycle 2)
 */
export async function listTasks(): Promise<Task[]> {
  // TODO: Implement in TASK-019
  return apiFetch<Task[]>('/api/tasks')
}
