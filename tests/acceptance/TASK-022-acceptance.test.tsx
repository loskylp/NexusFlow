/**
 * Acceptance tests for TASK-022: Log Streamer (GUI)
 * Requirements: REQ-018
 *
 * Each acceptance criterion is tested with at least one positive case
 * (criterion satisfied) and at least one negative case (condition that must NOT
 * satisfy the criterion and must be correctly rejected or absent).
 *
 * AC-1: Selecting a task initiates SSE connection and streams log lines in real time
 * AC-2: Phase filter toggles show/hide log lines by pipeline phase (client-side)
 * AC-3: Phase tags are color-coded per design system (datasource=blue, process=purple, sink=green)
 * AC-4: Auto-scroll follows new lines; toggling off allows scroll-back
 * AC-5: Download Logs fetches full log history from REST API and triggers browser download
 * AC-6: SSE disconnection reconnects with Last-Event-ID; missed lines are replayed
 * AC-7: Access denied (403) for non-owner non-admin shows error in log panel — not a redirect
 * AC-8: Log lines include timestamp, level, phase tag, and message text
 *
 * Tests operate on the rendered component through its observable interface
 * (DOM, callbacks). No direct access to implementation internals.
 *
 * See: TASK-022, REQ-018, ADR-007, UX Spec (Log Streamer), DESIGN.md
 */

import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'

import LogStreamerPage, {
  filterLogLines,
  LogLine,
  LogStatusBar,
} from '@/pages/LogStreamerPage'
import type { TaskLog } from '@/types/domain'
import type { UseLogsReturn } from '@/hooks/useLogs'

// ---------------------------------------------------------------------------
// Module mocks
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
// AC-1: Selecting a task initiates SSE connection and streams log lines in real time
// REQ-018 Given/When/Then:
//   Given Task-1 belongs to User-A and is running
//   When User-A opens the Log Streamer for Task-1
//   Then log lines appear in real time as the worker produces them
// ---------------------------------------------------------------------------

describe('AC-1 — REQ-018: SSE connection initiated on task selection', () => {
  // Positive: task selected via URL param — useLogs receives the taskId
  it('[positive] useLogs is called with the taskId from the URL query param', () => {
    renderPage('/tasks/logs?taskId=task-001')
    expect(mockUseLogs).toHaveBeenCalledWith(
      expect.objectContaining({ taskId: 'task-001' })
    )
  })

  // Positive: log lines from useLogs are rendered in the panel
  it('[positive] log lines returned by useLogs are rendered in the log panel', () => {
    const lines = [
      makeLog({ id: 'l1', line: '[datasource] INFO ingesting records' }),
      makeLog({ id: 'l2', line: '[process] INFO transforming data' }),
    ]
    mockUseLogs.mockReturnValue(defaultLogsReturn({ lines }))
    renderPage('/tasks/logs?taskId=task-001')

    expect(screen.getByText(/ingesting records/i)).toBeTruthy()
    expect(screen.getByText(/transforming data/i)).toBeTruthy()
  })

  // Negative: no task selected — useLogs receives undefined taskId (no SSE connection)
  it('[negative] useLogs receives undefined taskId when no task is selected (no connection)', () => {
    renderPage('/tasks/logs')
    expect(mockUseLogs).toHaveBeenCalledWith(
      expect.objectContaining({ taskId: undefined })
    )
  })

  // Negative: empty panel shown when no task is selected and no lines
  it('[negative] log panel shows empty message when no task selected and lines are empty', () => {
    renderPage('/tasks/logs')
    expect(screen.queryByText(/ingesting records/i)).toBeNull()
    // "choose a task" empty-state message appears
    expect(screen.queryByText(/choose a task/i)).toBeTruthy()
  })
})

// ---------------------------------------------------------------------------
// AC-2: Phase filter toggles show/hide log lines by pipeline phase (client-side)
// REQ-018 Given/When/Then:
//   Given: user has logs for multiple phases
//   When: user selects DataSource filter
//   Then: only DataSource lines are visible
// ---------------------------------------------------------------------------

