/**
 * TASK-034 Acceptance Test — Chaos Controller GUI.
 *
 * Validates all 6 acceptance criteria from the task plan:
 *
 *   AC-1 (Kill Worker): selecting a worker and clicking Kill (after confirmation)
 *        calls POST /api/chaos/kill-worker; activity log shows timeline.
 *
 *   AC-2 (Disconnect DB): clicking Disconnect (after confirmation) calls
 *        POST /api/chaos/disconnect-db; countdown timer shown for selected duration.
 *
 *   AC-3 (Flood Queue): submitting a burst calls POST /api/chaos/flood-queue and
 *        the log shows the submitted count.
 *
 *   AC-4 (System status): system status indicator reflects GET /api/health response.
 *
 *   AC-5 (Admin-only): User role cannot access this view (access denied shown).
 *
 *   AC-6 (Confirmation dialogs): all destructive actions require confirmation;
 *        cancelling confirmation does not call the endpoint.
 *
 * See: DEMO-004, UX Spec (Chaos Controller), TASK-034
 */

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, act, fireEvent, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import ChaosControllerPage from '../../web/src/pages/ChaosControllerPage'
import type { Worker, Pipeline, ChaosActivityEntry } from '../../web/src/types/domain'

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

vi.mock('../../web/src/context/AuthContext')
vi.mock('../../web/src/api/client')

import * as authModule from '../../web/src/context/AuthContext'
import * as clientModule from '../../web/src/api/client'

const mockUseAuth = vi.mocked(authModule.useAuth)
const mockListWorkers = vi.mocked(clientModule.listWorkers)
const mockListPipelines = vi.mocked(clientModule.listPipelines)
const mockKillWorker = vi.mocked(clientModule.killWorker)
const mockDisconnectDatabase = vi.mocked(clientModule.disconnectDatabase)
const mockFloodQueue = vi.mocked(clientModule.floodQueue)

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

const WORKER_ID = 'worker-test-001'
const PIPELINE_ID = 'aaaa0000-0000-0000-0000-000000000001'

function makeWorker(): Worker {
  return {
    id: WORKER_ID,
    tags: ['demo'],
    status: 'online',
    lastHeartbeat: '2026-04-15T10:00:00Z',
    registeredAt: '2026-04-15T09:00:00Z',
  }
}

function makePipeline(): Pipeline {
  return {
    id: PIPELINE_ID,
    name: 'Test Pipeline',
    userId: 'user-001',
    dataSourceConfig: { connectorType: 'demo', config: {}, outputSchema: [] },
    processConfig: { connectorType: 'simulated', config: {}, inputMappings: [], outputSchema: [] },
    sinkConfig: { connectorType: 'demo', config: {}, inputMappings: [] },
    createdAt: '2026-04-15T09:00:00Z',
    updatedAt: '2026-04-15T09:00:00Z',
  }
}

function makeLogEntry(message: string): ChaosActivityEntry {
  return { timestamp: '2026-04-15T10:00:00Z', message, level: 'info' }
}

// ---------------------------------------------------------------------------
// Setup / teardown
// ---------------------------------------------------------------------------

beforeEach(() => {
  // Default: admin user
  mockUseAuth.mockReturnValue({
    user: {
      id: 'user-admin',
      username: 'admin',
      role: 'admin',
      active: true,
      mustChangePassword: false,
      createdAt: '2026-01-01T00:00:00Z',
    },
    login: vi.fn(),
    logout: vi.fn(),
    isLoading: false,
  })

  // Default: one worker, one pipeline
  mockListWorkers.mockResolvedValue([makeWorker()])
  mockListPipelines.mockResolvedValue([makePipeline()])

  // Default health: nominal (fetch mocked globally)
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
    ok: true,
    json: async () => ({ status: 'ok', checks: { db: 'ok', redis: 'ok' } }),
  }))
})

afterEach(() => {
  vi.unstubAllGlobals()
  vi.clearAllMocks()
})

function renderPage() {
  return render(
    <MemoryRouter>
      <ChaosControllerPage />
    </MemoryRouter>
  )
}

/**
 * waitForPageLoad waits for the selector loading state to resolve.
 */
async function waitForPageLoad() {
  await waitFor(() => {
    expect(screen.queryByText('Loading...')).not.toBeInTheDocument()
  })
}

// ---------------------------------------------------------------------------
// AC-5: Admin-only access
// ---------------------------------------------------------------------------

describe('AC-5: Admin-only access', () => {
  it('shows access denied message for non-admin user', () => {
    mockUseAuth.mockReturnValue({
      user: {
        id: 'user-regular',
        username: 'user',
        role: 'user',
        active: true,
        mustChangePassword: false,
        createdAt: '2026-01-01T00:00:00Z',
      },
      login: vi.fn(),
      logout: vi.fn(),
      isLoading: false,
    })

    renderPage()

    expect(screen.getByRole('alert')).toBeInTheDocument()
    expect(screen.getByText(/access denied/i)).toBeInTheDocument()
  })

  it('shows access denied message when user is null (unauthenticated)', () => {
    mockUseAuth.mockReturnValue({
      user: null,
      login: vi.fn(),
      logout: vi.fn(),
      isLoading: false,
    })

    renderPage()

    expect(screen.getByRole('alert')).toBeInTheDocument()
  })

  it('renders Chaos Controller page for admin user', async () => {
    renderPage()
    await waitForPageLoad()
    expect(screen.getByText('Chaos Controller')).toBeInTheDocument()
  })
})

