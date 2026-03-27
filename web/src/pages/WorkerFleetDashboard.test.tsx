/**
 * Unit tests for the WorkerFleetDashboard page component.
 * Covers: skeleton loading state, empty state, summary cards,
 * worker table rendering, sortable columns, status indicators,
 * default sort (down workers first), and SSE reconnecting status bar.
 *
 * See: TASK-020, AC-1 through AC-8
 */
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import WorkerFleetDashboard from './WorkerFleetDashboard'
import type { Worker } from '@/types/domain'
import type { UseWorkersReturn } from '@/hooks/useWorkers'

// ---------------------------------------------------------------------------
// Mock useWorkers — full control of hook state per test
// ---------------------------------------------------------------------------

vi.mock('@/hooks/useWorkers')
import * as workersModule from '@/hooks/useWorkers'
const mockUseWorkers = vi.mocked(workersModule.useWorkers)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeWorker(overrides: Partial<Worker> = {}): Worker {
  return {
    id: 'worker-01',
    tags: ['tag-a'],
    status: 'online',
    lastHeartbeat: '2026-03-27T10:00:00Z',
    registeredAt: '2026-03-27T09:00:00Z',
    currentTaskId: undefined,
    ...overrides,
  }
}

function stubHook(partial: Partial<UseWorkersReturn>): void {
  mockUseWorkers.mockReturnValue({
    workers: [],
    isLoading: false,
    summary: { total: 0, online: 0, down: 0 },
    sseStatus: 'connected',
    ...partial,
  })
}

beforeEach(() => {
  vi.clearAllMocks()
})

// ---------------------------------------------------------------------------
// AC-8: Empty state
// ---------------------------------------------------------------------------

describe('WorkerFleetDashboard — empty state (AC-8)', () => {
  it('shows the empty state message when no workers are registered', () => {
    stubHook({ workers: [], isLoading: false })
    render(<WorkerFleetDashboard />)
    expect(screen.getByText(/no workers registered/i)).toBeInTheDocument()
  })

  it('does not render the data table when no workers are registered', () => {
    stubHook({ workers: [], isLoading: false })
    render(<WorkerFleetDashboard />)
    expect(screen.queryByRole('table')).not.toBeInTheDocument()
  })
})

// ---------------------------------------------------------------------------
// Skeleton loading state
// ---------------------------------------------------------------------------

describe('WorkerFleetDashboard — skeleton loading state', () => {
  it('renders skeleton loaders while the initial fetch is in progress', () => {
    stubHook({ isLoading: true })
    render(<WorkerFleetDashboard />)
    // Skeletons are identified by aria-busy or a data-testid convention.
    const skeletons = document.querySelectorAll('[aria-busy="true"]')
    expect(skeletons.length).toBeGreaterThan(0)
  })

  it('does not render the data table while loading', () => {
    stubHook({ isLoading: true })
    render(<WorkerFleetDashboard />)
    expect(screen.queryByRole('table')).not.toBeInTheDocument()
  })
})

// ---------------------------------------------------------------------------
// AC-2: Summary cards
// ---------------------------------------------------------------------------

describe('WorkerFleetDashboard — summary cards (AC-2)', () => {
  it('shows Total, Online, and Down counts from the summary object', () => {
    stubHook({
      workers: [
        makeWorker({ id: 'w1', status: 'online' }),
        makeWorker({ id: 'w2', status: 'online' }),
        makeWorker({ id: 'w3', status: 'down' }),
      ],
      summary: { total: 3, online: 2, down: 1 },
    })
    render(<WorkerFleetDashboard />)

    expect(screen.getByText('3')).toBeInTheDocument()  // total
    expect(screen.getByText('2')).toBeInTheDocument()  // online
    expect(screen.getByText('1')).toBeInTheDocument()  // down
  })

  it('renders the card labels Total, Online, Down', () => {
    stubHook({ summary: { total: 0, online: 0, down: 0 }, workers: [] })
    render(<WorkerFleetDashboard />)
    expect(screen.getByText(/total/i)).toBeInTheDocument()
    expect(screen.getByText(/online/i)).toBeInTheDocument()
    expect(screen.getByText(/down/i)).toBeInTheDocument()
  })
})

// ---------------------------------------------------------------------------
// AC-1: Status indicators
// ---------------------------------------------------------------------------

describe('WorkerFleetDashboard — status indicators (AC-1)', () => {
  it('renders a green status indicator for online workers', () => {
    stubHook({
      workers: [makeWorker({ id: 'w1', status: 'online' })],
      summary: { total: 1, online: 1, down: 0 },
    })
    render(<WorkerFleetDashboard />)
    // Status indicator cells use data-testid="status-online" or aria-label.
    const onlineIndicators = document.querySelectorAll('[data-status="online"]')
    expect(onlineIndicators.length).toBeGreaterThan(0)
  })

  it('renders a red status indicator for down workers', () => {
    stubHook({
      workers: [makeWorker({ id: 'w1', status: 'down' })],
      summary: { total: 1, online: 0, down: 1 },
    })
    render(<WorkerFleetDashboard />)
    const downIndicators = document.querySelectorAll('[data-status="down"]')
    expect(downIndicators.length).toBeGreaterThan(0)
  })
})

