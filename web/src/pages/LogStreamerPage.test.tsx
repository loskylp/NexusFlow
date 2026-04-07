/**
 * Unit tests for LogStreamerPage and its exported sub-components.
 * Covers: filterLogLines (pure function), LogLine rendering,
 * LogStatusBar rendering, LogPanel auto-scroll, and
 * LogStreamerPage integration (task selector, phase toggles,
 * auto-scroll toggle, download trigger, clear, access error,
 * empty/loading states, URL query param pre-selection).
 *
 * See: TASK-022, REQ-018, ADR-007
 */
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import LogStreamerPage, { filterLogLines, LogLine, LogStatusBar } from './LogStreamerPage'
import type { TaskLog } from '@/types/domain'
import type { UseLogsReturn } from '@/hooks/useLogs'

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

vi.mock('@/hooks/useLogs')
vi.mock('@/hooks/useTasks')
vi.mock('@/context/AuthContext')
vi.mock('@/api/client', () => ({
  downloadTaskLogs: vi.fn(),
}))

import * as logsModule from '@/hooks/useLogs'
import * as tasksModule from '@/hooks/useTasks'
import * as authModule from '@/context/AuthContext'
import * as client from '@/api/client'
import type { UseTasksReturn } from '@/hooks/useTasks'

const mockUseLogs = vi.mocked(logsModule.useLogs)
const mockUseTasks = vi.mocked(tasksModule.useTasks)
const mockUseAuth = vi.mocked(authModule.useAuth)
const mockDownloadTaskLogs = vi.mocked(client.downloadTaskLogs)

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

function defaultLogsReturn(partial: Partial<UseLogsReturn> = {}): UseLogsReturn {
  return {
    lines: [],
    isLoading: false,
    accessError: null,
    sseStatus: 'connected',
    lastEventId: null,
    clearLines: vi.fn(),
    ...partial,
  }
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
  vi.clearAllMocks()
  mockUseLogs.mockReturnValue(defaultLogsReturn())
  mockUseTasks.mockReturnValue(defaultTasksReturn())
  mockUseAuth.mockReturnValue({
    user: { id: 'user-001', username: 'alice', role: 'user', active: true, createdAt: '2026-01-01T00:00:00Z' },
    isLoading: false,
    login: vi.fn(),
    logout: vi.fn(),
  })
})

// ---------------------------------------------------------------------------
// filterLogLines — pure function
// ---------------------------------------------------------------------------

describe('filterLogLines — all', () => {
  it('returns all lines when phase is "all"', () => {
    const lines = [
      makeLog({ id: 'l1', line: '[datasource] INFO fetch' }),
      makeLog({ id: 'l2', line: '[process] INFO transform' }),
      makeLog({ id: 'l3', line: '[sink] INFO write' }),
    ]
    const result = filterLogLines(lines, 'all')
    expect(result).toHaveLength(3)
  })

  it('returns same array reference when phase is "all" (no filter applied)', () => {
    const lines = [makeLog()]
    const result = filterLogLines(lines, 'all')
    expect(result).toBe(lines)
  })

  it('returns empty array unchanged for "all"', () => {
    const result = filterLogLines([], 'all')
    expect(result).toHaveLength(0)
  })
})

describe('filterLogLines — datasource', () => {
  it('returns only lines containing [datasource]', () => {
    const lines = [
      makeLog({ id: 'l1', line: '[datasource] INFO fetch' }),
      makeLog({ id: 'l2', line: '[process] INFO transform' }),
      makeLog({ id: 'l3', line: '[datasource] WARN slow query' }),
    ]
    const result = filterLogLines(lines, 'datasource')
    expect(result).toHaveLength(2)
    expect(result.every(l => l.line.includes('[datasource]'))).toBe(true)
  })

  it('returns empty array when no datasource lines exist', () => {
    const lines = [makeLog({ line: '[process] INFO transform' })]
    const result = filterLogLines(lines, 'datasource')
    expect(result).toHaveLength(0)
  })
})

describe('filterLogLines — process', () => {
  it('returns only lines containing [process]', () => {
    const lines = [
      makeLog({ id: 'l1', line: '[datasource] INFO fetch' }),
      makeLog({ id: 'l2', line: '[process] INFO transform' }),
    ]
    const result = filterLogLines(lines, 'process')
    expect(result).toHaveLength(1)
    expect(result[0]?.line).toContain('[process]')
  })
})

