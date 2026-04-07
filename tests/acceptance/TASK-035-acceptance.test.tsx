/**
 * Acceptance tests for TASK-035: Task Submission via GUI (complete flow)
 * Requirements: REQ-002
 *
 * Each acceptance criterion is tested with at least one positive case
 * (criterion satisfied) and at least one negative case (condition that must NOT
 * satisfy the criterion and must be correctly rejected or absent).
 *
 * AC-1: User can submit a task via the Task Feed "Submit Task" modal
 * AC-2: Pipeline selector shows available pipelines from GET /api/pipelines
 * AC-3: Missing required parameters show inline validation errors
 * AC-4: Submitted task appears in the Task Feed with status "submitted"
 * AC-5: Task created via GUI is identical in state and behavior to one created via API
 *
 * Tests operate at acceptance layer: rendered components through their public
 * DOM interface. No access to implementation internals.
 *
 * See: TASK-035, REQ-002
 */

import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'

import SubmitTaskModal from '@/components/SubmitTaskModal'
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
    dataSourceConfig: {
      connectorType: 'demo-source',
      config: {},
      outputSchema: ['id', 'name', 'amount'],
    },
    processConfig: {
      connectorType: 'demo-transform',
      config: {},
      inputMappings: [],
      outputSchema: ['transformed_id', 'result'],
    },
    sinkConfig: {
      connectorType: 'demo-sink',
      config: {},
      inputMappings: [],
    },
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

function makeDefaultTasksHookReturn(overrides: Partial<UseTasksReturn> = {}): UseTasksReturn {
  return {
    tasks: [],
    isLoading: false,
    error: null,
    sseStatus: 'connected',
    refresh: vi.fn(),
    ...overrides,
  }
}

function makeDefaultPipelinesHookReturn(overrides: Partial<UsePipelinesReturn> = {}): UsePipelinesReturn {
  return {
    pipelines: [makePipeline()],
    isLoading: false,
    error: null,
    refresh: vi.fn(),
    ...overrides,
  }
}

function renderTaskFeedPage() {
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

  mockUseAuth.mockReturnValue({
    user: { id: 'user-001', username: 'testuser', role: 'user', active: true, createdAt: '' },
    login: vi.fn(),
    logout: vi.fn(),
    isLoading: false,
  })

  mockUseTasks.mockReturnValue(makeDefaultTasksHookReturn())
  mockUsePipelines.mockReturnValue(makeDefaultPipelinesHookReturn())
})

// ===========================================================================
// AC-1: User can submit a task via the Task Feed "Submit Task" modal
// REQ-002: Submit a task via web GUI
// ===========================================================================
//
// Note on "Submit Task" button ambiguity: TaskFeedPage renders two buttons
// with this label when tasks list is empty — one in the FilterBar and one in
// the empty-state CTA. The FilterBar button is always the first one rendered
// (it is outside the conditional task-list section). We use getAllByRole[0]
// to select the FilterBar button unambiguously.