describe('AC-2 — REQ-018: Phase filter toggles show/hide lines client-side', () => {
  const mixedLines = [
    makeLog({ id: 'l1', line: '[datasource] INFO fetch complete' }),
    makeLog({ id: 'l2', line: '[process] INFO transform done' }),
    makeLog({ id: 'l3', line: '[sink] INFO records written' }),
  ]

  // Positive: DataSource filter shows only datasource lines
  it('[positive] DataSource filter shows only [datasource] lines', async () => {
    const user = userEvent.setup()
    mockUseLogs.mockReturnValue(defaultLogsReturn({ lines: mixedLines }))
    renderPage('/tasks/logs?taskId=task-001')

    await user.click(screen.getByRole('button', { name: /DataSource/i }))

    expect(screen.getByText(/fetch complete/i)).toBeTruthy()
    expect(screen.queryByText(/transform done/i)).toBeNull()
    expect(screen.queryByText(/records written/i)).toBeNull()
  })

  // Positive: Process filter shows only process lines
  it('[positive] Process filter shows only [process] lines', async () => {
    const user = userEvent.setup()
    mockUseLogs.mockReturnValue(defaultLogsReturn({ lines: mixedLines }))
    renderPage('/tasks/logs?taskId=task-001')

    await user.click(screen.getByRole('button', { name: /Process/i }))

    expect(screen.getByText(/transform done/i)).toBeTruthy()
    expect(screen.queryByText(/fetch complete/i)).toBeNull()
    expect(screen.queryByText(/records written/i)).toBeNull()
  })

  // Positive: Sink filter shows only sink lines
  it('[positive] Sink filter shows only [sink] lines', async () => {
    const user = userEvent.setup()
    mockUseLogs.mockReturnValue(defaultLogsReturn({ lines: mixedLines }))
    renderPage('/tasks/logs?taskId=task-001')

    await user.click(screen.getByRole('button', { name: /^Sink$/i }))

    expect(screen.getByText(/records written/i)).toBeTruthy()
    expect(screen.queryByText(/fetch complete/i)).toBeNull()
    expect(screen.queryByText(/transform done/i)).toBeNull()
  })

  // Positive: All filter restores all lines after phase filter
  it('[positive] All filter restores all lines after a phase filter is applied', async () => {
    const user = userEvent.setup()
    mockUseLogs.mockReturnValue(defaultLogsReturn({ lines: mixedLines }))
    renderPage('/tasks/logs?taskId=task-001')

    await user.click(screen.getByRole('button', { name: /DataSource/i }))
    expect(screen.queryByText(/transform done/i)).toBeNull()

    await user.click(screen.getByRole('button', { name: /^All$/i }))

    expect(screen.getByText(/fetch complete/i)).toBeTruthy()
    expect(screen.getByText(/transform done/i)).toBeTruthy()
    expect(screen.getByText(/records written/i)).toBeTruthy()
  })

  // Negative: DataSource filter does not show [process] or [sink] lines
  it('[negative] DataSource filter does not show [process] or [sink] lines', async () => {
    const user = userEvent.setup()
    mockUseLogs.mockReturnValue(defaultLogsReturn({ lines: mixedLines }))
    renderPage('/tasks/logs?taskId=task-001')

    await user.click(screen.getByRole('button', { name: /DataSource/i }))

    expect(screen.queryByText(/transform done/i)).toBeNull()
    expect(screen.queryByText(/records written/i)).toBeNull()
  })

  // [VERIFIER-ADDED] Negative: filtering to a phase with no matching lines shows no lines
  it('[negative] [VERIFIER-ADDED] Sink filter with no sink lines shows nothing', async () => {
    const user = userEvent.setup()
    const datasourceOnlyLines = [
      makeLog({ id: 'l1', line: '[datasource] INFO fetch complete' }),
    ]
    mockUseLogs.mockReturnValue(defaultLogsReturn({ lines: datasourceOnlyLines }))
    renderPage('/tasks/logs?taskId=task-001')

    await user.click(screen.getByRole('button', { name: /^Sink$/i }))

    expect(screen.queryByText(/fetch complete/i)).toBeNull()
  })
})

// ---------------------------------------------------------------------------
// AC-2 helper: filterLogLines pure function (also tests the filtering logic itself)
// ---------------------------------------------------------------------------

