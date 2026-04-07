/**
 * Unit tests for TaskCard component and its exported helper functions.
 * Covers: isCancellable, statusBadgeStyle, TaskCard rendering,
 * action button visibility rules, and the recently-updated flash prop.
 *
 * See: TASK-021, REQ-010, REQ-017
 */
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import TaskCard, { isCancellable, statusBadgeStyle } from './TaskCard'
import type { Task } from '@/types/domain'

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

function renderCard(task: Task, overrides: {
  isAdmin?: boolean
  isOwner?: boolean
  isRecentlyUpdated?: boolean
} = {}) {
  const onViewLogs = vi.fn()
  const onCancel = vi.fn()
  const onRetry = vi.fn()

  render(
    <MemoryRouter>
      <TaskCard
        task={task}
        pipelineName="Test Pipeline"
        isAdmin={overrides.isAdmin ?? false}
        isOwner={overrides.isOwner ?? true}
        onViewLogs={onViewLogs}
        onCancel={onCancel}
        onRetry={onRetry}
        isRecentlyUpdated={overrides.isRecentlyUpdated}
      />
    </MemoryRouter>
  )

  return { onViewLogs, onCancel, onRetry }
}

// ---------------------------------------------------------------------------
// isCancellable
// ---------------------------------------------------------------------------

describe('isCancellable', () => {
  it('returns true for submitted', () => {
    expect(isCancellable('submitted')).toBe(true)
  })

  it('returns true for queued', () => {
    expect(isCancellable('queued')).toBe(true)
  })

  it('returns true for assigned', () => {
    expect(isCancellable('assigned')).toBe(true)
  })

  it('returns true for running', () => {
    expect(isCancellable('running')).toBe(true)
  })

  it('returns false for completed', () => {
    expect(isCancellable('completed')).toBe(false)
  })

  it('returns false for failed', () => {
    expect(isCancellable('failed')).toBe(false)
  })

  it('returns false for cancelled', () => {
    expect(isCancellable('cancelled')).toBe(false)
  })
})

// ---------------------------------------------------------------------------
// statusBadgeStyle
// ---------------------------------------------------------------------------

describe('statusBadgeStyle', () => {
  it('returns a style object for every valid status', () => {
    const statuses = ['submitted', 'queued', 'assigned', 'running', 'completed', 'failed', 'cancelled'] as const
    for (const status of statuses) {
      const style = statusBadgeStyle(status)
      expect(style).toBeDefined()
      expect(typeof style).toBe('object')
    }
  })

  it('includes a color property for each status', () => {
    const statuses = ['submitted', 'queued', 'assigned', 'running', 'completed', 'failed', 'cancelled'] as const
    for (const status of statuses) {
      const style = statusBadgeStyle(status)
      expect(style.color).toBeDefined()
    }
  })

  it('uses violet color token for submitted', () => {
    const style = statusBadgeStyle('submitted')
    expect(JSON.stringify(style)).toContain('8B5CF6')
  })

  it('uses green color token for completed', () => {
    const style = statusBadgeStyle('completed')
    expect(JSON.stringify(style)).toContain('16A34A')
  })

  it('uses red color token for failed', () => {
    const style = statusBadgeStyle('failed')
    expect(JSON.stringify(style)).toContain('DC2626')
  })
})

// ---------------------------------------------------------------------------
// TaskCard — basic rendering
// ---------------------------------------------------------------------------

describe('TaskCard — basic rendering', () => {
  it('renders the task ID', () => {
    renderCard(makeTask({ id: 'task-abc123' }))
    expect(screen.getByText(/task-abc123/i)).toBeInTheDocument()
  })

  it('renders the pipeline name', () => {
    renderCard(makeTask())
    expect(screen.getByText(/test pipeline/i)).toBeInTheDocument()
  })

  it('renders the task status badge', () => {
    renderCard(makeTask({ status: 'running' }))
    expect(screen.getByText(/running/i)).toBeInTheDocument()
  })

  it('always renders the View Logs button', () => {
    renderCard(makeTask())
    expect(screen.getByRole('button', { name: /view logs/i })).toBeInTheDocument()
  })
})

// ---------------------------------------------------------------------------
// TaskCard — Cancel button visibility
// ---------------------------------------------------------------------------

describe('TaskCard — Cancel button visibility', () => {
  it('shows Cancel for the task owner when status is cancellable', () => {
    renderCard(makeTask({ status: 'running' }), { isOwner: true, isAdmin: false })
    expect(screen.getByRole('button', { name: /cancel/i })).toBeInTheDocument()
  })

  it('shows Cancel for admin on any cancellable task', () => {
    renderCard(makeTask({ status: 'queued' }), { isAdmin: true, isOwner: false })
    expect(screen.getByRole('button', { name: /cancel/i })).toBeInTheDocument()
  })

  it('hides Cancel when status is completed', () => {
    renderCard(makeTask({ status: 'completed' }), { isOwner: true })
    expect(screen.queryByRole('button', { name: /cancel/i })).not.toBeInTheDocument()
  })

  it('hides Cancel when status is failed', () => {
    renderCard(makeTask({ status: 'failed' }), { isOwner: true })
    expect(screen.queryByRole('button', { name: /cancel/i })).not.toBeInTheDocument()
  })

  it('hides Cancel when status is cancelled', () => {
    renderCard(makeTask({ status: 'cancelled' }), { isOwner: true })
    expect(screen.queryByRole('button', { name: /cancel/i })).not.toBeInTheDocument()
  })

  it('hides Cancel for non-owner non-admin on cancellable status', () => {
    renderCard(makeTask({ status: 'running' }), { isOwner: false, isAdmin: false })
    expect(screen.queryByRole('button', { name: /cancel/i })).not.toBeInTheDocument()
  })
})

