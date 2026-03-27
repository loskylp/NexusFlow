/**
 * WorkerFleetDashboard — real-time operational view of all registered workers.
 * This is the Admin landing page after login (TASK-020).
 *
 * Layout (UX spec — Worker Fleet Dashboard):
 *   - Page header: "Worker Fleet" title
 *   - Summary cards row: Total Workers, Online, Down
 *   - Full-width sortable data table:
 *       Status (dot) | Worker ID | Tags | Current Task | Last Heartbeat
 *   - Down workers sorted to top by default (AC-6)
 *   - Skeleton loaders during initial fetch
 *   - Empty state when no workers are registered (AC-8)
 *   - Status bar with SSE connection state (AC-7)
 *
 * Real-time updates are handled by useWorkers, which merges
 * GET /api/workers (initial) with SSE /events/workers (live).
 *
 * See: REQ-016, TASK-020, TASK-025, TASK-015
 */

import React, { useState, useMemo } from 'react'
import type { Worker, WorkerStatus } from '@/types/domain'
import { useWorkers } from '@/hooks/useWorkers'
import type { SSEConnectionStatus } from '@/hooks/useSSE'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

type SortColumn = 'status' | 'id' | 'tags' | 'currentTask' | 'lastHeartbeat'
type SortDirection = 'asc' | 'desc'

interface SortState {
  column: SortColumn
  direction: SortDirection
}

// ---------------------------------------------------------------------------
// Sort helpers
// ---------------------------------------------------------------------------

/** Maps WorkerStatus to a numeric sort weight so 'down' sorts before 'online'. */
function statusSortWeight(status: WorkerStatus): number {
  return status === 'down' ? 0 : 1
}

/**
 * sortWorkers returns a new sorted array of workers.
 * When sorting by status the natural order is always down-first.
 * All other columns sort lexicographically with the current direction applied.
 *
 * @param workers   - Worker list to sort.
 * @param sortState - Column and direction to sort by.
 * @returns A new array in the requested sort order.
 */
function sortWorkers(workers: Worker[], sortState: SortState): Worker[] {
  return [...workers].sort((a, b) => {
    let comparison = 0

    switch (sortState.column) {
      case 'status':
        comparison = statusSortWeight(a.status) - statusSortWeight(b.status)
        break
      case 'id':
        comparison = a.id.localeCompare(b.id)
        break
      case 'tags':
        comparison = a.tags.join(',').localeCompare(b.tags.join(','))
        break
      case 'currentTask':
        comparison = (a.currentTaskId ?? '').localeCompare(b.currentTaskId ?? '')
        break
      case 'lastHeartbeat':
        comparison = a.lastHeartbeat.localeCompare(b.lastHeartbeat)
        break
    }

    return sortState.direction === 'asc' ? comparison : -comparison
  })
}

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

interface SummaryCardProps {
  label: string
  value: number
  /** CSS color for the count number (uses design token names). */
  valueColor?: string
}

/**
 * SummaryCard renders a single metric card for the summary row.
 * Displays a numeric value with a label below it.
 */
function SummaryCard({ label, value, valueColor }: SummaryCardProps): React.ReactElement {
  return (
    <div
      style={{
        backgroundColor: 'var(--color-surface-panel)',
        border: '1px solid var(--color-border)',
        borderRadius: '8px',
        padding: '16px',
        flex: 1,
      }}
    >
      <div
        style={{
          fontSize: '28px',
          fontWeight: 600,
          color: valueColor ?? 'var(--color-text-primary)',
          lineHeight: 1.2,
        }}
      >
        {value}
      </div>
      <div
        style={{
          fontFamily: 'var(--font-label)',
          fontSize: '12px',
          textTransform: 'uppercase' as const,
          letterSpacing: '0.05em',
          color: 'var(--color-text-secondary)',
          marginTop: '4px',
        }}
      >
        {label}
      </div>
    </div>
  )
}

/** SkeletonCard renders a placeholder card during initial data loading. */
function SkeletonCard(): React.ReactElement {
  return (
    <div
      aria-busy="true"
      style={{
        backgroundColor: 'var(--color-surface-panel)',
        border: '1px solid var(--color-border)',
        borderRadius: '8px',
        padding: '16px',
        flex: 1,
        height: '80px',
      }}
    >
      <div
        style={{
          height: '28px',
          width: '60px',
          backgroundColor: 'var(--color-surface-subtle)',
          borderRadius: '4px',
          marginBottom: '8px',
          animation: 'pulse 1.5s ease-in-out infinite',
        }}
      />
      <div
        style={{
          height: '12px',
          width: '80px',
          backgroundColor: 'var(--color-surface-subtle)',
          borderRadius: '4px',
        }}
      />
    </div>
  )
}

