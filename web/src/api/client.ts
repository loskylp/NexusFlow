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

/**
 * apiFetch is the base fetch wrapper for all NexusFlow API calls.
 * Attaches credentials and Content-Type headers to every request.
 * Throws an error with the HTTP status code prefix on non-2xx responses,
 * so callers can detect 401 vs 500 from the error message.
 *
 * @param path - Absolute path relative to origin (e.g. '/api/auth/login').
 * @param init - Standard fetch RequestInit; body should be a JSON string.
 * @returns Parsed JSON response body typed as T.
 * @throws Error with message starting with the HTTP status code on failure.
 */
async function apiFetch<T>(
  path: string,
  init?: RequestInit
): Promise<T> {
  const response = await fetch(path, {
    ...init,
    credentials: 'include',
    headers: {
      'Content-Type': 'application/json',
      ...(init?.headers ?? {}),
    },
  })

  if (!response.ok) {
    const body = await response.text()
    throw new Error(`${response.status}: ${body}`)
  }

  // 204 No Content has no body to parse
  if (response.status === 204) {
    return undefined as unknown as T
  }

  return response.json() as Promise<T>
}

// --- Auth ---

/**
 * Login with username and password.
 * The server sets an HTTP-only session cookie on success (ADR-006).
 * The response body also contains the session token and the user object.
 *
 * Request shape:  { username, password }
 * Response shape: { token, user: { id, username, role } }
 *
 * @throws Error with '401' prefix on invalid credentials.
 * @throws Error with '5xx' prefix on server error.
 * See: ADR-006, TASK-003, handlers_auth.go
 */
export async function login(username: string, password: string): Promise<AuthResponse> {
  return apiFetch<AuthResponse>('/api/auth/login', {
    method: 'POST',
    body: JSON.stringify({ username, password }),
  })
}

/**
 * Logout the current user.
 * Invalidates the server-side session. The HTTP-only cookie is cleared by the server.
 * Always succeeds from the client's perspective (server returns 204).
 * See: ADR-006, TASK-003, handlers_auth.go
 */
export async function logout(): Promise<void> {
  await apiFetch<void>('/api/auth/logout', { method: 'POST' })
}

// --- Workers ---

/**
 * List all registered workers.
 * All authenticated users can view all workers (Domain Invariant 5).
 * See: REQ-016, TASK-025
 */
export async function listWorkers(): Promise<Worker[]> {
  return apiFetch<Worker[]>('/api/workers')
}

// --- Pipelines ---

/**
 * List pipelines visible to the current user.
 * User role: own pipelines only. Admin: all pipelines.
 * See: REQ-022, TASK-013
 */
export async function listPipelines(): Promise<Pipeline[]> {
  return apiFetch<Pipeline[]>('/api/pipelines')
}

/**
 * Create a new pipeline.
 * See: REQ-022, TASK-013
 */
