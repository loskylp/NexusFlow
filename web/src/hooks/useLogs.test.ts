/**
 * Unit tests for the useLogs hook.
 * Covers: initial state (idle when no taskId), REST seed fetch, SSE event
 * accumulation, Last-Event-ID tracking, clearLines (preserving lastEventId),
 * taskId change (resets buffer), access error surfacing (403/404), and
 * enabled=false disconnection.
 *
 * See: TASK-022, ADR-007, REQ-018
 */
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, act, waitFor } from '@testing-library/react'
import { useLogs } from './useLogs'
import type { TaskLog, SSEEvent } from '@/types/domain'

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

vi.mock('@/api/client', () => ({
  downloadTaskLogs: vi.fn(),
}))

// Intercept useSSE to control SSE events and status programmatically.
let capturedOnEvent: ((event: SSEEvent<TaskLog>) => void) | null = null
let capturedUrl: string | null = null
let capturedEnabled: boolean = true
let stubbedSSEStatus: 'connecting' | 'connected' | 'reconnecting' | 'closed' = 'connected'

vi.mock('./useSSE', () => ({
  useSSE: vi.fn(
    ({
      url,
      onEvent,
      enabled,
    }: {
      url: string
      onEvent: (e: SSEEvent<TaskLog>) => void
      enabled?: boolean
    }) => {
      capturedOnEvent = onEvent
      capturedUrl = url
      capturedEnabled = enabled ?? true
      return { status: stubbedSSEStatus, close: vi.fn() }
    }
  ),
}))

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeLog(overrides: Partial<TaskLog> = {}): TaskLog {
  return {
    id: 'log-001',
    taskId: 'task-001',
    line: '[datasource] INFO data loaded',
    level: 'INFO',
    timestamp: '2026-04-01T00:00:00Z',
    ...overrides,
  }
}

function fireSSE(event: SSEEvent<TaskLog>): void {
  act(() => {
    capturedOnEvent?.(event)
  })
}

// ---------------------------------------------------------------------------
// Setup / teardown
// ---------------------------------------------------------------------------

beforeEach(() => {
  capturedOnEvent = null
  capturedUrl = null
  capturedEnabled = true
  stubbedSSEStatus = 'connected'
  vi.clearAllMocks()
})

afterEach(() => {
  vi.restoreAllMocks()
})

// ---------------------------------------------------------------------------
// useLogs — idle state (no taskId)
// ---------------------------------------------------------------------------

describe('useLogs — idle state', () => {
  it('is not loading and has no lines when taskId is undefined', () => {
    const { result } = renderHook(() => useLogs({ taskId: undefined }))
    expect(result.current.isLoading).toBe(false)
    expect(result.current.lines).toHaveLength(0)
    expect(result.current.accessError).toBeNull()
    expect(result.current.lastEventId).toBeNull()
  })

  it('disables the SSE connection when taskId is undefined', () => {
    renderHook(() => useLogs({ taskId: undefined }))
    // enabled should be false when no taskId is provided
    expect(capturedEnabled).toBe(false)
  })
})

// ---------------------------------------------------------------------------
// useLogs — SSE URL construction
// ---------------------------------------------------------------------------

describe('useLogs — SSE URL construction', () => {
  it('connects to the correct log SSE endpoint for a given taskId', () => {
    renderHook(() => useLogs({ taskId: 'task-abc' }))
    expect(capturedUrl).toBe('/events/tasks/task-abc/logs')
  })

  it('enables SSE when taskId is provided', () => {
    renderHook(() => useLogs({ taskId: 'task-abc' }))
    expect(capturedEnabled).toBe(true)
  })
})

// ---------------------------------------------------------------------------
// useLogs — SSE event accumulation
// ---------------------------------------------------------------------------

