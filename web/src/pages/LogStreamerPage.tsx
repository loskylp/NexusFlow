/**
 * LogStreamerPage — real-time task log viewer with phase filtering (TASK-022).
 *
 * Layout (UX spec — Log Streamer):
 *   - Page header: "Log Streamer" title, streaming status indicator
 *   - Task selector bar:
 *       - Task dropdown (select from recent/own tasks)
 *       - Phase filter toggles: All | DataSource | Process | Sink
 *       - Auto-scroll toggle
 *       - Download Logs button
 *       - Clear button
 *   - Log output panel: dark background, monospace text, full remaining viewport height
 *   - Status bar: SSE connection info, line count, Last-Event-ID
 *
 * Real-time updates are handled by useLogs, which connects SSE to
 * GET /events/tasks/{taskId}/logs with Last-Event-ID reconnect replay (ADR-007).
 *
 * Phase-colored tags per UX spec:
 *   [datasource]  → blue   (#2563EB / --color-info)
 *   [process]     → purple (#8B5CF6 / --color-submitted)
 *   [sink]        → green  (#16A34A / --color-success)
 *
 * The Log Streamer is accessible two ways:
 *   1. Direct navigation via sidebar: opens with empty task selector
 *   2. "View Logs" from Task Feed: opens with taskId pre-selected
 *      (passed via URL param: /tasks/logs?taskId=<uuid>)
 *
 * See: REQ-018, TASK-022, TASK-016, ADR-007, UX Spec (Log Streamer)
 */

