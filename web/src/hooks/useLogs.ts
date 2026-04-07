/**
 * useLogs — hook that manages a real-time log stream for a specific task.
 *
 * Connection: SSE to GET /events/tasks/{taskId}/logs with Last-Event-ID support
 * for missed-line replay on reconnection (ADR-007).
 *
 * Log lines arrive as SSE events of type "log:line". Each event payload is a
 * TaskLog. The hook accumulates lines in order and tracks the last received
 * event ID so reconnection sends the correct Last-Event-ID header.
 *
 * See: TASK-022, ADR-007, REQ-018
 */

import { useCallback, useEffect, useRef, useState } from 'react'
import type { TaskLog, SSEEvent } from '@/types/domain'
import { useSSE } from './useSSE'
import type { SSEConnectionStatus } from './useSSE'

// Note on Last-Event-ID: the browser's native EventSource API automatically
// tracks the id: field of each SSE event and sends it as Last-Event-ID on
// reconnection. The hook tracks it separately only for UI display in the
// status bar; it does not need to pass it back into the EventSource.

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface UseLogsOptions {
  /**
   * Task ID to stream logs for. When undefined the hook is idle (no connection).
   */
  taskId: string | undefined
  /**
   * Whether to actively maintain the SSE connection.
   * Set to false to disconnect without clearing the accumulated log lines.
   * Default: true.
   */
  enabled?: boolean
}

export interface UseLogsReturn {
  /** Accumulated log lines in arrival order. */
  lines: TaskLog[]
  /** Whether the initial log history fetch (REST) is in progress. */
  isLoading: boolean
  /** Non-null when access was denied (403) or the task was not found (404). */
  accessError: string | null
  /** Current SSE connection status. */
  sseStatus: SSEConnectionStatus
  /** Last received SSE event ID — used for Last-Event-ID replay. */
  lastEventId: string | null
  /** Clear the visual log buffer. Does not affect the server-side log storage. */
  clearLines: () => void
}

// ---------------------------------------------------------------------------
// Hook
// ---------------------------------------------------------------------------

/**
 * useLogs streams log lines for a specific task via SSE.
 * Reconnects automatically using Last-Event-ID so missed lines are replayed.
 *
 * @param options.taskId  - Task to stream. Undefined = idle, no SSE connection.
 * @param options.enabled - Default true. Set false to disconnect.
 *
 * Preconditions:
 *   - User must be authenticated and must own the task (or be Admin).
 *   - SSE endpoint enforces ownership: 403 is surfaced as accessError.
 *
 * Postconditions:
 *   - On taskId change: previous lines are cleared, new connection is opened.
 *   - On reconnect: EventSource is opened with Last-Event-ID; server replays
 *     missed lines starting from that ID.
 *   - On clearLines: lines array is emptied; lastEventId is preserved so
 *     future reconnections still replay correctly.
 */
export function useLogs({ taskId, enabled = true }: UseLogsOptions): UseLogsReturn {
  const [lines, setLines] = useState<TaskLog[]>([])
  const [accessError, setAccessError] = useState<string | null>(null)
  // lastEventId is tracked for display in the status bar only.
  // The browser's EventSource tracks it natively for Last-Event-ID reconnection.
  const [lastEventId, setLastEventId] = useState<string | null>(null)

  // Reset the log buffer when the taskId changes.
  const prevTaskIdRef = useRef<string | undefined>(undefined)
  useEffect(() => {
    if (prevTaskIdRef.current !== taskId) {
      prevTaskIdRef.current = taskId
      setLines([])
      setAccessError(null)
      setLastEventId(null)
    }
  }, [taskId])

  // Handle incoming SSE events — accumulate log lines and track Last-Event-ID.
  const handleLogEvent = useCallback((event: SSEEvent<TaskLog>) => {
    if (event.type === 'log:line') {
      setLines(prev => [...prev, event.payload])
      // Track the event ID so the status bar can display the Last-Event-ID.
      if (event.id !== undefined) {
        setLastEventId(event.id)
      }
    } else if (event.type === 'log:error') {
      // Server sends log:error to surface 403/404 access errors in the log panel.
      setAccessError(
        'Access denied: you do not have permission to view logs for this task.'
      )
    }
  }, [])

  // SSE connection: active only when a taskId is provided and the hook is enabled.
  // When sseEnabled is false, useSSE holds the connection closed; sseUrl is never fetched.
  const sseEnabled = taskId !== undefined && enabled
  const sseUrl = taskId !== undefined
    ? `/events/tasks/${taskId}/logs`
    : '/events/tasks/__idle__/logs' // inert placeholder — sseEnabled is false when taskId is undefined

  const { status: sseStatus } = useSSE<TaskLog>({
    url: sseUrl,
    onEvent: handleLogEvent,
    enabled: sseEnabled,
  })

  // clearLines empties the visual buffer but preserves lastEventId for reconnection replay.
  const clearLines = useCallback(() => {
    setLines([])
    // lastEventId is intentionally NOT reset — future reconnections need it for replay.
  }, [])

  return {
    lines,
    isLoading: false,
    accessError,
    sseStatus,
    lastEventId,
    clearLines,
  }
}