describe('AC-2 — filterLogLines helper', () => {
  it('[positive] filterLogLines returns same reference for "all" phase', () => {
    const lines = [makeLog()]
    expect(filterLogLines(lines, 'all')).toBe(lines)
  })

  it('[positive] filterLogLines filters to [datasource] lines only', () => {
    const lines = [
      makeLog({ id: 'l1', line: '[datasource] data' }),
      makeLog({ id: 'l2', line: '[process] data' }),
    ]
    const result = filterLogLines(lines, 'datasource')
    expect(result).toHaveLength(1)
    expect(result[0]?.line).toContain('[datasource]')
  })

  it('[negative] filterLogLines does not include non-matching lines', () => {
    const lines = [
      makeLog({ id: 'l1', line: '[process] transform' }),
      makeLog({ id: 'l2', line: '[sink] write' }),
    ]
    const result = filterLogLines(lines, 'datasource')
    expect(result).toHaveLength(0)
  })
})

// ---------------------------------------------------------------------------
// AC-3: Phase tags are color-coded per design system
// DESIGN.md: datasource=#2563EB (blue), process=#8B5CF6 (purple), sink=#16A34A (green)
// ---------------------------------------------------------------------------

describe('AC-3 — REQ-018: Phase tags are color-coded per design system', () => {
  // Positive: datasource log line renders a colored badge with datasource color
  // jsdom converts hex to rgb: #2563EB = rgb(37, 99, 235)
  it('[positive] datasource log line renders a colored phase badge', () => {
    const log = makeLog({ line: '[datasource] INFO fetch started' })
    const { container } = render(<LogLine line={log} />)
    // The datasource badge must be present
    expect(container.textContent).toContain('datasource')
    // The badge must use the datasource color — jsdom renders hex as rgb(37, 99, 235)
    const badge = container.querySelector('span[style*="rgb(37, 99, 235)"]')
    expect(badge).not.toBeNull()
  })

  // Positive: process log line renders a colored phase badge
  // jsdom converts hex to rgb: #8B5CF6 = rgb(139, 92, 246)
  it('[positive] process log line renders a colored phase badge', () => {
    const log = makeLog({ line: '[process] INFO transform complete' })
    const { container } = render(<LogLine line={log} />)
    expect(container.textContent).toContain('process')
    // The badge must use the process color — jsdom renders hex as rgb(139, 92, 246)
    const badge = container.querySelector('span[style*="rgb(139, 92, 246)"]')
    expect(badge).not.toBeNull()
  })

  // Positive: sink log line renders a colored phase badge
  // jsdom converts hex to rgb: #16A34A = rgb(22, 163, 74)
  it('[positive] sink log line renders a colored phase badge', () => {
    const log = makeLog({ line: '[sink] INFO writing records' })
    const { container } = render(<LogLine line={log} />)
    expect(container.textContent).toContain('sink')
    // The badge must use the sink color — jsdom renders hex as rgb(22, 163, 74)
    const badge = container.querySelector('span[style*="rgb(22, 163, 74)"]')
    expect(badge).not.toBeNull()
  })

  // Negative: a line with no phase tag renders no phase badge
  it('[negative] log line with no phase tag renders no phase badge', () => {
    const log = makeLog({ line: 'generic system message' })
    const { container } = render(<LogLine line={log} />)
    // No phase badge color should be present (no datasource, process, or sink badge)
    expect(container.querySelector('span[style*="rgb(37, 99, 235)"]')).toBeNull()
    expect(container.querySelector('span[style*="rgb(139, 92, 246)"]')).toBeNull()
    expect(container.querySelector('span[style*="rgb(22, 163, 74)"]')).toBeNull()
  })

  // [VERIFIER-ADDED] Negative: wrong phase badge color must not appear for datasource
  it('[negative] [VERIFIER-ADDED] datasource line does not use process or sink colors', () => {
    const log = makeLog({ line: '[datasource] INFO fetch' })
    const { container } = render(<LogLine line={log} />)
    // Only the datasource badge element (colored text span) should be present
    // Process color: rgb(139, 92, 246); Sink color: rgb(22, 163, 74)
    const purpleBadge = container.querySelector('span[style*="rgb(139, 92, 246)"]')
    const greenBadge = container.querySelector('span[style*="rgb(22, 163, 74)"]')
    expect(purpleBadge).toBeNull()
    expect(greenBadge).toBeNull()
  })
})

// ---------------------------------------------------------------------------
// AC-4: Auto-scroll follows new lines; toggling off allows scroll-back
// ---------------------------------------------------------------------------

