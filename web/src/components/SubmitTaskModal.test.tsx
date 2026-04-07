/**
 * Unit tests for SubmitTaskModal — full TASK-035 implementation.
 *
 * Covers:
 *   - Pipeline selector: renders pipelines, pre-selects initialPipelineId
 *   - Parameter form: add/remove key-value pairs, empty key validation
 *   - Retry configuration: max retries input, backoff strategy selector
 *   - Inline validation: duplicate keys, empty keys prevent submission
 *   - Submission: calls submitTask with correct payload, invokes onSuccess + onClose
 *   - Error handling: API error shown inline
 *   - State reset: form resets on re-open
 *   - Closed state: returns null when isOpen is false
 *
 * See: TASK-035, REQ-002, UX Spec (Task Feed and Monitor)
 */
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import SubmitTaskModal from './SubmitTaskModal'
import type { Pipeline } from '@/types/domain'

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

vi.mock('@/api/client', () => ({
  submitTask: vi.fn(),
}))

import * as clientModule from '@/api/client'

const mockSubmitTask = vi.mocked(clientModule.submitTask)

// ---------------------------------------------------------------------------
// Helpers
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

interface RenderProps {
  isOpen?: boolean
  pipelines?: Pipeline[]
  initialPipelineId?: string
  onClose?: () => void
  onSuccess?: (taskId: string) => void
}

function renderModal(props: RenderProps = {}) {
  const onClose = props.onClose ?? vi.fn()
  const onSuccess = props.onSuccess ?? vi.fn()
  const pipelines = props.pipelines ?? [makePipeline()]

  const result = render(
    <SubmitTaskModal
      isOpen={props.isOpen ?? true}
      onClose={onClose}
      onSuccess={onSuccess}
      pipelines={pipelines}
      initialPipelineId={props.initialPipelineId}
    />
  )

  return { ...result, onClose, onSuccess }
}

// ---------------------------------------------------------------------------
// Setup
// ---------------------------------------------------------------------------

beforeEach(() => {
  vi.clearAllMocks()
})

// ---------------------------------------------------------------------------
// Closed state
// ---------------------------------------------------------------------------

describe('SubmitTaskModal — closed state', () => {
  it('renders nothing when isOpen is false', () => {
    renderModal({ isOpen: false })
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })
})

// ---------------------------------------------------------------------------
// Open state — dialog presence
// ---------------------------------------------------------------------------

describe('SubmitTaskModal — open state', () => {
  it('renders the dialog when isOpen is true', () => {
    renderModal({ isOpen: true })
    expect(screen.getByRole('dialog')).toBeInTheDocument()
  })

  it('renders the "Submit Task" heading', () => {
    renderModal()
    expect(screen.getByRole('heading', { name: 'Submit Task' })).toBeInTheDocument()
  })

  it('renders Cancel and Submit Task buttons', () => {
    renderModal()
    expect(screen.getByRole('button', { name: /cancel/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /submit task/i })).toBeInTheDocument()
  })
})

// ---------------------------------------------------------------------------
// Pipeline selector
// ---------------------------------------------------------------------------

describe('SubmitTaskModal — pipeline selector', () => {
  it('renders a pipeline option for each pipeline in the list', () => {
    const pipelines = [
      makePipeline({ id: 'pipe-001', name: 'ETL Pipeline' }),
      makePipeline({ id: 'pipe-002', name: 'Billing Pipeline' }),
    ]
    renderModal({ pipelines })
    expect(screen.getByRole('option', { name: 'ETL Pipeline' })).toBeInTheDocument()
    expect(screen.getByRole('option', { name: 'Billing Pipeline' })).toBeInTheDocument()
  })

  it('pre-selects the pipeline matching initialPipelineId', () => {
    const pipelines = [
      makePipeline({ id: 'pipe-001', name: 'ETL Pipeline' }),
      makePipeline({ id: 'pipe-002', name: 'Billing Pipeline' }),
    ]
    renderModal({ pipelines, initialPipelineId: 'pipe-002' })
    const select = screen.getByLabelText(/pipeline/i) as HTMLSelectElement
    expect(select.value).toBe('pipe-002')
  })

  it('selects the first pipeline when no initialPipelineId is given', () => {
    const pipelines = [
      makePipeline({ id: 'pipe-001', name: 'ETL Pipeline' }),
      makePipeline({ id: 'pipe-002', name: 'Billing Pipeline' }),
    ]
    renderModal({ pipelines })
    const select = screen.getByLabelText(/pipeline/i) as HTMLSelectElement
    expect(select.value).toBe('pipe-001')
  })

  it('shows "No pipelines available" when the list is empty', () => {
    renderModal({ pipelines: [] })
    expect(screen.getByText(/no pipelines available/i)).toBeInTheDocument()
  })
})

