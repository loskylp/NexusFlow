/**
 * Unit tests for the useSinkInspector hook.
 *
 * Covers:
 *   - Idle state when taskId is null
 *   - Snapshot state reset on task change
 *   - Before panel populated on sink:before-snapshot event
 *   - After panel populated on sink:after-result event (success)
 *   - Rollback flag set and writeError populated on rollback after result
 *   - isWaitingForSinkPhase lifecycle
 *   - Access error surfaced on sink:error event
 *   - Before snapshot populated from after event when before was missed
 *
 * See: DEMO-003, ADR-007, TASK-032, TASK-033
 */
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { useSinkInspector } from './useSinkInspector'
import type { SSEEvent, SinkInspectorState, SinkSnapshot } from '@/types/domain'

// ---------------------------------------------------------------------------
// Fake EventSource — mirrors the pattern in useSSE.test.ts
// ---------------------------------------------------------------------------

interface FakeEventSourceInstance {
  url: string
  withCredentials: boolean
  onopen: ((e: Event) => void) | null
  onmessage: ((e: MessageEvent) => void) | null
  onerror: ((e: Event) => void) | null
  close: ReturnType<typeof vi.fn>
  simulateOpen(): void
  simulateMessage(data: string): void
  simulateError(): void
}

let instances: FakeEventSourceInstance[] = []

class FakeEventSource implements FakeEventSourceInstance {
  url: string
  withCredentials: boolean
  onopen: ((e: Event) => void) | null = null
  onmessage: ((e: MessageEvent) => void) | null = null
  onerror: ((e: Event) => void) | null = null
  close = vi.fn()
  addEventListener = vi.fn()
  removeEventListener = vi.fn()

  constructor(url: string, init?: EventSourceInit) {
    this.url = url
    this.withCredentials = init?.withCredentials ?? false
    instances.push(this)
  }

  simulateOpen() {
    this.onopen?.(new Event('open'))
  }

  simulateMessage(data: string) {
    this.onmessage?.(new MessageEvent('message', { data }))
  }

  simulateError() {
    this.onerror?.(new Event('error'))
  }
}

function latestInstance(): FakeEventSourceInstance {
  const inst = instances[instances.length - 1]
  if (!inst) throw new Error('No EventSource instance created')
  return inst
}

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

const TASK_ID = '11111111-1111-1111-1111-111111111111'
const TASK_ID_2 = '22222222-2222-2222-2222-222222222222'

function makeBeforeSnapshot(overrides: Partial<SinkSnapshot> = {}): SinkSnapshot {
  return {
    taskId: TASK_ID,
    phase: 'before',
    data: { rows: 3 },
    capturedAt: '2026-04-15T10:00:00Z',
    ...overrides,
  }
}

function makeAfterSnapshot(overrides: Partial<SinkSnapshot> = {}): SinkSnapshot {
  return {
    taskId: TASK_ID,
    phase: 'after',
    data: { rows: 5 },
    capturedAt: '2026-04-15T10:00:05Z',
    ...overrides,
  }
}

function makeBeforeEvent(before: SinkSnapshot): SSEEvent<SinkInspectorState> {
  return {
    type: 'sink:before-snapshot',
    payload: {
      eventType: 'sink:before-snapshot',
      taskId: TASK_ID,
      before,
      after: null,
      rolledBack: false,
      writeError: '',
    },
  }
}

function makeAfterEvent(
  before: SinkSnapshot,
  after: SinkSnapshot,
  rolledBack = false,
  writeError = ''
): SSEEvent<SinkInspectorState> {
  return {
    type: 'sink:after-result',
    payload: {
      eventType: 'sink:after-result',
      taskId: TASK_ID,
      before,
      after,
      rolledBack,
      writeError,
    },
  }
}

// ---------------------------------------------------------------------------
// Setup / teardown
// ---------------------------------------------------------------------------

beforeEach(() => {
  instances = []
  vi.stubGlobal('EventSource', FakeEventSource)
  vi.useFakeTimers()
})

afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

// ---------------------------------------------------------------------------
// Tests — idle state
// ---------------------------------------------------------------------------

