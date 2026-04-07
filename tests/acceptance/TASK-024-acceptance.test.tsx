/**
 * Acceptance tests for TASK-024: Pipeline Management GUI (list/edit/delete)
 * Requirements: REQ-023 (pipeline management GUI), REQ-015 (Pipeline Builder)
 *
 * These tests verify each acceptance criterion from the task plan at the
 * component-acceptance layer. Each criterion has at least one positive case
 * (criterion satisfied) and at least one negative case (invalid condition
 * correctly rejected or not triggered).
 *
 * AC1: Pipeline list shows user's own pipelines (User) or all pipelines (Admin)
 * AC2: Edit action loads the pipeline in the Pipeline Builder canvas
 * AC3: Delete action shows confirmation dialog; on confirm, deletes via API
 * AC4: Delete blocked with explanation when pipeline has active tasks (409 response)
 *
 * Note: PipelineCanvas is NOT mocked here — it renders via its real implementation
 * as in TASK-023 acceptance tests. Canvas state is observed via the pipeline name
 * input field (which reflects the loaded pipeline's name) and mock call assertions.
 *
 * See: TASK-024, REQ-023, REQ-015, REQ-022
 */

import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createMemoryRouter, RouterProvider } from 'react-router-dom'
import PipelineManagerPage from '@/pages/PipelineManagerPage'
import type { Pipeline } from '@/types/domain'
import type { UsePipelinesReturn } from '@/hooks/usePipelines'

// ---------------------------------------------------------------------------
// Module mocks
// ---------------------------------------------------------------------------

vi.mock('@/hooks/usePipelines')
vi.mock('@/context/AuthContext')
vi.mock('@/api/client', () => ({
  getPipeline: vi.fn(),
  deletePipeline: vi.fn(),
  createPipeline: vi.fn(),
  updatePipeline: vi.fn(),
  listPipelines: vi.fn(),
  submitTask: vi.fn(),
}))

// SubmitTaskModal is not under test here.
vi.mock('@/components/SubmitTaskModal', () => ({
  default: () => null,
}))

import * as pipelinesModule from '@/hooks/usePipelines'
import * as authModule from '@/context/AuthContext'
import { getPipeline, deletePipeline } from '@/api/client'

const mockUsePipelines = vi.mocked(pipelinesModule.usePipelines)
const mockUseAuth = vi.mocked(authModule.useAuth)
const mockGetPipeline = vi.mocked(getPipeline)
const mockDeletePipeline = vi.mocked(deletePipeline)

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