// ---------------------------------------------------------------------------
// Parameters section
// ---------------------------------------------------------------------------

describe('SubmitTaskModal — parameter form', () => {
  it('renders the Parameters section', () => {
    renderModal()
    // The "Parameters" section label is rendered as a <span> (not a heading)
    // alongside the "Retry Configuration" label — use getAllByText and confirm at least one matches
    const labels = screen.getAllByText(/parameters/i)
    expect(labels.length).toBeGreaterThan(0)
  })

  it('renders an "Add Parameter" button', () => {
    renderModal()
    expect(screen.getByRole('button', { name: /add parameter/i })).toBeInTheDocument()
  })

  it('adds a new parameter row when "Add Parameter" is clicked', async () => {
    const user = userEvent.setup()
    renderModal()
    await user.click(screen.getByRole('button', { name: /add parameter/i }))
    // A key input and value input should now appear
    const keyInputs = screen.getAllByPlaceholderText(/key/i)
    expect(keyInputs.length).toBeGreaterThan(0)
  })

  it('removes a parameter row when the remove button is clicked', async () => {
    const user = userEvent.setup()
    renderModal()
    await user.click(screen.getByRole('button', { name: /add parameter/i }))
    expect(screen.getAllByPlaceholderText(/key/i).length).toBe(1)
    await user.click(screen.getByRole('button', { name: /remove parameter/i }))
    expect(screen.queryAllByPlaceholderText(/key/i).length).toBe(0)
  })

  it('allows typing a key and value into a parameter row', async () => {
    const user = userEvent.setup()
    renderModal()
    await user.click(screen.getByRole('button', { name: /add parameter/i }))
    const keyInput = screen.getByPlaceholderText(/key/i)
    const valueInput = screen.getByPlaceholderText(/value/i)
    await user.type(keyInput, 'source')
    await user.type(valueInput, 's3://bucket/data')
    expect((keyInput as HTMLInputElement).value).toBe('source')
    expect((valueInput as HTMLInputElement).value).toBe('s3://bucket/data')
  })
})

// ---------------------------------------------------------------------------
// Retry configuration
// ---------------------------------------------------------------------------

describe('SubmitTaskModal — retry configuration', () => {
  it('renders the Retry Configuration section', () => {
    renderModal()
    expect(screen.getByText(/retry/i)).toBeInTheDocument()
  })

  it('renders a max retries input defaulting to 0', () => {
    renderModal()
    const maxRetriesInput = screen.getByLabelText(/max retries/i) as HTMLInputElement
    expect(maxRetriesInput.value).toBe('0')
  })

  it('renders a backoff strategy selector', () => {
    renderModal()
    expect(screen.getByLabelText(/backoff/i)).toBeInTheDocument()
  })

  it('allows changing max retries', async () => {
    const user = userEvent.setup()
    renderModal()
    const maxRetriesInput = screen.getByLabelText(/max retries/i) as HTMLInputElement
    await user.clear(maxRetriesInput)
    await user.type(maxRetriesInput, '3')
    expect(maxRetriesInput.value).toBe('3')
  })

  it('allows changing backoff strategy', async () => {
    const user = userEvent.setup()
    renderModal()
    const backoffSelect = screen.getByLabelText(/backoff/i) as HTMLSelectElement
    await user.selectOptions(backoffSelect, 'linear')
    expect(backoffSelect.value).toBe('linear')
  })
})

// ---------------------------------------------------------------------------
// Inline validation
// ---------------------------------------------------------------------------