describe('useSinkInspector — idle state (no task selected)', () => {
  it('returns sseStatus idle when taskId is null', () => {
    const { result } = renderHook(() => useSinkInspector({ taskId: null }))
    expect(result.current.sseStatus).toBe('idle')
  })

  it('returns null snapshots when taskId is null', () => {
    const { result } = renderHook(() => useSinkInspector({ taskId: null }))
    expect(result.current.beforeSnapshot).toBeNull()
    expect(result.current.afterSnapshot).toBeNull()
  })

  it('does not open an EventSource when taskId is null', () => {
    renderHook(() => useSinkInspector({ taskId: null }))
    expect(instances).toHaveLength(0)
  })

  it('isWaitingForSinkPhase is false when no task is selected', () => {
    const { result } = renderHook(() => useSinkInspector({ taskId: null }))
    expect(result.current.isWaitingForSinkPhase).toBe(false)
  })
})

// ---------------------------------------------------------------------------
// Tests — task selected
// ---------------------------------------------------------------------------

describe('useSinkInspector — task selected', () => {
  it('opens EventSource for the correct task URL', () => {
    renderHook(() => useSinkInspector({ taskId: TASK_ID }))
    expect(instances).toHaveLength(1)
    expect(latestInstance().url).toBe(`/events/sink/${TASK_ID}`)
  })

  it('sets isWaitingForSinkPhase to true immediately on task selection', () => {
    const { result } = renderHook(() => useSinkInspector({ taskId: TASK_ID }))
    expect(result.current.isWaitingForSinkPhase).toBe(true)
  })

  it('sseStatus is connecting after task selection', () => {
    const { result } = renderHook(() => useSinkInspector({ taskId: TASK_ID }))
    expect(result.current.sseStatus).toBe('connecting')
  })

  it('sseStatus is connected after EventSource opens', () => {
    const { result } = renderHook(() => useSinkInspector({ taskId: TASK_ID }))

    act(() => {
      latestInstance().simulateOpen()
    })

    expect(result.current.sseStatus).toBe('connected')
  })
})

// ---------------------------------------------------------------------------
// Tests — before snapshot
// ---------------------------------------------------------------------------

describe('useSinkInspector — sink:before-snapshot event', () => {
  it('populates beforeSnapshot when sink:before-snapshot event arrives', () => {
    const { result } = renderHook(() => useSinkInspector({ taskId: TASK_ID }))
    const before = makeBeforeSnapshot()

    act(() => {
      latestInstance().simulateOpen()
      latestInstance().simulateMessage(JSON.stringify(makeBeforeEvent(before)))
    })

    expect(result.current.beforeSnapshot).toEqual(before)
  })

  it('clears afterSnapshot when sink:before-snapshot event arrives', () => {
    const { result } = renderHook(() => useSinkInspector({ taskId: TASK_ID }))
    const before = makeBeforeSnapshot()
    const after = makeAfterSnapshot()

    act(() => {
      latestInstance().simulateOpen()
      // First simulate a complete cycle to populate afterSnapshot.
      latestInstance().simulateMessage(JSON.stringify(makeBeforeEvent(before)))
      latestInstance().simulateMessage(JSON.stringify(makeAfterEvent(before, after)))
    })

    expect(result.current.afterSnapshot).toEqual(after)

    // New before event clears after.
    act(() => {
      latestInstance().simulateMessage(JSON.stringify(makeBeforeEvent(before)))
    })

    expect(result.current.afterSnapshot).toBeNull()
  })

  it('sets isWaitingForSinkPhase to false after before snapshot arrives', () => {
    const { result } = renderHook(() => useSinkInspector({ taskId: TASK_ID }))
    const before = makeBeforeSnapshot()

    act(() => {
      latestInstance().simulateOpen()
      latestInstance().simulateMessage(JSON.stringify(makeBeforeEvent(before)))
    })

    expect(result.current.isWaitingForSinkPhase).toBe(false)
  })
})

// ---------------------------------------------------------------------------
// Tests — after result (success)
// ---------------------------------------------------------------------------