/** SkeletonRow renders a placeholder table row during initial data loading. */
function SkeletonRow(): React.ReactElement {
  return (
    <tr aria-busy="true">
      {[...Array(5)].map((_, i) => (
        <td key={i} style={{ padding: '12px 12px', borderBottom: '1px solid var(--color-border)' }}>
          <div
            style={{
              height: '14px',
              backgroundColor: 'var(--color-surface-subtle)',
              borderRadius: '4px',
              width: i === 0 ? '24px' : '80%',
            }}
          />
        </td>
      ))}
    </tr>
  )
}

interface StatusDotProps {
  status: WorkerStatus
}

/**
 * StatusDot renders a colored dot with an accessible text label for a worker status.
 * Color is never the sole indicator (WCAG 2.1 AA — DESIGN.md accessibility notes).
 */
function StatusDot({ status }: StatusDotProps): React.ReactElement {
  const isOnline = status === 'online'
  const color = isOnline ? 'var(--color-success)' : 'var(--color-error)'
  const label = isOnline ? 'Online' : 'Down'

  return (
    <span
      data-status={status}
      role="status"
      aria-label={`Worker status: ${label}`}
      style={{ display: 'inline-flex', alignItems: 'center', gap: '6px' }}
    >
      <span
        style={{
          display: 'inline-block',
          width: '8px',
          height: '8px',
          borderRadius: '50%',
          backgroundColor: color,
          flexShrink: 0,
        }}
      />
      <span style={{ fontSize: '12px', color, fontWeight: 500 }}>{label}</span>
    </span>
  )
}

interface SortableHeaderProps {
  column: SortColumn
  label: string
  sortState: SortState
  onSort: (column: SortColumn) => void
}

/**
 * SortableHeader renders a table column header that toggles sort direction on click.
 * The th itself carries the onClick so that any click within the cell triggers sorting.
 * The active column shows a directional caret indicator.
 */
function SortableHeader({ column, label, sortState, onSort }: SortableHeaderProps): React.ReactElement {
  const isActive = sortState.column === column
  const caret = isActive ? (sortState.direction === 'asc' ? ' ▲' : ' ▼') : ''

  return (
    <th
      scope="col"
      onClick={() => onSort(column)}
      aria-sort={
        isActive
          ? sortState.direction === 'asc' ? 'ascending' : 'descending'
          : 'none'
      }
      style={{
        padding: '12px 12px',
        textAlign: 'left',
        fontFamily: 'var(--font-label)',
        fontSize: '12px',
        fontWeight: 500,
        textTransform: 'uppercase' as const,
        letterSpacing: '0.05em',
        color: isActive ? 'var(--color-text-primary)' : 'var(--color-text-secondary)',
        borderBottom: '1px solid var(--color-border)',
        backgroundColor: 'var(--color-surface-subtle)',
        cursor: 'pointer',
        userSelect: 'none' as const,
        whiteSpace: 'nowrap' as const,
      }}
    >
      {label}{caret}
    </th>
  )
}

interface StatusBarProps {
  sseStatus: SSEConnectionStatus
  workerCount: number
}

/**
 * StatusBar renders the bottom status bar showing SSE connection state
 * and the current worker count.
 * When sseStatus is 'reconnecting' it shows a red "Reconnecting..." indicator (AC-7).
 */
