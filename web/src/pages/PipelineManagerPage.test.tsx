/**
 * Unit tests for PipelineManagerPage — pipeline management functionality (TASK-024).
 *
 * Covers:
 *   AC1: Pipeline list renders pipelines returned by usePipelines (role-filtered server-side)
 *   AC2: Edit action fetches pipeline by ID and loads it onto the canvas
 *   AC3: Delete action shows inline confirmation; on confirm calls deletePipeline
 *   AC4: Delete blocked with toast explanation when API returns 409
 *
 * See: TASK-024, REQ-023
 */
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createMemoryRouter, RouterProvider } from 'react-router-dom'
import PipelineManagerPage from './PipelineManagerPage'
import type { Pipeline } from '@/types/domain'
import type { UsePipelinesReturn } from '@/hooks/usePipelines'

// ---------------------------------------------------------------------------
// Mocks
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

// PipelineCanvas is complex (dnd-kit, schema mapping). Replace with a minimal
// stub so this test suite stays focused on management behaviour.
vi.mock('@/components/PipelineCanvas', () => {
  const CANVAS_DROP_ID = 'pipeline-canvas-drop'

  function PipelineCanvas() {
    return <div data-testid="pipeline-canvas" />
  }

  function DraggablePaletteCard({ phase }: { phase: string }) {
    return <div data-testid={`palette-card-${phase}`} />
  }

  function applyPhaseDropToState() {
    return 'stub'
  }

  return {
    default: PipelineCanvas,
    DraggablePaletteCard,
    applyPhaseDropToState,
    CANVAS_DROP_ID,
  }
})

// SubmitTaskModal — not under test here.
vi.mock('@/components/SubmitTaskModal', () => {
  function SubmitTaskModal() {
    return null
  }
  return { default: SubmitTaskModal }
})

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

// ---------------------------------------------------------------------------
// Stub helpers
// ---------------------------------------------------------------------------

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
      mustChangePassword: false,
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
  stubAuth()
  stubPipelinesHook()
  mockGetPipeline.mockResolvedValue(makePipeline())
  mockDeletePipeline.mockResolvedValue(undefined)
})

// ---------------------------------------------------------------------------
// AC1: Pipeline list
// ---------------------------------------------------------------------------

describe('AC1 — pipeline list', () => {
  it('renders pipeline names from usePipelines', () => {
    const pipelines = [
      makePipeline({ id: 'pipe-001', name: 'Alpha Pipeline' }),
      makePipeline({ id: 'pipe-002', name: 'Beta Pipeline' }),
    ]
    stubPipelinesHook({ pipelines })

    renderPage()

    expect(screen.getByText('Alpha Pipeline')).toBeInTheDocument()
    expect(screen.getByText('Beta Pipeline')).toBeInTheDocument()
  })

  it('shows loading indicator while pipelines are fetching', () => {
    stubPipelinesHook({ isLoading: true, pipelines: [] })

    renderPage()

    expect(screen.getByText(/loading/i)).toBeInTheDocument()
  })

  it('shows empty state when there are no saved pipelines', () => {
    stubPipelinesHook({ pipelines: [] })

    renderPage()

    expect(screen.getByText(/no saved pipelines/i)).toBeInTheDocument()
  })

  it('renders all pipelines returned by the hook (admin sees all, user sees own — server-enforced)', () => {
    // Both admin and user roles receive whatever the hook returns.
    // Role filtering is server-side; the frontend renders the full list.
    const adminPipelines = [
      makePipeline({ id: 'pipe-001', name: 'User Pipeline', userId: 'user-001' }),
      makePipeline({ id: 'pipe-002', name: 'Other User Pipeline', userId: 'user-002' }),
    ]
    stubAuth('admin')
    stubPipelinesHook({ pipelines: adminPipelines })

    renderPage()

    expect(screen.getByText('User Pipeline')).toBeInTheDocument()
    expect(screen.getByText('Other User Pipeline')).toBeInTheDocument()
  })
})

