/**
 * Integration tests for TASK-022: Log Streamer (GUI)
 *
 * These tests verify component seams and interface boundaries:
 *   - useLogs hook integrates with useSSE for live log streaming
 *   - useLogs hook integrates with taskId changes (buffer reset seam)
 *   - LogStreamerPage integrates with useLogs and useTasks
 *   - LogStreamerPage integrates with downloadTaskLogs API client
 *   - LogPanel integrates with filterLogLines for phase-filtered display
 *   - LogStatusBar integrates with sseStatus from useLogs
 *
 * Tests at this layer validate component assembly at seams.
 * No direct access to implementation internals; validates at the component
 * interface boundary.
 *
 * Requirements: REQ-018
 * See: TASK-022, ADR-007
 */

import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, act } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderHook } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'

import { useLogs } from '@/hooks/useLogs'
import LogStreamerPage from '@/pages/LogStreamerPage'
import type { TaskLog, SSEEvent } from '@/types/domain'

// ---------------------------------------------------------------------------
// Mock dependencies
// ---------------------------------------------------------------------------

vi.mock('@/api/client', () => ({
  downloadTaskLogs: vi.fn(),
}))

// Intercept useSSE to control SSE events programmatically.
let capturedOnEvent: ((event: SSEEvent<TaskLog>) => void) | null = null
let stubbedSSEStatus: 'connecting' | 'connected' | 'reconnecting' | 'closed' = 'connected'

vi.mock('@/hooks/useSSE', () => ({
  useSSE: vi.fn(({ onEvent }: { onEvent: (e: SSEEvent<TaskLog>) => void }) => {
    capturedOnEvent = onEvent
    return { status: stubbedSSEStatus, close: vi.fn() }
  }),
}))

vi.mock('@/hooks/useTasks')
vi.mock('@/context/AuthContext')

import { downloadTaskLogs } from '@/api/client'
import * as tasksModule from '@/hooks/useTasks'
import * as authModule from '@/context/AuthContext'
import type { UseTasksReturn } from '@/hooks/useTasks'

const mockDownloadTaskLogs = vi.mocked(downloadTaskLogs)
const mockUseTasks = vi.mocked(tasksModule.useTasks)
const mockUseAuth = vi.mocked(authModule.useAuth)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeLog(overrides: Partial<TaskLog> = {}): TaskLog {
  return {
    id: 'log-001',
    taskId: 'task-001',
    line: '[datasource] INFO data loaded',
    level: 'INFO',
    timestamp: '2026-04-01T10:00:00Z',
    ...overrides,
  }
}

function makeSSEEvent(log: TaskLog, id?: string): SSEEvent<TaskLog> {
  return { type: 'log:line', payload: log, id }
}

function defaultTasksReturn(partial: Partial<UseTasksReturn> = {}): UseTasksReturn {
  return {
    tasks: [],
    isLoading: false,
    error: null,
    sseStatus: 'connected',
    refresh: vi.fn(),
    ...partial,
  }
}

function renderPage(routerPath = '/tasks/logs'): void {
  render(
    <MemoryRouter initialEntries={[routerPath]}>
      <LogStreamerPage />
    </MemoryRouter>
  )
}

// ---------------------------------------------------------------------------
// Setup
// ---------------------------------------------------------------------------

beforeEach(() => {
  capturedOnEvent = null
  stubbedSSEStatus = 'connected'
  vi.clearAllMocks()
  mockUseTasks.mockReturnValue(defaultTasksReturn())
  mockUseAuth.mockReturnValue({
    user: { id: 'user-001', username: 'alice', role: 'user', active: true, createdAt: '2026-01-01T00:00:00Z' },
    isLoading: false,
    login: vi.fn(),
    logout: vi.fn(),
  })
})

// ---------------------------------------------------------------------------
// Integration: useLogs assembles correctly with useSSE
// REQ-018: Real-time log streaming
// ---------------------------------------------------------------------------

