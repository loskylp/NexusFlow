/**
 * Unit tests for the useWorkers hook.
 * Covers: initial REST fetch, SSE event merging (registered, heartbeat, down),
 * summary count computation, and SSE status exposure.
 *
 * See: TASK-020
 */
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, act, waitFor } from '@testing-library/react'
import { useWorkers } from './useWorkers'
import type { Worker, SSEEvent } from '@/types/domain'

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

vi.mock('@/api/client', () => ({
  listWorkers: vi.fn(),
}))

import * as client from '@/api/client'
const mockListWorkers = vi.mocked(client.listWorkers)

// We intercept useSSE to control SSE events programmatically.
let capturedOnEvent: ((event: SSEEvent<Worker>) => void) | null = null
let stubbedSSEStatus: 'connecting' | 'connected' | 'reconnecting' | 'closed' = 'connected'

vi.mock('./useSSE', () => ({
  useSSE: vi.fn(({ onEvent }: { onEvent: (e: SSEEvent<Worker>) => void }) => {
    capturedOnEvent = onEvent
    return { status: stubbedSSEStatus, close: vi.fn() }
  }),
}))

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeWorker(overrides: Partial<Worker> = {}): Worker {
  return {
    id: 'w1',
    tags: ['tag-a'],
    status: 'online',
    lastHeartbeat: '2026-03-27T00:00:00Z',
    registeredAt: '2026-03-27T00:00:00Z',
    currentTaskId: undefined,
    ...overrides,
  }
}

function fireSSE(event: SSEEvent<Worker>) {
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
// Tests
// ---------------------------------------------------------------------------

describe('useWorkers — initial fetch', () => {
  it('starts in loading state before the REST call resolves', () => {
    mockListWorkers.mockResolvedValue([])
    const { result } = renderHook(() => useWorkers())
    expect(result.current.isLoading).toBe(true)
  })

  it('populates workers from REST response after fetch resolves', async () => {
    const workers = [makeWorker({ id: 'w1' }), makeWorker({ id: 'w2', status: 'down' })]
    mockListWorkers.mockResolvedValue(workers)

    const { result } = renderHook(() => useWorkers())

    await waitFor(() => expect(result.current.isLoading).toBe(false))
    expect(result.current.workers).toEqual(workers)
  })

  it('leaves workers empty and clears loading on fetch error', async () => {
    mockListWorkers.mockRejectedValue(new Error('network error'))

    const { result } = renderHook(() => useWorkers())

    await waitFor(() => expect(result.current.isLoading).toBe(false))
    expect(result.current.workers).toHaveLength(0)
  })
})

describe('useWorkers — summary counts', () => {
  it('computes total, online, and down counts from the worker list', async () => {
    const workers = [
      makeWorker({ id: 'w1', status: 'online' }),
      makeWorker({ id: 'w2', status: 'online' }),
      makeWorker({ id: 'w3', status: 'down' }),
    ]
    mockListWorkers.mockResolvedValue(workers)

    const { result } = renderHook(() => useWorkers())
    await waitFor(() => expect(result.current.isLoading).toBe(false))

    expect(result.current.summary.total).toBe(3)
    expect(result.current.summary.online).toBe(2)
    expect(result.current.summary.down).toBe(1)
  })

  it('returns zero counts when no workers are registered', async () => {
    mockListWorkers.mockResolvedValue([])

    const { result } = renderHook(() => useWorkers())
    await waitFor(() => expect(result.current.isLoading).toBe(false))

    expect(result.current.summary).toEqual({ total: 0, online: 0, down: 0 })
  })
})

describe('useWorkers — SSE: worker:registered', () => {
  it('adds a new worker when worker:registered event arrives', async () => {
    mockListWorkers.mockResolvedValue([])
    const { result } = renderHook(() => useWorkers())
    await waitFor(() => expect(result.current.isLoading).toBe(false))

    const newWorker = makeWorker({ id: 'w-new' })
    fireSSE({ type: 'worker:registered', payload: newWorker })

    expect(result.current.workers).toHaveLength(1)
    expect(result.current.workers[0]?.id).toBe('w-new')
  })

  it('does not add a duplicate when the worker is already in the list', async () => {
    const existing = makeWorker({ id: 'w1' })
    mockListWorkers.mockResolvedValue([existing])
    const { result } = renderHook(() => useWorkers())
    await waitFor(() => expect(result.current.isLoading).toBe(false))

    fireSSE({ type: 'worker:registered', payload: existing })

    expect(result.current.workers).toHaveLength(1)
  })
})

describe('useWorkers — SSE: worker:heartbeat', () => {
  it('updates lastHeartbeat for the matching worker', async () => {
    const worker = makeWorker({ id: 'w1', lastHeartbeat: '2026-01-01T00:00:00Z' })
    mockListWorkers.mockResolvedValue([worker])
    const { result } = renderHook(() => useWorkers())
    await waitFor(() => expect(result.current.isLoading).toBe(false))

    const updated = { ...worker, lastHeartbeat: '2026-03-27T12:00:00Z' }
    fireSSE({ type: 'worker:heartbeat', payload: updated })

    expect(result.current.workers[0]?.lastHeartbeat).toBe('2026-03-27T12:00:00Z')
  })

  it('ignores heartbeat for an unknown worker ID', async () => {
    mockListWorkers.mockResolvedValue([makeWorker({ id: 'w1' })])
    const { result } = renderHook(() => useWorkers())
    await waitFor(() => expect(result.current.isLoading).toBe(false))

    fireSSE({ type: 'worker:heartbeat', payload: makeWorker({ id: 'w-unknown' }) })

    expect(result.current.workers).toHaveLength(1)
    expect(result.current.workers[0]?.id).toBe('w1')
  })
})

describe('useWorkers — SSE: worker:down', () => {
  it('sets status to "down" for the matching worker', async () => {
    const worker = makeWorker({ id: 'w1', status: 'online' })
    mockListWorkers.mockResolvedValue([worker])
    const { result } = renderHook(() => useWorkers())
    await waitFor(() => expect(result.current.isLoading).toBe(false))

    fireSSE({ type: 'worker:down', payload: { ...worker, status: 'down' } })

    expect(result.current.workers[0]?.status).toBe('down')
  })

  it('updates summary counts after worker:down', async () => {
    const workers = [
      makeWorker({ id: 'w1', status: 'online' }),
      makeWorker({ id: 'w2', status: 'online' }),
    ]
    mockListWorkers.mockResolvedValue(workers)
    const { result } = renderHook(() => useWorkers())
    await waitFor(() => expect(result.current.isLoading).toBe(false))

    fireSSE({ type: 'worker:down', payload: { ...workers[0]!, status: 'down' } })

    expect(result.current.summary.online).toBe(1)
    expect(result.current.summary.down).toBe(1)
  })
})

describe('useWorkers — SSE status passthrough', () => {
  it('exposes the sseStatus returned from useSSE', async () => {
    stubbedSSEStatus = 'reconnecting'
    mockListWorkers.mockResolvedValue([])

    const { result } = renderHook(() => useWorkers())
    await waitFor(() => expect(result.current.isLoading).toBe(false))

    expect(result.current.sseStatus).toBe('reconnecting')
  })
})