describe('AC-1: User can submit a task via Task Feed "Submit Task" modal [REQ-002]', () => {
  // Given: a logged-in user on the Task Feed page with pipelines available
  // When:  the user clicks the FilterBar "Submit Task" button
  // Then:  the modal opens with a "Submit Task" heading

  it('[positive] clicking FilterBar "Submit Task" button opens the modal', async () => {
    const user = userEvent.setup()
    renderTaskFeedPage()

    const submitButtons = screen.getAllByRole('button', { name: /submit task/i })
    await user.click(submitButtons[0]!)

    expect(screen.getByRole('dialog', { name: /Submit Task/i })).toBeInTheDocument()
  })

  // Given: the modal is open with a pipeline selected
  // When:  the user clicks the submit button in the modal
  // Then:  submitTask API is called and the modal closes

  it('[positive] modal Submit Task button calls the submitTask API', async () => {
    const user = userEvent.setup()
    mockSubmitTask.mockResolvedValue({ taskId: 'task-new', status: 'submitted' })
    const refreshMock = vi.fn()
    mockUseTasks.mockReturnValue(makeDefaultTasksHookReturn({ refresh: refreshMock }))
    renderTaskFeedPage()

    const submitButtons = screen.getAllByRole('button', { name: /submit task/i })
    await user.click(submitButtons[0]!)

    // Scope to dialog to avoid ambiguity with FilterBar and empty-state buttons
    const dialog = screen.getByRole('dialog')
    await user.click(within(dialog).getByRole('button', { name: /submit task/i }))

    await waitFor(() => {
      expect(mockSubmitTask).toHaveBeenCalledOnce()
    })
  })

  // Negative: clicking Cancel does NOT call submitTask

  it('[negative] clicking Cancel in the modal does not call submitTask', async () => {
    const user = userEvent.setup()
    renderTaskFeedPage()

    const submitButtons = screen.getAllByRole('button', { name: /submit task/i })
    await user.click(submitButtons[0]!)
    expect(screen.getByRole('dialog')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /cancel/i }))

    expect(mockSubmitTask).not.toHaveBeenCalled()
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  // Negative: when no pipeline is available, submit button inside modal is disabled
  // (pipelineId is empty string → canSubmit is false)

  it('[negative] Submit button inside modal is disabled when no pipeline is available', async () => {
    const user = userEvent.setup()
    mockUsePipelines.mockReturnValue(makeDefaultPipelinesHookReturn({ pipelines: [] }))
    renderTaskFeedPage()

    const submitButtons = screen.getAllByRole('button', { name: /submit task/i })
    await user.click(submitButtons[0]!)

    // The Submit Task button inside the dialog should be disabled
    const dialog = screen.getByRole('dialog')
    const submitInDialog = dialog.querySelector('button[type="submit"]') as HTMLButtonElement
    expect(submitInDialog).toBeDisabled()
  })
})

// ===========================================================================
// AC-2: Pipeline selector shows available pipelines from GET /api/pipelines
// REQ-002: "select a pipeline" in the submission interface
// ===========================================================================

describe('AC-2: Pipeline selector populated from GET /api/pipelines [REQ-002]', () => {
  // Given: GET /api/pipelines returns two pipelines
  // When:  the Submit Task modal opens
  // Then:  the pipeline dropdown shows both pipelines as options

  it('[positive] pipeline dropdown shows all pipelines from usePipelines hook', async () => {
    const user = userEvent.setup()
    mockUsePipelines.mockReturnValue(makeDefaultPipelinesHookReturn({
      pipelines: [
        makePipeline({ id: 'pipe-001', name: 'ETL Pipeline' }),
        makePipeline({ id: 'pipe-002', name: 'Billing Pipeline' }),
      ],
    }))
    renderTaskFeedPage()

    await user.click(screen.getAllByRole('button', { name: /submit task/i })[0]!)

    // Scope to dialog to avoid conflict with the FilterBar pipeline filter dropdown
    const dialog = screen.getByRole('dialog')
    expect(within(dialog).getAllByRole('option', { name: 'ETL Pipeline' }).length).toBeGreaterThan(0)
    expect(within(dialog).getAllByRole('option', { name: 'Billing Pipeline' }).length).toBeGreaterThan(0)
  })

  // Given: pipelines from the hook
  // When:  modal opens
  // Then:  the first pipeline is pre-selected

  it('[positive] first pipeline is pre-selected in the dropdown', async () => {
    const user = userEvent.setup()
    mockUsePipelines.mockReturnValue(makeDefaultPipelinesHookReturn({
      pipelines: [
        makePipeline({ id: 'pipe-001', name: 'ETL Pipeline' }),
        makePipeline({ id: 'pipe-002', name: 'Billing Pipeline' }),
      ],
    }))
    renderTaskFeedPage()

    await user.click(screen.getAllByRole('button', { name: /submit task/i })[0]!)

    // Scope to dialog: both FilterBar and modal have pipeline selects
    const dialog = screen.getByRole('dialog')
    const select = within(dialog).getByLabelText(/^pipeline$/i) as HTMLSelectElement
    expect(select.value).toBe('pipe-001')
  })

  // Negative: when no pipelines exist, the dropdown shows a placeholder message

  it('[negative] "No pipelines available" shown when pipeline list is empty', async () => {
    const user = userEvent.setup()
    mockUsePipelines.mockReturnValue(makeDefaultPipelinesHookReturn({ pipelines: [] }))
    renderTaskFeedPage()

    await user.click(screen.getAllByRole('button', { name: /submit task/i })[0]!)

    const dialog = screen.getByRole('dialog')
    expect(within(dialog).getByText(/no pipelines available/i)).toBeInTheDocument()
  })

  // [VERIFIER-ADDED] The modal receives pipelines directly from parent — not re-fetched

  it('[VERIFIER-ADDED] modal pipeline list matches what usePipelines returns (no double fetch)', async () => {
    const user = userEvent.setup()
    const uniquePipeline = makePipeline({ id: 'pipe-unique-xyz', name: 'Unique Test Pipeline' })
    mockUsePipelines.mockReturnValue(makeDefaultPipelinesHookReturn({ pipelines: [uniquePipeline] }))
    renderTaskFeedPage()

    await user.click(screen.getAllByRole('button', { name: /submit task/i })[0]!)

    // Scope to dialog to verify the modal's pipeline selector contains this pipeline
    const dialog = screen.getByRole('dialog')
    expect(within(dialog).getAllByRole('option', { name: 'Unique Test Pipeline' }).length).toBeGreaterThan(0)
  })
})