// ---------------------------------------------------------------------------
// AC2: Edit action — load pipeline onto canvas
// ---------------------------------------------------------------------------

describe('AC2 — edit action loads pipeline onto canvas', () => {
  it('calls getPipeline with the correct ID when a pipeline name is clicked', async () => {
    const user = userEvent.setup()
    const pipeline = makePipeline({ id: 'pipe-001', name: 'Load Me' })
    stubPipelinesHook({ pipelines: [pipeline] })

    renderPage()

    // The load button shows the pipeline name as its text content.
    await user.click(screen.getByText('Load Me'))

    expect(mockGetPipeline).toHaveBeenCalledWith('pipe-001')
  })

  it('updates the pipeline name input after loading a pipeline', async () => {
    const user = userEvent.setup()
    const pipeline = makePipeline({ id: 'pipe-001', name: 'Loaded Pipeline' })
    stubPipelinesHook({ pipelines: [pipeline] })
    mockGetPipeline.mockResolvedValue(pipeline)

    renderPage()

    await user.click(screen.getByText('Loaded Pipeline'))

    await waitFor(() => {
      const nameInput = screen.getByRole('textbox', { name: /pipeline name/i })
      expect((nameInput as HTMLInputElement).value).toBe('Loaded Pipeline')
    })
  })

  it('shows a toast error when getPipeline fails', async () => {
    const user = userEvent.setup()
    const pipeline = makePipeline({ id: 'pipe-001', name: 'Failing Pipeline' })
    stubPipelinesHook({ pipelines: [pipeline] })
    mockGetPipeline.mockRejectedValue(new Error('404: pipeline not found'))

    renderPage()

    await user.click(screen.getByText('Failing Pipeline'))

    await waitFor(() => {
      expect(screen.getByRole('alert')).toBeInTheDocument()
    })
    expect(screen.getByText(/404: pipeline not found/i)).toBeInTheDocument()
  })
})

// ---------------------------------------------------------------------------
// AC3: Delete action — confirmation dialog then API call
// ---------------------------------------------------------------------------

describe('AC3 — delete action shows confirmation and calls API on confirm', () => {
  it('shows an inline confirmation prompt when the delete button is clicked', async () => {
    const user = userEvent.setup()
    const pipeline = makePipeline({ id: 'pipe-001', name: 'Delete Me' })
    stubPipelinesHook({ pipelines: [pipeline] })

    renderPage()

    await user.click(screen.getByRole('button', { name: /delete pipeline Delete Me/i }))

    expect(screen.getByText(/delete "Delete Me"/i)).toBeInTheDocument()
  })

  it('calls deletePipeline with the correct ID when the user confirms deletion', async () => {
    const user = userEvent.setup()
    const refreshMock = vi.fn()
    const pipeline = makePipeline({ id: 'pipe-001', name: 'Confirm Delete' })
    stubPipelinesHook({ pipelines: [pipeline], refresh: refreshMock })
    mockDeletePipeline.mockResolvedValue(undefined)

    renderPage()

    // Click the delete icon to reveal inline confirmation.
    await user.click(screen.getByRole('button', { name: /delete pipeline Confirm Delete/i }))

    // Click the confirm "Delete" button in the inline dialog.
    await user.click(screen.getByRole('button', { name: /^delete$/i }))

    await waitFor(() => {
      expect(mockDeletePipeline).toHaveBeenCalledWith('pipe-001')
    })
  })

  it('refreshes the pipeline list after successful deletion', async () => {
    const user = userEvent.setup()
    const refreshMock = vi.fn()
    const pipeline = makePipeline({ id: 'pipe-001', name: 'To Be Deleted' })
    stubPipelinesHook({ pipelines: [pipeline], refresh: refreshMock })
    mockDeletePipeline.mockResolvedValue(undefined)

    renderPage()

    await user.click(screen.getByRole('button', { name: /delete pipeline To Be Deleted/i }))
    await user.click(screen.getByRole('button', { name: /^delete$/i }))

    await waitFor(() => {
      expect(refreshMock).toHaveBeenCalled()
    })
  })

  it('does NOT call deletePipeline when the user cancels the confirmation', async () => {
    const user = userEvent.setup()
    const pipeline = makePipeline({ id: 'pipe-001', name: 'Cancel Delete' })
    stubPipelinesHook({ pipelines: [pipeline] })

    renderPage()

    await user.click(screen.getByRole('button', { name: /delete pipeline Cancel Delete/i }))
    await user.click(screen.getByRole('button', { name: /cancel/i }))

    expect(mockDeletePipeline).not.toHaveBeenCalled()
  })

  it('shows a success toast after deletion', async () => {
    const user = userEvent.setup()
    const pipeline = makePipeline({ id: 'pipe-001', name: 'Gone Pipeline' })
    stubPipelinesHook({ pipelines: [pipeline] })
    mockDeletePipeline.mockResolvedValue(undefined)

    renderPage()

    await user.click(screen.getByRole('button', { name: /delete pipeline Gone Pipeline/i }))
    await user.click(screen.getByRole('button', { name: /^delete$/i }))

    await waitFor(() => {
      expect(screen.getByText(/pipeline deleted/i)).toBeInTheDocument()
    })
  })
})