import React, { useCallback, useEffect, useRef, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import type { TaskLog } from '@/types/domain'
import type { SSEConnectionStatus } from '@/hooks/useSSE'
import { useLogs } from '@/hooks/useLogs'
import { useTasks } from '@/hooks/useTasks'
import { downloadTaskLogs } from '@/api/client'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

/** Phase filter options — 'all' means no filtering. */
export type PhaseFilter = 'all' | 'datasource' | 'process' | 'sink'

// Phase color constants per DESIGN.md and UX spec.
const PHASE_COLORS: Record<Exclude<PhaseFilter, 'all'>, string> = {
  datasource: '#2563EB',
  process: '#8B5CF6',
  sink: '#16A34A',
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/**
 * filterLogLines applies a phase filter to a log line array.
 * When phase is 'all', returns the original array reference unchanged.
 * Otherwise returns a new array containing only lines whose text includes
 * the [<phase>] tag (e.g., "[datasource]").
 *
 * @param lines  - Full log line array.
 * @param phase  - Phase filter to apply.
 * @returns Filtered array (same reference when phase is 'all'; new array otherwise).
 */
export function filterLogLines(lines: TaskLog[], phase: PhaseFilter): TaskLog[] {
  if (phase === 'all') return lines
  const tag = `[${phase}]`
  return lines.filter(l => l.line.includes(tag))
}

/**
 * detectPhase extracts the pipeline phase from a log line string.
 * Returns the phase if a [datasource], [process], or [sink] tag is found,
 * or null if no phase tag is present.
 *
 * @param line - The log line text to inspect.
 * @returns Detected phase or null.
 */
function detectPhase(line: string): Exclude<PhaseFilter, 'all'> | null {
  if (line.includes('[datasource]')) return 'datasource'
  if (line.includes('[process]')) return 'process'
  if (line.includes('[sink]')) return 'sink'
  return null
}

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

interface LogLineProps {
  line: TaskLog
}

/**
 * LogLine renders a single log line in the terminal panel.
 * Format: <timestamp> [<phase>] <message>
 * Phase tag is color-coded per DESIGN.md. Level ERROR is rendered in red text.
 * No animation — lines appear instantly (UX spec: "animation would be distracting
 * at high throughput").
 */
export function LogLine({ line }: LogLineProps): React.ReactElement {
  const phase = detectPhase(line.line)
  const isError = line.level === 'ERROR' || line.level === 'error'

  // Format timestamp to HH:MM:SS for compact display.
  const timestamp = line.timestamp
    ? new Date(line.timestamp).toLocaleTimeString('en-US', { hour12: false })
    : ''

  // Strip phase tag from the message body to avoid duplication with the colored badge.
  let message = line.line
  if (phase) {
    message = message.replace(`[${phase}]`, '').trim()
  }

  return (
    <div
      style={{
        display: 'flex',
        alignItems: 'flex-start',
        gap: '8px',
        fontFamily: 'var(--font-mono, "JetBrains Mono", monospace)',
        fontSize: '13px',
        lineHeight: '1.5',
        color: isError ? '#EF4444' : '#CBD5E1',
        padding: '1px 0',
        wordBreak: 'break-all',
      }}
    >
      {/* Timestamp */}
      <span
        style={{
          color: '#64748B',
          flexShrink: 0,
          minWidth: '72px',
          fontVariantNumeric: 'tabular-nums',
        }}
      >
        {timestamp}
      </span>

      {/* Phase tag */}
      {phase && (
        <span
          style={{
            flexShrink: 0,
            padding: '0 5px',
            borderRadius: '4px',
            backgroundColor: `${PHASE_COLORS[phase]}20`,
            color: PHASE_COLORS[phase],
            fontSize: '11px',
            fontWeight: 500,
            textTransform: 'uppercase',
            letterSpacing: '0.04em',
          }}
        >
          {phase}
        </span>
      )}

      {/* Message */}
      <span style={{ flex: 1 }}>{message}</span>
    </div>
  )
}

interface LogPanelProps {
  lines: TaskLog[]
  phaseFilter: PhaseFilter
  autoScroll: boolean
  /** Called with the DOM element ref so the parent can manage scroll position. */
  panelRef: React.RefObject<HTMLDivElement>
  /** Message shown when no task is selected. */
  emptyMessage?: string
  /** Access error message to display inside the panel. */
  accessError: string | null
}

/**
 * LogPanel renders the terminal-style dark log output area.
 * Displays filtered log lines using LogLine sub-components.
 * Respects autoScroll: scrolls to bottom on new lines when true.
 *
 * Preconditions:
 *   - panelRef is attached to the scrollable container div.
 *
 * Postconditions:
 *   - When autoScroll is true and new lines arrive, the panel scrolls to the bottom.
 *   - When autoScroll is false, scroll position is preserved; user can scroll freely.
 */
export function LogPanel({
  lines,
  phaseFilter,
  autoScroll,
  panelRef,
  emptyMessage,
  accessError,
}: LogPanelProps): React.ReactElement {
  const filteredLines = filterLogLines(lines, phaseFilter)
  const bottomRef = useRef<HTMLDivElement | null>(null)

  // Auto-scroll to the bottom sentinel when new lines arrive and autoScroll is active.
  useEffect(() => {
    if (autoScroll && bottomRef.current && typeof bottomRef.current.scrollIntoView === 'function') {
      bottomRef.current.scrollIntoView({ behavior: 'instant' })
    }
  }, [autoScroll, filteredLines.length])

  return (
    <div
      ref={panelRef}
      role="log"
      aria-live="polite"
      aria-atomic="false"
      aria-label="Log output"
      style={{
        backgroundColor: '#0F172A',
        borderRadius: '8px',
        border: '1px solid #1E293B',
        padding: '16px',
        flex: 1,
        overflowY: 'auto',
        minHeight: '300px',
        maxHeight: 'calc(100vh - 320px)',
      }}
    >
      {/* Access error state */}
      {accessError && (
        <div
          style={{
            color: '#EF4444',
            fontFamily: 'var(--font-mono, "JetBrains Mono", monospace)',
            fontSize: '13px',
          }}
        >
          {accessError}
        </div>
      )}

      {/* Empty state */}
      {!accessError && filteredLines.length === 0 && emptyMessage && (
        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            height: '100%',
            minHeight: '200px',
            color: '#64748B',
            fontFamily: 'var(--font-mono, "JetBrains Mono", monospace)',
            fontSize: '13px',
          }}
        >
          {emptyMessage}
        </div>
      )}

      {/* Log lines */}
      {!accessError && filteredLines.map(line => (
        <LogLine key={line.id} line={line} />
      ))}

      {/* Auto-scroll sentinel */}
      <div ref={bottomRef} />
    </div>
  )
}

