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
  return status === 'submitted' || status === 'queued' || status === 'assigned' || status === 'running'
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
  const colorMap: Record<TaskStatus, string> = {
    submitted: '#8B5CF6',   // violet-500
    queued:    '#D97706',   // amber-600
    assigned:  '#F59E0B',   // amber-500
    running:   '#2563EB',   // blue-600
    completed: '#16A34A',   // green-600
    failed:    '#DC2626',   // red-600
    cancelled: '#64748B',   // slate-500
  }

  const color = colorMap[status]

  return {
    display: 'inline-flex',
    alignItems: 'center',
    gap: '4px',
    padding: '2px 8px',
    borderRadius: '9999px',
    backgroundColor: `${color}1A`, // 10% opacity background
    color,
    fontSize: '12px',
    fontWeight: 500,
    fontFamily: 'var(--font-label)',
    textTransform: 'uppercase' as const,
    letterSpacing: '0.04em',
  }
}

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

interface StatusBadgeProps {
  status: TaskStatus
}

/**
 * StatusBadge renders the colored pill badge for a task status.
 * Running status includes a pulse dot to indicate active processing.
 */
function StatusBadge({ status }: StatusBadgeProps): React.ReactElement {
  const style = statusBadgeStyle(status)
  const isRunning = status === 'running'

  return (
    <span style={style} role="status" aria-label={`Task status: ${status}`}>
      {isRunning && (
        <span
          style={{
            display: 'inline-block',
            width: '6px',
            height: '6px',
            borderRadius: '50%',
            backgroundColor: 'currentColor',
            animation: 'pulse 1.5s ease-in-out infinite',
            flexShrink: 0,
          }}
        />
      )}
      {status}
    </span>
  )
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
  const isFailed = task.status === 'failed'
  const showCancel = (isOwner || isAdmin) && isCancellable(task.status)
  const showRetry = task.status === 'failed'

  const cardStyle: React.CSSProperties = {
    backgroundColor: isRecentlyUpdated ? '#FEFCE8' : 'var(--color-surface-panel)', // yellow-50 flash
    border: '1px solid var(--color-border)',
    borderRadius: '8px',
    padding: '16px',
    borderLeft: isFailed ? '4px solid #DC2626' : undefined,
    transition: 'background-color 200ms ease',
  }

  const buttonBaseStyle: React.CSSProperties = {
    padding: '4px 10px',
    border: '1px solid var(--color-border)',
    borderRadius: '6px',
    fontSize: '12px',
    cursor: 'pointer',
    background: 'none',
    color: 'var(--color-text-secondary)',
  }

  /** handleCancelClick shows a confirmation dialog before invoking onCancel. */
  function handleCancelClick(): void {
    const confirmed = window.confirm(`Cancel task ${task.id}?`)
    if (confirmed) {
      onCancel(task.id)
    }
  }

  return (
    <div style={cardStyle} data-task-id={task.id}>
      {/* Card header: task ID + pipeline name + status badge */}
      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: '12px', marginBottom: '10px' }}>
        <div style={{ display: 'flex', flexDirection: 'column', gap: '4px', minWidth: 0 }}>
          <span
            style={{
              fontFamily: 'var(--font-mono)',
              fontSize: '13px',
              color: 'var(--color-text-primary)',
              fontWeight: 600,
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              whiteSpace: 'nowrap',
            }}
          >
            {task.id}
          </span>
          <span style={{ fontSize: '13px', color: 'var(--color-text-secondary)' }}>
            {pipelineName}
          </span>
        </div>
        <StatusBadge status={task.status} />
      </div>

      {/* Metadata row: worker assignment + timing */}
      <div
        style={{
          display: 'flex',
          gap: '16px',
          fontSize: '12px',
          color: 'var(--color-text-secondary)',
          marginBottom: '12px',
          flexWrap: 'wrap',
        }}
      >
        {task.workerId ? (
          <span>
            Worker: <span style={{ fontFamily: 'var(--font-mono)' }}>{task.workerId}</span>
          </span>
        ) : (
          <span style={{ color: 'var(--color-text-tertiary)' }}>No worker assigned</span>
        )}
        <span>Submitted: {formatTimestamp(task.createdAt)}</span>
        {task.updatedAt !== task.createdAt && (
          <span>Updated: {formatTimestamp(task.updatedAt)}</span>
        )}
      </div>

      {/* Failed state: error indicator */}
      {isFailed && (
        <div
          role="alert"
          style={{
            fontSize: '12px',
            color: '#DC2626',
            marginBottom: '10px',
            fontFamily: 'var(--font-mono)',
          }}
        >
          Task failed — check logs for details
        </div>
      )}

      {/* Action buttons */}
      <div style={{ display: 'flex', gap: '8px' }}>
        <button
          type="button"
          onClick={() => onViewLogs(task.id)}
          style={buttonBaseStyle}
        >
          View Logs
        </button>

        {showCancel && (
          <button
            type="button"
            onClick={handleCancelClick}
            style={{
              ...buttonBaseStyle,
              color: '#DC2626',
              borderColor: '#FECACA',
            }}
          >
            Cancel
          </button>
        )}

        {showRetry && (
          <button
            type="button"
            onClick={() => onRetry(task)}
            style={{
              ...buttonBaseStyle,
              color: '#D97706',
              borderColor: '#FDE68A',
            }}
          >
            Retry
          </button>
        )}
      </div>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Formatting helper
// ---------------------------------------------------------------------------

/**
 * formatTimestamp formats an ISO 8601 timestamp as a human-readable relative time
 * (e.g., "2s ago") or as a locale time string for older timestamps.
 *
 * @param isoTimestamp - ISO 8601 timestamp string.
 * @returns Human-readable time string.
 */
function formatTimestamp(isoTimestamp: string): string {
  try {
    const date = new Date(isoTimestamp)
    const diffMs = Date.now() - date.getTime()
    const diffSec = Math.floor(diffMs / 1000)

    if (diffSec < 60) return `${diffSec}s ago`
    const diffMin = Math.floor(diffSec / 60)
    if (diffMin < 60) return `${diffMin}m ago`
    const diffHr = Math.floor(diffMin / 60)
    if (diffHr < 24) return `${diffHr}h ago`
    return date.toLocaleDateString()
  } catch {
    return isoTimestamp
  }
}

export default TaskCard
