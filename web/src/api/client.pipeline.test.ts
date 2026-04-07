/**
 * Unit tests for pipeline-related API client functions added in TASK-023.
 * Verifies request shape and error propagation for getPipeline, updatePipeline,
 * deletePipeline, and new task/user endpoints.
 *
 * See: TASK-023, TASK-026, ADR-004
 */
import { describe, it, expect, vi, beforeEach } from 'vitest'
import {
  getPipeline,
  updatePipeline,
  deletePipeline,
  getTask,
  cancelTask,
  listTasksWithFilters,
  listUsers,
} from './client'
import type { Pipeline, Task } from '@/types/domain'

const mockFetch = vi.fn()

beforeEach(() => {
  vi.stubGlobal('fetch', mockFetch)
  mockFetch.mockReset()
})

// ---------------------------------------------------------------------------
// getPipeline
// ---------------------------------------------------------------------------

describe('getPipeline()', () => {
  it('GETs /api/pipelines/{id} with credentials', async () => {
    const pipeline: Pipeline = {
      id: 'p1',
      name: 'Test',
      userId: 'u1',
      dataSourceConfig: { connectorType: 'generic', config: {}, outputSchema: [] },
      processConfig: { connectorType: 'generic', config: {}, inputMappings: [], outputSchema: [] },
      sinkConfig: { connectorType: 'generic', config: {}, inputMappings: [] },
      createdAt: '',
      updatedAt: '',
    }
    mockFetch.mockResolvedValueOnce({ ok: true, json: async () => pipeline })

    const result = await getPipeline('p1')

    const [url, init] = mockFetch.mock.calls[0] as [string, RequestInit]
    expect(url).toBe('/api/pipelines/p1')
    expect(init.credentials).toBe('include')
    expect(result).toEqual(pipeline)
  })

  it('throws with 404 prefix when pipeline not found', async () => {
    mockFetch.mockResolvedValueOnce({ ok: false, status: 404, text: async () => 'not found' })
    await expect(getPipeline('missing')).rejects.toThrow('404')
  })

  it('throws with 403 prefix when access denied', async () => {
    mockFetch.mockResolvedValueOnce({ ok: false, status: 403, text: async () => 'forbidden' })
    await expect(getPipeline('other')).rejects.toThrow('403')
  })
})

// ---------------------------------------------------------------------------
// updatePipeline
// ---------------------------------------------------------------------------

describe('updatePipeline()', () => {
  it('PUTs /api/pipelines/{id} with the updates payload', async () => {
    const updated: Pipeline = {
      id: 'p1',
      name: 'Updated',
      userId: 'u1',
      dataSourceConfig: { connectorType: 'generic', config: {}, outputSchema: ['x'] },
      processConfig: { connectorType: 'generic', config: {}, inputMappings: [{ sourceField: 'x', targetField: 'y' }], outputSchema: [] },
      sinkConfig: { connectorType: 'generic', config: {}, inputMappings: [] },
      createdAt: '',
      updatedAt: '',
    }
    mockFetch.mockResolvedValueOnce({ ok: true, json: async () => updated })

    const updates = {
      name: 'Updated',
      dataSourceConfig: updated.dataSourceConfig,
      processConfig: updated.processConfig,
      sinkConfig: updated.sinkConfig,
    }
    const result = await updatePipeline('p1', updates)

    const [url, init] = mockFetch.mock.calls[0] as [string, RequestInit]
    expect(url).toBe('/api/pipelines/p1')
    expect(init.method).toBe('PUT')
    expect(init.credentials).toBe('include')
    expect(JSON.parse(init.body as string)).toEqual(updates)
    expect(result).toEqual(updated)
  })

  it('throws with 400 prefix on schema validation failure', async () => {
    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 400,
      text: async () => "process input mapping: source field 'bad' not found in datasource output schema",
    })
    await expect(updatePipeline('p1', { name: '', dataSourceConfig: { connectorType: '', config: {}, outputSchema: [] }, processConfig: { connectorType: '', config: {}, inputMappings: [], outputSchema: [] }, sinkConfig: { connectorType: '', config: {}, inputMappings: [] } }))
      .rejects.toThrow('400')
  })
})

// ---------------------------------------------------------------------------
// deletePipeline
// ---------------------------------------------------------------------------

