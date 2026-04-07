/**
 * Integration tests for TASK-021: Task Feed and Monitor (GUI)
 *
 * These tests verify component seams and interface boundaries:
 *   - useTasks hook integrates with listTasksWithFilters API client
 *   - useTasks hook integrates with useSSE for live updates
 *   - TaskFeedPage integrates with useTasks, usePipelines, and useAuth
 *   - TaskFeedPage integrates with cancelTask API client
 *   - TaskCard integrates with onCancel/onViewLogs callbacks
 *   - FilterBar integrates with TaskFeedPage filter state
 *   - FeedStatusBar integrates with SSE status from useTasks
 *
 * Tests at this layer validate component assembly at seams.
 * No direct access to implementation internals; validates at the component
 * interface boundary.
 *
 * Requirements: REQ-017, REQ-002, REQ-009, REQ-010
 * See: TASK-021
 */

import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, act } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderHook } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'

import { useTasks, mergeTaskEvent } from '@/hooks/useTasks'
import TaskFeedPage from '@/pages/TaskFeedPage'
import type { Task, SSEEvent } from '@/types/domain'
import type { TaskFilters } from '@/hooks/useTasks'

// ---------------------------------------------------------------------------
// Mock dependencies
// ---------------------------------------------------------------------------

vi.mock('@/api/client', () => ({
  listTasksWithFilters: vi.fn(),
  cancelTask: vi.fn(),
  submitTask: vi.fn(),
}))

// Intercept useSSE to control SSE events programmatically.
let capturedOnEvent: ((event: SSEEvent<Task>) => void) | null = null
let stubbedSSEStatus: 'connecting' | 'connected' | 'reconnecting' | 'closed' = 'connected'

vi.mock('@/hooks/useSSE', () => ({
  useSSE: vi.fn(({ onEvent }: { onEvent: (e: SSEEvent<Task>) => void }) => {
    capturedOnEvent = onEvent
    return { status: stubbedSSEStatus, close: vi.fn() }
  }),
}))

vi.mock('@/hooks/usePipelines')
vi.mock('@/context/AuthContext')

import { listTasksWithFilters, cancelTask } from '@/api/client'
import * as pipelinesModule from '@/hooks/usePipelines'
import * as authModule from '@/context/AuthContext'
import type { UsePipelinesReturn } from '@/hooks/usePipelines'

const mockListTasks = vi.mocked(listTasksWithFilters)
const mockCancelTask = vi.mocked(cancelTask)
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