describe('useSinkInspector — sink:after-result event (success)', () => {
  it('populates afterSnapshot when sink:after-result event arrives', () => {
    const { result } = renderHook(() => useSinkInspector({ taskId: TASK_ID }))
    const before = makeBeforeSnapshot()
    const after = makeAfterSnapshot()

    act(() => {
      latestInstance().simulateOpen()
      latestInstance().simulateMessage(JSON.stringify(makeBeforeEvent(before)))
      latestInstance().simulateMessage(JSON.stringify(makeAfterEvent(before, after)))
    })

    expect(result.current.afterSnapshot).toEqual(after)
  })

  it('rolledBack is false on successful after result', () => {
    const { result } = renderHook(() => useSinkInspector({ taskId: TASK_ID }))
    const before = makeBeforeSnapshot()
    const after = makeAfterSnapshot()

    act(() => {
      latestInstance().simulateOpen()
      latestInstance().simulateMessage(JSON.stringify(makeBeforeEvent(before)))
      latestInstance().simulateMessage(JSON.stringify(makeAfterEvent(before, after, false)))
    })

    expect(result.current.rolledBack).toBe(false)
  })

  it('writeError is null on successful after result', () => {
    const { result } = renderHook(() => useSinkInspector({ taskId: TASK_ID }))
    const before = makeBeforeSnapshot()
    const after = makeAfterSnapshot()

    act(() => {
      latestInstance().simulateOpen()
      latestInstance().simulateMessage(JSON.stringify(makeBeforeEvent(before)))
      latestInstance().simulateMessage(JSON.stringify(makeAfterEvent(before, after, false, '')))
    })

    expect(result.current.writeError).toBeNull()
  })

  it('sets isWaitingForSinkPhase to false after after result arrives', () => {
    const { result } = renderHook(() => useSinkInspector({ taskId: TASK_ID }))
    const before = makeBeforeSnapshot()
    const after = makeAfterSnapshot()

    act(() => {
      latestInstance().simulateOpen()
      latestInstance().simulateMessage(JSON.stringify(makeBeforeEvent(before)))
      latestInstance().simulateMessage(JSON.stringify(makeAfterEvent(before, after)))
    })

    expect(result.current.isWaitingForSinkPhase).toBe(false)
  })
})

// ---------------------------------------------------------------------------
// Tests — after result (rollback)
// ---------------------------------------------------------------------------

describe('useSinkInspector — sink:after-result event (rollback)', () => {
  it('sets rolledBack to true when rollback is indicated', () => {
    const { result } = renderHook(() => useSinkInspector({ taskId: TASK_ID }))
    const before = makeBeforeSnapshot()
    // After snapshot matches before on rollback.
    const after = makeAfterSnapshot({ data: before.data })

    act(() => {
      latestInstance().simulateOpen()
      latestInstance().simulateMessage(JSON.stringify(makeBeforeEvent(before)))
      latestInstance().simulateMessage(JSON.stringify(makeAfterEvent(before, after, true, 'write failed')))
    })

    expect(result.current.rolledBack).toBe(true)
  })

  it('sets writeError when rollback carries an error message', () => {
    const { result } = renderHook(() => useSinkInspector({ taskId: TASK_ID }))
    const before = makeBeforeSnapshot()
    const after = makeAfterSnapshot({ data: before.data })

    act(() => {
      latestInstance().simulateOpen()
      latestInstance().simulateMessage(JSON.stringify(makeBeforeEvent(before)))
      latestInstance().simulateMessage(JSON.stringify(makeAfterEvent(before, after, true, 'write failed')))
    })

    expect(result.current.writeError).toBe('write failed')
  })
})

// ---------------------------------------------------------------------------
// Tests — task change resets state
// ---------------------------------------------------------------------------

