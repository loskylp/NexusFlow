/**
 * WorkerFleetDashboard — real-time view of all registered workers.
 * This is the Admin landing page after login (TASK-020).
 *
 * Layout per UX spec (process/designer/ux-spec.md, Worker Fleet Dashboard):
 *   - Page header: "Worker Fleet" title + SSE connection indicator (status bar)
 *   - Summary cards row: Total Workers, Online, Down, Avg Load
 *   - Full-width sortable data table:
 *       Status (dot) | Worker ID | Hostname | Tags | Current Task | CPU% | Memory% | Last Heartbeat
 *   - Down workers sorted to top by default
 *   - Skeleton loaders during initial fetch
 *   - Empty state when no workers registered
 *
 * Real-time updates via GET /events/workers (SSE).
 * Data seeded from GET /api/workers on mount.
 *
 * See: REQ-016, TASK-020, TASK-025, TASK-015
 */

import React, { useCallback, useEffect, useState } from 'react'
import type { Worker, SSEEvent } from '@/types/domain'
import { listWorkers } from '@/api/client'
import { useSSE } from '@/hooks/useSSE'

function WorkerFleetDashboard(): React.ReactElement {
  const [workers, setWorkers] = useState<Worker[]>([])
  const [isLoading, setIsLoading] = useState(true)

  // Seed worker list from REST API on mount.
  // See: TASK-025
  useEffect(() => {
    listWorkers()
      .then(setWorkers)
      .catch(console.error)
      .finally(() => setIsLoading(false))
  }, [])

  // Receive real-time worker updates via SSE.
  // See: ADR-007, TASK-015, TASK-020
  const handleWorkerEvent = useCallback((event: SSEEvent<Worker>) => {
    // TODO: Implement in TASK-020
    // - On 'worker:registered': add worker to list
    // - On 'worker:heartbeat': update lastHeartbeat field
    // - On 'worker:down': update status to 'down'; re-sort (down workers first)
    void event
  }, [])

  const { status: sseStatus } = useSSE<Worker>({
    url: '/events/workers',
    onEvent: handleWorkerEvent,
  })

  // TODO: Implement full WorkerFleetDashboard in TASK-020
  // Requirements (DESIGN.md, UX spec):
  //   - SummaryCard components for Total, Online, Down, Avg Load
  //   - DataTable with sortable columns (click header to sort)
  //   - StatusBadge component (colored dot + text per DESIGN.md)
  //   - SSE connection indicator in status bar
  //   - Skeleton loaders while isLoading
  //   - Empty state: "No workers registered. Start a worker process."

  if (isLoading) {
    return <div>Loading workers...</div>
  }

  const onlineCount = workers.filter(w => w.status === 'online').length
  const downCount = workers.filter(w => w.status === 'down').length

  return (
    <div>
      {/* TODO: Replace with styled dashboard in TASK-020 */}
      <h1>Worker Fleet</h1>
      <p>SSE: {sseStatus}</p>
      <p>Total: {workers.length} | Online: {onlineCount} | Down: {downCount}</p>
      {workers.length === 0 ? (
        <p>No workers registered.</p>
      ) : (
        <table>
          <thead>
            <tr>
              <th>Status</th>
              <th>Worker ID</th>
              <th>Tags</th>
              <th>Current Task</th>
              <th>Last Heartbeat</th>
            </tr>
          </thead>
          <tbody>
            {workers.map(worker => (
              <tr key={worker.id}>
                <td>{worker.status}</td>
                <td>{worker.id}</td>
                <td>{worker.tags.join(', ')}</td>
                <td>{worker.currentTaskId ?? '—'}</td>
                <td>{worker.lastHeartbeat}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  )
}

export default WorkerFleetDashboard
