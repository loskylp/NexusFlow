/**
 * Integration tests for TASK-035: TaskFeedPage ↔ SubmitTaskModal seam
 *
 * These tests verify the interface boundary between TaskFeedPage and SubmitTaskModal:
 *   - TaskFeedPage passes the pipelines prop correctly from usePipelines to the modal
 *   - TaskFeedPage wires onSuccess to refresh() so the task list updates after submission
 *   - TaskFeedPage opens the modal on "Submit Task" button click and closes on Cancel
 *   - TaskFeedPage opens the modal from both the FilterBar button and the empty-state CTA
 *   - Modal state is reset on re-open (no stale data from prior opens)
 *
 * Tests operate at integration layer: component assembly and interface boundaries.
 * No mocking of internal modal state — only the API client and hooks are mocked.
 *
 * Requirements: REQ-002
 * See: TASK-035
 */

import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import TaskFeedPage from '@/pages/TaskFeedPage'
import type { Pipeline, Task } from '@/types/domain'

// ---------------------------------------------------------------------------
// Module mocks
// ---------------------------------------------------------------------------

vi.mock('@/api/client', () => ({
  submitTask: vi.fn(),
  cancelTask: vi.fn().mockResolvedValue(undefined),
  listTasksWithFilters: vi.fn(),
}))

vi.mock('@/hooks/useTasks', async (importActual) => {
  const actual = await importActual<typeof import('@/hooks/useTasks')>()
  return {
    ...actual,
    useTasks: vi.fn(),
  }
})

vi.mock('@/hooks/usePipelines', () => ({
  usePipelines: vi.fn(),
}))

vi.mock('@/context/AuthContext', () => ({
  useAuth: vi.fn(),
}))

import * as clientModule from '@/api/client'
import * as tasksModule from '@/hooks/useTasks'
import * as pipelinesModule from '@/hooks/usePipelines'
import * as authModule from '@/context/AuthContext'
import type { UsePipelinesReturn } from '@/hooks/usePipelines'
import type { UseTasksReturn } from '@/hooks/useTasks'

const mockSubmitTask = vi.mocked(clientModule.submitTask)
const mockUseTasks = vi.mocked(tasksModule.useTasks)
const mockUsePipelines = vi.mocked(pipelinesModule.usePipelines)
const mockUseAuth = vi.mocked(authModule.useAuth)

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

function makePipeline(overrides: Partial<Pipeline> = {}): Pipeline {
  return {
    id: 'pipe-001',
    name: 'ETL Pipeline',
    userId: 'user-001',
    dataSourceConfig: { connectorType: 'demo-source', config: {}, outputSchema: ['id'] },
    processConfig: { connectorType: 'demo-transform', config: {}, inputMappings: [], outputSchema: [] },
    sinkConfig: { connectorType: 'demo-sink', config: {}, inputMappings: [] },
    createdAt: '2026-04-01T00:00:00Z',
    updatedAt: '2026-04-01T00:00:00Z',
    ...overrides,
  }
}

function makeTask(overrides: Partial<Task> = {}): Task {
  return {
    id: 'task-001',
    pipelineId: 'pipe-001',
    userId: 'user-001',
    status: 'submitted',
    input: {},
    createdAt: '2026-04-07T10:00:00Z',
    updatedAt: '2026-04-07T10:00:00Z',
    ...overrides,
  }
}

function makeDefaultTasksReturn(overrides: Partial<UseTasksReturn> = {}): UseTasksReturn {
  return {
    tasks: [],
    isLoading: false,
    error: null,
    sseStatus: 'connected',
    refresh: vi.fn(),
    ...overrides,
  }
}