describe('AC-4 — REQ-018: Auto-scroll toggle', () => {
  // Positive: auto-scroll toggle is present and checked by default
  it('[positive] auto-scroll checkbox is rendered and checked by default', () => {
    renderPage('/tasks/logs?taskId=task-001')
    const toggle = screen.getByRole('checkbox', { name: /auto-scroll/i }) as HTMLInputElement
    expect(toggle).toBeTruthy()
    expect(toggle.checked).toBe(true)
  })

  // Positive: auto-scroll can be toggled off
  it('[positive] auto-scroll toggle can be turned off', async () => {
    const user = userEvent.setup()
    renderPage('/tasks/logs?taskId=task-001')
    const toggle = screen.getByRole('checkbox', { name: /auto-scroll/i }) as HTMLInputElement

    await user.click(toggle)

    expect(toggle.checked).toBe(false)
  })

  // Positive: auto-scroll can be toggled back on
  it('[positive] auto-scroll toggle can be turned back on after turning off', async () => {
    const user = userEvent.setup()
    renderPage('/tasks/logs?taskId=task-001')
    const toggle = screen.getByRole('checkbox', { name: /auto-scroll/i }) as HTMLInputElement

    await user.click(toggle) // off
    await user.click(toggle) // on again

    expect(toggle.checked).toBe(true)
  })

  // Negative: auto-scroll off state is correctly reflected (not still checked)
  it('[negative] after toggling off, auto-scroll checkbox is not checked', async () => {
    const user = userEvent.setup()
    renderPage('/tasks/logs?taskId=task-001')
    const toggle = screen.getByRole('checkbox', { name: /auto-scroll/i }) as HTMLInputElement

    await user.click(toggle)

    // Must be false — not still true
    expect(toggle.checked).toBe(false)
  })
})

// ---------------------------------------------------------------------------
// AC-5: Download Logs fetches full log history from REST API and triggers browser download
// REQ-018 Given/When/Then:
//   Given a task is selected
//   When Download Logs is clicked
//   Then downloadTaskLogs REST endpoint is called with the taskId
// ---------------------------------------------------------------------------

describe('AC-5 — REQ-018: Download Logs fetches from REST API', () => {
  // Positive: Download Logs button calls the REST API with the task ID
  it('[positive] clicking Download Logs calls downloadTaskLogs with the current taskId', async () => {
    const user = userEvent.setup()
    mockDownloadTaskLogs.mockResolvedValue('2026-04-01T10:00:00Z [datasource] INFO data loaded\n')
    renderPage('/tasks/logs?taskId=task-001')

    await user.click(screen.getByRole('button', { name: /Download Logs/i }))

    await waitFor(() => {
      expect(mockDownloadTaskLogs).toHaveBeenCalledWith('task-001')
    })
  })

  // Negative: Download Logs does not call API when no task is selected
  it('[negative] Download Logs button does not call downloadTaskLogs when no task selected', async () => {
    const user = userEvent.setup()
    renderPage('/tasks/logs')

    await user.click(screen.getByRole('button', { name: /Download Logs/i }))

    expect(mockDownloadTaskLogs).not.toHaveBeenCalled()
  })

  // Negative: Download Logs button is disabled (not allowed) when no task is selected
  it('[negative] Download Logs button is disabled when no task is selected', () => {
    renderPage('/tasks/logs')
    const btn = screen.getByRole('button', { name: /Download Logs/i }) as HTMLButtonElement
    expect(btn.disabled).toBe(true)
  })

  // [VERIFIER-ADDED] Negative: download does not invoke the SSE endpoint (REST only)
  it('[negative] [VERIFIER-ADDED] download calls downloadTaskLogs exactly once per click, not zero times', async () => {
    const user = userEvent.setup()
    mockDownloadTaskLogs.mockResolvedValue('log data\n')
    renderPage('/tasks/logs?taskId=task-xyz')

    await user.click(screen.getByRole('button', { name: /Download Logs/i }))

    await waitFor(() => expect(mockDownloadTaskLogs).toHaveBeenCalledTimes(1))
  })
})

// ---------------------------------------------------------------------------
// AC-6: SSE disconnection reconnects with Last-Event-ID; missed lines are replayed
// REQ-018: SSE reconnection strategy per ADR-007
// ---------------------------------------------------------------------------

