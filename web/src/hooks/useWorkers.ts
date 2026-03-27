/**
 * useWorkers — hook that combines an initial REST fetch with real-time SSE updates
 * to maintain a live list of all registered workers.
 *
 * Seeding: GET /api/workers on mount populates the initial worker list.
 * Updates: SSE events from GET /events/workers are merged into the list:
 *   - worker:registered → add if not already present
 *   - worker:heartbeat  → update lastHeartbeat (and status) for matching worker
 *   - worker:down       → update status to 'down' for matching worker
 *
 * See: TASK-020, ADR-007
 */

import { useCallback, useEffect, useState } from 'react'
import type { Worker, SSEEvent } from '@/types/domain'
import { listWorkers } from '@/api/client'
import { useSSE } from './useSSE'
import type { SSEConnectionStatus } from './useSSE'

export interface WorkerSummary {
  total: number
  online: number
  down: number
}

export interface UseWorkersReturn {
  /** Ordered worker list — caller is responsible for sorting if desired. */
  workers: Worker[]
  /** Whether the initial REST fetch is still in progress. */
  isLoading: boolean
  /** Aggregated status counts derived from the current worker list. */
  summary: WorkerSummary
  /** Current SSE connection status for display in the status bar. */
  sseStatus: SSEConnectionStatus
}

/**
 * computeSummary derives total, online, and down counts from a worker list.
 *
 * @param workers - The current worker list.
 * @returns WorkerSummary with total, online, and down counts.
 */
function computeSummary(workers: Worker[]): WorkerSummary {
  const online = workers.filter(w => w.status === 'online').length
  const down = workers.filter(w => w.status === 'down').length
  return { total: workers.length, online, down }
}

/**
 * mergeWorkerEvent applies a single SSE event to the current worker list,
 * returning a new array.  The function is pure and creates no side effects.
 *
 * Handles:
 *   worker:registered — adds the worker if it is not already in the list.
 *   worker:heartbeat  — merges the full payload into the matching worker.
 *   worker:down       — sets status to 'down' on the matching worker.
 * Unknown event types are ignored and the original list is returned unchanged.
 *
 * @param workers - Current worker list.
 * @param event   - Incoming SSE event from /events/workers.
 * @returns New worker array with the event applied.
 */
export function mergeWorkerEvent(workers: Worker[], event: SSEEvent<Worker>): Worker[] {
  switch (event.type) {
    case 'worker:registered': {
      const exists = workers.some(w => w.id === event.payload.id)
      return exists ? workers : [...workers, event.payload]
    }

    case 'worker:heartbeat': {
      return workers.map(w =>
        w.id === event.payload.id ? { ...w, ...event.payload } : w
      )
    }

    case 'worker:down': {
      return workers.map(w =>
        w.id === event.payload.id ? { ...w, status: 'down' } : w
      )
    }

    default:
      return workers
  }
}

/**
 * useWorkers maintains a live worker list by combining an initial REST fetch
 * with real-time SSE event merging.
 *
 * Preconditions:
 *   - The user must be authenticated (the API and SSE endpoint require a session cookie).
 *
 * Postconditions:
 *   - workers is populated from GET /api/workers on mount.
 *   - workers is updated in place as SSE events arrive.
 *   - summary counts are always consistent with the current workers array.
 */
export function useWorkers(): UseWorkersReturn {
  const [workers, setWorkers] = useState<Worker[]>([])
  const [isLoading, setIsLoading] = useState(true)

  // Seed initial state from the REST API.
  useEffect(() => {
    listWorkers()
      .then(setWorkers)
      .catch(() => {
        // Fail silently — the worker list starts empty; SSE may still deliver updates.
      })
      .finally(() => setIsLoading(false))
  }, [])

  // Merge incoming SSE events into the worker list.
  const handleWorkerEvent = useCallback((event: SSEEvent<Worker>) => {
    setWorkers(current => mergeWorkerEvent(current, event))
  }, [])

  const { status: sseStatus } = useSSE<Worker>({
    url: '/events/workers',
    onEvent: handleWorkerEvent,
  })

  const summary = computeSummary(workers)

  return { workers, isLoading, summary, sseStatus }
}