function makePipeline(overrides: Partial<Pipeline> = {}): Pipeline {
  return {
    id: 'pipe-001',
    name: 'My Pipeline',
    userId: 'user-001',
    dataSourceConfig: {
      connectorType: 'generic',
      config: {},
      outputSchema: ['fieldA'],
    },
    processConfig: {
      connectorType: 'generic',
      config: {},
      inputMappings: [{ sourceField: 'fieldA', targetField: 'fieldA_in' }],
      outputSchema: ['fieldB'],
    },
    sinkConfig: {
      connectorType: 'generic',
      config: {},
      inputMappings: [{ sourceField: 'fieldB', targetField: 'fieldB_out' }],
    },
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function stubPipelinesHook(partial: Partial<UsePipelinesReturn> = {}): void {
  mockUsePipelines.mockReturnValue({
    pipelines: [],
    isLoading: false,
    error: null,
    refresh: vi.fn(),
    ...partial,
  })
}

function stubAuth(role: 'user' | 'admin' = 'user'): void {
  mockUseAuth.mockReturnValue({
    user: {
      id: 'user-001',
      username: 'testuser',
      role,
      active: true,
      createdAt: '2026-01-01T00:00:00Z',
    },
    login: vi.fn(),
    logout: vi.fn(),
    isLoading: false,
  })
}

function renderPage() {
  const router = createMemoryRouter(
    [{ path: '/pipelines', element: <PipelineManagerPage /> }],
    { initialEntries: ['/pipelines'] }
  )
  return render(<RouterProvider router={router} />)
}

// ---------------------------------------------------------------------------
// Setup
// ---------------------------------------------------------------------------

beforeEach(() => {
  vi.clearAllMocks()
  stubAuth('user')
  stubPipelinesHook()
  mockGetPipeline.mockResolvedValue(makePipeline())
  mockDeletePipeline.mockResolvedValue(undefined)
})

// ---------------------------------------------------------------------------
// AC1: Pipeline list — role-scoped via server, rendered client-side as returned
// REQ-023: Pipeline management GUI; pipeline list is visible and role-appropriate
// ---------------------------------------------------------------------------

describe('AC1 — pipeline list shows role-appropriate pipelines', () => {
  // REQ-023: User role sees own pipelines (server-filtered, hook returns them)
  it('[positive] User role: pipelines returned by usePipelines hook are shown in the list', () => {
    // Given: a logged-in User whose hook returns their own two pipelines
    stubAuth('user')
    stubPipelinesHook({
      pipelines: [
        makePipeline({ id: 'pipe-u1', name: 'My ETL Pipeline', userId: 'user-001' }),
        makePipeline({ id: 'pipe-u2', name: 'My Report Pipeline', userId: 'user-001' }),
      ],
    })

    // When: the user views the Pipeline Builder page
    renderPage()

    // Then: both pipelines appear in the saved pipelines list
    expect(screen.getByText('My ETL Pipeline')).toBeInTheDocument()
    expect(screen.getByText('My Report Pipeline')).toBeInTheDocument()
  })

  // REQ-023: Admin role sees all pipelines (server returns all; frontend renders all)
  it('[positive] Admin role: all pipelines from the hook are rendered without client-side filtering', () => {
    // Given: a logged-in Admin whose hook returns pipelines from multiple users
    stubAuth('admin')
    stubPipelinesHook({
      pipelines: [
        makePipeline({ id: 'pipe-u1', name: 'User-A Pipeline', userId: 'user-001' }),
        makePipeline({ id: 'pipe-u2', name: 'User-B Pipeline', userId: 'user-002' }),
        makePipeline({ id: 'pipe-u3', name: 'User-C Pipeline', userId: 'user-003' }),
      ],
    })

    // When: the admin views the Pipeline Builder page
    renderPage()

    // Then: all three pipelines are visible
    expect(screen.getByText('User-A Pipeline')).toBeInTheDocument()
    expect(screen.getByText('User-B Pipeline')).toBeInTheDocument()
    expect(screen.getByText('User-C Pipeline')).toBeInTheDocument()
  })

  // [VERIFIER-ADDED] Negative: empty state is shown when hook returns no pipelines
  it('[negative] Empty state shown when no pipelines are available; no phantom pipeline entries rendered', () => {
    // Given: a User whose hook returns an empty list
    stubPipelinesHook({ pipelines: [] })

    // When: the page renders
    renderPage()

    // Then: "No saved pipelines" message appears; no delete buttons for pipelines exist
    expect(screen.getByText(/no saved pipelines/i)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /delete pipeline/i })).not.toBeInTheDocument()
  })

  // [VERIFIER-ADDED] Negative: loading state is shown while pipelines are being fetched
  it('[negative] Loading state is shown while pipelines are being fetched (list not prematurely shown as empty)', () => {
    // Given: pipelines are still loading
    stubPipelinesHook({ isLoading: true, pipelines: [] })

    // When: the page renders
    renderPage()

    // Then: "Loading..." is shown, not "No saved pipelines"
    expect(screen.getByText(/loading/i)).toBeInTheDocument()
    expect(screen.queryByText(/no saved pipelines/i)).not.toBeInTheDocument()
  })
})

// ---------------------------------------------------------------------------
// AC2: Edit action loads pipeline onto canvas
// REQ-023: Clicking a pipeline in the list fetches it and populates the canvas
// ---------------------------------------------------------------------------

