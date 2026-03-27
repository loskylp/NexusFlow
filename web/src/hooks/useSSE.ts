/**
 * useSSE — React hook wrapping the browser's native EventSource API.
 * Connects to a NexusFlow SSE endpoint and calls onEvent for each received event.
 * Manages connection lifecycle (open, reconnect, close) and exposes connection status
 * for display in the status bar (DESIGN.md — Real-time Indicators).
 *
 * See: ADR-007, TASK-019, TASK-020
 */

import { useCallback, useEffect, useRef, useState } from 'react'
import type { SSEEvent } from '@/types/domain'

export type SSEConnectionStatus = 'connecting' | 'connected' | 'reconnecting' | 'closed'

interface UseSSEOptions<T> {
  /** The SSE endpoint URL (e.g. '/events/workers'). */
  url: string
  /** Called for each SSE event received. Must be stable (wrapped in useCallback). */
  onEvent: (event: SSEEvent<T>) => void
  /** Whether to actively maintain the connection. Set to false to disconnect. Default: true. */
  enabled?: boolean
}

interface UseSSEReturn {
  /** Current connection status for display in the status bar. */
  status: SSEConnectionStatus
  /** Imperatively close the SSE connection and transition to 'closed'. */
  close: () => void
}

/** Initial backoff delay in milliseconds. Doubles on each consecutive error. */
const INITIAL_BACKOFF_MS = 1000
/** Maximum backoff delay in milliseconds. */
const MAX_BACKOFF_MS = 30_000

/**
 * useSSE establishes and maintains a Server-Sent Events connection.
 * Automatically reconnects using exponential backoff when the connection drops.
 *
 * @param options.url     - The SSE endpoint to connect to. A URL change closes the
 *                          current connection and opens a new one.
 * @param options.onEvent - Stable callback (wrapped in useCallback) called for each
 *                          successfully parsed SSE event.
 * @param options.enabled - Default true. Set to false to close the connection and
 *                          enter 'closed' state without reconnecting.
 *
 * @returns { status, close }
 *   status: 'connected' | 'connecting' | 'reconnecting' | 'closed'
 *   close:  Imperatively close the connection; transitions status to 'closed'.
 *
 * Preconditions:
 *   - The user must be authenticated; the server rejects unauthenticated SSE connections.
 *   - onEvent must be stable (wrapped in useCallback) to avoid reconnection on every render.
 *
 * Postconditions:
 *   - On unmount: EventSource is closed and no further events are delivered.
 *   - On error: status transitions to 'reconnecting'; a new EventSource is created after
 *     an exponential backoff delay, resetting to 'connecting'.
 *
 * See: ADR-007, TASK-020
 */
export function useSSE<T = unknown>({
  url,
  onEvent,
  enabled = true,
}: UseSSEOptions<T>): UseSSEReturn {
  const [status, setStatus] = useState<SSEConnectionStatus>(
    enabled ? 'connecting' : 'closed'
  )

  // Stable reference to onEvent so the effect closure always calls the latest version
  // without needing to list it as an effect dependency (which would re-open the stream).
  const onEventRef = useRef(onEvent)
  useEffect(() => {
    onEventRef.current = onEvent
  }, [onEvent])

  // Whether the hook has been imperatively closed via close().
  // Stored in a ref so the effect closure can read the latest value without re-running.
  const closedManually = useRef(false)

  // Live reference to the current EventSource so the imperative close() can reach it.
  const sourceRef = useRef<EventSource | null>(null)

  // Imperative close handle returned to the caller.
  const close = useCallback(() => {
    closedManually.current = true
    sourceRef.current?.close()
    setStatus('closed')
  }, [])

  useEffect(() => {
    if (!enabled) {
      setStatus('closed')
      return
    }

    // Reset manual-close flag when the effect re-runs (url changed or re-enabled).
    closedManually.current = false

    let source: EventSource | null = null
    let backoffMs = INITIAL_BACKOFF_MS
    let reconnectTimer: ReturnType<typeof setTimeout> | null = null

    function connect() {
      setStatus('connecting')
      source = new EventSource(url, { withCredentials: true })
      sourceRef.current = source

      source.onopen = () => {
        backoffMs = INITIAL_BACKOFF_MS
        setStatus('connected')
      }

      source.onmessage = (event: MessageEvent) => {
        try {
          const parsed = JSON.parse(event.data as string) as SSEEvent<T>
          onEventRef.current(parsed)
        } catch {
          // Discard malformed messages — non-JSON data is not a valid SSEEvent.
        }
      }

      source.onerror = () => {
        source?.close()
        source = null

        if (closedManually.current) return

        setStatus('reconnecting')

        reconnectTimer = setTimeout(() => {
          if (closedManually.current) return
          connect()
        }, backoffMs)

        backoffMs = Math.min(backoffMs * 2, MAX_BACKOFF_MS)
      }
    }

    connect()

    return () => {
      closedManually.current = true
      if (reconnectTimer !== null) clearTimeout(reconnectTimer)
      source?.close()
      sourceRef.current = null
    }
  }, [url, enabled])

  return { status, close }
}