interface LogStatusBarProps {
  sseStatus: SSEConnectionStatus
  lineCount: number
  lastEventId: string | null
  /** Whether streaming is complete (task is in terminal state). */
  isComplete: boolean
}

/**
 * LogStatusBar renders the bottom status bar with SSE info, line count,
 * and Last-Event-ID for debugging reconnection state.
 *
 * @param sseStatus   - Current SSE connection status.
 * @param lineCount   - Total number of accumulated log lines.
 * @param lastEventId - Last received SSE event ID for display.
 * @param isComplete  - True when the task has reached a terminal state.
 */
export function LogStatusBar({
  sseStatus,
  lineCount,
  lastEventId,
  isComplete,
}: LogStatusBarProps): React.ReactElement {
  const isReconnecting = sseStatus === 'reconnecting'
  const isConnected = sseStatus === 'connected'

  const dotColor = isConnected
    ? '#16A34A'
    : isReconnecting
      ? '#DC2626'
      : '#D97706'

  const statusLabel = isComplete
    ? `Complete — ${lineCount} line${lineCount !== 1 ? 's' : ''}`
    : isReconnecting
      ? 'Reconnecting...'
      : isConnected
        ? 'Connected'
        : 'Connecting...'

  return (
    <div
      role="status"
      aria-live="polite"
      style={{
        display: 'flex',
        alignItems: 'center',
        gap: '16px',
        padding: '8px 12px',
        backgroundColor: 'var(--color-surface-panel)',
        border: '1px solid var(--color-border)',
        borderRadius: '6px',
        fontSize: '12px',
        color: 'var(--color-text-secondary)',
        flexWrap: 'wrap',
      }}
    >
      {/* SSE dot + label */}
      <span style={{ display: 'inline-flex', alignItems: 'center', gap: '6px' }}>
        <span
          style={{
            display: 'inline-block',
            width: '6px',
            height: '6px',
            borderRadius: '50%',
            backgroundColor: dotColor,
          }}
        />
        <span style={{ color: isReconnecting ? '#DC2626' : 'inherit' }}>
          {statusLabel}
        </span>
      </span>

      {/* Line count (shown separately when not complete) */}
      {!isComplete && (
        <span>{lineCount} line{lineCount !== 1 ? 's' : ''}</span>
      )}

      {/* Last-Event-ID */}
      {lastEventId && (
        <span
          style={{
            fontFamily: 'var(--font-mono, "JetBrains Mono", monospace)',
            fontSize: '11px',
            color: '#64748B',
          }}
        >
          Last-Event-ID: {lastEventId}
        </span>
      )}
    </div>
  )
}

// ---------------------------------------------------------------------------
// Main component
// ---------------------------------------------------------------------------

/**
 * LogStreamerPage is the real-time log viewer.
 * Composes useLogs (SSE streaming), useTasks (task selector options),
 * and the download trigger.
 *
 * States rendered:
 *   - No task selected: dark panel with centered "Select a task to stream its logs"
 *   - Connecting: status shows "Connecting..." with amber dot
 *   - Streaming: log lines appear in real time; status shows "Connected"
 *   - Complete: final line shown; status shows "Complete — N lines"
 *   - Reconnecting: status shows "Reconnecting..." with red dot
 *   - Access error (403): error message shown in the log panel, not a redirect
 *
 * Preconditions:
 *   - User must be authenticated.
 *   - URL query param `taskId` is optional; if present, task is pre-selected on mount.
 *
 * Postconditions:
 *   - Download Logs fetches GET /api/tasks/{id}/logs (REST, not SSE) and
 *     triggers a browser download of the raw text.
 *   - Clear clears the visual buffer; lastEventId is preserved for future reconnection.
 */