describe('AC2 — edit action loads pipeline in the Pipeline Builder canvas', () => {
  // REQ-023: Clicking a pipeline name calls getPipeline with the correct ID
  it('[positive] Clicking a pipeline name fetches the pipeline via API with the correct ID', async () => {
    // Given: a logged-in user with one saved pipeline in the list
    const user = userEvent.setup()
    const pipeline = makePipeline({ id: 'pipe-edit', name: 'Sales Report Pipeline' })
    stubPipelinesHook({ pipelines: [pipeline] })
    mockGetPipeline.mockResolvedValue(pipeline)

    // When: the user clicks the pipeline name
    renderPage()
    await user.click(screen.getByText('Sales Report Pipeline'))

    // Then: getPipeline is called with the correct pipeline ID
    expect(mockGetPipeline).toHaveBeenCalledWith('pipe-edit')
  })

  // REQ-023: After load, the pipeline name input reflects the loaded pipeline's name
  it('[positive] Pipeline name input is updated with the fetched pipeline name after load', async () => {
    // Given: a pipeline with a specific name
    const user = userEvent.setup()
    const pipeline = makePipeline({ id: 'pipe-edit', name: 'Quarterly Analysis Pipeline' })
    stubPipelinesHook({ pipelines: [pipeline] })
    mockGetPipeline.mockResolvedValue(pipeline)

    // When: the user loads the pipeline
    renderPage()
    await user.click(screen.getByText('Quarterly Analysis Pipeline'))

    // Then: the name input reflects the loaded pipeline name (canvas was populated)
    await waitFor(() => {
      const nameInput = screen.getByRole('textbox', { name: /pipeline name/i })
      expect((nameInput as HTMLInputElement).value).toBe('Quarterly Analysis Pipeline')
    })
  })

  // [VERIFIER-ADDED] Negative: failed load shows error toast and does not update name field
  it('[negative] Failed pipeline load shows error toast; pipeline name field remains empty', async () => {
    // Given: getPipeline will reject with a 404
    const user = userEvent.setup()
    const pipeline = makePipeline({ id: 'pipe-missing', name: 'Missing Pipeline' })
    stubPipelinesHook({ pipelines: [pipeline] })
    mockGetPipeline.mockRejectedValue(new Error('404: pipeline not found'))

    // When: the user clicks the pipeline
    renderPage()
    await user.click(screen.getByText('Missing Pipeline'))

    // Then: an error toast is shown
    await waitFor(() => {
      expect(screen.getByRole('alert')).toBeInTheDocument()
    })
    expect(screen.getByText(/404: pipeline not found/i)).toBeInTheDocument()

    // And: the name input remains empty (canvas was not populated)
    const nameInput = screen.getByRole('textbox', { name: /pipeline name/i })
    expect((nameInput as HTMLInputElement).value).toBe('')
  })

  // [VERIFIER-ADDED] Negative: the correct ID is fetched when selecting from a multi-pipeline list
  it('[negative] Correct pipeline ID is fetched when second pipeline selected from a multi-pipeline list', async () => {
    // Given: two pipelines in the list
    const user = userEvent.setup()
    const pipe1 = makePipeline({ id: 'pipe-001', name: 'First Pipeline' })
    const pipe2 = makePipeline({ id: 'pipe-002', name: 'Second Pipeline' })
    stubPipelinesHook({ pipelines: [pipe1, pipe2] })
    mockGetPipeline.mockResolvedValue(pipe2)

    // When: the user clicks the second pipeline
    renderPage()
    await user.click(screen.getByText('Second Pipeline'))

    // Then: only pipe-002 is fetched, not pipe-001
    expect(mockGetPipeline).toHaveBeenCalledWith('pipe-002')
    expect(mockGetPipeline).not.toHaveBeenCalledWith('pipe-001')
  })
})

// ---------------------------------------------------------------------------
// AC3: Delete action — confirmation dialog then API call on confirm
// REQ-023: Delete requires an inline confirmation before calling the API
// ---------------------------------------------------------------------------

