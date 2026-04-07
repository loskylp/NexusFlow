/**
 * Unit tests for the useTasks hook.
 * Covers: mergeTaskEvent pure function (all event types, unknown events),
 * initial REST fetch, error handling, SSE event application, and refresh.
 *
 * See: TASK-021, REQ-017
 */
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, act, waitFor } from '@testing-library/react'
import { mergeTaskEvent, useTasks } from './useTasks'
import type { Task, SSEEvent } from '@/types/domain'

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

vi.mock('@/api/client', () => ({
  listTasksWithFilters: vi.fn(),
}))

import * as client from '@/api/client'
const mockListTasks = vi.mocked(client.listTasksWithFilters)

// Intercept useSSE to control SSE events programmatically.
let capturedOnEvent: ((event: SSEEvent<Task>) => void) | null = null
let stubbedSSEStatus: 'connecting' | 'connected' | 'reconnecting' | 'closed' = 'connected'

vi.mock('./useSSE', () => ({
  useSSE: vi.fn(({ onEvent }: { onEvent: (e: SSEEvent<Task>) => void }) => {
    capturedOnEvent = onEvent
    return { status: stubbedSSEStatus, close: vi.fn() }
  }),
}))

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
    createdAt: '2026-04-01T00:00:00Z',
    updatedAt: '2026-04-01T00:00:00Z',
    ...overrides,
  }
}

function fireSSE(event: SSEEvent<Task>) {
  act(() => {
    capturedOnEvent?.(event)
  })
}

// ---------------------------------------------------------------------------
// Setup / teardown
// ---------------------------------------------------------------------------

beforeEach(() => {
  capturedOnEvent = null
  stubbedSSEStatus = 'connected'
  vi.clearAllMocks()
})

afterEach(() => {
  vi.restoreAllMocks()
})

// ---------------------------------------------------------------------------
// mergeTaskEvent — pure function tests
// ---------------------------------------------------------------------------

describe('mergeTaskEvent — task:submitted', () => {
  it('adds a new task when not already present', () => {
    const existing = makeTask({ id: 'task-001' })
    const newTask = makeTask({ id: 'task-002' })
    const result = mergeTaskEvent([existing], { type: 'task:submitted', payload: newTask })
    expect(result).toHaveLength(2)
    expect(result.some(t => t.id === 'task-002')).toBe(true)
  })

  it('does not add a duplicate when the task is already present', () => {
    const existing = makeTask({ id: 'task-001' })
    const result = mergeTaskEvent([existing], { type: 'task:submitted', payload: existing })
    expect(result).toHaveLength(1)
  })

  it('returns a new array (does not mutate input)', () => {
    const tasks: Task[] = []
    const newTask = makeTask({ id: 'task-001' })
    const result = mergeTaskEvent(tasks, { type: 'task:submitted', payload: newTask })
    expect(result).not.toBe(tasks)
  })
})

describe('mergeTaskEvent — status update events', () => {
  const updateEvents = [
    'task:queued',
    'task:assigned',
    'task:running',
    'task:completed',
    'task:failed',
    'task:cancelled',
  ] as const

  for (const eventType of updateEvents) {
    it(`merges updated status for ${eventType}`, () => {
      const task = makeTask({ id: 'task-001', status: 'submitted' })
      const updatedTask = { ...task, status: 'queued' as const, updatedAt: '2026-04-02T00:00:00Z' }
      const result = mergeTaskEvent([task], { type: eventType, payload: updatedTask })
      expect(result).toHaveLength(1)
      expect(result[0]).toEqual(updatedTask)
    })

    it(`ignores ${eventType} for an unknown task ID`, () => {
      const task = makeTask({ id: 'task-001' })
      const unknown = makeTask({ id: 'task-unknown' })
      const result = mergeTaskEvent([task], { type: eventType, payload: unknown })
      expect(result).toHaveLength(1)
      expect(result[0]?.id).toBe('task-001')
    })
  }

  it('returns a new array on status update (does not mutate input)', () => {
    const tasks = [makeTask({ id: 'task-001' })]
    const updated = makeTask({ id: 'task-001', status: 'queued' })
    const result = mergeTaskEvent(tasks, { type: 'task:queued', payload: updated })
    expect(result).not.toBe(tasks)
  })
})

