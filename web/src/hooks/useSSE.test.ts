/**
 * Unit tests for the useSSE hook.
 * Covers: connection lifecycle, status transitions, event parsing,
 * reconnection state on error, and cleanup on unmount.
 *
 * See: TASK-020, ADR-007
 */
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { useSSE } from './useSSE'
import type { SSEEvent } from '@/types/domain'

// ---------------------------------------------------------------------------
// Fake EventSource
// ---------------------------------------------------------------------------

interface FakeEventSourceInstance {
  url: string
  withCredentials: boolean
  onopen: ((e: Event) => void) | null
  onmessage: ((e: MessageEvent) => void) | null
  onerror: ((e: Event) => void) | null
  close: ReturnType<typeof vi.fn>
  addEventListener: ReturnType<typeof vi.fn>
  removeEventListener: ReturnType<typeof vi.fn>
  /** Simulate server opening the stream. */
  simulateOpen(): void
  /** Simulate a raw message event with the given JSON string as data. */
  simulateMessage(data: string): void
  /** Simulate an error (triggers onerror). */
  simulateError(): void
}

// Track all EventSource instances created during a test.
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

// ---------------------------------------------------------------------------
// Setup / teardown
// ---------------------------------------------------------------------------

beforeEach(() => {
  instances = []
  vi.stubGlobal('EventSource', FakeEventSource)
  // Prevent real timers from interfering — replace setTimeout with fake timers.
  vi.useFakeTimers()
})

afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function latestInstance(): FakeEventSourceInstance {
  const inst = instances[instances.length - 1]
  if (!inst) throw new Error('No EventSource instance created')
  return inst
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('useSSE — initial status', () => {
  it('starts in "connecting" state when enabled', () => {
    const onEvent = vi.fn()
    const { result } = renderHook(() =>
      useSSE({ url: '/events/workers', onEvent })
    )
    expect(result.current.status).toBe('connecting')
  })

  it('stays "closed" when enabled is false', () => {
    const onEvent = vi.fn()
    const { result } = renderHook(() =>
      useSSE({ url: '/events/workers', onEvent, enabled: false })
    )
    expect(result.current.status).toBe('closed')
    expect(instances).toHaveLength(0)
  })
})

describe('useSSE — connected state', () => {
  it('transitions to "connected" when EventSource fires onopen', () => {
    const onEvent = vi.fn()
    const { result } = renderHook(() =>
      useSSE({ url: '/events/workers', onEvent })
    )

    act(() => {
      latestInstance().simulateOpen()
    })

    expect(result.current.status).toBe('connected')
  })
})

describe('useSSE — event parsing', () => {
  it('calls onEvent with the parsed SSEEvent when a valid JSON message arrives', () => {
    const onEvent = vi.fn()
    renderHook(() => useSSE({ url: '/events/workers', onEvent }))

    const payload = { type: 'worker:heartbeat', payload: { id: 'w1' } }

    act(() => {
      latestInstance().simulateOpen()
      latestInstance().simulateMessage(JSON.stringify(payload))
    })

    expect(onEvent).toHaveBeenCalledOnce()
    expect(onEvent).toHaveBeenCalledWith(payload)
  })

  it('does not call onEvent when message data is not valid JSON', () => {
    const onEvent = vi.fn()
    renderHook(() => useSSE({ url: '/events/workers', onEvent }))

    act(() => {
      latestInstance().simulateOpen()
      latestInstance().simulateMessage('not-json')
    })

    expect(onEvent).not.toHaveBeenCalled()
  })
})

describe('useSSE — reconnecting state', () => {
  it('transitions to "reconnecting" when EventSource fires onerror', () => {
    const onEvent = vi.fn()
    const { result } = renderHook(() =>
      useSSE({ url: '/events/workers', onEvent })
    )

    act(() => {
      latestInstance().simulateOpen()
      latestInstance().simulateError()
    })

    expect(result.current.status).toBe('reconnecting')
  })

  it('creates a new EventSource after the backoff delay on error', () => {
    const onEvent = vi.fn()
    renderHook(() => useSSE({ url: '/events/workers', onEvent }))

    act(() => {
      latestInstance().simulateOpen()
      latestInstance().simulateError()
    })

    expect(instances).toHaveLength(1)

    // Advance past the initial 1 s backoff.
    act(() => {
      vi.advanceTimersByTime(1500)
    })

    expect(instances).toHaveLength(2)
  })
})

describe('useSSE — cleanup on unmount', () => {
  it('closes the EventSource when the hook unmounts', () => {
    const onEvent = vi.fn()
    const { unmount } = renderHook(() =>
      useSSE({ url: '/events/workers', onEvent })
    )

    const inst = latestInstance()
    act(() => {
      inst.simulateOpen()
    })

    unmount()

    expect(inst.close).toHaveBeenCalledOnce()
  })
})

describe('useSSE — close() imperative handle', () => {
  it('closes the EventSource and transitions to "closed"', () => {
    const onEvent = vi.fn()
    const { result } = renderHook(() =>
      useSSE({ url: '/events/workers', onEvent })
    )

    const inst = latestInstance()
    act(() => {
      inst.simulateOpen()
    })

    act(() => {
      result.current.close()
    })

    expect(inst.close).toHaveBeenCalled()
    expect(result.current.status).toBe('closed')
  })
})

describe('useSSE — URL change causes reconnection', () => {
  it('closes old EventSource and opens a new one when url prop changes', () => {
    const onEvent = vi.fn()
    let url = '/events/workers'
    const { rerender } = renderHook(() =>
      useSSE({ url, onEvent })
    )

    const firstInst = latestInstance()
    act(() => { firstInst.simulateOpen() })

    url = '/events/tasks'
    rerender()

    expect(firstInst.close).toHaveBeenCalled()
    expect(instances).toHaveLength(2)
    expect(instances[1]?.url).toBe('/events/tasks')
  })
})

describe('useSSE — type-level: generic payload', () => {
  it('onEvent receives typed payload matching SSEEvent<Worker>', () => {
    interface Worker { id: string; status: 'online' | 'down' }
    const received: SSEEvent<Worker>[] = []
    const onEvent = (e: SSEEvent<Worker>) => { received.push(e) }

    renderHook(() => useSSE<Worker>({ url: '/events/workers', onEvent }))

    const event: SSEEvent<Worker> = {
      type: 'worker:down',
      payload: { id: 'w1', status: 'down' },
    }

    act(() => {
      latestInstance().simulateOpen()
      latestInstance().simulateMessage(JSON.stringify(event))
    })

    expect(received).toHaveLength(1)
    expect(received[0]).toEqual(event)
  })
})
