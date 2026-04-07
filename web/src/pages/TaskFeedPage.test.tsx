/**
 * Unit tests for TaskFeedPage and its sub-components.
 * Covers: loading skeleton, empty states, task list rendering,
 * role badge display, SSE status bar, filter bar, and modal trigger.
 *
 * See: TASK-021, REQ-017, REQ-002, REQ-010
 */
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import TaskFeedPage from './TaskFeedPage'
import type { Task } from '@/types/domain'
import type { UseTasksReturn } from '@/hooks/useTasks'

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

vi.mock('@/hooks/useTasks')
vi.mock('@/hooks/usePipelines')
vi.mock('@/context/AuthContext')
vi.mock('@/api/client', () => ({
  cancelTask: vi.fn(),
  submitTask: vi.fn(),
}))

import * as tasksModule from '@/hooks/useTasks'
import * as pipelinesModule from '@/hooks/usePipelines'
import * as authModule from '@/context/AuthContext'
import type { UsePipelinesReturn } from '@/hooks/usePipelines'

const mockUseTasks = vi.mocked(tasksModule.useTasks)
const mockUsePipelines = vi.mocked(pipelinesModule.usePipelines)
const mockUseAuth = vi.mocked(authModule.useAuth)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeTask(overrides: Partial<Task> = {}): Task {
  return {
    id: 'task-001',
    pipelineId: 'pipe-001',
    userId: 'user-001',
    status: 'submitted',
    retryConfig: { maxRetries: 0, backoff: 'fixed' },
    retryCount: 0,
    executionId: 'exec-001',
    input: {},
    createdAt: '2026-04-01T10:00:00Z',
    updatedAt: '2026-04-01T10:00:00Z',
    ...overrides,
  }
}


function stubTasksHook(partial: Partial<UseTasksReturn>): void {
  mockUseTasks.mockReturnValue({
    tasks: [],
    isLoading: false,
    error: null,
    sseStatus: 'connected',
    refresh: vi.fn(),
    ...partial,
  })
}

function stubPipelinesHook(partial: Partial<UsePipelinesReturn>): void {
  mockUsePipelines.mockReturnValue({
    pipelines: [],
    isLoading: false,
    error: null,
    refresh: vi.fn(),
    ...partial,
  })
}

function stubAuth(isAdmin: boolean): void {
  mockUseAuth.mockReturnValue({
    user: { id: 'user-001', username: 'testuser', role: isAdmin ? 'admin' : 'user', active: true, createdAt: '2026-01-01T00:00:00Z' },
    login: vi.fn(),
    logout: vi.fn(),
    isLoading: false,
  })
}

function renderPage() {
  return render(
    <MemoryRouter>
      <TaskFeedPage />
    </MemoryRouter>
  )
}

// ---------------------------------------------------------------------------
// Setup
// ---------------------------------------------------------------------------

beforeEach(() => {
  vi.clearAllMocks()
  stubAuth(false)
  stubTasksHook({})
  stubPipelinesHook({})
})

// ---------------------------------------------------------------------------
// Loading skeleton
// ---------------------------------------------------------------------------

describe('TaskFeedPage — loading skeleton', () => {
  it('renders skeleton cards while the initial fetch is in progress', () => {
    stubTasksHook({ isLoading: true })
    renderPage()
    const skeletons = document.querySelectorAll('[aria-busy="true"]')
    expect(skeletons.length).toBeGreaterThan(0)
  })

  it('does not render task cards while loading', () => {
    stubTasksHook({ isLoading: true })
    renderPage()
    expect(screen.queryByText(/task-001/i)).not.toBeInTheDocument()
  })
})

// ---------------------------------------------------------------------------
// Empty state — no tasks
// ---------------------------------------------------------------------------

