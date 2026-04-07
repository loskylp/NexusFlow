/**
 * TaskCard — a single task card in the Task Feed (TASK-021).
 *
 * Displays task ID, pipeline name, status badge, worker assignment, timing,
 * and action buttons (View Logs, Cancel, Retry). Receives SSE-driven live
 * state via props — it is a pure presentational component; state lives in
 * TaskFeedPage via useTasks.
 *
 * Status badge colors follow the task state color map (DESIGN.md):
 *   submitted  → violet  (#8B5CF6 / --color-submitted)
 *   queued     → amber   (--color-warning)
 *   assigned   → amber   (--color-warning)
 *   running    → blue    (--color-info) — with subtle pulse animation
 *   completed  → green   (--color-success)
 *   failed     → red     (--color-error) — card gains red-50 left border accent
 *   cancelled  → slate   (--color-cancelled)
 *
 * See: TASK-021, REQ-017, REQ-010, UX Spec (Task Feed and Monitor)
 */

import React from 'react'
import type { Task, TaskStatus } from '@/types/domain'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface TaskCardProps {
  /** The task to render. SSE updates arrive as new task objects; React re-renders. */
  task: Task
  /**
   * Pipeline name to display alongside the pipeline ID.
   * The caller resolves name from the pipeline list; TaskCard is unaware of pipelines.
   */
  pipelineName: string
  /**
   * Whether the current user is Admin.
   * Controls visibility of the cancel button for other users' tasks.
   */
  isAdmin: boolean
  /**
   * Whether the current user owns this task.
   * Controls visibility of the cancel button.
   */
  isOwner: boolean
  /** Called when the user clicks "View Logs". Navigates to Log Streamer. */
  onViewLogs: (taskId: string) => void
  /** Called when the user confirms cancellation via the confirmation dialog. */
  onCancel: (taskId: string) => void
  /** Called when the user clicks "Retry" on a failed task. */
  onRetry: (task: Task) => void
  /**
   * Whether the task was recently updated via SSE (triggers 200ms highlight flash).
   * The parent sets this flag on SSE update and clears it after 200ms.
   */
  isRecentlyUpdated?: boolean
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/**
 * isCancellable returns true when a task is in a state that can be cancelled.
 * Terminal states (completed, failed, cancelled) cannot be cancelled.
 *
 * @param status - The task's current status.
 * @returns True if the task may be cancelled.
 */
export function isCancellable(status: TaskStatus): boolean {
  // TODO: implement
  throw new Error('Not implemented')
}

/**
 * statusBadgeStyle returns the CSS style object for a status badge.
 * Uses the semantic color tokens from DESIGN.md.
 * Color is never the sole indicator — the status label text is always shown.
 *
 * @param status - The task's current status.
 * @returns A React CSSProperties object for the badge element.
 */
export function statusBadgeStyle(status: TaskStatus): React.CSSProperties {
  // TODO: implement
  throw new Error('Not implemented')
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

/**
 * TaskCard renders a single task as a card in the Task Feed.
 *
 * States handled:
 *   - Normal: standard card layout with status badge and action buttons
 *   - Failed: red-50 left border accent (4px), error reason text displayed
 *   - Running: blue badge with subtle pulse animation on the status dot
 *   - Recently updated (isRecentlyUpdated): 200ms yellow-50 background flash
 *
 * Preconditions:
 *   - task is a valid Task object.
 *   - onViewLogs, onCancel, onRetry are stable callbacks (wrapped in useCallback in parent).
 *
 * Postconditions:
 *   - Cancel button is shown only when (isOwner || isAdmin) && isCancellable(task.status).
 *   - Retry button is shown only when task.status === 'failed'.
 *   - View Logs button is always shown.
 */
function TaskCard({
  task,
  pipelineName,
  isAdmin,
  isOwner,
  onViewLogs,
  onCancel,
  onRetry,
  isRecentlyUpdated = false,
}: TaskCardProps): React.ReactElement {
  // TODO: implement
  throw new Error('Not implemented')
}

export default TaskCard