function LogStreamerPage(): React.ReactElement {
  const [searchParams, setSearchParams] = useSearchParams()
  const initialTaskId = searchParams.get('taskId') ?? undefined

  const [selectedTaskId, setSelectedTaskId] = useState<string | undefined>(initialTaskId)
  const [phaseFilter, setPhaseFilter] = useState<PhaseFilter>('all')
  const [autoScroll, setAutoScroll] = useState(true)
  const [isDownloading, setIsDownloading] = useState(false)

  const panelRef = useRef<HTMLDivElement>(null)

  const { lines, accessError, sseStatus, lastEventId, clearLines } = useLogs({
    taskId: selectedTaskId,
  })

  // Load task list for the task selector dropdown.
  const { tasks } = useTasks()

  // Determine if the streaming is complete by checking if the selected task's status
  // is in a terminal state.
  const selectedTask = tasks.find(t => t.id === selectedTaskId)
  const terminalStatuses = new Set(['completed', 'failed', 'cancelled'])
  const isComplete = selectedTask !== undefined && terminalStatuses.has(selectedTask.status)

  /**
   * handleTaskChange updates the selected task ID when the user picks a task
   * from the dropdown and reflects the choice in the URL query param.
   */
  const handleTaskChange = useCallback((taskId: string) => {
    const newTaskId = taskId || undefined
    setSelectedTaskId(newTaskId)
    if (newTaskId) {
      setSearchParams({ taskId: newTaskId }, { replace: true })
    } else {
      setSearchParams({}, { replace: true })
    }
  }, [setSearchParams])

  /**
   * handleDownload fetches the full log history from the REST API and triggers
   * a browser file download. Does nothing when no task is selected.
   */
  const handleDownload = useCallback(async () => {
    if (!selectedTaskId || isDownloading) return
    setIsDownloading(true)
    try {
      const text = await downloadTaskLogs(selectedTaskId)
      const blob = new Blob([text], { type: 'text/plain' })
      const url = URL.createObjectURL(blob)
      const anchor = document.createElement('a')
      anchor.href = url
      anchor.download = `task-${selectedTaskId}-logs.txt`
      document.body.appendChild(anchor)
      anchor.click()
      document.body.removeChild(anchor)
      URL.revokeObjectURL(url)
    } catch {
      // Download errors are non-critical; a toast notification would go here in future.
    } finally {
      setIsDownloading(false)
    }
  }, [selectedTaskId, isDownloading])

  return (
    <div
      style={{
        display: 'flex',
        flexDirection: 'column',
        gap: '12px',
        height: '100%',
      }}
    >
      {/* Page header */}
      <h1
        style={{
          fontSize: '20px',
          fontWeight: 600,
          color: 'var(--color-text-primary)',
          margin: 0,
        }}
      >
        Log Streamer
      </h1>

      {/* Task selector and controls bar */}
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: '8px',
          flexWrap: 'wrap',
        }}
      >
        {/* Task selector dropdown */}
        <select
          aria-label="Select task"
          value={selectedTaskId ?? ''}
          onChange={e => handleTaskChange(e.target.value)}
          style={{
            height: '34px',
            padding: '0 10px',
            fontSize: '13px',
            border: '1px solid var(--color-border)',
            borderRadius: '6px',
            backgroundColor: 'var(--color-surface-subtle)',
            color: 'var(--color-text-primary)',
            outline: 'none',
            minWidth: '200px',
          }}
        >
          <option value="">Select a task...</option>
          {tasks.map(t => (
            <option key={t.id} value={t.id}>
              {t.id.slice(0, 8)}... — {t.status}
            </option>
          ))}
        </select>

        {/* Phase filter toggles */}
        <PhaseFilterToggleGroup
          activeFilter={phaseFilter}
          onChange={setPhaseFilter}
        />

        {/* Spacer */}
        <div style={{ flex: 1 }} />

        {/* Auto-scroll toggle */}
        <label
          style={{
            display: 'inline-flex',
            alignItems: 'center',
            gap: '6px',
            fontSize: '13px',
            color: 'var(--color-text-secondary)',
            cursor: 'pointer',
            userSelect: 'none',
          }}
        >
          <input
            type="checkbox"
            aria-label="Auto-scroll"
            checked={autoScroll}
            onChange={e => setAutoScroll(e.target.checked)}
            style={{ cursor: 'pointer' }}
          />
          Auto-scroll
        </label>

        {/* Download button */}
        <button
          type="button"
          aria-label="Download Logs"
          onClick={() => { void handleDownload() }}
          disabled={isDownloading || !selectedTaskId}
          style={{
            height: '34px',
            padding: '0 12px',
            fontSize: '13px',
            fontWeight: 500,
            backgroundColor: 'var(--color-surface-panel)',
            color: 'var(--color-text-primary)',
            border: '1px solid var(--color-border)',
            borderRadius: '6px',
            cursor: selectedTaskId && !isDownloading ? 'pointer' : 'not-allowed',
            opacity: selectedTaskId && !isDownloading ? 1 : 0.5,
          }}
        >
          {isDownloading ? 'Downloading...' : 'Download Logs'}
        </button>

        {/* Clear button */}
        <button
          type="button"
          aria-label="Clear"
          onClick={clearLines}
          style={{
            height: '34px',
            padding: '0 12px',
            fontSize: '13px',
            fontWeight: 500,
            backgroundColor: 'var(--color-surface-panel)',
            color: 'var(--color-text-primary)',
            border: '1px solid var(--color-border)',
            borderRadius: '6px',
            cursor: 'pointer',
          }}
        >
          Clear
        </button>
      </div>

      {/* Log output panel */}
      <LogPanel
        lines={lines}
        phaseFilter={phaseFilter}
        autoScroll={autoScroll}
        panelRef={panelRef}
        accessError={accessError}
        emptyMessage={
          selectedTaskId
            ? 'Waiting for log lines...'
            : 'No task selected — choose a task above to stream its logs'
        }
      />

      {/* Status bar */}
      <LogStatusBar
        sseStatus={sseStatus}
        lineCount={lines.length}
        lastEventId={lastEventId}
        isComplete={isComplete}
      />
    </div>
  )
}