describe('SubmitTaskModal — inline validation', () => {
  it('shows an error and blocks submission when a parameter key is empty', async () => {
    const user = userEvent.setup()
    mockSubmitTask.mockResolvedValue({ taskId: 'task-new', status: 'submitted' })
    renderModal()

    // Add a parameter row but leave the key empty
    await user.click(screen.getByRole('button', { name: /add parameter/i }))
    const valueInput = screen.getByPlaceholderText(/value/i)
    await user.type(valueInput, 'somevalue')

    await user.click(screen.getByRole('button', { name: /submit task/i }))

    // Submission should NOT have been called
    expect(mockSubmitTask).not.toHaveBeenCalled()
    // An error should be shown
    expect(screen.getByText(/parameter key.*empty/i)).toBeInTheDocument()
  })

  it('shows an error and blocks submission when duplicate parameter keys exist', async () => {
    const user = userEvent.setup()
    mockSubmitTask.mockResolvedValue({ taskId: 'task-new', status: 'submitted' })
    renderModal()

    // Add two parameters with the same key
    await user.click(screen.getByRole('button', { name: /add parameter/i }))
    await user.click(screen.getByRole('button', { name: /add parameter/i }))

    const keyInputs = screen.getAllByPlaceholderText(/key/i)
    await user.type(keyInputs[0]!, 'source')
    await user.type(keyInputs[1]!, 'source')

    await user.click(screen.getByRole('button', { name: /submit task/i }))

    expect(mockSubmitTask).not.toHaveBeenCalled()
    expect(screen.getByText(/duplicate parameter key/i)).toBeInTheDocument()
  })

  it('clears validation error when parameter issue is corrected', async () => {
    const user = userEvent.setup()
    mockSubmitTask.mockResolvedValue({ taskId: 'task-new', status: 'submitted' })
    renderModal()

    await user.click(screen.getByRole('button', { name: /add parameter/i }))
    // Attempt submission with empty key
    await user.click(screen.getByRole('button', { name: /submit task/i }))
    expect(screen.getByText(/parameter key.*empty/i)).toBeInTheDocument()

    // Fix the key
    const keyInput = screen.getByPlaceholderText(/key/i)
    await user.type(keyInput, 'source')

    // Error should be gone
    expect(screen.queryByText(/parameter key.*empty/i)).not.toBeInTheDocument()
  })
})

// ---------------------------------------------------------------------------
// Successful submission
// ---------------------------------------------------------------------------

describe('SubmitTaskModal — submission', () => {
  it('calls submitTask with correct payload on valid submission (no params, default retry)', async () => {
    const user = userEvent.setup()
    mockSubmitTask.mockResolvedValue({ taskId: 'task-abc', status: 'submitted' })
    const onSuccess = vi.fn()
    const onClose = vi.fn()
    renderModal({ onSuccess, onClose })

    await user.click(screen.getByRole('button', { name: /submit task/i }))

    // retryConfig is omitted when maxRetries=0 (system default) per REQ-001
    expect(mockSubmitTask).toHaveBeenCalledWith({
      pipelineId: 'pipe-001',
      input: {},
    })
  })

  it('calls submitTask with input parameters in the payload', async () => {
    const user = userEvent.setup()
    mockSubmitTask.mockResolvedValue({ taskId: 'task-abc', status: 'submitted' })
    renderModal()

    await user.click(screen.getByRole('button', { name: /add parameter/i }))
    await user.type(screen.getByPlaceholderText(/key/i), 'source')
    await user.type(screen.getByPlaceholderText(/value/i), 's3://bucket')

    await user.click(screen.getByRole('button', { name: /submit task/i }))

    expect(mockSubmitTask).toHaveBeenCalledWith(
      expect.objectContaining({
        input: { source: 's3://bucket' },
      })
    )
  })

  it('calls submitTask with retry config when values are changed', async () => {
    const user = userEvent.setup()
    mockSubmitTask.mockResolvedValue({ taskId: 'task-abc', status: 'submitted' })
    renderModal()

    const maxRetriesInput = screen.getByLabelText(/max retries/i)
    await user.clear(maxRetriesInput)
    await user.type(maxRetriesInput, '3')

    const backoffSelect = screen.getByLabelText(/backoff/i)
    await user.selectOptions(backoffSelect, 'exponential')

    await user.click(screen.getByRole('button', { name: /submit task/i }))

    expect(mockSubmitTask).toHaveBeenCalledWith(
      expect.objectContaining({
        retryConfig: { maxRetries: 3, backoff: 'exponential' },
      })
    )
  })

  it('calls onSuccess with the new task ID on success', async () => {
    const user = userEvent.setup()
    mockSubmitTask.mockResolvedValue({ taskId: 'task-abc', status: 'submitted' })
    const onSuccess = vi.fn()
    renderModal({ onSuccess })

    await user.click(screen.getByRole('button', { name: /submit task/i }))

    expect(onSuccess).toHaveBeenCalledWith('task-abc')
  })

  it('calls onClose after successful submission', async () => {
    const user = userEvent.setup()
    mockSubmitTask.mockResolvedValue({ taskId: 'task-abc', status: 'submitted' })
    const onClose = vi.fn()
    renderModal({ onClose })

    await user.click(screen.getByRole('button', { name: /submit task/i }))

    expect(onClose).toHaveBeenCalled()
  })

  it('disables the submit button during submission', async () => {
    const user = userEvent.setup()
    // Never resolves so we can check the mid-submission state
    mockSubmitTask.mockReturnValue(new Promise(() => {}))
    renderModal()

    await user.click(screen.getByRole('button', { name: /submit task/i }))

    expect(screen.getByRole('button', { name: /submitting/i })).toBeDisabled()
  })

  it('shows spinner text "Submitting..." during submission', async () => {
    const user = userEvent.setup()
    mockSubmitTask.mockReturnValue(new Promise(() => {}))
    renderModal()

    await user.click(screen.getByRole('button', { name: /submit task/i }))

    expect(screen.getByText(/submitting/i)).toBeInTheDocument()
  })
})