describe('useSinkInspector — task change resets state', () => {
  it('clears all snapshot state when taskId changes', () => {
    let taskId: string | null = TASK_ID
    const { result, rerender } = renderHook(() => useSinkInspector({ taskId }))
    const before = makeBeforeSnapshot()
    const after = makeAfterSnapshot()

    act(() => {
      latestInstance().simulateOpen()
      latestInstance().simulateMessage(JSON.stringify(makeBeforeEvent(before)))
      latestInstance().simulateMessage(JSON.stringify(makeAfterEvent(before, after)))
    })

    expect(result.current.beforeSnapshot).not.toBeNull()
    expect(result.current.afterSnapshot).not.toBeNull()

    // Change to a different task.
    taskId = TASK_ID_2
    rerender()

    expect(result.current.beforeSnapshot).toBeNull()
    expect(result.current.afterSnapshot).toBeNull()
    expect(result.current.rolledBack).toBe(false)
    expect(result.current.writeError).toBeNull()
  })

  it('sets isWaitingForSinkPhase true when switching to a new task', () => {
    let taskId: string | null = null
    const { result, rerender } = renderHook(() => useSinkInspector({ taskId }))

    taskId = TASK_ID
    rerender()

    expect(result.current.isWaitingForSinkPhase).toBe(true)
  })

  it('opens a new EventSource for the new task URL on task change', () => {
    let taskId: string | null = TASK_ID
    const { rerender } = renderHook(() => useSinkInspector({ taskId }))

    expect(instances).toHaveLength(1)
    expect(instances[0]?.url).toBe(`/events/sink/${TASK_ID}`)

    taskId = TASK_ID_2
    rerender()

    expect(instances.length).toBeGreaterThanOrEqual(2)
    expect(instances[instances.length - 1]?.url).toBe(`/events/sink/${TASK_ID_2}`)
  })
})

// ---------------------------------------------------------------------------
// Tests — access error
// ---------------------------------------------------------------------------

describe('useSinkInspector — access error', () => {
  it('surfaces accessError on sink:error event', () => {
    const { result } = renderHook(() => useSinkInspector({ taskId: TASK_ID }))

    act(() => {
      latestInstance().simulateOpen()
      latestInstance().simulateMessage(JSON.stringify({
        type: 'sink:error',
        payload: {
          eventType: 'sink:error',
          taskId: TASK_ID,
          before: null,
          after: null,
          rolledBack: false,
          writeError: '',
        },
      }))
    })

    expect(result.current.accessError).not.toBeNull()
    expect(result.current.isWaitingForSinkPhase).toBe(false)
  })

  it('clears accessError when taskId changes', () => {
    let taskId: string | null = TASK_ID
    const { result, rerender } = renderHook(() => useSinkInspector({ taskId }))

    act(() => {
      latestInstance().simulateOpen()
      latestInstance().simulateMessage(JSON.stringify({
        type: 'sink:error',
        payload: {
          eventType: 'sink:error',
          taskId: TASK_ID,
          before: null,
          after: null,
          rolledBack: false,
          writeError: '',
        },
      }))
    })

    expect(result.current.accessError).not.toBeNull()

    taskId = TASK_ID_2
    rerender()

    expect(result.current.accessError).toBeNull()
  })
})

// ---------------------------------------------------------------------------
// Tests — before snapshot recovered from after event
// ---------------------------------------------------------------------------

describe('useSinkInspector — before snapshot recovered from after event', () => {
  it('populates beforeSnapshot from after event when before event was missed', () => {
    const { result } = renderHook(() => useSinkInspector({ taskId: TASK_ID }))
    const before = makeBeforeSnapshot()
    const after = makeAfterSnapshot()

    act(() => {
      latestInstance().simulateOpen()
      // Send only the after event (before event was missed).
      latestInstance().simulateMessage(JSON.stringify(makeAfterEvent(before, after)))
    })

    expect(result.current.beforeSnapshot).toEqual(before)
    expect(result.current.afterSnapshot).toEqual(after)
  })
})

// ---------------------------------------------------------------------------
// Tests — unmount cleanup
// ---------------------------------------------------------------------------

describe('useSinkInspector — cleanup on unmount', () => {
  it('closes the EventSource when the hook unmounts', () => {
    const { unmount } = renderHook(() => useSinkInspector({ taskId: TASK_ID }))
    const inst = latestInstance()

    act(() => {
      inst.simulateOpen()
    })

    unmount()

    expect(inst.close).toHaveBeenCalledOnce()
  })
})