export async function createPipeline(
  pipeline: Omit<Pipeline, 'id' | 'userId' | 'createdAt' | 'updatedAt'>
): Promise<Pipeline> {
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
  return apiFetch('/api/tasks', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

/**
 * List tasks visible to the current user with optional filters.
 * User role: own tasks only. Admin: all tasks.
 * See: REQ-009, TASK-008, TASK-021
 *
 * @param status     - Filter by task status. Omit for all statuses.
 * @param pipelineId - Filter by pipeline ID. Omit for all pipelines.
 * @param search     - Search by task ID (exact prefix) or pipeline name (substring).
 */
export async function listTasksWithFilters(params?: {
  status?: string
  pipelineId?: string
  search?: string
}): Promise<Task[]> {
  const query = new URLSearchParams()
  if (params?.status) query.set('status', params.status)
  if (params?.pipelineId) query.set('pipelineId', params.pipelineId)
  if (params?.search) query.set('search', params.search)
  const qs = query.toString()
  return apiFetch<Task[]>(qs ? `/api/tasks?${qs}` : '/api/tasks')
}

/**
 * Get a single task by ID, including its state transition history.
 * See: REQ-009, TASK-008
 *
 * @throws Error with '403' prefix if the caller does not own the task.
 * @throws Error with '404' prefix if the task does not exist.
 */
export async function getTask(taskId: string): Promise<Task> {
  return apiFetch<Task>(`/api/tasks/${taskId}`)
}

/**
 * Cancel a task.
 * Only the task owner or Admin may cancel.
 * Returns void on success (server returns 204).
 *
 * @throws Error with '403' prefix if the caller is not the owner or Admin.
 * @throws Error with '409' prefix if the task is in a terminal state.
 * See: REQ-010, TASK-012
 */
export async function cancelTask(taskId: string): Promise<void> {
  return apiFetch<void>(`/api/tasks/${taskId}/cancel`, { method: 'POST' })
}

/**
 * Download the full log history for a task as a text blob.
 * Fetches from GET /api/tasks/{id}/logs and returns the raw text content
 * for the browser download trigger in the Log Streamer.
 *
 * @throws Error with '403' prefix if the caller does not own the task.
 * @throws Error with '404' prefix if the task does not exist.
 * See: REQ-018, TASK-022, TASK-016
 */
export async function downloadTaskLogs(taskId: string): Promise<string> {
  const response = await fetch(`/api/tasks/${taskId}/logs`, {
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
  })
  if (!response.ok) {
    const body = await response.text()
    throw new Error(`${response.status}: ${body}`)
  }
  return response.text()
}

// --- Pipelines (extended) ---

/**
 * Get a single pipeline by ID.
 * See: REQ-022, TASK-013
 *
 * @throws Error with '403' prefix if the caller does not own the pipeline.
 * @throws Error with '404' prefix if the pipeline does not exist.
 */
export async function getPipeline(pipelineId: string): Promise<Pipeline> {
  return apiFetch<Pipeline>(`/api/pipelines/${pipelineId}`)
}

/**
 * Update an existing pipeline.
 * Only the pipeline owner or Admin may update.
 *
 * @param pipelineId - ID of the pipeline to update.
 * @param updates    - Fields to update. At minimum name, dataSourceConfig,
 *                     processConfig, and sinkConfig must be provided.
 * See: REQ-022, TASK-013
 *
 * @throws Error with '400' prefix if schema mapping validation fails.
 * @throws Error with '403' prefix if the caller does not own the pipeline.
 */
export async function updatePipeline(
  pipelineId: string,
  updates: Omit<Pipeline, 'id' | 'userId' | 'createdAt' | 'updatedAt'>
): Promise<Pipeline> {
  return apiFetch<Pipeline>(`/api/pipelines/${pipelineId}`, {
    method: 'PUT',
    body: JSON.stringify(updates),
  })
}

/**
 * Delete a pipeline by ID.
 * Returns void on success (server returns 204).
 * Deletion is rejected if the pipeline has active (non-terminal) tasks.
 *
 * @throws Error with '403' prefix if the caller does not own the pipeline.
 * @throws Error with '409' prefix if the pipeline has active tasks.
 * See: REQ-022, TASK-013
 */
export async function deletePipeline(pipelineId: string): Promise<void> {
  return apiFetch<void>(`/api/pipelines/${pipelineId}`, { method: 'DELETE' })
}

// --- Users (admin) ---

/**
 * List all user accounts. Admin only.
 * See: REQ-020, TASK-017
 *
 * @throws Error with '403' prefix if the caller is not Admin.
 */
export async function listUsers(): Promise<import('@/types/domain').User[]> {
  return apiFetch<import('@/types/domain').User[]>('/api/users')
}

// --- Auth: password change (SEC-001) ---

/**
 * Change the authenticated user's password.
 * The server clears the MustChangePassword flag on success and invalidates all
 * existing sessions. The client must re-authenticate after calling this.
 *
 * Request shape:  { currentPassword, newPassword }
 * Response:       204 No Content on success.
 *
 * @throws Error with '401' prefix if currentPassword is incorrect.
 * @throws Error with '400' prefix if newPassword is shorter than 8 characters
 *         or equals currentPassword.
 * @throws Error with '403' prefix if not authenticated.
 *
 * See: SEC-001, SEC-007, ADR-006
 */
export async function changePassword(
  currentPassword: string,
  newPassword: string
): Promise<void> {
  return apiFetch<void>('/api/auth/change-password', {
    method: 'POST',
    body: JSON.stringify({ currentPassword, newPassword }),
  })
}

// --- Chaos Controller (TASK-034) ---

/**
 * Kill a worker container by worker ID.
 * Admin only. Stops the worker's Docker container. The Monitor will detect the
 * heartbeat absence and reclaim in-flight tasks (ADR-002).
 *
 * Response shape: { log: ChaosActivityEntry[] }
 *
 * @throws Error with '400' prefix if workerId is missing.
 * @throws Error with '403' prefix if caller is not Admin.
 * @throws Error with '404' prefix if workerId does not match a registered worker.
 * @throws Error with '500' prefix on Docker daemon error.
 *
 * See: DEMO-004, TASK-034
 */
export async function killWorker(workerId: string): Promise<{ log: import('@/types/domain').ChaosActivityEntry[] }> {
  return apiFetch('/api/chaos/kill-worker', {
    method: 'POST',
    body: JSON.stringify({ workerId }),
  })
}

/**
 * Simulate database unavailability for a fixed duration.
 * Admin only. Blocks PostgreSQL connections from NexusFlow services for
 * durationSeconds (15, 30, or 60). Connectivity restores automatically.
 *
 * Response shape: { log: ChaosActivityEntry[], durationSeconds: number }
 *
 * @throws Error with '400' prefix if durationSeconds is not 15, 30, or 60.
 * @throws Error with '403' prefix if caller is not Admin.
 * @throws Error with '409' prefix if a disconnect is already active.
 * @throws Error with '500' prefix on network manipulation error.
 *
 * See: DEMO-004, TASK-034
 */
export async function disconnectDatabase(durationSeconds: 15 | 30 | 60): Promise<{
  log: import('@/types/domain').ChaosActivityEntry[]
  durationSeconds: number
}> {
  return apiFetch('/api/chaos/disconnect-db', {
    method: 'POST',
    body: JSON.stringify({ durationSeconds }),
  })
}

/**
 * Submit a burst of tasks to a pipeline to flood the queue.
 * Admin only. Non-destructive: creates taskCount tasks for the given pipeline.
 * No confirmation required (UX spec: flood is non-destructive).
 *
 * Response shape: { submittedCount: number, log: ChaosActivityEntry[] }
 *
 * @throws Error with '400' prefix if pipelineId is missing or taskCount is out of [1, 1000].
 * @throws Error with '403' prefix if caller is not Admin.
 * @throws Error with '404' prefix if pipelineId does not match an existing pipeline.
 * @throws Error with '500' prefix on queue enqueue error.
 *
 * See: DEMO-004, TASK-034
 */
export async function floodQueue(
  pipelineId: string,
  taskCount: number
): Promise<{ submittedCount: number; log: import('@/types/domain').ChaosActivityEntry[] }> {
  return apiFetch('/api/chaos/flood-queue', {
    method: 'POST',
    body: JSON.stringify({ pipelineId, taskCount }),
  })
}
