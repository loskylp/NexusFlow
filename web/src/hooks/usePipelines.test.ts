/**
 * Unit tests for usePipelines hook.
 * Tests REST fetch on mount, loading state transitions, error handling,
 * and refresh trigger.
 *
 * See: TASK-023, REQ-022
 */
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { usePipelines } from './usePipelines'
import type { Pipeline } from '@/types/domain'

// Mock the API client module.
vi.mock('@/api/client', () => ({
  listPipelines: vi.fn(),
}))

import { listPipelines } from '@/api/client'

const mockListPipelines = vi.mocked(listPipelines)

const PIPELINE_FIXTURE: Pipeline = {
  id: 'p1',
  name: 'Test Pipeline',
  userId: 'u1',
  dataSourceConfig: { connectorType: 'generic', config: {}, outputSchema: [] },
  processConfig: { connectorType: 'generic', config: {}, inputMappings: [], outputSchema: [] },
  sinkConfig: { connectorType: 'generic', config: {}, inputMappings: [] },
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
}

beforeEach(() => {
  mockListPipelines.mockReset()
})

describe('usePipelines()', () => {
  it('starts in loading state', () => {
    mockListPipelines.mockImplementation(() => new Promise(() => {}))
    const { result } = renderHook(() => usePipelines())
    expect(result.current.isLoading).toBe(true)
    expect(result.current.pipelines).toEqual([])
    expect(result.current.error).toBeNull()
  })

  it('populates pipelines from GET /api/pipelines on mount', async () => {
    mockListPipelines.mockResolvedValueOnce([PIPELINE_FIXTURE])

    const { result } = renderHook(() => usePipelines())

    await waitFor(() => expect(result.current.isLoading).toBe(false))
    expect(result.current.pipelines).toEqual([PIPELINE_FIXTURE])
    expect(result.current.error).toBeNull()
    expect(mockListPipelines).toHaveBeenCalledOnce()
  })

  it('sets error when fetch fails', async () => {
    mockListPipelines.mockRejectedValueOnce(new Error('503: service unavailable'))

    const { result } = renderHook(() => usePipelines())

    await waitFor(() => expect(result.current.isLoading).toBe(false))
    expect(result.current.pipelines).toEqual([])
    expect(result.current.error).toContain('503')
  })

  it('re-fetches when refresh() is called', async () => {
    mockListPipelines
      .mockResolvedValueOnce([PIPELINE_FIXTURE])
      .mockResolvedValueOnce([PIPELINE_FIXTURE, { ...PIPELINE_FIXTURE, id: 'p2', name: 'Pipeline 2' }])

    const { result } = renderHook(() => usePipelines())

    await waitFor(() => expect(result.current.isLoading).toBe(false))
    expect(result.current.pipelines).toHaveLength(1)

    result.current.refresh()

    await waitFor(() => expect(result.current.pipelines).toHaveLength(2))
    expect(mockListPipelines).toHaveBeenCalledTimes(2)
  })

  it('clears error and re-fetches on successful refresh after failure', async () => {
    mockListPipelines
      .mockRejectedValueOnce(new Error('500: error'))
      .mockResolvedValueOnce([PIPELINE_FIXTURE])

    const { result } = renderHook(() => usePipelines())

    await waitFor(() => expect(result.current.isLoading).toBe(false))
    expect(result.current.error).not.toBeNull()

    result.current.refresh()

    await waitFor(() => expect(result.current.error).toBeNull())
    expect(result.current.pipelines).toEqual([PIPELINE_FIXTURE])
  })
})