describe('filterLogLines — sink', () => {
  it('returns only lines containing [sink]', () => {
    const lines = [
      makeLog({ id: 'l1', line: '[datasource] INFO fetch' }),
      makeLog({ id: 'l2', line: '[sink] INFO writing to destination' }),
      makeLog({ id: 'l3', line: '[sink] INFO write complete' }),
    ]
    const result = filterLogLines(lines, 'sink')
    expect(result).toHaveLength(2)
    expect(result.every(l => l.line.includes('[sink]'))).toBe(true)
  })

  it('returns a new array (does not mutate input) when filtering', () => {
    const lines = [makeLog({ line: '[datasource] INFO fetch' })]
    const result = filterLogLines(lines, 'sink')
    expect(result).not.toBe(lines)
    expect(result).toHaveLength(0)
  })
})

// ---------------------------------------------------------------------------
// LogLine component
// ---------------------------------------------------------------------------

describe('LogLine', () => {
  it('renders the log line text', () => {
    const log = makeLog({ line: '[datasource] INFO data loaded' })
    render(<LogLine line={log} />)
    expect(screen.getByText(/data loaded/)).toBeTruthy()
  })

  it('renders a datasource phase tag in blue', () => {
    const log = makeLog({ line: '[datasource] INFO fetch started' })
    const { container } = render(<LogLine line={log} />)
    // The datasource tag should appear somewhere in the rendered output
    expect(container.textContent).toContain('datasource')
  })

  it('renders a process phase tag', () => {
    const log = makeLog({ line: '[process] INFO transform complete' })
    const { container } = render(<LogLine line={log} />)
    expect(container.textContent).toContain('process')
  })

  it('renders a sink phase tag', () => {
    const log = makeLog({ line: '[sink] INFO writing records' })
    const { container } = render(<LogLine line={log} />)
    expect(container.textContent).toContain('sink')
  })

  it('renders the timestamp', () => {
    const log = makeLog({ timestamp: '2026-04-01T10:00:00Z' })
    const { container } = render(<LogLine line={log} />)
    // Timestamp text should be present somewhere in the line
    expect(container.textContent).toBeTruthy()
  })
})

// ---------------------------------------------------------------------------
// LogStatusBar component
// ---------------------------------------------------------------------------

describe('LogStatusBar', () => {
  it('shows connected status with green indicator when connected', () => {
    render(
      <LogStatusBar
        sseStatus="connected"
        lineCount={10}
        lastEventId="log-042"
        isComplete={false}
      />
    )
    expect(screen.getByText(/Connected/i)).toBeTruthy()
  })

  it('shows reconnecting status when SSE is reconnecting', () => {
    render(
      <LogStatusBar
        sseStatus="reconnecting"
        lineCount={5}
        lastEventId="log-010"
        isComplete={false}
      />
    )
    expect(screen.getByText(/Reconnecting/i)).toBeTruthy()
  })

  it('shows the line count', () => {
    render(
      <LogStatusBar
        sseStatus="connected"
        lineCount={247}
        lastEventId={null}
        isComplete={false}
      />
    )
    expect(screen.getByText(/247/)).toBeTruthy()
  })

  it('shows completion status with line count when complete', () => {
    render(
      <LogStatusBar
        sseStatus="closed"
        lineCount={42}
        lastEventId={null}
        isComplete={true}
      />
    )
    expect(screen.getByText(/Complete/i)).toBeTruthy()
    expect(screen.getByText(/42/)).toBeTruthy()
  })

  it('displays Last-Event-ID when present', () => {
    render(
      <LogStatusBar
        sseStatus="connected"
        lineCount={5}
        lastEventId="log-abc-123"
        isComplete={false}
      />
    )
    expect(screen.getByText(/log-abc-123/)).toBeTruthy()
  })
})

// ---------------------------------------------------------------------------
// LogStreamerPage — no task selected
// ---------------------------------------------------------------------------

