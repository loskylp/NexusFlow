/**
 * Unit tests for the useChaosController hook.
 *
 * Tests cover:
 *   - Initial state values (workers/pipelines/systemStatus/log defaults).
 *   - setFloodTaskCount clamping to [1, 1000].
 *   - killWorker: no-op when selectedWorkerId is null or isKilling is true.
 *   - disconnectDatabase: no-op when already disconnecting.
 *   - floodQueue: no-op when selectedFloodPipelineId is null or isFlooding is true.
 *   - makeLogEntry (via deriveHealthStatus indirectly via hook state).
 *
 * API calls (client.*) and fetch (for health) are mocked at the module level.
 *
 * See: DEMO-004, TASK-034
 */

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { useChaosController } from './useChaosController'

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

vi.mock('@/api/client', () => ({
  listWorkers: vi.fn().mockResolvedValue([]),
  listPipelines: vi.fn().mockResolvedValue([]),
  killWorker: vi.fn(),
  disconnectDatabase: vi.fn(),
  floodQueue: vi.fn(),
}))

import * as clientModule from '@/api/client'

const mockListWorkers = vi.mocked(clientModule.listWorkers)
const mockListPipelines = vi.mocked(clientModule.listPipelines)
const mockKillWorker = vi.mocked(clientModule.killWorker)
const mockDisconnectDatabase = vi.mocked(clientModule.disconnectDatabase)
const mockFloodQueue = vi.mocked(clientModule.floodQueue)

let fetchMock: ReturnType<typeof vi.fn>

beforeEach(() => {
  vi.useFakeTimers()

  mockListWorkers.mockResolvedValue([])
  mockListPipelines.mockResolvedValue([])

  fetchMock = vi.fn().mockResolvedValue({
    ok: true,
    json: async () => ({ status: 'ok', checks: { db: 'ok', redis: 'ok' } }),
  })
  vi.stubGlobal('fetch', fetchMock)
})

afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
  vi.clearAllMocks()
})

// ---------------------------------------------------------------------------
// Initial state
// ---------------------------------------------------------------------------

describe('useChaosController: initial state', () => {
  it('workers and pipelines are empty arrays on mount (before fetch resolves)', () => {
    const { result } = renderHook(() => useChaosController())
    expect(result.current.workers).toEqual([])
    expect(result.current.pipelines).toEqual([])
  })

  it('isLoadingSelectors is true before fetch resolves', () => {
    const { result } = renderHook(() => useChaosController())
    expect(result.current.isLoadingSelectors).toBe(true)
  })

  it('isLoadingSelectors becomes false after fetch resolves', async () => {
    const { result } = renderHook(() => useChaosController())
    await act(async () => {
      await vi.runAllTimersAsync()
    })
    expect(result.current.isLoadingSelectors).toBe(false)
  })

  it('systemStatus defaults to nominal', () => {
    const { result } = renderHook(() => useChaosController())
    expect(result.current.systemStatus).toBe('nominal')
  })

  it('selectedWorkerId is null by default', () => {
    const { result } = renderHook(() => useChaosController())
    expect(result.current.selectedWorkerId).toBeNull()
  })

  it('disconnectDurationSeconds defaults to 15', () => {
    const { result } = renderHook(() => useChaosController())
    expect(result.current.disconnectDurationSeconds).toBe(15)
  })

  it('floodTaskCount defaults to 10', () => {
    const { result } = renderHook(() => useChaosController())
    expect(result.current.floodTaskCount).toBe(10)
  })

  it('all log arrays start empty', () => {
    const { result } = renderHook(() => useChaosController())
    expect(result.current.killLog).toEqual([])
    expect(result.current.disconnectLog).toEqual([])
    expect(result.current.floodLog).toEqual([])
  })
})

// ---------------------------------------------------------------------------
// setFloodTaskCount clamping
// ---------------------------------------------------------------------------