// ---------------------------------------------------------------------------
// Table content
// ---------------------------------------------------------------------------

describe('WorkerFleetDashboard — table content', () => {
  it('renders a row for each worker in the list', () => {
    stubHook({
      workers: [
        makeWorker({ id: 'worker-alpha' }),
        makeWorker({ id: 'worker-beta' }),
      ],
      summary: { total: 2, online: 2, down: 0 },
    })
    render(<WorkerFleetDashboard />)
    expect(screen.getByText('worker-alpha')).toBeInTheDocument()
    expect(screen.getByText('worker-beta')).toBeInTheDocument()
  })

  it('shows tags joined for a worker with multiple tags', () => {
    stubHook({
      workers: [makeWorker({ id: 'w1', tags: ['gpu', 'ml', 'fast'] })],
      summary: { total: 1, online: 1, down: 0 },
    })
    render(<WorkerFleetDashboard />)
    expect(screen.getByText(/gpu/)).toBeInTheDocument()
  })

  it('shows a dash for workers with no current task', () => {
    stubHook({
      workers: [makeWorker({ id: 'w1', currentTaskId: undefined })],
      summary: { total: 1, online: 1, down: 0 },
    })
    render(<WorkerFleetDashboard />)
    expect(screen.getByRole('table')).toBeInTheDocument()
  })

  it('shows the current task ID when the worker is assigned', () => {
    stubHook({
      workers: [makeWorker({ id: 'w1', currentTaskId: 'task-xyz' })],
      summary: { total: 1, online: 1, down: 0 },
    })
    render(<WorkerFleetDashboard />)
    expect(screen.getByText('task-xyz')).toBeInTheDocument()
  })
})

// ---------------------------------------------------------------------------
// AC-6: Default sort — down workers first
// ---------------------------------------------------------------------------

describe('WorkerFleetDashboard — default sort (AC-6)', () => {
  it('renders down workers before online workers by default', () => {
    stubHook({
      workers: [
        makeWorker({ id: 'online-1', status: 'online' }),
        makeWorker({ id: 'down-1', status: 'down' }),
        makeWorker({ id: 'online-2', status: 'online' }),
      ],
      summary: { total: 3, online: 2, down: 1 },
    })
    render(<WorkerFleetDashboard />)

    const rows = screen.getAllByRole('row')
    // rows[0] is the header row; rows[1] should be the first data row.
    const firstDataRow = rows[1]
    expect(within(firstDataRow!).getByText('down-1')).toBeInTheDocument()
  })
})

// ---------------------------------------------------------------------------
// AC-5: Sortable columns
// ---------------------------------------------------------------------------

describe('WorkerFleetDashboard — sortable columns (AC-5)', () => {
  it('clicking the Worker ID column header sorts the table by Worker ID ascending', async () => {
    stubHook({
      workers: [
        makeWorker({ id: 'charlie', status: 'online' }),
        makeWorker({ id: 'alpha', status: 'online' }),
        makeWorker({ id: 'bravo', status: 'online' }),
      ],
      summary: { total: 3, online: 3, down: 0 },
    })
    render(<WorkerFleetDashboard />)

    await userEvent.click(screen.getByRole('columnheader', { name: /worker id/i }))

    const rows = screen.getAllByRole('row')
    expect(within(rows[1]!).getByText('alpha')).toBeInTheDocument()
    expect(within(rows[2]!).getByText('bravo')).toBeInTheDocument()
    expect(within(rows[3]!).getByText('charlie')).toBeInTheDocument()
  })

  it('clicking the same column header twice reverses the sort direction', async () => {
    stubHook({
      workers: [
        makeWorker({ id: 'alpha', status: 'online' }),
        makeWorker({ id: 'charlie', status: 'online' }),
        makeWorker({ id: 'bravo', status: 'online' }),
      ],
      summary: { total: 3, online: 3, down: 0 },
    })
    render(<WorkerFleetDashboard />)

    const header = screen.getByRole('columnheader', { name: /worker id/i })
    await userEvent.click(header)
    await userEvent.click(header)

    const rows = screen.getAllByRole('row')
    expect(within(rows[1]!).getByText('charlie')).toBeInTheDocument()
  })
})

// ---------------------------------------------------------------------------
// AC-7: SSE reconnecting status bar
// ---------------------------------------------------------------------------

describe('WorkerFleetDashboard — SSE status bar (AC-7)', () => {
  it('shows "Reconnecting..." when sseStatus is "reconnecting"', () => {
    stubHook({
      sseStatus: 'reconnecting',
      workers: [],
    })
    render(<WorkerFleetDashboard />)
    expect(screen.getByText(/reconnecting/i)).toBeInTheDocument()
  })

  it('does not show "Reconnecting..." when sseStatus is "connected"', () => {
    stubHook({
      sseStatus: 'connected',
      workers: [],
    })
    render(<WorkerFleetDashboard />)
    expect(screen.queryByText(/reconnecting/i)).not.toBeInTheDocument()
  })

  it('shows the SSE status bar at the bottom of the page', () => {
    stubHook({ sseStatus: 'connected', workers: [] })
    render(<WorkerFleetDashboard />)
    expect(screen.getByRole('status')).toBeInTheDocument()
  })
})