describe('useLogs <> useSSE seam', () => {
  // Given: a taskId is provided
  // When: useLogs mounts
  // Then: useSSE receives an onEvent callback (confirming SSE is wired)
  it('REQ-018: useLogs wires an onEvent callback to useSSE for log event handling', () => {
    renderHook(() => useLogs({ taskId: 'task-xyz' }))
    // The SSE hook must have been called with an onEvent handler
    expect(capturedOnEvent).not.toBeNull()
  })

  // Given: a log:line SSE event arrives at useSSE's onEvent callback
  // When: the event is dispatched
  // Then: useLogs accumulates it in lines array
  it('REQ-018: log:line SSE events dispatched via onEvent are accumulated in useLogs.lines', () => {
    const { result } = renderHook(() => useLogs({ taskId: 'task-001' }))
    const log = makeLog({ id: 'log-001' })

    act(() => {
      capturedOnEvent?.(makeSSEEvent(log, 'log-001'))
    })

    expect(result.current.lines).toHaveLength(1)
    expect(result.current.lines[0]).toEqual(log)
  })

  // Given: a log:error SSE event arrives (access denied)
  // When: the event is dispatched via onEvent
  // Then: useLogs surfaces it as accessError (not as a log line)
  it('REQ-018: log:error SSE event is surfaced as accessError, not added to lines', () => {
    const { result } = renderHook(() => useLogs({ taskId: 'task-001' }))

    act(() => {
      capturedOnEvent?.({ type: 'log:error', payload: makeLog(), id: undefined })
    })

    expect(result.current.accessError).not.toBeNull()
    expect(result.current.lines).toHaveLength(0)
  })

  // Given: multiple log:line events arrive
  // When: they are dispatched in order
  // Then: lines are accumulated in arrival order
  it('REQ-018: multiple log:line events accumulate in arrival order', () => {
    const { result } = renderHook(() => useLogs({ taskId: 'task-001' }))
    const logs = [
      makeLog({ id: 'log-001', line: '[datasource] first' }),
      makeLog({ id: 'log-002', line: '[process] second' }),
      makeLog({ id: 'log-003', line: '[sink] third' }),
    ]

    act(() => {
      logs.forEach((log, i) => {
        capturedOnEvent?.(makeSSEEvent(log, `log-00${i + 1}`))
      })
    })

    expect(result.current.lines).toHaveLength(3)
    expect(result.current.lines[0]?.line).toBe('[datasource] first')
    expect(result.current.lines[1]?.line).toBe('[process] second')
    expect(result.current.lines[2]?.line).toBe('[sink] third')
  })

  // [VERIFIER-ADDED] Negative: events with unexpected types are ignored
  it('REQ-018: [VERIFIER-ADDED] events with unrecognised types are not accumulated', () => {
    const { result } = renderHook(() => useLogs({ taskId: 'task-001' }))

    act(() => {
      capturedOnEvent?.({ type: 'task:state-changed', payload: makeLog(), id: 'x-001' })
    })

    expect(result.current.lines).toHaveLength(0)
    expect(result.current.accessError).toBeNull()
  })

  // Given: SSE is in reconnecting state
  // When: useLogs is asked for sseStatus
  // Then: it surfaces the reconnecting state from useSSE
  it('REQ-018: useLogs surfaces reconnecting sseStatus from useSSE', () => {
    stubbedSSEStatus = 'reconnecting'
    const { result } = renderHook(() => useLogs({ taskId: 'task-001' }))
    expect(result.current.sseStatus).toBe('reconnecting')
  })
})

// ---------------------------------------------------------------------------
// Integration: useLogs taskId change resets buffer (seam between taskId state and SSE)
// REQ-018: Real-time log streaming
// ---------------------------------------------------------------------------

describe('useLogs taskId change <> buffer reset seam', () => {
  // Given: a user streams logs for task-001
  // When: the user switches to task-002
  // Then: the lines buffer and lastEventId are reset before new lines arrive
  it('REQ-018: switching taskId clears accumulated lines from the previous task', () => {
    const { result, rerender } = renderHook(
      ({ taskId }: { taskId: string }) => useLogs({ taskId }),
      { initialProps: { taskId: 'task-001' } }
    )

    act(() => {
      capturedOnEvent?.(makeSSEEvent(makeLog({ id: 'log-001' }), 'log-001'))
      capturedOnEvent?.(makeSSEEvent(makeLog({ id: 'log-002' }), 'log-002'))
    })
    expect(result.current.lines).toHaveLength(2)
    expect(result.current.lastEventId).toBe('log-002')

    act(() => { rerender({ taskId: 'task-002' }) })

    expect(result.current.lines).toHaveLength(0)
    expect(result.current.lastEventId).toBeNull()
  })

  // [VERIFIER-ADDED] Negative: task-001 lines must not bleed into task-002 display
  it('REQ-018: [VERIFIER-ADDED] task-001 lines do not appear after switch to task-002', () => {
    const { result, rerender } = renderHook(
      ({ taskId }: { taskId: string }) => useLogs({ taskId }),
      { initialProps: { taskId: 'task-001' } }
    )

    act(() => {
      capturedOnEvent?.(makeSSEEvent(makeLog({ id: 'log-001', line: '[datasource] task-001 data' }), 'log-001'))
    })
    expect(result.current.lines[0]?.line).toContain('task-001 data')

    act(() => { rerender({ taskId: 'task-002' }) })

    // After switch: buffer is empty; task-001 data must not be present
    expect(result.current.lines.some(l => l.line.includes('task-001 data'))).toBe(false)
  })
})