// ---------------------------------------------------------------------------
// PhaseFilterToggleGroup (internal)
// ---------------------------------------------------------------------------

interface PhaseFilterToggleGroupProps {
  activeFilter: PhaseFilter
  onChange: (filter: PhaseFilter) => void
}

/**
 * PhaseFilterToggleGroup renders the All / DataSource / Process / Sink toggle buttons.
 * Only one filter is active at a time. Phase colors match DESIGN.md tokens.
 *
 * @param activeFilter - Currently selected phase filter.
 * @param onChange     - Called when the user selects a different filter.
 */
function PhaseFilterToggleGroup({
  activeFilter,
  onChange,
}: PhaseFilterToggleGroupProps): React.ReactElement {
  const filters: Array<{ value: PhaseFilter; label: string; activeColor?: string }> = [
    { value: 'all', label: 'All' },
    { value: 'datasource', label: 'DataSource', activeColor: '#2563EB' },
    { value: 'process', label: 'Process', activeColor: '#8B5CF6' },
    { value: 'sink', label: 'Sink', activeColor: '#16A34A' },
  ]

  return (
    <div
      role="group"
      aria-label="Phase filter"
      style={{ display: 'inline-flex', gap: '4px' }}
    >
      {filters.map(({ value, label, activeColor }) => {
        const isActive = activeFilter === value
        return (
          <button
            key={value}
            type="button"
            aria-label={label}
            aria-pressed={isActive}
            onClick={() => onChange(value)}
            style={{
              height: '34px',
              padding: '0 10px',
              fontSize: '12px',
              fontWeight: isActive ? 600 : 400,
              border: `1px solid ${isActive && activeColor ? activeColor : 'var(--color-border)'}`,
              borderRadius: '6px',
              backgroundColor: isActive && activeColor ? `${activeColor}15` : 'var(--color-surface-panel)',
              color: isActive && activeColor ? activeColor : 'var(--color-text-secondary)',
              cursor: 'pointer',
            }}
          >
            {label}
          </button>
        )
      })}
    </div>
  )
}

export default LogStreamerPage