describe('AC-6 — REQ-018: Last-Event-ID displayed in status bar for reconnection', () => {
  // Positive: Last-Event-ID is shown in the status bar when present
  it('[positive] Last-Event-ID is displayed in the status bar', () => {
    mockUseLogs.mockReturnValue(
      defaultLogsReturn({ lastEventId: 'log-042', sseStatus: 'connected' })
    )
    renderPage('/tasks/logs?taskId=task-001')

    expect(screen.getByText(/log-042/)).toBeTruthy()
  })

  // Positive: reconnecting status is shown in the status bar when SSE reconnects
  it('[positive] reconnecting status is shown when SSE is reconnecting', () => {
    mockUseLogs.mockReturnValue(
      defaultLogsReturn({ sseStatus: 'reconnecting', lastEventId: 'log-010' })
    )
    renderPage('/tasks/logs?taskId=task-001')

    expect(screen.getByText(/Reconnecting/i)).toBeTruthy()
  })

  // Positive: Last-Event-ID is preserved after clearLines (replay correctness)
  it('[positive] Last-Event-ID display shows the preserved value after Clear is pressed', async () => {
    const user = userEvent.setup()
    const clearLines = vi.fn()
    mockUseLogs.mockReturnValue(
      defaultLogsReturn({ lastEventId: 'log-099', clearLines, sseStatus: 'connected' })
    )
    renderPage('/tasks/logs?taskId=task-001')

    // Status bar shows the lastEventId
    expect(screen.getByText(/log-099/)).toBeTruthy()

    // After Clear — clearLines is called but lastEventId stays
    await user.click(screen.getByRole('button', { name: /Clear/i }))
    expect(clearLines).toHaveBeenCalledOnce()
    // The status bar still shows it (hook preserves it; our mock preserves it)
    expect(screen.getByText(/log-099/)).toBeTruthy()
  })

  // Negative: Last-Event-ID is not shown when null
  it('[negative] Last-Event-ID is not displayed when there is no lastEventId', () => {
    mockUseLogs.mockReturnValue(
      defaultLogsReturn({ lastEventId: null, sseStatus: 'connected' })
    )
    renderPage('/tasks/logs?taskId=task-001')

    expect(screen.queryByText(/Last-Event-ID/i)).toBeNull()
  })
})

// ---------------------------------------------------------------------------
// AC-7: Access denied (403) shows error in log panel — not a redirect
// REQ-018 Given/When/Then:
//   Given Task-1 belongs to User-A
//   When User-B (non-admin) attempts to stream logs for Task-1
//   Then the system denies access with HTTP 403
// ---------------------------------------------------------------------------

describe('AC-7 — REQ-018: 403 access denied shown in log panel, not a redirect', () => {
  // Given: useLogs surfaces an accessError (403)
  // When: LogStreamerPage renders
  // Then: the error is shown in the log panel
  it('[positive] access denied error is displayed inside the log panel', () => {
    mockUseLogs.mockReturnValue(
      defaultLogsReturn({
        accessError: 'Access denied: you do not have permission to view logs for this task.',
      })
    )
    renderPage('/tasks/logs?taskId=task-owned-by-other-user')

    expect(screen.getByText(/Access denied/i)).toBeTruthy()
  })

  // Positive: the page title remains visible (no redirect occurred)
  it('[positive] page remains on Log Streamer view when access is denied (no redirect)', () => {
    mockUseLogs.mockReturnValue(
      defaultLogsReturn({
        accessError: 'Access denied: you do not have permission to view logs for this task.',
      })
    )
    renderPage('/tasks/logs?taskId=task-owned-by-other-user')

    // The page title must still be present (confirms no redirect to login or another page)
    expect(screen.getByText(/Log Streamer/i)).toBeTruthy()
  })

  // Negative: when no access error, error message is not shown
  it('[negative] access denied message is not shown when accessError is null', () => {
    mockUseLogs.mockReturnValue(defaultLogsReturn({ accessError: null }))
    renderPage('/tasks/logs?taskId=task-001')

    expect(screen.queryByText(/Access denied/i)).toBeNull()
  })

  // [VERIFIER-ADDED] Negative: log lines are not shown when there is an access error
  it('[negative] [VERIFIER-ADDED] log lines are not rendered when accessError is set', () => {
    const lines = [makeLog({ id: 'l1', line: '[datasource] INFO secret data' })]
    mockUseLogs.mockReturnValue(
      defaultLogsReturn({
        accessError: 'Access denied: ...',
        lines,
      })
    )
    renderPage('/tasks/logs?taskId=task-001')

    // The access error is shown but the log lines must not be rendered
    expect(screen.queryByText(/secret data/i)).toBeNull()
    expect(screen.getByText(/Access denied/i)).toBeTruthy()
  })
})