function StatusBar({ sseStatus, workerCount }: StatusBarProps): React.ReactElement {
  const isReconnecting = sseStatus === 'reconnecting'
  const isConnected = sseStatus === 'connected'

  const dotColor = isConnected
    ? 'var(--color-success)'
    : isReconnecting
      ? 'var(--color-error)'
      : 'var(--color-warning)'

  const statusLabel = isReconnecting
    ? 'Reconnecting...'
    : isConnected
      ? 'Connected'
      : 'Connecting...'

  return (
    <div
      role="status"
      aria-live="polite"
      style={{
        display: 'flex',
        alignItems: 'center',
        gap: '16px',
        padding: '8px 12px',
        backgroundColor: 'var(--color-surface-panel)',
        border: '1px solid var(--color-border)',
        borderRadius: '6px',
        marginTop: '16px',
        fontSize: '12px',
        color: 'var(--color-text-secondary)',
      }}
    >
      <span style={{ display: 'inline-flex', alignItems: 'center', gap: '6px' }}>
        <span
          style={{
            display: 'inline-block',
            width: '6px',
            height: '6px',
            borderRadius: '50%',
            backgroundColor: dotColor,
          }}
        />
        <span style={{ color: isReconnecting ? 'var(--color-error)' : 'inherit' }}>
          {statusLabel}
        </span>
      </span>
      <span>{workerCount} worker{workerCount !== 1 ? 's' : ''}</span>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Main component
// ---------------------------------------------------------------------------

/** Default sort state: down workers first (AC-6). */
const DEFAULT_SORT: SortState = { column: 'status', direction: 'asc' }

/**
 * WorkerFleetDashboard is the real-time operational view of all registered workers.
 * It composes useWorkers for data and renders summary cards, a sortable table,
 * and an SSE connection status bar.
 *
 * States rendered:
 *   - Loading: skeleton cards and skeleton table rows (isLoading === true)
 *   - Empty: centered message when workers list is empty after load (AC-8)
 *   - Populated: summary cards + sortable table with status dots (AC-1, AC-2, AC-5, AC-6)
 *   - SSE reconnecting: status bar shows red "Reconnecting..." text (AC-7)
 */
function WorkerFleetDashboard(): React.ReactElement {
  const { workers, isLoading, summary, sseStatus } = useWorkers()
  const [sortState, setSortState] = useState<SortState>(DEFAULT_SORT)

  const sortedWorkers = useMemo(
    () => sortWorkers(workers, sortState),
    [workers, sortState]
  )

  /**
   * handleSort toggles the sort direction if the same column is clicked again,
   * or switches to the new column in ascending order.
   */
  function handleSort(column: SortColumn): void {
    setSortState(prev => {
      if (prev.column === column) {
        return { column, direction: prev.direction === 'asc' ? 'desc' : 'asc' }
      }
      return { column, direction: 'asc' }
    })
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '24px' }}>
      {/* Page header */}
      <h1
        style={{
          fontSize: '20px',
          fontWeight: 600,
          color: 'var(--color-text-primary)',
          margin: 0,
        }}
      >
        Worker Fleet
      </h1>

      {/* Summary cards row */}
      <div style={{ display: 'flex', gap: '16px' }}>
        {isLoading ? (
          <>
            <SkeletonCard />
            <SkeletonCard />
            <SkeletonCard />
          </>
        ) : (
          <>
            <SummaryCard label="Total" value={summary.total} />
            <SummaryCard
              label="Online"
              value={summary.online}
              valueColor="var(--color-success)"
            />
            <SummaryCard
              label="Down"
              value={summary.down}
              valueColor={summary.down > 0 ? 'var(--color-error)' : 'var(--color-text-primary)'}
            />
          </>
        )}
      </div>

      {/* Content area: skeleton rows, empty state, or data table */}
      {isLoading ? (
        <div
          style={{
            backgroundColor: 'var(--color-surface-panel)',
            border: '1px solid var(--color-border)',
            borderRadius: '8px',
            overflow: 'hidden',
          }}
        >
          <table aria-hidden="true" style={{ width: '100%', borderCollapse: 'collapse' }}>
            <tbody>
              {[...Array(5)].map((_, i) => <SkeletonRow key={i} />)}
            </tbody>
          </table>
        </div>
      ) : workers.length === 0 ? (
        <div
          style={{
            backgroundColor: 'var(--color-surface-panel)',
            border: '1px solid var(--color-border)',
            borderRadius: '8px',
            padding: '48px 24px',
            textAlign: 'center',
          }}
        >
          <p
            style={{
              color: 'var(--color-text-secondary)',
              fontSize: '14px',
              margin: 0,
            }}
          >
            No workers registered. Workers self-register when they start.
          </p>
        </div>
      ) : (
        <div
          style={{
            backgroundColor: 'var(--color-surface-panel)',
            border: '1px solid var(--color-border)',
            borderRadius: '8px',
            overflow: 'hidden',
          }}
        >
          <table
            role="table"
            style={{ width: '100%', borderCollapse: 'collapse' }}
          >
            <thead>
              <tr>
                <SortableHeader
                  column="status"
                  label="Status"
                  sortState={sortState}
                  onSort={handleSort}
                />
                <SortableHeader
                  column="id"
                  label="Worker ID"
                  sortState={sortState}
                  onSort={handleSort}
                />
                <SortableHeader
                  column="tags"
                  label="Tags"
                  sortState={sortState}
                  onSort={handleSort}
                />
                <SortableHeader
                  column="currentTask"
                  label="Current Task"
                  sortState={sortState}
                  onSort={handleSort}
                />
                <SortableHeader
                  column="lastHeartbeat"
                  label="Last Heartbeat"
                  sortState={sortState}
                  onSort={handleSort}
                />
              </tr>
            </thead>
            <tbody>
              {sortedWorkers.map((worker, index) => (
                <WorkerRow key={worker.id} worker={worker} rowIndex={index} />
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* SSE connection status bar (AC-7) */}
      <StatusBar sseStatus={sseStatus} workerCount={workers.length} />
    </div>
  )
}

// ---------------------------------------------------------------------------
// WorkerRow — extracted for readability
// ---------------------------------------------------------------------------

interface WorkerRowProps {
  worker: Worker
  rowIndex: number
}

/**
 * WorkerRow renders a single worker as a table row.
 * Down workers receive a red-50 background per the UX spec.
 * Hovering a row changes the background to blue-50 per the component spec.
 */
function WorkerRow({ worker, rowIndex }: WorkerRowProps): React.ReactElement {
  const [isHovered, setIsHovered] = useState(false)

  const isDown = worker.status === 'down'
  const isEvenRow = rowIndex % 2 === 0

  let backgroundColor = isEvenRow ? '#FFFFFF' : '#FAFAFA'
  if (isDown) backgroundColor = '#FEF2F2'   // red-50
  if (isHovered) backgroundColor = '#EFF6FF' // blue-50

  const cellStyle: React.CSSProperties = {
    padding: '12px 12px',
    fontSize: '14px',
    color: 'var(--color-text-primary)',
    borderBottom: '1px solid var(--color-border)',
    verticalAlign: 'middle',
  }

  return (
    <tr
      style={{ backgroundColor, transition: 'background-color 0.3s ease' }}
      onMouseEnter={() => setIsHovered(true)}
      onMouseLeave={() => setIsHovered(false)}
    >
      <td style={cellStyle}>
        <StatusDot status={worker.status} />
      </td>
      <td style={{ ...cellStyle, fontFamily: 'var(--font-mono)', fontSize: '13px' }}>
        {worker.id}
      </td>
      <td style={cellStyle}>
        {worker.tags.length > 0
          ? worker.tags.map(tag => (
              <span
                key={tag}
                style={{
                  display: 'inline-block',
                  padding: '2px 6px',
                  borderRadius: '4px',
                  backgroundColor: 'var(--color-surface-subtle)',
                  border: '1px solid var(--color-border)',
                  fontSize: '12px',
                  fontFamily: 'var(--font-mono)',
                  marginRight: '4px',
                  marginBottom: '2px',
                }}
              >
                {tag}
              </span>
            ))
          : <span style={{ color: 'var(--color-text-tertiary)' }}>—</span>
        }
      </td>
      <td style={{ ...cellStyle, fontFamily: 'var(--font-mono)', fontSize: '13px' }}>
        {worker.currentTaskId
          ? worker.currentTaskId
          : <span style={{ color: 'var(--color-text-tertiary)' }}>—</span>
        }
      </td>
      <td style={{ ...cellStyle, color: 'var(--color-text-secondary)', fontSize: '13px' }}>
        {formatHeartbeat(worker.lastHeartbeat)}
      </td>
    </tr>
  )
}

// ---------------------------------------------------------------------------
// Formatting helpers
// ---------------------------------------------------------------------------

/**
 * formatHeartbeat formats an ISO timestamp as a human-readable relative time
 * (e.g. "2 seconds ago") or falls back to the raw string on parse failure.
 *
 * @param isoTimestamp - ISO 8601 timestamp string.
 * @returns Human-readable relative time string.
 */
function formatHeartbeat(isoTimestamp: string): string {
  try {
    const date = new Date(isoTimestamp)
    const diffMs = Date.now() - date.getTime()
    const diffSec = Math.floor(diffMs / 1000)

    if (diffSec < 60) return `${diffSec}s ago`
    const diffMin = Math.floor(diffSec / 60)
    if (diffMin < 60) return `${diffMin}m ago`
    const diffHr = Math.floor(diffMin / 60)
    return `${diffHr}h ago`
  } catch {
    return isoTimestamp
  }
}

export default WorkerFleetDashboard