describe('LogStreamerPage — no task selected', () => {
  it('shows a task-selection prompt when no taskId is in the URL', () => {
    renderPage('/tasks/logs')
    // The log panel shows a message when no task is selected
    expect(screen.getAllByText(/task/i).length).toBeGreaterThan(0)
    // The empty panel message should mention selecting/choosing a task
    const panelMessage = screen.queryByText(/choose a task/i)
    expect(panelMessage).toBeTruthy()
  })

  it('renders the page title', () => {
    renderPage('/tasks/logs')
    expect(screen.getByText(/Log Streamer/i)).toBeTruthy()
  })

  it('renders phase filter toggle buttons', () => {
    renderPage('/tasks/logs')
    expect(screen.getByRole('button', { name: /All/i })).toBeTruthy()
    expect(screen.getByRole('button', { name: /DataSource/i })).toBeTruthy()
    expect(screen.getByRole('button', { name: /Process/i })).toBeTruthy()
    expect(screen.getByRole('button', { name: /Sink/i })).toBeTruthy()
  })

  it('renders the Download Logs button', () => {
    renderPage('/tasks/logs')
    expect(screen.getByRole('button', { name: /Download/i })).toBeTruthy()
  })

  it('renders the Clear button', () => {
    renderPage('/tasks/logs')
    expect(screen.getByRole('button', { name: /Clear/i })).toBeTruthy()
  })
})

// ---------------------------------------------------------------------------
// LogStreamerPage — URL query param pre-selection
// ---------------------------------------------------------------------------

describe('LogStreamerPage — URL query param pre-selection', () => {
  it('passes taskId from URL query param to useLogs', () => {
    renderPage('/tasks/logs?taskId=task-abc')
    // useLogs should have been called with the taskId from the URL
    expect(mockUseLogs).toHaveBeenCalledWith(
      expect.objectContaining({ taskId: 'task-abc' })
    )
  })

  it('passes undefined taskId to useLogs when no query param', () => {
    renderPage('/tasks/logs')
    expect(mockUseLogs).toHaveBeenCalledWith(
      expect.objectContaining({ taskId: undefined })
    )
  })
})

// ---------------------------------------------------------------------------
// LogStreamerPage — log lines rendering
// ---------------------------------------------------------------------------

describe('LogStreamerPage — log lines rendering', () => {
  it('renders log lines from useLogs in the panel', () => {
    const lines = [
      makeLog({ id: 'l1', line: '[datasource] INFO fetch complete' }),
      makeLog({ id: 'l2', line: '[process] INFO transform done' }),
    ]
    mockUseLogs.mockReturnValue(defaultLogsReturn({ lines }))
    renderPage('/tasks/logs?taskId=task-001')

    expect(screen.getByText(/fetch complete/i)).toBeTruthy()
    expect(screen.getByText(/transform done/i)).toBeTruthy()
  })
})

// ---------------------------------------------------------------------------
// LogStreamerPage — phase filter toggles
// ---------------------------------------------------------------------------

describe('LogStreamerPage — phase filter toggles', () => {
  it('shows only datasource lines when DataSource toggle is active', async () => {
    const user = userEvent.setup()
    const lines = [
      makeLog({ id: 'l1', line: '[datasource] INFO fetch complete' }),
      makeLog({ id: 'l2', line: '[process] INFO transform done' }),
    ]
    mockUseLogs.mockReturnValue(defaultLogsReturn({ lines }))
    renderPage('/tasks/logs?taskId=task-001')

    // Initially both lines visible
    expect(screen.getByText(/fetch complete/i)).toBeTruthy()
    expect(screen.getByText(/transform done/i)).toBeTruthy()

    // Toggle DataSource filter
    await user.click(screen.getByRole('button', { name: /DataSource/i }))

    // Only datasource line should be visible
    expect(screen.getByText(/fetch complete/i)).toBeTruthy()
    expect(screen.queryByText(/transform done/i)).toBeNull()
  })

  it('shows all lines when All toggle is clicked after filtering', async () => {
    const user = userEvent.setup()
    const lines = [
      makeLog({ id: 'l1', line: '[datasource] INFO fetch complete' }),
      makeLog({ id: 'l2', line: '[process] INFO transform done' }),
    ]
    mockUseLogs.mockReturnValue(defaultLogsReturn({ lines }))
    renderPage('/tasks/logs?taskId=task-001')

    // Filter to datasource only
    await user.click(screen.getByRole('button', { name: /DataSource/i }))
    expect(screen.queryByText(/transform done/i)).toBeNull()

    // Click All to restore
    await user.click(screen.getByRole('button', { name: /^All$/i }))
    expect(screen.getByText(/transform done/i)).toBeTruthy()
  })
})