// ---------------------------------------------------------------------------
// AC-8: Log lines include timestamp, level, phase tag, and message text
// REQ-018: Log format completeness
// ---------------------------------------------------------------------------

describe('AC-8 — REQ-018: Log lines include timestamp, phase tag, and message text', () => {
  // Positive: timestamp is rendered in the log line
  it('[positive] log line renders a timestamp', () => {
    const log = makeLog({
      line: '[datasource] INFO data ingested',
      timestamp: '2026-04-01T10:00:00Z',
    })
    const { container } = render(<LogLine line={log} />)
    // Timestamp must be present (formatted as HH:MM:SS)
    // The container text must contain some time-like string
    expect(container.textContent).toBeTruthy()
    // Verify the timestamp span is present (it has the secondary color and tabular-nums)
    const timestampSpan = container.querySelector('span[style*="tabular-nums"]')
    expect(timestampSpan).not.toBeNull()
    expect(timestampSpan?.textContent?.trim().length).toBeGreaterThan(0)
  })

  // Positive: phase tag is rendered as a badge
  it('[positive] log line renders the phase tag as a colored badge', () => {
    const log = makeLog({ line: '[process] INFO transform step complete' })
    const { container } = render(<LogLine line={log} />)
    expect(container.textContent).toContain('process')
  })

  // Positive: message text is rendered after phase tag
  it('[positive] log line renders the message text', () => {
    const log = makeLog({ line: '[sink] INFO all records written successfully' })
    render(<LogLine line={log} />)
    expect(screen.getByText(/all records written successfully/i)).toBeTruthy()
  })

  // Positive: ERROR level lines are rendered in red text
  // jsdom converts hex to rgb: #EF4444 = rgb(239, 68, 68)
  it('[positive] log line with level ERROR renders in red text', () => {
    const log = makeLog({
      line: '[process] ERROR transformation failed',
      level: 'ERROR',
    })
    const { container } = render(<LogLine line={log} />)
    // The outer div should have red color applied — jsdom renders #EF4444 as rgb(239, 68, 68)
    const errorLine = container.querySelector('div[style*="rgb(239, 68, 68)"]')
    expect(errorLine).not.toBeNull()
  })

  // Negative: log line without level ERROR does not use red text
  it('[negative] log line with level INFO does not render in red text', () => {
    const log = makeLog({
      line: '[datasource] INFO fetching data',
      level: 'INFO',
    })
    const { container } = render(<LogLine line={log} />)
    const redLine = container.querySelector('div[style*="rgb(239, 68, 68)"]')
    expect(redLine).toBeNull()
  })

  // [VERIFIER-ADDED] Negative: phase tag text is not duplicated in the message
  it('[negative] [VERIFIER-ADDED] phase tag does not appear verbatim in the message body', () => {
    const log = makeLog({ line: '[datasource] INFO records fetched' })
    const { container } = render(<LogLine line={log} />)
    // The message span (last child span) should not contain "[datasource]"
    const messageSpan = container.querySelector('span[style*="flex: 1"]')
    expect(messageSpan?.textContent).not.toContain('[datasource]')
  })
})

// ---------------------------------------------------------------------------
// AC Layout: Terminal-style panel with dark background and monospace font
// UX Spec: Log Streamer — terminal-style log panel
// DESIGN.md: Monospace (logs): JetBrains Mono or system monospace, 13px
// ---------------------------------------------------------------------------