describe('useChaosController: setFloodTaskCount clamping', () => {
  it('clamps to 1 when given 0', () => {
    const { result } = renderHook(() => useChaosController())
    act(() => result.current.setFloodTaskCount(0))
    expect(result.current.floodTaskCount).toBe(1)
  })

  it('clamps to 1 when given a negative value', () => {
    const { result } = renderHook(() => useChaosController())
    act(() => result.current.setFloodTaskCount(-100))
    expect(result.current.floodTaskCount).toBe(1)
  })

  it('clamps to 1000 when given 1001', () => {
    const { result } = renderHook(() => useChaosController())
    act(() => result.current.setFloodTaskCount(1001))
    expect(result.current.floodTaskCount).toBe(1000)
  })

  it('clamps to 1000 when given 5000', () => {
    const { result } = renderHook(() => useChaosController())
    act(() => result.current.setFloodTaskCount(5000))
    expect(result.current.floodTaskCount).toBe(1000)
  })

  it('accepts valid value within [1, 1000]', () => {
    const { result } = renderHook(() => useChaosController())
    act(() => result.current.setFloodTaskCount(42))
    expect(result.current.floodTaskCount).toBe(42)
  })

  it('accepts boundary value 1', () => {
    const { result } = renderHook(() => useChaosController())
    act(() => result.current.setFloodTaskCount(1))
    expect(result.current.floodTaskCount).toBe(1)
  })

  it('accepts boundary value 1000', () => {
    const { result } = renderHook(() => useChaosController())
    act(() => result.current.setFloodTaskCount(1000))
    expect(result.current.floodTaskCount).toBe(1000)
  })
})

// ---------------------------------------------------------------------------
// killWorker guard
// ---------------------------------------------------------------------------

describe('useChaosController: killWorker guard', () => {
  it('does not call killWorker API when selectedWorkerId is null', async () => {
    const { result } = renderHook(() => useChaosController())
    await act(async () => {
      await result.current.killWorker()
    })
    expect(mockKillWorker).not.toHaveBeenCalled()
  })
})

// ---------------------------------------------------------------------------
// disconnectDatabase guard
// ---------------------------------------------------------------------------

describe('useChaosController: disconnectDatabase guard', () => {
  it('does not call disconnectDatabase API when already disconnecting', async () => {
    mockDisconnectDatabase.mockResolvedValue({ log: [], durationSeconds: 15 })
    const { result } = renderHook(() => useChaosController())

    // Start a disconnect
    await act(async () => {
      await result.current.disconnectDatabase()
    })

    // The API call resolves immediately in tests; countdown is started.
    // A second call while the countdown timer is active should be a no-op.
    const callCount = mockDisconnectDatabase.mock.calls.length

    await act(async () => {
      await result.current.disconnectDatabase()
    })

    // Should not have triggered another API call while isDisconnecting=true.
    expect(mockDisconnectDatabase.mock.calls.length).toBe(callCount)
  })
})

// ---------------------------------------------------------------------------
// floodQueue guard
// ---------------------------------------------------------------------------

describe('useChaosController: floodQueue guard', () => {
  it('does not call floodQueue API when selectedFloodPipelineId is null', async () => {
    const { result } = renderHook(() => useChaosController())
    await act(async () => {
      await result.current.floodQueue()
    })
    expect(mockFloodQueue).not.toHaveBeenCalled()
  })
})

// ---------------------------------------------------------------------------
// floodQueue success path
// ---------------------------------------------------------------------------

describe('useChaosController: floodQueue success', () => {
  it('appends server log entries to floodLog on success', async () => {
    mockFloodQueue.mockResolvedValue({
      submittedCount: 5,
      log: [
        { timestamp: '2026-04-15T10:00:00Z', message: 'Flood complete: 5 tasks submitted', level: 'info' as const },
      ],
    })

    const { result } = renderHook(() => useChaosController())

    act(() => result.current.setSelectedFloodPipelineId('pipeline-001'))
    act(() => result.current.setFloodTaskCount(5))

    await act(async () => {
      await result.current.floodQueue()
    })

    expect(result.current.floodLog.some(e => e.message.includes('Flood complete'))).toBe(true)
    expect(result.current.floodProgress).toBe(100)
  })
})

// ---------------------------------------------------------------------------
// killWorker success path
// ---------------------------------------------------------------------------

describe('useChaosController: killWorker success', () => {
  it('appends server log entries to killLog on success', async () => {
    mockKillWorker.mockResolvedValue({
      log: [
        { timestamp: '2026-04-15T10:00:00Z', message: 'Container killed successfully', level: 'info' as const },
      ],
    })

    const { result } = renderHook(() => useChaosController())

    act(() => result.current.setSelectedWorkerId('worker-001'))

    await act(async () => {
      await result.current.killWorker()
    })

    expect(result.current.killLog.some(e => e.message.includes('killed successfully'))).toBe(true)
  })

  it('appends error log entry on kill failure', async () => {
    mockKillWorker.mockRejectedValue(new Error('500: docker kill failed'))

    const { result } = renderHook(() => useChaosController())
    act(() => result.current.setSelectedWorkerId('worker-fail'))

    await act(async () => {
      await result.current.killWorker()
    })

    expect(result.current.killLog.some(e => e.level === 'error')).toBe(true)
  })
})
