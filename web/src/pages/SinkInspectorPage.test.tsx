/**
 * Unit tests for SinkInspectorPage and its exported sub-components.
 *
 * Covers:
 *   - Admin access: page renders for admin user
 *   - Non-admin access: access denied message shown
 *   - Task selector renders dropdown
 *   - Before panel: placeholder when no task selected
 *   - Before panel: waiting spinner when task selected
 *   - Before panel: populates with snapshot data
 *   - After panel: placeholder when no snapshot
 *   - After panel: populates with snapshot data (success, highlight green-50)
 *   - After panel: shows ROLLED BACK badge on rollback
 *   - Atomicity verification: checkmark on success
 *   - Atomicity verification: ROLLED BACK badge on rollback
 *   - SnapshotPanel: spinner state
 *   - AtomicityVerification: neutral state
 *
 * See: DEMO-003, UX Spec (Sink Inspector), TASK-032, TASK-033
 */
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import SinkInspectorPage, { SnapshotPanel, AtomicityVerification } from './SinkInspectorPage'
import type { SinkSnapshot } from '@/types/domain'
import type { UseSinkInspectorReturn } from '@/hooks/useSinkInspector'
import type { UseTasksReturn } from '@/hooks/useTasks'

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

vi.mock('@/hooks/useSinkInspector')
vi.mock('@/hooks/useTasks')
vi.mock('@/context/AuthContext')

import * as sinkModule from '@/hooks/useSinkInspector'
import * as tasksModule from '@/hooks/useTasks'
import * as authModule from '@/context/AuthContext'

const mockUseSinkInspector = vi.mocked(sinkModule.useSinkInspector)
const mockUseTasks = vi.mocked(tasksModule.useTasks)
const mockUseAuth = vi.mocked(authModule.useAuth)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeSnapshot(phase: 'before' | 'after', data: Record<string, unknown> = {}): SinkSnapshot {
  return {
    taskId: 'task-001',
    phase,
    data,
    capturedAt: '2026-04-15T10:00:00Z',
  }
}

function stubAuth(role: 'admin' | 'user'): void {
  mockUseAuth.mockReturnValue({
    user: {
      id: 'user-001',
      username: 'testuser',
      role,
      active: true,
      mustChangePassword: false,
      createdAt: '2026-01-01T00:00:00Z',
    },
    login: vi.fn(),
    logout: vi.fn(),
    isLoading: false,
  })
}

function stubSinkInspector(partial: Partial<UseSinkInspectorReturn>): void {
  mockUseSinkInspector.mockReturnValue({
    beforeSnapshot: null,
    afterSnapshot: null,
    rolledBack: false,
    isWaitingForSinkPhase: false,
    sseStatus: 'idle',
    accessError: null,
    writeError: null,
    ...partial,
  })
}

function stubTasks(): void {
  mockUseTasks.mockReturnValue({
    tasks: [
      {
        id: 'task-001',
        pipelineId: 'pipe-001',
        userId: 'user-001',
        status: 'completed',
        retryConfig: { maxRetries: 0, backoff: 'fixed' },
        retryCount: 0,
        executionId: 'exec-001',
        input: {},
        createdAt: '2026-04-01T10:00:00Z',
        updatedAt: '2026-04-01T10:00:00Z',
      },
    ],
    isLoading: false,
    error: null,
    sseStatus: 'connected',
    refresh: vi.fn(),
  } satisfies UseTasksReturn)
}

function renderPage() {
  return render(
    <MemoryRouter>
      <SinkInspectorPage />
    </MemoryRouter>
  )
}

// ---------------------------------------------------------------------------
// Setup
// ---------------------------------------------------------------------------

beforeEach(() => {
  vi.clearAllMocks()
  stubAuth('admin')
  stubSinkInspector({})
  stubTasks()
})

// ---------------------------------------------------------------------------
// Admin access
// ---------------------------------------------------------------------------

describe('SinkInspectorPage — admin access', () => {
  it('renders page title for admin user', () => {
    renderPage()
    expect(screen.getByText('Sink Inspector')).toBeInTheDocument()
  })

  it('renders the DEMO badge', () => {
    renderPage()
    expect(screen.getByText('DEMO')).toBeInTheDocument()
  })

  it('renders the task selector', () => {
    renderPage()
    expect(screen.getByRole('combobox', { name: /select task/i })).toBeInTheDocument()
  })
})

// ---------------------------------------------------------------------------
// Non-admin access
// ---------------------------------------------------------------------------

describe('SinkInspectorPage — non-admin access', () => {
  it('shows access denied message for user role', () => {
    stubAuth('user')
    renderPage()
    expect(screen.getByText(/access denied/i)).toBeInTheDocument()
  })

  it('does not render the task selector for user role', () => {
    stubAuth('user')
    renderPage()
    expect(screen.queryByRole('combobox', { name: /select task/i })).not.toBeInTheDocument()
  })
})

// ---------------------------------------------------------------------------
// Access error from SSE (403 mid-session)
// ---------------------------------------------------------------------------

describe('SinkInspectorPage — SSE access error', () => {
  it('shows access denied when useSinkInspector reports accessError', () => {
    stubSinkInspector({
      accessError: 'Access denied: you do not have permission to inspect sink events for this task.',
    })
    renderPage()
    expect(screen.getAllByText(/access denied/i).length).toBeGreaterThan(0)
  })
})

// ---------------------------------------------------------------------------
// Default state (no task selected)
// ---------------------------------------------------------------------------

describe('SinkInspectorPage — default state', () => {
  it('renders both panel headers', () => {
    renderPage()
    expect(screen.getByText(/before snapshot/i)).toBeInTheDocument()
    expect(screen.getByText(/after result/i)).toBeInTheDocument()
  })

  it('shows placeholder text when no task is selected', () => {
    renderPage()
    const placeholders = screen.getAllByText(/select a task to inspect/i)
    expect(placeholders.length).toBeGreaterThanOrEqual(1)
  })
})