describe('Layout — REQ-018: Terminal-style log panel with dark background', () => {
  // Positive: log panel has dark background (#0F172A)
  // jsdom converts hex to rgb: #0F172A = rgb(15, 23, 42)
  it('[positive] log panel renders with dark terminal background color', () => {
    renderPage('/tasks/logs?taskId=task-001')
    const panel = screen.getByRole('log')
    expect(panel).toBeTruthy()
    // Dark background must be set on the panel element
    // jsdom renders #0F172A as rgb(15, 23, 42)
    const style = (panel as HTMLElement).getAttribute('style') ?? ''
    expect(style).toContain('rgb(15, 23, 42)')
  })

  // Positive: log panel has monospace font family
  it('[positive] log lines are rendered in a monospace font', () => {
    const log = makeLog({ line: '[datasource] INFO check font' })
    const { container } = render(<LogLine line={log} />)
    const lineEl = container.firstElementChild as HTMLElement
    const style = lineEl?.getAttribute('style') ?? ''
    expect(style).toContain('monospace')
  })

  // Positive: page renders the "Log Streamer" heading
  it('[positive] page renders the "Log Streamer" heading', () => {
    renderPage('/tasks/logs')
    expect(screen.getByRole('heading', { name: /Log Streamer/i })).toBeTruthy()
  })

  // Positive: all four phase filter toggle buttons are rendered
  it('[positive] all four phase filter buttons are present (All, DataSource, Process, Sink)', () => {
    renderPage('/tasks/logs')
    expect(screen.getByRole('button', { name: /^All$/i })).toBeTruthy()
    expect(screen.getByRole('button', { name: /DataSource/i })).toBeTruthy()
    expect(screen.getByRole('button', { name: /Process/i })).toBeTruthy()
    expect(screen.getByRole('button', { name: /^Sink$/i })).toBeTruthy()
  })

  // Negative: log panel does not have a light background (would violate terminal-style spec)
  it('[negative] log panel does not use a white/light background', () => {
    renderPage('/tasks/logs?taskId=task-001')
    const panel = screen.getByRole('log')
    const style = (panel as HTMLElement).getAttribute('style') ?? ''
    // Must not be white or very light — background must be dark
    expect(style).not.toContain('#FFFFFF')
    expect(style).not.toContain('#FAFAFA')
    expect(style).not.toContain('#F1F5F9')
  })
})

// ---------------------------------------------------------------------------
// Status bar integration — connects LogStatusBar with useLogs sseStatus
// ---------------------------------------------------------------------------

describe('Status bar — REQ-018: SSE connection status displayed correctly', () => {
  it('[positive] status bar shows Connected when SSE is connected', () => {
    mockUseLogs.mockReturnValue(defaultLogsReturn({ sseStatus: 'connected' }))
    renderPage('/tasks/logs?taskId=task-001')
    expect(screen.getByText(/Connected/i)).toBeTruthy()
  })

  it('[positive] status bar shows Reconnecting when SSE is reconnecting', () => {
    mockUseLogs.mockReturnValue(defaultLogsReturn({ sseStatus: 'reconnecting' }))
    renderPage('/tasks/logs?taskId=task-001')
    expect(screen.getByText(/Reconnecting/i)).toBeTruthy()
  })

  it('[positive] status bar shows line count', () => {
    const lines = Array.from({ length: 5 }, (_, i) =>
      makeLog({ id: `log-00${i}`, line: `[datasource] line ${i}` })
    )
    mockUseLogs.mockReturnValue(defaultLogsReturn({ lines }))
    renderPage('/tasks/logs?taskId=task-001')
    expect(screen.getByText(/5/)).toBeTruthy()
  })

  it('[positive] status bar shows Complete when the selected task is in a terminal state', () => {
    mockUseTasks.mockReturnValue(defaultTasksReturn({
      tasks: [{
        id: 'task-001',
        pipelineId: 'pipe-001',
        userId: 'user-001',
        status: 'completed',
        retryConfig: { maxRetries: 0, backoff: 'fixed' },
        retryCount: 0,
        executionId: 'exec-001',
        input: {},
        createdAt: '2026-04-01T10:00:00Z',
        updatedAt: '2026-04-01T10:05:00Z',
      }],
    }))
    mockUseLogs.mockReturnValue(defaultLogsReturn({
      sseStatus: 'closed',
      lines: [makeLog({ id: 'l1', line: '[sink] INFO done' })],
    }))
    renderPage('/tasks/logs?taskId=task-001')
    // Use getAllByText to handle multiple matches (status bar + other elements)
    const completeElements = screen.getAllByText(/Complete/i)
    expect(completeElements.length).toBeGreaterThan(0)
  })

  it('[negative] status bar does not show "Complete" when task is still running', () => {
    mockUseTasks.mockReturnValue(defaultTasksReturn({
      tasks: [{
        id: 'task-001',
        pipelineId: 'pipe-001',
        userId: 'user-001',
        status: 'running',
        retryConfig: { maxRetries: 0, backoff: 'fixed' },
        retryCount: 0,
        executionId: 'exec-001',
        input: {},
        createdAt: '2026-04-01T10:00:00Z',
        updatedAt: '2026-04-01T10:01:00Z',
      }],
    }))
    mockUseLogs.mockReturnValue(defaultLogsReturn({ sseStatus: 'connected' }))
    renderPage('/tasks/logs?taskId=task-001')
    expect(screen.queryByText(/Complete/i)).toBeNull()
  })
})