// ---------------------------------------------------------------------------
// Error handling
// ---------------------------------------------------------------------------

describe('SubmitTaskModal — error handling', () => {
  it('shows the API error message when submission fails', async () => {
    const user = userEvent.setup()
    mockSubmitTask.mockRejectedValue(new Error('500: Internal server error'))
    renderModal()

    await user.click(screen.getByRole('button', { name: /submit task/i }))

    expect(await screen.findByRole('alert')).toBeInTheDocument()
    expect(screen.getByText(/500: Internal server error/i)).toBeInTheDocument()
  })

  it('re-enables the submit button after a failed submission', async () => {
    const user = userEvent.setup()
    mockSubmitTask.mockRejectedValue(new Error('500: server error'))
    renderModal()

    await user.click(screen.getByRole('button', { name: /submit task/i }))

    await screen.findByRole('alert')
    expect(screen.getByRole('button', { name: /submit task/i })).not.toBeDisabled()
  })

  it('does not call onSuccess when submission fails', async () => {
    const user = userEvent.setup()
    mockSubmitTask.mockRejectedValue(new Error('400: bad request'))
    const onSuccess = vi.fn()
    renderModal({ onSuccess })

    await user.click(screen.getByRole('button', { name: /submit task/i }))

    await screen.findByRole('alert')
    expect(onSuccess).not.toHaveBeenCalled()
  })
})

// ---------------------------------------------------------------------------
// Cancel button
// ---------------------------------------------------------------------------

describe('SubmitTaskModal — cancel', () => {
  it('calls onClose when Cancel is clicked', async () => {
    const user = userEvent.setup()
    const onClose = vi.fn()
    renderModal({ onClose })

    await user.click(screen.getByRole('button', { name: /cancel/i }))

    expect(onClose).toHaveBeenCalled()
  })

  it('does not call submitTask when Cancel is clicked', async () => {
    const user = userEvent.setup()
    renderModal()

    await user.click(screen.getByRole('button', { name: /cancel/i }))

    expect(mockSubmitTask).not.toHaveBeenCalled()
  })

  it('calls onClose when the close (×) button is clicked', async () => {
    const user = userEvent.setup()
    const onClose = vi.fn()
    renderModal({ onClose })

    await user.click(screen.getByRole('button', { name: /close/i }))

    expect(onClose).toHaveBeenCalled()
  })
})

// ---------------------------------------------------------------------------
// State reset on re-open
// ---------------------------------------------------------------------------

describe('SubmitTaskModal — state reset', () => {
  it('resets added parameters when the modal is closed and re-opened', async () => {
    const user = userEvent.setup()
    const { rerender, onClose } = renderModal()

    // Add a parameter
    await user.click(screen.getByRole('button', { name: /add parameter/i }))
    expect(screen.getAllByPlaceholderText(/key/i).length).toBe(1)

    // Close by re-rendering with isOpen=false then isOpen=true
    rerender(
      <SubmitTaskModal
        isOpen={false}
        onClose={onClose}
        onSuccess={vi.fn()}
        pipelines={[makePipeline()]}
      />
    )
    rerender(
      <SubmitTaskModal
        isOpen={true}
        onClose={onClose}
        onSuccess={vi.fn()}
        pipelines={[makePipeline()]}
      />
    )

    // Parameters should be gone
    expect(screen.queryAllByPlaceholderText(/key/i).length).toBe(0)
  })

  it('resets error message when the modal is re-opened', async () => {
    const user = userEvent.setup()
    mockSubmitTask.mockRejectedValue(new Error('500: server error'))
    const { rerender, onClose } = renderModal()

    await user.click(screen.getByRole('button', { name: /submit task/i }))
    await screen.findByRole('alert')

    rerender(
      <SubmitTaskModal
        isOpen={false}
        onClose={onClose}
        onSuccess={vi.fn()}
        pipelines={[makePipeline()]}
      />
    )
    rerender(
      <SubmitTaskModal
        isOpen={true}
        onClose={onClose}
        onSuccess={vi.fn()}
        pipelines={[makePipeline()]}
      />
    )

    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })
})