describe('AC3 — delete shows confirmation dialog; on confirm, deletes via API', () => {
  // REQ-023: Clicking the delete icon shows an inline confirmation
  it('[positive] Clicking the delete icon reveals an inline confirmation prompt with pipeline name', async () => {
    // Given: a pipeline in the list
    const user = userEvent.setup()
    const pipeline = makePipeline({ id: 'pipe-del', name: 'Deletable Pipeline' })
    stubPipelinesHook({ pipelines: [pipeline] })

    // When: the user clicks the delete icon
    renderPage()
    await user.click(screen.getByRole('button', { name: /delete pipeline Deletable Pipeline/i }))

    // Then: a confirmation prompt appears with the pipeline name
    expect(screen.getByText(/delete "Deletable Pipeline"/i)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /^delete$/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /cancel/i })).toBeInTheDocument()
  })

  // REQ-023: Confirming the deletion calls deletePipeline with the correct ID
  it('[positive] Confirming deletion calls deletePipeline API with the correct pipeline ID', async () => {
    // Given: a pipeline in the list
    const user = userEvent.setup()
    const pipeline = makePipeline({ id: 'pipe-del', name: 'Confirmed Delete' })
    stubPipelinesHook({ pipelines: [pipeline] })
    mockDeletePipeline.mockResolvedValue(undefined)

    // When: the user clicks delete then confirms
    renderPage()
    await user.click(screen.getByRole('button', { name: /delete pipeline Confirmed Delete/i }))
    await user.click(screen.getByRole('button', { name: /^delete$/i }))

    // Then: the API is called with the correct ID
    await waitFor(() => {
      expect(mockDeletePipeline).toHaveBeenCalledWith('pipe-del')
    })
  })

  // REQ-023: After successful deletion, a success toast is shown and the list is refreshed
  it('[positive] Successful deletion shows success toast and triggers pipeline list refresh', async () => {
    // Given: a pipeline with a refresh mock
    const user = userEvent.setup()
    const refreshMock = vi.fn()
    const pipeline = makePipeline({ id: 'pipe-del', name: 'Deleted Pipeline' })
    stubPipelinesHook({ pipelines: [pipeline], refresh: refreshMock })
    mockDeletePipeline.mockResolvedValue(undefined)

    // When: the user confirms deletion
    renderPage()
    await user.click(screen.getByRole('button', { name: /delete pipeline Deleted Pipeline/i }))
    await user.click(screen.getByRole('button', { name: /^delete$/i }))

    // Then: success toast appears and the pipeline list is refreshed
    await waitFor(() => {
      expect(screen.getByText(/pipeline deleted/i)).toBeInTheDocument()
    })
    expect(refreshMock).toHaveBeenCalled()
  })

  // REQ-023: Cancelling the confirmation does NOT call the delete API
  it('[negative] Cancelling the confirmation does not call deletePipeline', async () => {
    // Given: a pipeline in the list
    const user = userEvent.setup()
    const pipeline = makePipeline({ id: 'pipe-del', name: 'Cancelled Delete' })
    stubPipelinesHook({ pipelines: [pipeline] })

    // When: the user clicks the delete icon but then cancels
    renderPage()
    await user.click(screen.getByRole('button', { name: /delete pipeline Cancelled Delete/i }))
    await user.click(screen.getByRole('button', { name: /cancel/i }))

    // Then: the API is never called
    expect(mockDeletePipeline).not.toHaveBeenCalled()
  })

  // [VERIFIER-ADDED] Negative: clicking the delete icon alone does NOT immediately delete
  it('[negative] Clicking the delete icon does not immediately invoke deletePipeline — confirmation is mandatory', async () => {
    // Given: a pipeline in the list
    const user = userEvent.setup()
    const pipeline = makePipeline({ id: 'pipe-del', name: 'Guarded Pipeline' })
    stubPipelinesHook({ pipelines: [pipeline] })

    // When: the user clicks the delete icon (but does not confirm)
    renderPage()
    await user.click(screen.getByRole('button', { name: /delete pipeline Guarded Pipeline/i }))

    // Then: deletePipeline is not called; only the confirmation prompt is shown
    expect(mockDeletePipeline).not.toHaveBeenCalled()
    expect(screen.getByText(/delete "Guarded Pipeline"/i)).toBeInTheDocument()
  })
})

// ---------------------------------------------------------------------------
// AC4: Delete blocked with explanation when pipeline has active tasks (409)
// REQ-023: 409 response produces a domain-specific explanation; no list refresh
// ---------------------------------------------------------------------------