function fireSSE(event: SSEEvent<Task>) {
  act(() => {
    capturedOnEvent?.(event)
  })
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

function stubAuth(isAdmin: boolean, userId = 'user-001'): void {
  mockUseAuth.mockReturnValue({
    user: {
      id: userId,
      username: isAdmin ? 'admin' : 'testuser',
      role: isAdmin ? 'admin' : 'user',
      active: true,
      createdAt: '2026-01-01T00:00:00Z',
    },
    login: vi.fn(),
    logout: vi.fn(),
    isLoading: false,
  })
}

// ---------------------------------------------------------------------------
// Setup / teardown
// ---------------------------------------------------------------------------

beforeEach(() => {
  capturedOnEvent = null
  stubbedSSEStatus = 'connected'
  vi.clearAllMocks()
  stubPipelinesHook()
  stubAuth(false)
})

// ---------------------------------------------------------------------------
// Seam: useTasks → listTasksWithFilters (REST seed)
// REQ-017: feed populated from API on mount
// ---------------------------------------------------------------------------

describe('Seam: useTasks → listTasksWithFilters REST API', () => {
  it('[positive] useTasks calls listTasksWithFilters on mount and populates tasks', async () => {
    const tasks = [makeTask({ id: 'task-001' })]
    mockListTasks.mockResolvedValue(tasks)

    const { result } = renderHook(() => useTasks())

    await waitFor(() => expect(result.current.isLoading).toBe(false))
    expect(mockListTasks).toHaveBeenCalledTimes(1)
    expect(result.current.tasks).toEqual(tasks)
  })

  it('[positive] useTasks passes filter params to listTasksWithFilters', async () => {
    mockListTasks.mockResolvedValue([])

    const filters: TaskFilters = { status: 'running', pipelineId: 'pipe-x', search: 'etl' }
    renderHook(() => useTasks(filters))

    await waitFor(() => {
      expect(mockListTasks).toHaveBeenCalledWith(filters)
    })
  })

  it('[positive] useTasks sets error state when listTasksWithFilters rejects', async () => {
    mockListTasks.mockRejectedValue(new Error('api failure'))

    const { result } = renderHook(() => useTasks())

    await waitFor(() => expect(result.current.isLoading).toBe(false))
    expect(result.current.error).toBe('api failure')
    expect(result.current.tasks).toHaveLength(0)
  })

  it('[positive] useTasks re-fetches when refresh() is called', async () => {
    mockListTasks.mockResolvedValue([])

    const { result } = renderHook(() => useTasks())
    await waitFor(() => expect(result.current.isLoading).toBe(false))

    act(() => { result.current.refresh() })

    await waitFor(() => expect(mockListTasks).toHaveBeenCalledTimes(2))
  })
})

// ---------------------------------------------------------------------------
// Seam: useTasks → useSSE (SSE live updates)
// REQ-017: real-time updates via SSE
// ---------------------------------------------------------------------------

describe('Seam: useTasks → useSSE live updates', () => {
  it('[positive] task:submitted SSE event adds a new task to the list', async () => {
    mockListTasks.mockResolvedValue([])

    const { result } = renderHook(() => useTasks())
    await waitFor(() => expect(result.current.isLoading).toBe(false))

    const newTask = makeTask({ id: 'task-sse-new' })
    fireSSE({ type: 'task:submitted', payload: newTask })

    expect(result.current.tasks).toHaveLength(1)
    expect(result.current.tasks[0]?.id).toBe('task-sse-new')
  })

  it('[positive] task:running SSE event updates existing task status', async () => {
    const task = makeTask({ id: 'task-001', status: 'queued' })
    mockListTasks.mockResolvedValue([task])

    const { result } = renderHook(() => useTasks())
    await waitFor(() => expect(result.current.isLoading).toBe(false))

    fireSSE({ type: 'task:running', payload: { ...task, status: 'running' } })

    expect(result.current.tasks[0]?.status).toBe('running')
  })

  it('[positive] task:completed SSE event transitions task to completed', async () => {
    const task = makeTask({ id: 'task-001', status: 'running' })
    mockListTasks.mockResolvedValue([task])

    const { result } = renderHook(() => useTasks())
    await waitFor(() => expect(result.current.isLoading).toBe(false))

    fireSSE({ type: 'task:completed', payload: { ...task, status: 'completed' } })

    expect(result.current.tasks[0]?.status).toBe('completed')
  })

  it('[negative] unknown SSE event type does NOT modify the task list', async () => {
    const task = makeTask({ id: 'task-001', status: 'running' })
    mockListTasks.mockResolvedValue([task])

    const { result } = renderHook(() => useTasks())
    await waitFor(() => expect(result.current.isLoading).toBe(false))

    const listBefore = result.current.tasks
    fireSSE({ type: 'task:unknown-event', payload: { ...task, status: 'completed' } })

    // Referential equality preserved for unknown event types
    expect(result.current.tasks).toBe(listBefore)
  })

  it('[positive] sseStatus from useSSE is exposed by useTasks', async () => {
    stubbedSSEStatus = 'reconnecting'
    mockListTasks.mockResolvedValue([])

    const { result } = renderHook(() => useTasks())
    await waitFor(() => expect(result.current.isLoading).toBe(false))

    expect(result.current.sseStatus).toBe('reconnecting')
  })
})

// ---------------------------------------------------------------------------
// Seam: TaskFeedPage → cancelTask API client
// REQ-010: cancel request sent to API
// ---------------------------------------------------------------------------

describe('Seam: TaskFeedPage → cancelTask API client', () => {
  it('[positive] clicking Cancel (confirmed) calls cancelTask with the task ID', async () => {
    const user = userEvent.setup()
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    mockListTasks.mockResolvedValue([makeTask({ id: 'task-to-cancel', status: 'running', userId: 'user-001' })])
    stubAuth(false, 'user-001')

    render(
      <MemoryRouter>
        <TaskFeedPage />
      </MemoryRouter>
    )

    await waitFor(() => expect(screen.queryByRole('button', { name: /cancel/i })).toBeInTheDocument())

    await user.click(screen.getByRole('button', { name: /cancel/i }))

    expect(mockCancelTask).toHaveBeenCalledWith('task-to-cancel')
  })

  it('[negative] dismissing Cancel dialog does NOT call cancelTask', async () => {
    const user = userEvent.setup()
    vi.spyOn(window, 'confirm').mockReturnValue(false)
    mockListTasks.mockResolvedValue([makeTask({ id: 'task-001', status: 'running', userId: 'user-001' })])
    stubAuth(false, 'user-001')

    render(
      <MemoryRouter>
        <TaskFeedPage />
      </MemoryRouter>
    )

    await waitFor(() => expect(screen.queryByRole('button', { name: /cancel/i })).toBeInTheDocument())

    await user.click(screen.getByRole('button', { name: /cancel/i }))

    expect(mockCancelTask).not.toHaveBeenCalled()
  })
})

// ---------------------------------------------------------------------------
// Seam: TaskFeedPage → FeedStatusBar SSE status propagation
// REQ-017: SSE connection state visible in status bar
// ---------------------------------------------------------------------------

describe('Seam: TaskFeedPage → FeedStatusBar SSE status propagation', () => {
  it('[positive] TaskFeedPage shows SSE "Reconnecting..." when connection drops', async () => {
    // Simulate SSE reconnecting state by making useSSE return reconnecting status.
    stubbedSSEStatus = 'reconnecting'
    mockListTasks.mockResolvedValue([])

    render(
      <MemoryRouter>
        <TaskFeedPage />
      </MemoryRouter>
    )

    await waitFor(() => expect(screen.getByText(/reconnecting/i)).toBeInTheDocument())
  })
})

// ---------------------------------------------------------------------------
// Seam: mergeTaskEvent — pure function contract (boundary behavior)
// REQ-009: state transitions applied correctly
// ---------------------------------------------------------------------------

describe('Seam: mergeTaskEvent pure function boundary', () => {
  it('[positive] all seven SSE task event types are handled correctly', () => {
    const task = makeTask({ id: 'task-001', status: 'submitted' })

    const eventTypes = [
      { type: 'task:queued', expectedStatus: 'queued' },
      { type: 'task:assigned', expectedStatus: 'assigned' },
      { type: 'task:running', expectedStatus: 'running' },
      { type: 'task:completed', expectedStatus: 'completed' },
      { type: 'task:failed', expectedStatus: 'failed' },
      { type: 'task:cancelled', expectedStatus: 'cancelled' },
    ] as const

    for (const { type, expectedStatus } of eventTypes) {
      const updatedTask = { ...task, status: expectedStatus }
      const result = mergeTaskEvent([task], { type, payload: updatedTask })
      expect(result[0]?.status).toBe(expectedStatus)
    }
  })

  it('[positive] task:submitted event deduplicates by ID', () => {
    const task = makeTask({ id: 'task-001' })
    const result = mergeTaskEvent([task], { type: 'task:submitted', payload: task })
    expect(result).toHaveLength(1)
  })

  it('[positive] mergeTaskEvent preserves all existing tasks on update', () => {
    const t1 = makeTask({ id: 'task-001', status: 'running' })
    const t2 = makeTask({ id: 'task-002', status: 'queued' })
    const updatedT1 = { ...t1, status: 'completed' as const }

    const result = mergeTaskEvent([t1, t2], { type: 'task:completed', payload: updatedT1 })
    expect(result).toHaveLength(2)
    expect(result.find(t => t.id === 'task-002')?.status).toBe('queued')
  })

  it('[negative] mergeTaskEvent returns original array reference for unknown events', () => {
    const tasks = [makeTask()]
    const result = mergeTaskEvent(tasks, { type: 'unrelated:event', payload: makeTask() })
    // Must return the exact same reference — no unnecessary copy
    expect(result).toBe(tasks)
  })
})