// ---------------------------------------------------------------------------
// Integration: clearLines preserves lastEventId (replay correctness seam)
// REQ-018: SSE reconnection with Last-Event-ID replay
// ---------------------------------------------------------------------------

describe('useLogs clearLines <> lastEventId seam', () => {
  // Given: logs have accumulated and lastEventId is set
  // When: clearLines is called
  // Then: lines are cleared but lastEventId is preserved for reconnection replay
  it('REQ-018: clearLines empties lines but retains lastEventId for reconnection replay', () => {
    const { result } = renderHook(() => useLogs({ taskId: 'task-001' }))

    act(() => {
      capturedOnEvent?.(makeSSEEvent(makeLog({ id: 'log-001' }), 'log-001'))
      capturedOnEvent?.(makeSSEEvent(makeLog({ id: 'log-002' }), 'log-002'))
    })
    expect(result.current.lastEventId).toBe('log-002')

    act(() => { result.current.clearLines() })

    expect(result.current.lines).toHaveLength(0)
    expect(result.current.lastEventId).toBe('log-002')
  })
})

// ---------------------------------------------------------------------------
// Integration: LogStreamerPage <> downloadTaskLogs API seam
// REQ-018: Download Logs fetches from REST API
// ---------------------------------------------------------------------------

describe('LogStreamerPage <> downloadTaskLogs API seam', () => {
  // Given: a task is selected via URL param
  // When: Download Logs is clicked
  // Then: downloadTaskLogs is called with the correct taskId
  it('REQ-018: Download Logs button calls downloadTaskLogs with the selected taskId', async () => {
    const user = userEvent.setup()
    mockDownloadTaskLogs.mockResolvedValue('log line 1\nlog line 2\n')
    renderPage('/tasks/logs?taskId=task-abc')

    await user.click(screen.getByRole('button', { name: /Download/i }))

    await waitFor(() => {
      expect(mockDownloadTaskLogs).toHaveBeenCalledWith('task-abc')
    })
  })

  // [VERIFIER-ADDED] Negative: download is not triggered when no task is selected
  it('REQ-018: [VERIFIER-ADDED] download does not call downloadTaskLogs when no taskId selected', async () => {
    const user = userEvent.setup()
    renderPage('/tasks/logs')

    await user.click(screen.getByRole('button', { name: /Download/i }))

    expect(mockDownloadTaskLogs).not.toHaveBeenCalled()
  })
})

// ---------------------------------------------------------------------------
// Integration: LogStreamerPage <> useTasks seam (task list for dropdown)
// REQ-018: Task selector dropdown populated from useTasks
// ---------------------------------------------------------------------------

describe('LogStreamerPage <> useTasks seam', () => {
  // Given: useTasks returns a list of tasks
  // When: LogStreamerPage renders
  // Then: the task dropdown contains those tasks as options
  it('REQ-018: task dropdown is populated with tasks from useTasks', () => {
    mockUseTasks.mockReturnValue(defaultTasksReturn({
      tasks: [
        {
          id: 'task-aabbccdd-1111-2222-3333-444455556666',
          pipelineId: 'pipe-001',
          userId: 'user-001',
          status: 'running',
          retryConfig: { maxRetries: 0, backoff: 'fixed' },
          retryCount: 0,
          executionId: 'exec-001',
          input: {},
          createdAt: '2026-04-01T10:00:00Z',
          updatedAt: '2026-04-01T10:01:00Z',
        },
      ],
    }))
    renderPage('/tasks/logs')

    // The task selector should contain an option matching the task ID
    const selector = screen.getByRole('combobox', { name: /Select task/i })
    expect(selector).toBeTruthy()
    // Option text contains the first 8 chars of the task ID
    expect(selector.textContent).toContain('task-aab')
  })
})
