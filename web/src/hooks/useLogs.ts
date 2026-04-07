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

import type { TaskLog } from '@/types/domain'
import type { SSEConnectionStatus } from './useSSE'

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
export function useLogs(options: UseLogsOptions): UseLogsReturn {
  // TODO: implement
  throw new Error('Not implemented')
}