describe('deletePipeline()', () => {
  it('DELETEs /api/pipelines/{id} and returns void on 204', async () => {
    mockFetch.mockResolvedValueOnce({ ok: true, status: 204, json: async () => ({}) })

    await deletePipeline('p1')

    const [url, init] = mockFetch.mock.calls[0] as [string, RequestInit]
    expect(url).toBe('/api/pipelines/p1')
    expect(init.method).toBe('DELETE')
    expect(init.credentials).toBe('include')
  })

  it('throws with 409 prefix when pipeline has active tasks', async () => {
    mockFetch.mockResolvedValueOnce({ ok: false, status: 409, text: async () => 'active tasks' })
    await expect(deletePipeline('p1')).rejects.toThrow('409')
  })
})

// ---------------------------------------------------------------------------
// getTask
// ---------------------------------------------------------------------------

describe('getTask()', () => {
  it('GETs /api/tasks/{id} and returns a Task', async () => {
    const task: Task = {
      id: 't1',
      pipelineId: 'p1',
      userId: 'u1',
      status: 'running',
      retryConfig: { maxRetries: 3, backoff: 'exponential' },
      retryCount: 0,
      executionId: 'e1',
      input: {},
      createdAt: '',
      updatedAt: '',
    }
    mockFetch.mockResolvedValueOnce({ ok: true, json: async () => task })

    const result = await getTask('t1')

    const [url] = mockFetch.mock.calls[0] as [string]
    expect(url).toBe('/api/tasks/t1')
    expect(result).toEqual(task)
  })
})

// ---------------------------------------------------------------------------
// cancelTask
// ---------------------------------------------------------------------------

describe('cancelTask()', () => {
  it('POSTs to /api/tasks/{id}/cancel', async () => {
    mockFetch.mockResolvedValueOnce({ ok: true, status: 204, json: async () => ({}) })

    await cancelTask('t1')

    const [url, init] = mockFetch.mock.calls[0] as [string, RequestInit]
    expect(url).toBe('/api/tasks/t1/cancel')
    expect(init.method).toBe('POST')
  })

  it('throws with 409 prefix if task is in terminal state', async () => {
    mockFetch.mockResolvedValueOnce({ ok: false, status: 409, text: async () => 'terminal state' })
    await expect(cancelTask('t1')).rejects.toThrow('409')
  })
})

// ---------------------------------------------------------------------------
// listTasksWithFilters
// ---------------------------------------------------------------------------

describe('listTasksWithFilters()', () => {
  it('GETs /api/tasks with no query string when no filters', async () => {
    mockFetch.mockResolvedValueOnce({ ok: true, json: async () => [] })
    await listTasksWithFilters()
    const [url] = mockFetch.mock.calls[0] as [string]
    expect(url).toBe('/api/tasks')
  })

  it('builds query string from provided filter params', async () => {
    mockFetch.mockResolvedValueOnce({ ok: true, json: async () => [] })
    await listTasksWithFilters({ status: 'running', pipelineId: 'p1', search: 'my-task' })
    const [url] = mockFetch.mock.calls[0] as [string]
    expect(url).toContain('status=running')
    expect(url).toContain('pipelineId=p1')
    expect(url).toContain('search=my-task')
  })

  it('omits absent filter params from query string', async () => {
    mockFetch.mockResolvedValueOnce({ ok: true, json: async () => [] })
    await listTasksWithFilters({ status: 'failed' })
    const [url] = mockFetch.mock.calls[0] as [string]
    expect(url).toContain('status=failed')
    expect(url).not.toContain('pipelineId')
    expect(url).not.toContain('search')
  })
})

// ---------------------------------------------------------------------------
// listUsers
// ---------------------------------------------------------------------------

describe('listUsers()', () => {
  it('GETs /api/users and returns a User array', async () => {
    mockFetch.mockResolvedValueOnce({ ok: true, json: async () => [] })
    const result = await listUsers()
    const [url] = mockFetch.mock.calls[0] as [string]
    expect(url).toBe('/api/users')
    expect(result).toEqual([])
  })

  it('throws with 403 prefix when caller is not Admin', async () => {
    mockFetch.mockResolvedValueOnce({ ok: false, status: 403, text: async () => 'forbidden' })
    await expect(listUsers()).rejects.toThrow('403')
  })
})