// ---------------------------------------------------------------------------
// AC-4: System status indicator
// ---------------------------------------------------------------------------

describe('AC-4: System status indicator', () => {
  it('shows Nominal status when all health checks pass', async () => {
    renderPage()
    await waitForPageLoad()

    await waitFor(() => {
      expect(screen.getByRole('status', { name: /system status: nominal/i })).toBeInTheDocument()
    })
  })

  it('shows Degraded status when a health check fails', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ status: 'degraded', checks: { db: 'error', redis: 'ok' } }),
    }))

    renderPage()
    await waitForPageLoad()

    await waitFor(() => {
      expect(screen.getByRole('status', { name: /system status: degraded/i })).toBeInTheDocument()
    })
  })

  it('shows Critical status when health endpoint is unreachable', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('Network error')))

    renderPage()
    await waitForPageLoad()

    await waitFor(() => {
      expect(screen.getByRole('status', { name: /system status: critical/i })).toBeInTheDocument()
    })
  })
})

// ---------------------------------------------------------------------------
// AC-1: Kill Worker
// ---------------------------------------------------------------------------

describe('AC-1: Kill Worker', () => {
  it('populates worker selector from workers list', async () => {
    renderPage()
    await waitForPageLoad()

    const selector = screen.getByRole('combobox', { name: /select worker/i })
    expect(selector).toBeInTheDocument()
    expect(screen.getByText(new RegExp(WORKER_ID))).toBeInTheDocument()
  })

  it('kill button is disabled when no worker is selected', async () => {
    renderPage()
    await waitForPageLoad()

    const killBtn = screen.getByRole('button', { name: /kill selected worker/i })
    expect(killBtn).toBeDisabled()
  })

  it('shows confirmation dialog before kill action', async () => {
    renderPage()
    await waitForPageLoad()

    fireEvent.change(screen.getByRole('combobox', { name: /select worker/i }), {
      target: { value: WORKER_ID },
    })

    fireEvent.click(screen.getByRole('button', { name: /kill selected worker/i }))

    expect(screen.getByRole('dialog')).toBeInTheDocument()
  })

  it('AC-6: cancelling confirmation does not call kill endpoint', async () => {
    renderPage()
    await waitForPageLoad()

    fireEvent.change(screen.getByRole('combobox', { name: /select worker/i }), {
      target: { value: WORKER_ID },
    })
    fireEvent.click(screen.getByRole('button', { name: /kill selected worker/i }))
    fireEvent.click(screen.getByRole('button', { name: /cancel/i }))

    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(mockKillWorker).not.toHaveBeenCalled()
  })

  it('AC-1 + AC-6: confirming calls POST /api/chaos/kill-worker and updates activity log', async () => {
    mockKillWorker.mockResolvedValue({
      log: [makeLogEntry(`Container "${WORKER_ID}" killed successfully`)],
    })

    renderPage()
    await waitForPageLoad()

    fireEvent.change(screen.getByRole('combobox', { name: /select worker/i }), {
      target: { value: WORKER_ID },
    })
    fireEvent.click(screen.getByRole('button', { name: /kill selected worker/i }))

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /confirm/i }))
    })

    expect(mockKillWorker).toHaveBeenCalledWith(WORKER_ID)

    await waitFor(() => {
      expect(screen.getByText(/killed successfully/i)).toBeInTheDocument()
    })
  })
})

// ---------------------------------------------------------------------------
// AC-2: Disconnect Database
// ---------------------------------------------------------------------------