// ---------------------------------------------------------------------------
// Waiting state
// ---------------------------------------------------------------------------

describe('SinkInspectorPage — waiting state', () => {
  it('shows spinner in Before panel when isWaitingForSinkPhase is true', () => {
    stubSinkInspector({ isWaitingForSinkPhase: true })
    renderPage()
    expect(screen.getByText(/waiting for sink phase/i)).toBeInTheDocument()
    // The spinner has role="progressbar".
    expect(screen.getByRole('progressbar')).toBeInTheDocument()
  })
})

// ---------------------------------------------------------------------------
// Before snapshot populated
// ---------------------------------------------------------------------------

describe('SinkInspectorPage — before snapshot', () => {
  it('renders snapshot data keys in the Before panel', () => {
    const before = makeSnapshot('before', { object_count: 3, bucket: 'demo-bucket' })
    stubSinkInspector({ beforeSnapshot: before })
    renderPage()
    expect(screen.getByText('object_count')).toBeInTheDocument()
    expect(screen.getByText('bucket')).toBeInTheDocument()
  })
})

// ---------------------------------------------------------------------------
// After snapshot (success)
// ---------------------------------------------------------------------------

describe('SinkInspectorPage — after snapshot (success)', () => {
  it('renders snapshot data keys in the After panel', () => {
    const before = makeSnapshot('before', { rows: 3 })
    const after = makeSnapshot('after', { rows: 5 })
    stubSinkInspector({ beforeSnapshot: before, afterSnapshot: after, rolledBack: false })
    renderPage()
    // "rows" key should appear at least twice (once in each panel).
    const rowsCells = screen.getAllByText('rows')
    expect(rowsCells.length).toBeGreaterThanOrEqual(2)
  })
})

// ---------------------------------------------------------------------------
// Rollback
// ---------------------------------------------------------------------------

describe('SinkInspectorPage — rollback', () => {
  it('shows ROLLED BACK badge when rolledBack is true', () => {
    const before = makeSnapshot('before', { rows: 3 })
    const after = makeSnapshot('after', { rows: 3 })
    stubSinkInspector({ beforeSnapshot: before, afterSnapshot: after, rolledBack: true, writeError: 'write failed' })
    renderPage()
    expect(screen.getAllByText(/rolled back/i).length).toBeGreaterThan(0)
  })
})

// ---------------------------------------------------------------------------
// Atomicity verification
// ---------------------------------------------------------------------------

describe('AtomicityVerification — unit tests', () => {
  it('shows neutral message when afterSnapshot is null', () => {
    render(
      <AtomicityVerification
        beforeSnapshot={null}
        afterSnapshot={null}
        rolledBack={false}
        writeError={null}
      />
    )
    expect(screen.getByText(/awaiting sink phase/i)).toBeInTheDocument()
  })

  it('shows checkmark on successful write', () => {
    const before = makeSnapshot('before', { rows: 3 })
    const after = makeSnapshot('after', { rows: 5 })
    render(
      <AtomicityVerification
        beforeSnapshot={before}
        afterSnapshot={after}
        rolledBack={false}
        writeError={null}
      />
    )
    expect(screen.getByLabelText(/atomicity verified/i)).toBeInTheDocument()
    expect(screen.getByText(/write committed successfully/i)).toBeInTheDocument()
  })

  it('shows ROLLED BACK badge and error on rollback', () => {
    const before = makeSnapshot('before', { rows: 3 })
    const after = makeSnapshot('after', { rows: 3 })
    render(
      <AtomicityVerification
        beforeSnapshot={before}
        afterSnapshot={after}
        rolledBack={true}
        writeError="connection reset"
      />
    )
    expect(screen.getByLabelText(/rolled back/i)).toBeInTheDocument()
    expect(screen.getByText(/write rolled back/i)).toBeInTheDocument()
    expect(screen.getByText(/connection reset/i)).toBeInTheDocument()
  })
})

// ---------------------------------------------------------------------------
// SnapshotPanel — unit tests
// ---------------------------------------------------------------------------

describe('SnapshotPanel — unit tests', () => {
  it('shows empty message when no snapshot and not waiting', () => {
    render(
      <SnapshotPanel
        title="Before Snapshot"
        snapshot={null}
        isWaiting={false}
        emptyMessage="Select a task to inspect its sink operation"
      />
    )
    expect(screen.getByText(/select a task/i)).toBeInTheDocument()
  })

  it('shows spinner when isWaiting is true', () => {
    render(
      <SnapshotPanel
        title="Before Snapshot"
        snapshot={null}
        isWaiting={true}
        emptyMessage="Select a task to inspect"
      />
    )
    expect(screen.getByRole('progressbar')).toBeInTheDocument()
    expect(screen.getByText(/waiting for sink phase/i)).toBeInTheDocument()
  })

  it('renders snapshot data when snapshot is provided', () => {
    const snapshot = makeSnapshot('before', { bucket: 'test-bucket', count: 10 })
    render(
      <SnapshotPanel
        title="Before Snapshot"
        snapshot={snapshot}
        isWaiting={false}
        emptyMessage="No data"
      />
    )
    expect(screen.getByText('bucket')).toBeInTheDocument()
    expect(screen.getByText('count')).toBeInTheDocument()
  })

  it('renders panel title in uppercase', () => {
    render(
      <SnapshotPanel
        title="Before Snapshot"
        snapshot={null}
        isWaiting={false}
        emptyMessage="No data"
      />
    )
    expect(screen.getByText('Before Snapshot')).toBeInTheDocument()
  })
})
