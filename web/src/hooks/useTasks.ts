/**
 * useTasks — hook that combines an initial REST fetch with real-time SSE updates
 * to maintain a live list of tasks visible to the current user.
 *
 * Seeding: GET /api/tasks on mount (with optional filter params) populates the
 * initial task list.
 * Updates: SSE events from GET /events/tasks are merged into the list:
 *   - task:submitted    → add to list if not already present
 *   - task:queued       → update status for matching task
 *   - task:assigned     → update status and workerId for matching task
 *   - task:running      → update status for matching task
 *   - task:completed    → update status for matching task
 *   - task:failed       → update status for matching task
 *   - task:cancelled    → update status for matching task
 *
 * Visibility isolation: the SSE channel already filters by user role (server-side).
 * The UI trusts the server — there is no client-side ownership filter.
 *
 * See: TASK-021, ADR-007, REQ-017
 */

import { useCallback, useEffect, useState } from 'react'
import type { Task, TaskStatus, SSEEvent } from '@/types/domain'
import { listTasksWithFilters } from '@/api/client'
import { useSSE } from './useSSE'
import type { SSEConnectionStatus } from './useSSE'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

/** Filter parameters for the task list query. */
export interface TaskFilters {
  /** Filter by task status. Undefined means all statuses. */
  status?: TaskStatus
  /** Filter by pipeline ID. Undefined means all pipelines. */
  pipelineId?: string
  /** Search by task ID (exact prefix) or pipeline name (substring). */
  search?: string
}

export interface UseTasksReturn {
  /** Ordered task list — reverse chronological (newest first). */
  tasks: Task[]
  /** Whether the initial REST fetch is still in progress. */
  isLoading: boolean
  /** Non-null when the initial fetch failed with an error message. */
  error: string | null
  /** Current SSE connection status for display in the status bar. */
  sseStatus: SSEConnectionStatus
  /** Re-run the initial fetch with current filters. */
  refresh: () => void
}

// ---------------------------------------------------------------------------
// Pure helper — merge a single SSE task event into the task list
// ---------------------------------------------------------------------------

/**
 * mergeTaskEvent applies a single SSE event to the current task list,
 * returning a new array.  The function is pure and creates no side effects.
 *
 * Handles:
 *   task:submitted  — adds the task if not already present.
 *   task:queued, task:assigned, task:running, task:completed,
 *   task:failed, task:cancelled — merges the full payload into the
 *     matching task, updating status and any changed fields.
 * Unknown event types are ignored and the original list is returned unchanged.
 *
 * @param tasks   - Current task list.
 * @param event   - Incoming SSE event from /events/tasks.
 * @returns New task array with the event applied.
 */
export function mergeTaskEvent(tasks: Task[], event: SSEEvent<Task>): Task[] {
  // TODO: implement
  throw new Error('Not implemented')
}

// ---------------------------------------------------------------------------
// Hook
// ---------------------------------------------------------------------------

/**
 * useTasks maintains a live task list by combining an initial REST fetch
 * with real-time SSE event merging.
 *
 * @param filters - Optional filter parameters for the initial fetch and display.
 *
 * Preconditions:
 *   - The user must be authenticated (the API and SSE endpoint require a session cookie).
 *
 * Postconditions:
 *   - tasks is populated from GET /api/tasks on mount (filtered by params).
 *   - tasks is updated in place as SSE events arrive.
 *   - isLoading is true only during the initial fetch; SSE updates do not affect it.
 */
export function useTasks(filters?: TaskFilters): UseTasksReturn {
  // TODO: implement
  throw new Error('Not implemented')
}
