/**
 * useSSE — React hook wrapping the browser's native EventSource API.
 * Connects to a NexusFlow SSE endpoint and calls onEvent for each received event.
 * Manages connection lifecycle (open, reconnect, close) and exposes connection status
 * for display in the status bar (DESIGN.md — Real-time Indicators).
 *
 * See: ADR-007, TASK-019, TASK-020
 */

import { useEffect, useRef, useState } from 'react'
import type { SSEEvent } from '@/types/domain'

export type SSEConnectionStatus = 'connecting' | 'connected' | 'reconnecting' | 'closed'

interface UseSSEOptions<T> {
  /** The SSE endpoint URL (e.g. '/events/workers'). */
  url: string
  /** Called for each SSE event received. */
  onEvent: (event: SSEEvent<T>) => void
  /** Whether to actively maintain the connection. Set to false to disconnect. */
  enabled?: boolean
}

interface UseSSEReturn {
  /** Current connection status for display in the status bar. */
  status: SSEConnectionStatus
  /** Manually close the SSE connection. */
  close: () => void
}

/**
 * useSSE establishes and maintains a Server-Sent Events connection.
 * Automatically reconnects using exponential backoff when the connection drops.
 *
 * @param options.url     - The SSE endpoint to connect to. Changes cause reconnection.
 * @param options.onEvent - Stable callback (use useCallback) called for each event.
 * @param options.enabled - Default true. Set to false to close the connection.
 *
 * @returns { status, close }
 *   status: 'connected' | 'connecting' | 'reconnecting' | 'closed'
 *   close:  Imperatively close the connection.
 *
 * Preconditions:
 *   - The user must be authenticated; the server will reject unauthenticated SSE connections.
 *   - onEvent must be stable (wrapped in useCallback) to avoid reconnection on every render.
 *
 * Postconditions:
 *   - On unmount: EventSource is closed and no further events are delivered.
 *
 * See: ADR-007, TASK-019
 */
export function useSSE<T = unknown>({
  url,
  onEvent,
  enabled = true,
}: UseSSEOptions<T>): UseSSEReturn {
  // TODO: Implement in TASK-019
  const [status, _setStatus] = useState<SSEConnectionStatus>('closed')
  const sourceRef = useRef<EventSource | null>(null)

  useEffect(() => {
    // TODO: Implement in TASK-019
    // - Create EventSource for url
    // - Set status to 'connecting'
    // - On open: set status to 'connected'
    // - On message: parse JSON, call onEvent
    // - On error: set status to 'reconnecting', implement exponential backoff
    // - On cleanup: close EventSource
    void url
    void onEvent
    void enabled
    return () => {
      sourceRef.current?.close()
    }
  }, [url, onEvent, enabled])

  const close = () => {
    // TODO: Implement in TASK-019
    sourceRef.current?.close()
  }

  return { status, close }
}