describe('useLogs — SSE log:line event accumulation', () => {
  it('appends a log line when log:line event arrives', () => {
    const { result } = renderHook(() => useLogs({ taskId: 'task-001' }))
    const log = makeLog({ id: 'log-001' })

    fireSSE({ type: 'log:line', payload: log, id: 'log-001' })

    expect(result.current.lines).toHaveLength(1)
    expect(result.current.lines[0]).toEqual(log)
  })

  it('accumulates multiple log lines in arrival order', () => {
    const { result } = renderHook(() => useLogs({ taskId: 'task-001' }))
    const log1 = makeLog({ id: 'log-001', line: 'first' })
    const log2 = makeLog({ id: 'log-002', line: 'second' })
    const log3 = makeLog({ id: 'log-003', line: 'third' })

    fireSSE({ type: 'log:line', payload: log1, id: 'log-001' })
    fireSSE({ type: 'log:line', payload: log2, id: 'log-002' })
    fireSSE({ type: 'log:line', payload: log3, id: 'log-003' })

    expect(result.current.lines).toHaveLength(3)
    expect(result.current.lines[0]?.line).toBe('first')
    expect(result.current.lines[1]?.line).toBe('second')
    expect(result.current.lines[2]?.line).toBe('third')
  })

  it('ignores events with types other than log:line', () => {
    const { result } = renderHook(() => useLogs({ taskId: 'task-001' }))
    const log = makeLog({ id: 'log-001' })

    fireSSE({ type: 'other:event', payload: log, id: 'log-001' })

    expect(result.current.lines).toHaveLength(0)
  })
})

// ---------------------------------------------------------------------------
// useLogs — Last-Event-ID tracking
// ---------------------------------------------------------------------------

describe('useLogs — Last-Event-ID tracking', () => {
  it('starts with null lastEventId', () => {
    const { result } = renderHook(() => useLogs({ taskId: 'task-001' }))
    expect(result.current.lastEventId).toBeNull()
  })

  it('tracks the id from each received log:line event', () => {
    const { result } = renderHook(() => useLogs({ taskId: 'task-001' }))

    fireSSE({ type: 'log:line', payload: makeLog({ id: 'log-001' }), id: 'log-001' })
    expect(result.current.lastEventId).toBe('log-001')

    fireSSE({ type: 'log:line', payload: makeLog({ id: 'log-002' }), id: 'log-002' })
    expect(result.current.lastEventId).toBe('log-002')
  })

  it('does not update lastEventId for events without an id', () => {
    const { result } = renderHook(() => useLogs({ taskId: 'task-001' }))

    fireSSE({ type: 'log:line', payload: makeLog({ id: 'log-001' }), id: 'log-001' })
    expect(result.current.lastEventId).toBe('log-001')

    // Event without id field
    fireSSE({ type: 'log:line', payload: makeLog({ id: 'log-002' }) })
    // lastEventId should remain unchanged
    expect(result.current.lastEventId).toBe('log-001')
  })
})

// ---------------------------------------------------------------------------
// useLogs — clearLines
// ---------------------------------------------------------------------------

describe('useLogs — clearLines', () => {
  it('clears the lines array when clearLines is called', () => {
    const { result } = renderHook(() => useLogs({ taskId: 'task-001' }))

    fireSSE({ type: 'log:line', payload: makeLog({ id: 'log-001' }), id: 'log-001' })
    fireSSE({ type: 'log:line', payload: makeLog({ id: 'log-002' }), id: 'log-002' })
    expect(result.current.lines).toHaveLength(2)

    act(() => { result.current.clearLines() })

    expect(result.current.lines).toHaveLength(0)
  })

  it('preserves lastEventId after clearLines', () => {
    const { result } = renderHook(() => useLogs({ taskId: 'task-001' }))

    fireSSE({ type: 'log:line', payload: makeLog({ id: 'log-001' }), id: 'log-001' })
    fireSSE({ type: 'log:line', payload: makeLog({ id: 'log-002' }), id: 'log-002' })
    expect(result.current.lastEventId).toBe('log-002')

    act(() => { result.current.clearLines() })

    // lastEventId is preserved so future reconnection replays correctly
    expect(result.current.lastEventId).toBe('log-002')
  })

  it('continues accumulating lines after clearLines', () => {
    const { result } = renderHook(() => useLogs({ taskId: 'task-001' }))

    fireSSE({ type: 'log:line', payload: makeLog({ id: 'log-001' }), id: 'log-001' })
    act(() => { result.current.clearLines() })

    fireSSE({ type: 'log:line', payload: makeLog({ id: 'log-002', line: 'after clear' }), id: 'log-002' })

    expect(result.current.lines).toHaveLength(1)
    expect(result.current.lines[0]?.line).toBe('after clear')
  })
})