describe('mergeTaskEvent — unknown event types', () => {
  it('returns the original array unchanged for unknown event types', () => {
    const tasks = [makeTask({ id: 'task-001' })]
    const result = mergeTaskEvent(tasks, { type: 'task:unknown-event', payload: makeTask() })
    expect(result).toBe(tasks)
  })

  it('handles empty task list with unknown event', () => {
    const result = mergeTaskEvent([], { type: 'irrelevant', payload: makeTask() })
    expect(result).toHaveLength(0)
  })
})

// ---------------------------------------------------------------------------
// useTasks — initial fetch
// ---------------------------------------------------------------------------

describe('useTasks — initial fetch', () => {
  it('starts in loading state before the REST call resolves', () => {
    mockListTasks.mockResolvedValue([])
    const { result } = renderHook(() => useTasks())
    expect(result.current.isLoading).toBe(true)
  })

  it('populates tasks from REST response after fetch resolves', async () => {
    const tasks = [makeTask({ id: 'task-001' }), makeTask({ id: 'task-002', status: 'running' })]
    mockListTasks.mockResolvedValue(tasks)

    const { result } = renderHook(() => useTasks())

    await waitFor(() => expect(result.current.isLoading).toBe(false))
    expect(result.current.tasks).toEqual(tasks)
  })

  it('sets error and clears loading on fetch failure', async () => {
    mockListTasks.mockRejectedValue(new Error('network error'))

    const { result } = renderHook(() => useTasks())

    await waitFor(() => expect(result.current.isLoading).toBe(false))
    expect(result.current.error).toBeTruthy()
    expect(result.current.tasks).toHaveLength(0)
  })

  it('passes filter params to the API', async () => {
    mockListTasks.mockResolvedValue([])

    renderHook(() => useTasks({ status: 'running', pipelineId: 'pipe-1' }))

    await waitFor(() => {
      expect(mockListTasks).toHaveBeenCalledWith({ status: 'running', pipelineId: 'pipe-1' })
    })
  })
})

// ---------------------------------------------------------------------------
// useTasks — SSE event merging
// ---------------------------------------------------------------------------

describe('useTasks — SSE event merging', () => {
  it('adds a new task when task:submitted arrives', async () => {
    mockListTasks.mockResolvedValue([])
    const { result } = renderHook(() => useTasks())
    await waitFor(() => expect(result.current.isLoading).toBe(false))

    const newTask = makeTask({ id: 'task-new' })
    fireSSE({ type: 'task:submitted', payload: newTask })

    expect(result.current.tasks).toHaveLength(1)
    expect(result.current.tasks[0]?.id).toBe('task-new')
  })

  it('updates task status when task:running arrives', async () => {
    const task = makeTask({ id: 'task-001', status: 'queued' })
    mockListTasks.mockResolvedValue([task])
    const { result } = renderHook(() => useTasks())
    await waitFor(() => expect(result.current.isLoading).toBe(false))

    const updated = { ...task, status: 'running' as const }
    fireSSE({ type: 'task:running', payload: updated })

    expect(result.current.tasks[0]?.status).toBe('running')
  })

  it('updates task status when task:completed arrives', async () => {
    const task = makeTask({ id: 'task-001', status: 'running' })
    mockListTasks.mockResolvedValue([task])
    const { result } = renderHook(() => useTasks())
    await waitFor(() => expect(result.current.isLoading).toBe(false))

    const updated = { ...task, status: 'completed' as const }
    fireSSE({ type: 'task:completed', payload: updated })

    expect(result.current.tasks[0]?.status).toBe('completed')
  })
})

// ---------------------------------------------------------------------------
// useTasks — refresh
// ---------------------------------------------------------------------------

describe('useTasks — refresh', () => {
  it('re-fetches the task list when refresh() is called', async () => {
    mockListTasks.mockResolvedValue([])
    const { result } = renderHook(() => useTasks())
    await waitFor(() => expect(result.current.isLoading).toBe(false))

    act(() => { result.current.refresh() })

    await waitFor(() => expect(mockListTasks).toHaveBeenCalledTimes(2))
  })
})

// ---------------------------------------------------------------------------
// useTasks — SSE status passthrough
// ---------------------------------------------------------------------------

describe('useTasks — SSE status passthrough', () => {
  it('exposes the sseStatus returned from useSSE', async () => {
    stubbedSSEStatus = 'reconnecting'
    mockListTasks.mockResolvedValue([])

    const { result } = renderHook(() => useTasks())
    await waitFor(() => expect(result.current.isLoading).toBe(false))

    expect(result.current.sseStatus).toBe('reconnecting')
  })
})