describe('AC4 — delete blocked with explanation when pipeline has active tasks', () => {
  // REQ-023: 409 from deletePipeline shows an active-tasks explanation toast
  it('[positive] 409 response shows the active-tasks explanation toast', async () => {
    // Given: a pipeline with active tasks that returns 409 on delete
    const user = userEvent.setup()
    const pipeline = makePipeline({ id: 'pipe-active', name: 'Active Pipeline' })
    stubPipelinesHook({ pipelines: [pipeline] })
    mockDeletePipeline.mockRejectedValue(new Error('409: pipeline has active tasks'))

    // When: the user attempts to confirm deletion
    renderPage()
    await user.click(screen.getByRole('button', { name: /delete pipeline Active Pipeline/i }))
    await user.click(screen.getByRole('button', { name: /^delete$/i }))

    // Then: the active-tasks explanation toast is shown
    await waitFor(() => {
      expect(screen.getByRole('alert')).toBeInTheDocument()
    })
    expect(screen.getByText(/cannot delete pipeline: it has active tasks/i)).toBeInTheDocument()
  })

  // REQ-023: 409 does not refresh the list (no deletion occurred)
  it('[positive] 409 does not trigger a pipeline list refresh', async () => {
    // Given: a pipeline with active tasks
    const user = userEvent.setup()
    const refreshMock = vi.fn()
    const pipeline = makePipeline({ id: 'pipe-active', name: 'Active Pipeline' })
    stubPipelinesHook({ pipelines: [pipeline], refresh: refreshMock })
    mockDeletePipeline.mockRejectedValue(new Error('409: pipeline has active tasks'))

    // When: the user confirms deletion and receives a 409
    renderPage()
    await user.click(screen.getByRole('button', { name: /delete pipeline Active Pipeline/i }))
    await user.click(screen.getByRole('button', { name: /^delete$/i }))

    // Then: the list is not refreshed (pipeline was not deleted)
    await waitFor(() => {
      expect(screen.getByText(/cannot delete pipeline/i)).toBeInTheDocument()
    })
    expect(refreshMock).not.toHaveBeenCalled()
  })

  // [VERIFIER-ADDED] Negative: non-409 errors show a different message, not the active-tasks explanation
  it('[negative] Non-409 deletion failure shows raw error message, not the active-tasks explanation', async () => {
    // Given: a server error (500) on deletion
    const user = userEvent.setup()
    const pipeline = makePipeline({ id: 'pipe-err', name: 'Error Pipeline' })
    stubPipelinesHook({ pipelines: [pipeline] })
    mockDeletePipeline.mockRejectedValue(new Error('500: internal server error'))

    // When: the user confirms deletion and receives a 500
    renderPage()
    await user.click(screen.getByRole('button', { name: /delete pipeline Error Pipeline/i }))
    await user.click(screen.getByRole('button', { name: /^delete$/i }))

    // Then: the raw error appears, NOT the active-tasks message
    await waitFor(() => {
      expect(screen.getByRole('alert')).toBeInTheDocument()
    })
    expect(screen.queryByText(/cannot delete pipeline: it has active tasks/i)).not.toBeInTheDocument()
    expect(screen.getByText(/500: internal server error/i)).toBeInTheDocument()
  })

  // [VERIFIER-ADDED] Negative: the 409 toast message includes "active tasks" (domain-correct explanation, not generic failure)
  it('[negative] The 409 toast specifically mentions active tasks (domain-correct, not a generic failure message)', async () => {
    // Given: a 409 response
    const user = userEvent.setup()
    const pipeline = makePipeline({ id: 'pipe-active-2', name: 'Busy Pipeline' })
    stubPipelinesHook({ pipelines: [pipeline] })
    mockDeletePipeline.mockRejectedValue(new Error('409: pipeline has active tasks'))

    // When: the user confirms deletion
    renderPage()
    await user.click(screen.getByRole('button', { name: /delete pipeline Busy Pipeline/i }))
    await user.click(screen.getByRole('button', { name: /^delete$/i }))

    // Then: the toast includes "active tasks" — domain-specific explanation, not just "delete failed"
    await waitFor(() => {
      const alert = screen.getByRole('alert')
      expect(alert.textContent).toMatch(/active tasks/i)
    })
  })
})