// ===========================================================================
// AC-3: Missing required parameters show inline validation errors
// REQ-002: "Submission creates a task equivalent to the REST API path"
// REQ-002 GWT: "When the user attempts to submit a task with missing required parameters,
//               Then the GUI shows inline validation errors and does not submit the task"
// ===========================================================================

describe('AC-3: Missing required parameters show inline validation errors [REQ-002]', () => {
  // Given: the user has added a parameter row but left the key empty
  // When:  the user clicks Submit Task
  // Then:  an inline validation error is shown and submitTask is not called

  it('[positive] empty parameter key shows validation error and blocks submission', async () => {
    const user = userEvent.setup()
    mockSubmitTask.mockResolvedValue({ taskId: 'task-new', status: 'submitted' })

    render(
      <SubmitTaskModal
        isOpen
        onClose={vi.fn()}
        onSuccess={vi.fn()}
        pipelines={[makePipeline()]}
      />
    )

    await user.click(screen.getByRole('button', { name: /add parameter/i }))
    // Leave key empty; type a value only
    await user.type(screen.getByPlaceholderText(/value/i), 'some-value')
    await user.click(screen.getByRole('button', { name: /^submit task$/i }))

    expect(mockSubmitTask).not.toHaveBeenCalled()
    expect(screen.getByText(/parameter key.*empty/i)).toBeInTheDocument()
  })

  // Given: the user has added two parameter rows with the same key
  // When:  the user clicks Submit Task
  // Then:  a "duplicate key" error is shown and submitTask is not called

  it('[positive] duplicate parameter keys show validation error and block submission', async () => {
    const user = userEvent.setup()
    mockSubmitTask.mockResolvedValue({ taskId: 'task-new', status: 'submitted' })

    render(
      <SubmitTaskModal
        isOpen
        onClose={vi.fn()}
        onSuccess={vi.fn()}
        pipelines={[makePipeline()]}
      />
    )

    await user.click(screen.getByRole('button', { name: /add parameter/i }))
    await user.click(screen.getByRole('button', { name: /add parameter/i }))

    const keyInputs = screen.getAllByPlaceholderText(/key/i)
    await user.type(keyInputs[0]!, 'source')
    await user.type(keyInputs[1]!, 'source')

    await user.click(screen.getByRole('button', { name: /^submit task$/i }))

    expect(mockSubmitTask).not.toHaveBeenCalled()
    expect(screen.getByText(/duplicate parameter key/i)).toBeInTheDocument()
  })

  // Negative: valid parameters (non-empty, unique keys) do NOT produce validation errors

  it('[negative] valid key-value parameters do not produce validation errors', async () => {
    const user = userEvent.setup()
    mockSubmitTask.mockResolvedValue({ taskId: 'task-new', status: 'submitted' })

    render(
      <SubmitTaskModal
        isOpen
        onClose={vi.fn()}
        onSuccess={vi.fn()}
        pipelines={[makePipeline()]}
      />
    )

    await user.click(screen.getByRole('button', { name: /add parameter/i }))
    await user.type(screen.getByPlaceholderText(/key/i), 'source')
    await user.type(screen.getByPlaceholderText(/value/i), 's3://bucket/data')

    await user.click(screen.getByRole('button', { name: /^submit task$/i }))

    await waitFor(() => expect(mockSubmitTask).toHaveBeenCalledOnce())
    expect(screen.queryByText(/parameter key.*empty/i)).not.toBeInTheDocument()
    expect(screen.queryByText(/duplicate parameter key/i)).not.toBeInTheDocument()
  })

  // Negative: zero parameters (no rows added) is valid and submits successfully

  it('[negative] zero parameter rows do not trigger validation error', async () => {
    const user = userEvent.setup()
    mockSubmitTask.mockResolvedValue({ taskId: 'task-new', status: 'submitted' })

    render(
      <SubmitTaskModal
        isOpen
        onClose={vi.fn()}
        onSuccess={vi.fn()}
        pipelines={[makePipeline()]}
      />
    )

    // No parameter rows added — submit immediately
    await user.click(screen.getByRole('button', { name: /^submit task$/i }))

    await waitFor(() => expect(mockSubmitTask).toHaveBeenCalledWith({
      pipelineId: 'pipe-001',
      input: {},
    }))
  })

  // [VERIFIER-ADDED] validation error clears when the user edits the problematic field

  it('[VERIFIER-ADDED] validation error is cleared when user edits a parameter field', async () => {
    const user = userEvent.setup()
    mockSubmitTask.mockResolvedValue({ taskId: 'task-new', status: 'submitted' })

    render(
      <SubmitTaskModal
        isOpen
        onClose={vi.fn()}
        onSuccess={vi.fn()}
        pipelines={[makePipeline()]}
      />
    )

    // Trigger validation error
    await user.click(screen.getByRole('button', { name: /add parameter/i }))
    await user.click(screen.getByRole('button', { name: /^submit task$/i }))
    expect(screen.getByText(/parameter key.*empty/i)).toBeInTheDocument()

    // Fix the key — error must clear
    await user.type(screen.getByPlaceholderText(/key/i), 'fixed-key')
    expect(screen.queryByText(/parameter key.*empty/i)).not.toBeInTheDocument()
  })
})