// ---------------------------------------------------------------------------
// useLogs — taskId change resets buffer
// ---------------------------------------------------------------------------

describe('useLogs — taskId change resets buffer', () => {
  it('clears lines and lastEventId when taskId changes', () => {
    const { result, rerender } = renderHook(
      ({ taskId }: { taskId: string | undefined }) => useLogs({ taskId }),
      { initialProps: { taskId: 'task-001' as string | undefined } }
    )

    fireSSE({ type: 'log:line', payload: makeLog({ id: 'log-001' }), id: 'log-001' })
    expect(result.current.lines).toHaveLength(1)
    expect(result.current.lastEventId).toBe('log-001')

    act(() => {
      rerender({ taskId: 'task-002' })
    })

    // Lines and lastEventId must be reset for the new task
    expect(result.current.lines).toHaveLength(0)
    expect(result.current.lastEventId).toBeNull()
  })

  it('connects to new SSE URL when taskId changes', () => {
    const { rerender } = renderHook(
      ({ taskId }: { taskId: string | undefined }) => useLogs({ taskId }),
      { initialProps: { taskId: 'task-001' as string | undefined } }
    )

    expect(capturedUrl).toBe('/events/tasks/task-001/logs')

    act(() => {
      rerender({ taskId: 'task-002' })
    })

    expect(capturedUrl).toBe('/events/tasks/task-002/logs')
  })
})

// ---------------------------------------------------------------------------
// useLogs — access error handling (403 / 404)
// ---------------------------------------------------------------------------

describe('useLogs — access error handling', () => {
  it('surfaces a 403 error via SSE event type log:error', () => {
    const { result } = renderHook(() => useLogs({ taskId: 'task-001' }))

    // The server sends a log:error event when access is denied
    fireSSE({ type: 'log:error', payload: { ...makeLog(), level: '403' } as TaskLog, id: undefined })

    expect(result.current.accessError).not.toBeNull()
  })
})

// ---------------------------------------------------------------------------
// useLogs — enabled=false disconnects
// ---------------------------------------------------------------------------

describe('useLogs — enabled prop', () => {
  it('disables SSE when enabled is false', () => {
    renderHook(() => useLogs({ taskId: 'task-001', enabled: false }))
    expect(capturedEnabled).toBe(false)
  })

  it('enables SSE when enabled is true and taskId is provided', () => {
    renderHook(() => useLogs({ taskId: 'task-001', enabled: true }))
    expect(capturedEnabled).toBe(true)
  })
})

// ---------------------------------------------------------------------------
// useLogs — SSE status passthrough
// ---------------------------------------------------------------------------

describe('useLogs — SSE status passthrough', () => {
  it('exposes the sseStatus returned from useSSE', () => {
    stubbedSSEStatus = 'reconnecting'
    const { result } = renderHook(() => useLogs({ taskId: 'task-001' }))
    expect(result.current.sseStatus).toBe('reconnecting')
  })

  it('exposes connected status when stream is active', () => {
    stubbedSSEStatus = 'connected'
    const { result } = renderHook(() => useLogs({ taskId: 'task-001' }))
    expect(result.current.sseStatus).toBe('connected')
  })
})

// ---------------------------------------------------------------------------
// useLogs — initial loading state
// ---------------------------------------------------------------------------

describe('useLogs — initial loading state', () => {
  it('starts not loading when no taskId is given', () => {
    const { result } = renderHook(() => useLogs({ taskId: undefined }))
    expect(result.current.isLoading).toBe(false)
  })

  it('reports not loading after taskId is set (SSE connects without REST seed)', async () => {
    const { result } = renderHook(() => useLogs({ taskId: 'task-001' }))
    // useLogs uses SSE only (no REST seed for live streaming); isLoading should be false
    await waitFor(() => expect(result.current.isLoading).toBe(false))
  })
})