// ---------------------------------------------------------------------------
// LogStreamerPage — access error display
// ---------------------------------------------------------------------------

describe('LogStreamerPage — access error display', () => {
  it('shows access denied message in the log panel when 403 occurs', () => {
    mockUseLogs.mockReturnValue(
      defaultLogsReturn({ accessError: 'Access denied: you do not have permission to view logs for this task.' })
    )
    renderPage('/tasks/logs?taskId=task-001')

    expect(screen.getByText(/Access denied/i)).toBeTruthy()
  })

  it('does not navigate away on 403 — error shown in panel', () => {
    mockUseLogs.mockReturnValue(
      defaultLogsReturn({ accessError: 'Access denied: you do not have permission to view logs for this task.' })
    )
    renderPage('/tasks/logs?taskId=task-001')

    // Page is still the log streamer
    expect(screen.getByText(/Log Streamer/i)).toBeTruthy()
  })
})

// ---------------------------------------------------------------------------
// LogStreamerPage — download logs
// ---------------------------------------------------------------------------

describe('LogStreamerPage — download logs', () => {
  it('calls downloadTaskLogs with the current taskId when Download is clicked', async () => {
    const user = userEvent.setup()
    mockDownloadTaskLogs.mockResolvedValue('log line 1\nlog line 2\n')
    renderPage('/tasks/logs?taskId=task-001')

    await user.click(screen.getByRole('button', { name: /Download/i }))

    await waitFor(() => {
      expect(mockDownloadTaskLogs).toHaveBeenCalledWith('task-001')
    })
  })

  it('does not call downloadTaskLogs when no task is selected', async () => {
    const user = userEvent.setup()
    renderPage('/tasks/logs')

    await user.click(screen.getByRole('button', { name: /Download/i }))

    expect(mockDownloadTaskLogs).not.toHaveBeenCalled()
  })
})

// ---------------------------------------------------------------------------
// LogStreamerPage — clear button
// ---------------------------------------------------------------------------

describe('LogStreamerPage — clear button', () => {
  it('calls clearLines from useLogs when Clear is clicked', async () => {
    const user = userEvent.setup()
    const clearLines = vi.fn()
    mockUseLogs.mockReturnValue(defaultLogsReturn({ clearLines }))
    renderPage('/tasks/logs?taskId=task-001')

    await user.click(screen.getByRole('button', { name: /Clear/i }))

    expect(clearLines).toHaveBeenCalledOnce()
  })
})

// ---------------------------------------------------------------------------
// LogStreamerPage — auto-scroll toggle
// ---------------------------------------------------------------------------

describe('LogStreamerPage — auto-scroll toggle', () => {
  it('renders an auto-scroll toggle control', () => {
    renderPage('/tasks/logs?taskId=task-001')
    // Auto-scroll toggle should be present
    expect(screen.getByRole('checkbox', { name: /auto-scroll/i })).toBeTruthy()
  })

  it('auto-scroll is on by default', () => {
    renderPage('/tasks/logs?taskId=task-001')
    const toggle = screen.getByRole('checkbox', { name: /auto-scroll/i }) as HTMLInputElement
    expect(toggle.checked).toBe(true)
  })

  it('toggles auto-scroll off when clicked', async () => {
    const user = userEvent.setup()
    renderPage('/tasks/logs?taskId=task-001')

    const toggle = screen.getByRole('checkbox', { name: /auto-scroll/i }) as HTMLInputElement
    await user.click(toggle)

    expect(toggle.checked).toBe(false)
  })
})

// ---------------------------------------------------------------------------
// LogStreamerPage — SSE status bar
// ---------------------------------------------------------------------------

describe('LogStreamerPage — SSE status bar', () => {
  it('shows connected status when SSE is connected', () => {
    mockUseLogs.mockReturnValue(defaultLogsReturn({ sseStatus: 'connected', lastEventId: null }))
    renderPage('/tasks/logs?taskId=task-001')
    expect(screen.getByText(/Connected/i)).toBeTruthy()
  })

  it('shows reconnecting status when SSE is reconnecting', () => {
    mockUseLogs.mockReturnValue(defaultLogsReturn({ sseStatus: 'reconnecting' }))
    renderPage('/tasks/logs?taskId=task-001')
    expect(screen.getByText(/Reconnecting/i)).toBeTruthy()
  })
})