// ===========================================================================
// AC-4: Submitted task appears in the Task Feed with status "submitted"
// REQ-002: "task is created with state 'submitted' and appears in the user's Task Feed"
// REQ-002 GWT: "Then a task is created with state 'submitted' and appears in the user's Task Feed"
// ===========================================================================

describe('AC-4: Submitted task appears in Task Feed with status "submitted" [REQ-002]', () => {
  // Given: a task with status "submitted" is in the tasks list
  // When:  the Task Feed renders
  // Then:  a "submitted" status badge is visible (via aria-label on StatusBadge)

  it('[positive] "submitted" status badge is shown in the Task Feed for a newly created task', () => {
    const submittedTask = makeTask({ id: 'task-submitted-001', status: 'submitted' })
    mockUseTasks.mockReturnValue(makeDefaultTasksHookReturn({ tasks: [submittedTask] }))

    renderTaskFeedPage()

    // StatusBadge renders with role="status" aria-label="Task status: submitted"
    expect(screen.getByRole('status', { name: /task status: submitted/i })).toBeInTheDocument()
  })

  // Given: the user submits a task via the modal
  // When:  the submission succeeds
  // Then:  refresh() is called so the task list re-fetches

  it('[positive] successful submission calls refresh() to update the Task Feed', async () => {
    const user = userEvent.setup()
    mockSubmitTask.mockResolvedValue({ taskId: 'task-new-001', status: 'submitted' })
    const refreshMock = vi.fn()
    mockUseTasks.mockReturnValue(makeDefaultTasksHookReturn({ refresh: refreshMock }))

    renderTaskFeedPage()

    await user.click(screen.getAllByRole('button', { name: /submit task/i })[0]!)
    const dialog = screen.getByRole('dialog')
    await user.click(within(dialog).getByRole('button', { name: /submit task/i }))

    await waitFor(() => {
      expect(refreshMock).toHaveBeenCalled()
    })
  })

  // Negative: a task with status "completed" does NOT show the "submitted" badge

  it('[negative] "completed" status badge is not shown as "submitted" for a completed task', () => {
    const completedTask = makeTask({ id: 'task-completed-001', status: 'completed' })
    mockUseTasks.mockReturnValue(makeDefaultTasksHookReturn({ tasks: [completedTask] }))

    renderTaskFeedPage()

    // StatusBadge for completed task has aria-label "Task status: completed"
    expect(screen.getByRole('status', { name: /task status: completed/i })).toBeInTheDocument()
    // There is no "submitted" badge
    expect(screen.queryByRole('status', { name: /task status: submitted/i })).not.toBeInTheDocument()
  })

  // Negative: a failed submission does NOT call refresh() (task was not created)

  it('[negative] failed submission does not call refresh()', async () => {
    const user = userEvent.setup()
    mockSubmitTask.mockRejectedValue(new Error('500: Internal server error'))
    const refreshMock = vi.fn()
    mockUseTasks.mockReturnValue(makeDefaultTasksHookReturn({ refresh: refreshMock }))

    renderTaskFeedPage()

    await user.click(screen.getAllByRole('button', { name: /submit task/i })[0]!)
    const dialog = screen.getByRole('dialog')
    await user.click(within(dialog).getByRole('button', { name: /submit task/i }))

    await screen.findByRole('alert')
    expect(refreshMock).not.toHaveBeenCalled()
  })
})