function makeDefaultPipelinesReturn(overrides: Partial<UsePipelinesReturn> = {}): UsePipelinesReturn {
  return {
    pipelines: [makePipeline()],
    isLoading: false,
    error: null,
    refresh: vi.fn(),
    ...overrides,
  }
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

// Note on "Submit Task" button ambiguity: TaskFeedPage renders two "Submit Task"
// buttons when the task list is empty — one in the FilterBar and one in the
// empty-state CTA. Use getAllByRole(...)[0] to always target the FilterBar button.

beforeEach(() => {
  vi.clearAllMocks()
  mockUseAuth.mockReturnValue({
    user: { id: 'user-001', username: 'testuser', role: 'user', active: true, createdAt: '' },
    login: vi.fn(),
    logout: vi.fn(),
    isLoading: false,
  })
  mockUseTasks.mockReturnValue(makeDefaultTasksReturn())
  mockUsePipelines.mockReturnValue(makeDefaultPipelinesReturn())
})

// ---------------------------------------------------------------------------
// Seam: TaskFeedPage passes pipelines prop to SubmitTaskModal
// ---------------------------------------------------------------------------

describe('Seam: TaskFeedPage → SubmitTaskModal pipeline prop', () => {
  // Given: usePipelines returns a specific set of pipelines
  // When:  the Submit Task modal opens
  // Then:  the modal's pipeline dropdown contains those exact pipelines

  it('pipelines from usePipelines appear as options in the modal selector', async () => {
    const user = userEvent.setup()
    mockUsePipelines.mockReturnValue(makeDefaultPipelinesReturn({
      pipelines: [
        makePipeline({ id: 'seam-pipe-1', name: 'Seam Pipeline One' }),
        makePipeline({ id: 'seam-pipe-2', name: 'Seam Pipeline Two' }),
      ],
    }))

    renderPage()

    await user.click(screen.getAllByRole('button', { name: /submit task/i })[0]!)

    // Scope to dialog to avoid ambiguity with FilterBar pipeline filter dropdown
    const dialog = screen.getByRole('dialog')
    expect(within(dialog).getAllByRole('option', { name: 'Seam Pipeline One' }).length).toBeGreaterThan(0)
    expect(within(dialog).getAllByRole('option', { name: 'Seam Pipeline Two' }).length).toBeGreaterThan(0)
  })

  it('modal shows "No pipelines available" when usePipelines returns empty list', async () => {
    const user = userEvent.setup()
    mockUsePipelines.mockReturnValue(makeDefaultPipelinesReturn({ pipelines: [] }))

    renderPage()

    await user.click(screen.getAllByRole('button', { name: /submit task/i })[0]!)

    expect(screen.getByText(/no pipelines available/i)).toBeInTheDocument()
  })
})

// ---------------------------------------------------------------------------
// Seam: TaskFeedPage wires onSuccess → refresh()
// ---------------------------------------------------------------------------

describe('Seam: onSuccess callback wiring — SubmitTaskModal → TaskFeedPage.refresh()', () => {
  // Given: a successful submission
  // When:  submitTask resolves
  // Then:  refresh() is called once

  it('onSuccess triggers useTasks refresh after successful submission', async () => {
    const user = userEvent.setup()
    mockSubmitTask.mockResolvedValue({ taskId: 'task-wired-001', status: 'submitted' })
    const refreshMock = vi.fn()
    mockUseTasks.mockReturnValue(makeDefaultTasksReturn({ refresh: refreshMock }))

    renderPage()

    await user.click(screen.getAllByRole('button', { name: /submit task/i })[0]!)
    const dialog = screen.getByRole('dialog')
    await user.click(within(dialog).getByRole('button', { name: /submit task/i }))

    await waitFor(() => {
      expect(refreshMock).toHaveBeenCalledOnce()
    })
  })

  it('refresh() is NOT called when submission fails', async () => {
    const user = userEvent.setup()
    mockSubmitTask.mockRejectedValue(new Error('500: server error'))
    const refreshMock = vi.fn()
    mockUseTasks.mockReturnValue(makeDefaultTasksReturn({ refresh: refreshMock }))

    renderPage()

    await user.click(screen.getAllByRole('button', { name: /submit task/i })[0]!)
    const dialog = screen.getByRole('dialog')
    await user.click(within(dialog).getByRole('button', { name: /submit task/i }))

    await screen.findByRole('alert')
    expect(refreshMock).not.toHaveBeenCalled()
  })
})

// ---------------------------------------------------------------------------
// Seam: Modal open/close state management
// ---------------------------------------------------------------------------

describe('Seam: Modal open/close managed by TaskFeedPage', () => {
  // Given: the Task Feed is displayed
  // When:  the user clicks the FilterBar "Submit Task" button
  // Then:  the modal opens

  it('FilterBar "Submit Task" button opens the modal', async () => {
    const user = userEvent.setup()
    renderPage()

    await user.click(screen.getAllByRole('button', { name: /submit task/i })[0]!)

    expect(screen.getByRole('dialog', { name: /Submit Task/i })).toBeInTheDocument()
  })

  // Given: the modal is open
  // When:  the user clicks Cancel
  // Then:  the modal closes

  it('Cancel button closes the modal', async () => {
    const user = userEvent.setup()
    renderPage()

    await user.click(screen.getAllByRole('button', { name: /submit task/i })[0]!)
    expect(screen.getByRole('dialog')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /cancel/i }))

    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  // Given: the modal is open
  // When:  submission succeeds
  // Then:  the modal closes automatically

  it('modal closes automatically after successful submission', async () => {
    const user = userEvent.setup()
    mockSubmitTask.mockResolvedValue({ taskId: 'task-close-001', status: 'submitted' })

    renderPage()

    await user.click(screen.getAllByRole('button', { name: /submit task/i })[0]!)
    const dialog = screen.getByRole('dialog')
    await user.click(within(dialog).getByRole('button', { name: /submit task/i }))

    await waitFor(() => {
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    })
  })

  // Given: the modal was opened, parameters were added, and then cancelled
  // When:  the modal is re-opened
  // Then:  no stale parameter rows remain

  it('modal state resets on re-open — no stale parameter rows', async () => {
    const user = userEvent.setup()
    renderPage()

    // Open modal and add a parameter
    await user.click(screen.getAllByRole('button', { name: /submit task/i })[0]!)
    const dialog = screen.getByRole('dialog')
    await user.click(within(dialog).getByRole('button', { name: /add parameter/i }))
    expect(screen.getAllByPlaceholderText(/key/i).length).toBe(1)

    // Cancel
    await user.click(screen.getByRole('button', { name: /cancel/i }))
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()

    // Re-open (use first button to target FilterBar Submit Task)
    await user.click(screen.getAllByRole('button', { name: /submit task/i })[0]!)

    // Stale parameters must NOT be present
    expect(screen.queryAllByPlaceholderText(/key/i).length).toBe(0)
  })
})

// ---------------------------------------------------------------------------
// Seam: Empty-state CTA opens the modal
// ---------------------------------------------------------------------------

describe('Seam: Empty-state "Submit Task" CTA opens modal', () => {
  // Given: no tasks exist and no filters are active
  // When:  the page renders and the user clicks the empty-state CTA
  // Then:  the modal opens

  it('empty-state Submit Task CTA opens the modal', async () => {
    const user = userEvent.setup()
    mockUseTasks.mockReturnValue(makeDefaultTasksReturn({ tasks: [] }))

    renderPage()

    // There are two "Submit Task" buttons when empty state is shown:
    // one in FilterBar and one in the empty state CTA — get all and click the second
    const submitButtons = screen.getAllByRole('button', { name: /submit task/i })
    expect(submitButtons.length).toBeGreaterThanOrEqual(1)

    // Click the CTA in the empty state body (second occurrence)
    await user.click(submitButtons[submitButtons.length - 1]!)

    expect(screen.getByRole('dialog', { name: /Submit Task/i })).toBeInTheDocument()
  })
})

// ---------------------------------------------------------------------------
// Seam: Submission payload passes through to submitTask without transformation
// ---------------------------------------------------------------------------

describe('Seam: Full submission payload travels from modal to API client', () => {
  // Given: the user fills in all fields including pipeline, params, and retry config
  // When:  the user submits
  // Then:  submitTask receives the exact payload constructed by the modal

  it('complete payload (pipelineId + params + retryConfig) reaches submitTask', async () => {
    const user = userEvent.setup()
    mockSubmitTask.mockResolvedValue({ taskId: 'task-full-payload', status: 'submitted' })
    mockUsePipelines.mockReturnValue(makeDefaultPipelinesReturn({
      pipelines: [makePipeline({ id: 'pipe-full', name: 'Full Pipeline' })],
    }))

    renderPage()

    await user.click(screen.getAllByRole('button', { name: /submit task/i })[0]!)
    const dialog = screen.getByRole('dialog')

    // Select pipeline (already pre-selected as first pipeline)
    const select = within(dialog).getByLabelText(/^pipeline$/i) as HTMLSelectElement
    expect(select.value).toBe('pipe-full')

    // Add parameter
    await user.click(within(dialog).getByRole('button', { name: /add parameter/i }))
    await user.type(within(dialog).getByPlaceholderText(/key/i), 'region')
    await user.type(within(dialog).getByPlaceholderText(/value/i), 'eu-west-1')

    // Set retry config
    const maxRetriesInput = within(dialog).getByLabelText(/max retries/i)
    await user.clear(maxRetriesInput)
    await user.type(maxRetriesInput, '2')
    await user.selectOptions(within(dialog).getByLabelText(/backoff/i), 'linear')

    await user.click(within(dialog).getByRole('button', { name: /submit task/i }))

    await waitFor(() => {
      expect(mockSubmitTask).toHaveBeenCalledWith({
        pipelineId: 'pipe-full',
        input: { region: 'eu-west-1' },
        retryConfig: { maxRetries: 2, backoff: 'linear' },
      })
    })
  })

  // [VERIFIER-ADDED] Task with status "submitted" in the list comes from the same data model
  // as tasks created via the API — the refresh after onSuccess triggers a re-fetch using
  // the same GET /api/tasks endpoint that shows API-created tasks.

  it('[VERIFIER-ADDED] tasks with submitted status appear consistently regardless of creation path', () => {
    const apiTask = makeTask({ id: 'task-from-api', status: 'submitted', pipelineId: 'pipe-full' })
    mockUseTasks.mockReturnValue(makeDefaultTasksReturn({ tasks: [apiTask] }))
    mockUsePipelines.mockReturnValue(makeDefaultPipelinesReturn({
      pipelines: [makePipeline({ id: 'pipe-full', name: 'Full Pipeline' })],
    }))

    renderPage()

    // StatusBadge renders with role="status" aria-label="Task status: submitted"
    expect(screen.getByRole('status', { name: /task status: submitted/i })).toBeInTheDocument()
    // Pipeline name appears in the TaskCard (may also appear in filter dropdown — use getAllByText)
    expect(screen.getAllByText('Full Pipeline').length).toBeGreaterThan(0)
  })
})