describe('AC-2: Disconnect Database', () => {
  it('duration selector shows 15/30/60 options', async () => {
    renderPage()
    await waitForPageLoad()

    const selector = screen.getByRole('combobox', { name: /select disconnect duration/i })
    const options = Array.from(selector.querySelectorAll('option')).map(o => o.textContent)
    expect(options).toContain('15 seconds')
    expect(options).toContain('30 seconds')
    expect(options).toContain('60 seconds')
  })

  it('AC-6: confirmation dialog required before disconnect', async () => {
    renderPage()
    await waitForPageLoad()

    fireEvent.click(screen.getByRole('button', { name: /disconnect database/i }))

    expect(screen.getByRole('dialog')).toBeInTheDocument()
    expect(mockDisconnectDatabase).not.toHaveBeenCalled()
  })

  it('AC-6: cancelling disconnect confirmation does not call endpoint', async () => {
    renderPage()
    await waitForPageLoad()

    fireEvent.click(screen.getByRole('button', { name: /disconnect database/i }))
    fireEvent.click(screen.getByRole('button', { name: /cancel/i }))

    expect(mockDisconnectDatabase).not.toHaveBeenCalled()
  })

  it('AC-2: confirming calls POST /api/chaos/disconnect-db', async () => {
    mockDisconnectDatabase.mockResolvedValue({
      log: [makeLogEntry('Postgres container stopped for 15s')],
      durationSeconds: 15,
    })

    renderPage()
    await waitForPageLoad()

    fireEvent.click(screen.getByRole('button', { name: /disconnect database/i }))

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /confirm/i }))
    })

    expect(mockDisconnectDatabase).toHaveBeenCalledWith(15)
  })

  it('AC-2: countdown timer appears after successful disconnect', async () => {
    // Mock resolves quickly; use fake timers only in this test.
    vi.useFakeTimers()
    try {
      mockDisconnectDatabase.mockResolvedValue({
        log: [],
        durationSeconds: 15,
      })

      const { unmount } = renderPage()

      // Flush promises to let the hook load selectors.
      await act(async () => {
        // Resolve all pending promises
        for (let i = 0; i < 10; i++) await Promise.resolve()
      })

      // Trigger the disconnect
      fireEvent.click(screen.getByRole('button', { name: /disconnect database/i }))

      await act(async () => {
        fireEvent.click(screen.getByRole('button', { name: /confirm/i }))
        // Flush the disconnectDatabase promise
        for (let i = 0; i < 10; i++) await Promise.resolve()
      })

      // Advance one tick to trigger the first interval callback
      act(() => {
        vi.advanceTimersByTime(1100)
      })

      expect(screen.getByTestId('disconnect-countdown')).toBeInTheDocument()
      unmount()
    } finally {
      vi.useRealTimers()
    }
  }, 15000)

  it('AC-2: disconnect button is disabled while countdown is active', async () => {
    vi.useFakeTimers()
    try {
      mockDisconnectDatabase.mockResolvedValue({
        log: [],
        durationSeconds: 15,
      })

      const { unmount } = renderPage()

      await act(async () => {
        for (let i = 0; i < 10; i++) await Promise.resolve()
      })

      fireEvent.click(screen.getByRole('button', { name: /disconnect database/i }))

      await act(async () => {
        fireEvent.click(screen.getByRole('button', { name: /confirm/i }))
        for (let i = 0; i < 10; i++) await Promise.resolve()
      })

      // After the API resolves, isDisconnecting should be true, so button is disabled
      const disconnectBtn = screen.getByRole('button', { name: /disconnect database/i })
      expect(disconnectBtn).toBeDisabled()

      unmount()
    } finally {
      vi.useRealTimers()
    }
  }, 15000)
})

// ---------------------------------------------------------------------------
// AC-3: Flood Queue
// ---------------------------------------------------------------------------

describe('AC-3: Flood Queue', () => {
  it('pipeline selector is populated from pipelines list', async () => {
    renderPage()
    await waitForPageLoad()

    const selector = screen.getByRole('combobox', { name: /select pipeline for flood/i })
    expect(selector).toBeInTheDocument()
    expect(screen.getByText('Test Pipeline')).toBeInTheDocument()
  })

  it('task count input is present with default value', async () => {
    renderPage()
    await waitForPageLoad()

    const input = screen.getByRole('spinbutton', { name: /task count/i })
    expect(Number((input as HTMLInputElement).value)).toBeGreaterThan(0)
  })

  it('Submit Burst button is disabled without pipeline selection', async () => {
    renderPage()
    await waitForPageLoad()

    expect(screen.getByRole('button', { name: /submit burst/i })).toBeDisabled()
  })

  it('Submit Burst button calls POST /api/chaos/flood-queue without confirmation', async () => {
    mockFloodQueue.mockResolvedValue({
      submittedCount: 10,
      log: [makeLogEntry('Flood complete: 10 tasks submitted')],
    })

    renderPage()
    await waitForPageLoad()

    fireEvent.change(
      screen.getByRole('combobox', { name: /select pipeline for flood/i }),
      { target: { value: PIPELINE_ID } }
    )

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /submit burst/i }))
    })

    // No confirmation dialog shown
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(mockFloodQueue).toHaveBeenCalledWith(PIPELINE_ID, expect.any(Number))
  })

  it('AC-3: activity log shows submission count after completion', async () => {
    mockFloodQueue.mockResolvedValue({
      submittedCount: 10,
      log: [makeLogEntry('Flood complete: 10 tasks submitted to queue')],
    })

    renderPage()
    await waitForPageLoad()

    fireEvent.change(
      screen.getByRole('combobox', { name: /select pipeline for flood/i }),
      { target: { value: PIPELINE_ID } }
    )

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /submit burst/i }))
    })

    await waitFor(() => {
      expect(screen.getByText(/flood complete: 10 tasks/i)).toBeInTheDocument()
    })
  })
})