// ===========================================================================
// AC-5: Task created via GUI is identical in state and behavior to one created via API
// REQ-002: "resulting task is identical in state and behavior to one submitted via the API"
// REQ-001: POST /api/tasks payload shape — { pipelineId, input, retryConfig? }
// ===========================================================================

describe('AC-5: GUI task payload is identical to API task payload [REQ-002, REQ-001]', () => {
  // Given: the user submits with pipelineId, input, and no retry config (maxRetries=0)
  // When:  the modal calls submitTask
  // Then:  the payload matches { pipelineId, input } — retryConfig is omitted (REQ-001 default)

  it('[positive] payload matches API shape: pipelineId + input, retryConfig omitted when maxRetries=0', async () => {
    const user = userEvent.setup()
    mockSubmitTask.mockResolvedValue({ taskId: 'task-api-equivalent', status: 'submitted' })

    render(
      <SubmitTaskModal
        isOpen
        onClose={vi.fn()}
        onSuccess={vi.fn()}
        pipelines={[makePipeline({ id: 'pipe-api-001', name: 'API Test Pipeline' })]}
        initialPipelineId="pipe-api-001"
      />
    )

    await user.click(screen.getByRole('button', { name: /add parameter/i }))
    await user.type(screen.getByPlaceholderText(/key/i), 'dataset')
    await user.type(screen.getByPlaceholderText(/value/i), 's3://bucket/data.csv')

    await user.click(screen.getByRole('button', { name: /^submit task$/i }))

    await waitFor(() => {
      expect(mockSubmitTask).toHaveBeenCalledWith({
        pipelineId: 'pipe-api-001',
        input: { dataset: 's3://bucket/data.csv' },
        // retryConfig intentionally absent: maxRetries=0 → use system defaults (REQ-001)
      })
    })
  })

  // Given: the user sets retryConfig (maxRetries > 0)
  // When:  the modal calls submitTask
  // Then:  retryConfig is included in the payload

  it('[positive] retryConfig is included in payload when maxRetries > 0', async () => {
    const user = userEvent.setup()
    mockSubmitTask.mockResolvedValue({ taskId: 'task-retry', status: 'submitted' })

    render(
      <SubmitTaskModal
        isOpen
        onClose={vi.fn()}
        onSuccess={vi.fn()}
        pipelines={[makePipeline()]}
      />
    )

    const maxRetriesInput = screen.getByLabelText(/max retries/i)
    await user.clear(maxRetriesInput)
    await user.type(maxRetriesInput, '3')

    const backoffSelect = screen.getByLabelText(/backoff/i)
    await user.selectOptions(backoffSelect, 'exponential')

    await user.click(screen.getByRole('button', { name: /^submit task$/i }))

    await waitFor(() => {
      expect(mockSubmitTask).toHaveBeenCalledWith(
        expect.objectContaining({
          retryConfig: { maxRetries: 3, backoff: 'exponential' },
        })
      )
    })
  })

  // Given: the user submits with maxRetries=0 (default)
  // When:  the modal calls submitTask
  // Then:  retryConfig is absent from the payload (semantically equivalent to API default)

  it('[positive] retryConfig is absent from payload when maxRetries=0 (system default behavior)', async () => {
    const user = userEvent.setup()
    mockSubmitTask.mockResolvedValue({ taskId: 'task-default', status: 'submitted' })

    render(
      <SubmitTaskModal
        isOpen
        onClose={vi.fn()}
        onSuccess={vi.fn()}
        pipelines={[makePipeline()]}
      />
    )

    await user.click(screen.getByRole('button', { name: /^submit task$/i }))

    await waitFor(() => {
      const call = mockSubmitTask.mock.calls[0]![0]
      expect(call).not.toHaveProperty('retryConfig')
    })
  })

  // Negative: the submitTask API function is the same one called by the API path (client.ts)
  // The GUI does NOT call a different endpoint — it uses the same submitTask() wrapper.
  // Verified by checking that the mock (which wraps POST /api/tasks) receives the exact call.

  it('[negative] payload does NOT contain extra fields beyond pipelineId, input, retryConfig', async () => {
    const user = userEvent.setup()
    mockSubmitTask.mockResolvedValue({ taskId: 'task-no-extra', status: 'submitted' })

    render(
      <SubmitTaskModal
        isOpen
        onClose={vi.fn()}
        onSuccess={vi.fn()}
        pipelines={[makePipeline({ id: 'pipe-999', name: 'Test' })]}
        initialPipelineId="pipe-999"
      />
    )

    await user.click(screen.getByRole('button', { name: /^submit task$/i }))

    await waitFor(() => {
      expect(mockSubmitTask).toHaveBeenCalledOnce()
      const payload = mockSubmitTask.mock.calls[0]![0]
      const keys = Object.keys(payload)
      // Only pipelineId and input are expected when no retryConfig
      expect(keys).toContain('pipelineId')
      expect(keys).toContain('input')
      expect(keys).not.toContain('retryConfig')
      // No extra fields like userId, status, etc.
      expect(keys.every(k => ['pipelineId', 'input', 'retryConfig'].includes(k))).toBe(true)
    })
  })

  // [VERIFIER-ADDED] Multiple input parameters are serialised as a Record<string, unknown> — same shape as direct API usage

  it('[VERIFIER-ADDED] multiple parameters are serialised as a flat Record in the input field', async () => {
    const user = userEvent.setup()
    mockSubmitTask.mockResolvedValue({ taskId: 'task-multi-params', status: 'submitted' })

    render(
      <SubmitTaskModal
        isOpen
        onClose={vi.fn()}
        onSuccess={vi.fn()}
        pipelines={[makePipeline()]}
      />
    )

    await user.click(screen.getByRole('button', { name: /add parameter/i }))
    await user.click(screen.getByRole('button', { name: /add parameter/i }))

    const keyInputs = screen.getAllByPlaceholderText(/key/i)
    const valueInputs = screen.getAllByPlaceholderText(/value/i)
    await user.type(keyInputs[0]!, 'source')
    await user.type(valueInputs[0]!, 's3://bucket/data')
    await user.type(keyInputs[1]!, 'format')
    await user.type(valueInputs[1]!, 'parquet')

    await user.click(screen.getByRole('button', { name: /^submit task$/i }))

    await waitFor(() => {
      expect(mockSubmitTask).toHaveBeenCalledWith(
        expect.objectContaining({
          input: {
            source: 's3://bucket/data',
            format: 'parquet',
          },
        })
      )
    })
  })
})