describe('TaskFeedPage — empty state (no tasks)', () => {
  it('shows the empty state message when no tasks exist', () => {
    stubTasksHook({ tasks: [], isLoading: false })
    renderPage()
    expect(screen.getByText(/no tasks found/i)).toBeInTheDocument()
  })

  it('shows a Submit Task button in the empty state', () => {
    stubTasksHook({ tasks: [], isLoading: false })
    renderPage()
    // There should be at least one Submit Task button (in empty state or filter bar)
    expect(screen.getAllByText(/submit task/i).length).toBeGreaterThan(0)
  })
})

// ---------------------------------------------------------------------------
// Task list rendering
// ---------------------------------------------------------------------------

describe('TaskFeedPage — task list', () => {
  it('renders a card for each task returned by useTasks', () => {
    stubTasksHook({
      tasks: [
        makeTask({ id: 'task-alpha' }),
        makeTask({ id: 'task-beta' }),
      ],
    })
    renderPage()
    expect(screen.getByText(/task-alpha/i)).toBeInTheDocument()
    expect(screen.getByText(/task-beta/i)).toBeInTheDocument()
  })

  it('renders the page title "Task Feed"', () => {
    stubTasksHook({ tasks: [] })
    renderPage()
    expect(screen.getByText(/task feed/i)).toBeInTheDocument()
  })
})

// ---------------------------------------------------------------------------
// Role indicator badge
// ---------------------------------------------------------------------------

describe('TaskFeedPage — role indicator badge', () => {
  it('shows "Viewing: All Tasks" badge for Admin', () => {
    stubAuth(true)
    stubTasksHook({ tasks: [] })
    renderPage()
    expect(screen.getByText(/viewing.*all tasks/i)).toBeInTheDocument()
  })

  it('shows "Viewing: My Tasks" badge for User', () => {
    stubAuth(false)
    stubTasksHook({ tasks: [] })
    renderPage()
    expect(screen.getByText(/viewing.*my tasks/i)).toBeInTheDocument()
  })
})

// ---------------------------------------------------------------------------
// SSE status bar
// ---------------------------------------------------------------------------

describe('TaskFeedPage — SSE status bar', () => {
  it('renders the status bar', () => {
    stubTasksHook({ tasks: [] })
    renderPage()
    expect(screen.getByRole('status')).toBeInTheDocument()
  })

  it('shows "Reconnecting..." when sseStatus is reconnecting', () => {
    stubTasksHook({ tasks: [], sseStatus: 'reconnecting' })
    renderPage()
    expect(screen.getByText(/reconnecting/i)).toBeInTheDocument()
  })
})

// ---------------------------------------------------------------------------
// Filter bar
// ---------------------------------------------------------------------------

describe('TaskFeedPage — filter bar', () => {
  it('renders the Submit Task button in the filter bar', () => {
    stubTasksHook({ tasks: [makeTask()] })
    renderPage()
    expect(screen.getAllByText(/submit task/i).length).toBeGreaterThan(0)
  })

  it('renders the status filter dropdown', () => {
    renderPage()
    const selects = screen.getAllByRole('combobox')
    expect(selects.length).toBeGreaterThan(0)
  })
})

// ---------------------------------------------------------------------------
// Submit Task modal
// ---------------------------------------------------------------------------

describe('TaskFeedPage — Submit Task modal', () => {
  it('opens the Submit Task modal when the Submit Task button is clicked', async () => {
    const user = userEvent.setup()
    stubTasksHook({ tasks: [] })
    renderPage()

    const submitBtn = screen.getAllByText(/submit task/i)[0]
    await user.click(submitBtn!)

    expect(screen.getByRole('dialog')).toBeInTheDocument()
  })
})

// ---------------------------------------------------------------------------
// Error state
// ---------------------------------------------------------------------------

describe('TaskFeedPage — error state', () => {
  it('shows an error message when the initial fetch fails', () => {
    stubTasksHook({ error: 'Failed to load tasks', isLoading: false })
    renderPage()
    expect(screen.getByText(/failed to load tasks/i)).toBeInTheDocument()
  })
})