// ---------------------------------------------------------------------------
// AC4: Delete blocked with explanation on 409 (active tasks)
// ---------------------------------------------------------------------------

describe('AC4 — delete blocked with explanation when pipeline has active tasks', () => {
  it('shows a 409 error toast instead of deleting when pipeline has active tasks', async () => {
    const user = userEvent.setup()
    const pipeline = makePipeline({ id: 'pipe-active', name: 'Active Pipeline' })
    stubPipelinesHook({ pipelines: [pipeline] })
    mockDeletePipeline.mockRejectedValue(new Error('409: pipeline has active tasks'))

    renderPage()

    await user.click(screen.getByRole('button', { name: /delete pipeline Active Pipeline/i }))
    await user.click(screen.getByRole('button', { name: /^delete$/i }))

    await waitFor(() => {
      expect(screen.getByRole('alert')).toBeInTheDocument()
    })
    expect(screen.getByText(/cannot delete pipeline: it has active tasks/i)).toBeInTheDocument()
  })

  it('does not refresh the pipeline list after a 409 rejection', async () => {
    const user = userEvent.setup()
    const refreshMock = vi.fn()
    const pipeline = makePipeline({ id: 'pipe-active', name: 'Active Pipeline' })
    stubPipelinesHook({ pipelines: [pipeline], refresh: refreshMock })
    mockDeletePipeline.mockRejectedValue(new Error('409: pipeline has active tasks'))

    renderPage()

    await user.click(screen.getByRole('button', { name: /delete pipeline Active Pipeline/i }))
    await user.click(screen.getByRole('button', { name: /^delete$/i }))

    await waitFor(() => {
      expect(screen.getByText(/cannot delete pipeline/i)).toBeInTheDocument()
    })
    expect(refreshMock).not.toHaveBeenCalled()
  })

  it('shows a generic error toast for non-409 deletion failures', async () => {
    const user = userEvent.setup()
    const pipeline = makePipeline({ id: 'pipe-err', name: 'Error Pipeline' })
    stubPipelinesHook({ pipelines: [pipeline] })
    mockDeletePipeline.mockRejectedValue(new Error('500: internal server error'))

    renderPage()

    await user.click(screen.getByRole('button', { name: /delete pipeline Error Pipeline/i }))
    await user.click(screen.getByRole('button', { name: /^delete$/i }))

    await waitFor(() => {
      expect(screen.getByRole('alert')).toBeInTheDocument()
    })
    // Non-409 errors surface the raw message, not the active-tasks message.
    expect(screen.queryByText(/cannot delete pipeline: it has active tasks/i)).not.toBeInTheDocument()
    expect(screen.getByText(/500: internal server error/i)).toBeInTheDocument()
  })
})