// ---------------------------------------------------------------------------
// TaskCard — Retry button visibility
// ---------------------------------------------------------------------------

describe('TaskCard — Retry button visibility', () => {
  it('shows Retry only when status is failed', () => {
    renderCard(makeTask({ status: 'failed' }), { isOwner: true })
    expect(screen.getByRole('button', { name: /retry/i })).toBeInTheDocument()
  })

  it('hides Retry when status is not failed', () => {
    const nonFailedStatuses = ['submitted', 'queued', 'assigned', 'running', 'completed', 'cancelled'] as const
    for (const status of nonFailedStatuses) {
      const { unmount } = render(
        <MemoryRouter>
          <TaskCard
            task={makeTask({ status })}
            pipelineName="Pipeline"
            isAdmin={false}
            isOwner={true}
            onViewLogs={vi.fn()}
            onCancel={vi.fn()}
            onRetry={vi.fn()}
          />
        </MemoryRouter>
      )
      expect(screen.queryByRole('button', { name: /retry/i })).not.toBeInTheDocument()
      unmount()
    }
  })
})

// ---------------------------------------------------------------------------
// TaskCard — action callbacks
// ---------------------------------------------------------------------------

describe('TaskCard — action callbacks', () => {
  it('calls onViewLogs with the task ID when View Logs is clicked', async () => {
    const user = userEvent.setup()
    const { onViewLogs } = renderCard(makeTask({ id: 'task-xyz' }))
    await user.click(screen.getByRole('button', { name: /view logs/i }))
    expect(onViewLogs).toHaveBeenCalledWith('task-xyz')
  })

  it('calls onCancel with task ID after confirmation dialog confirms', async () => {
    const user = userEvent.setup()
    vi.spyOn(window, 'confirm').mockReturnValue(true)

    const { onCancel } = renderCard(makeTask({ id: 'task-xyz', status: 'running' }), { isOwner: true })
    await user.click(screen.getByRole('button', { name: /cancel/i }))
    expect(onCancel).toHaveBeenCalledWith('task-xyz')
  })

  it('does not call onCancel when confirmation dialog is dismissed', async () => {
    const user = userEvent.setup()
    vi.spyOn(window, 'confirm').mockReturnValue(false)

    const { onCancel } = renderCard(makeTask({ id: 'task-xyz', status: 'running' }), { isOwner: true })
    await user.click(screen.getByRole('button', { name: /cancel/i }))
    expect(onCancel).not.toHaveBeenCalled()
  })

  it('calls onRetry with the task when Retry is clicked', async () => {
    const user = userEvent.setup()
    const task = makeTask({ status: 'failed' })
    const { onRetry } = renderCard(task, { isOwner: true })
    await user.click(screen.getByRole('button', { name: /retry/i }))
    expect(onRetry).toHaveBeenCalledWith(task)
  })
})

// ---------------------------------------------------------------------------
// TaskCard — failed state styling
// ---------------------------------------------------------------------------

describe('TaskCard — failed state', () => {
  it('renders the failed status indicator for a failed task', () => {
    // The task has no errorReason field in the domain type, but status is 'failed'.
    // Both the status badge and the error alert text contain "failed".
    renderCard(makeTask({ status: 'failed' }))
    expect(screen.getAllByText(/failed/i).length).toBeGreaterThan(0)
  })
})

// ---------------------------------------------------------------------------
// TaskCard — recently updated flash
// ---------------------------------------------------------------------------

describe('TaskCard — isRecentlyUpdated', () => {
  it('renders without error when isRecentlyUpdated is true', () => {
    expect(() => {
      renderCard(makeTask(), { isRecentlyUpdated: true })
    }).not.toThrow()
  })

  it('renders without error when isRecentlyUpdated is false', () => {
    expect(() => {
      renderCard(makeTask(), { isRecentlyUpdated: false })
    }).not.toThrow()
  })
})

// ---------------------------------------------------------------------------
// TaskCard — worker assignment display
// ---------------------------------------------------------------------------

describe('TaskCard — worker assignment', () => {
  it('shows the worker ID when a task has a workerId', () => {
    renderCard(makeTask({ workerId: 'worker-42' }))
    expect(screen.getByText(/worker-42/i)).toBeInTheDocument()
  })
})

beforeEach(() => {
  vi.clearAllMocks()
})
