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

import React from 'react'
import type { TaskLog } from '@/types/domain'
import type { SSEConnectionStatus } from '@/hooks/useSSE'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

/** Phase filter options — 'all' means no filtering. */
export type PhaseFilter = 'all' | 'datasource' | 'process' | 'sink'

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/**
 * filterLogLines applies a phase filter to a log line array.
 * When phase is 'all', all lines are returned unchanged.
 * Otherwise only lines whose level field matches the phase are returned.
 *
 * @param lines  - Full log line array.
 * @param phase  - Phase filter to apply.
 * @returns Filtered array (same references, new array when filtered).
 */
export function filterLogLines(_lines: TaskLog[], _phase: PhaseFilter): TaskLog[] {
  // TODO: implement
  throw new Error('Not implemented')
}

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

interface LogLineProps {
  line: TaskLog
}

/**
 * LogLine renders a single log line in the terminal panel.
 * Format: <timestamp> [<phase>] [<level>] <message>
 * Phase tag is color-coded. Level ERROR is rendered in red text.
 * No animation — lines appear instantly (UX spec: "animation would be distracting
 * at high throughput").
 */
export function LogLine({ line: _line }: LogLineProps): React.ReactElement {
  // TODO: implement
  throw new Error('Not implemented')
}

interface LogPanelProps {
  lines: TaskLog[]
  phaseFilter: PhaseFilter
  autoScroll: boolean
  /** Called with the DOM element ref so the parent can manage scroll position. */
  panelRef: React.RefObject<HTMLDivElement>
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
  lines: _lines,
  phaseFilter: _phaseFilter,
  autoScroll: _autoScroll,
  panelRef: _panelRef,
}: LogPanelProps): React.ReactElement {
  // TODO: implement
  throw new Error('Not implemented')
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
 */
export function LogStatusBar({
  sseStatus: _sseStatus,
  lineCount: _lineCount,
  lastEventId: _lastEventId,
  isComplete: _isComplete,
}: LogStatusBarProps): React.ReactElement {
  // TODO: implement
  throw new Error('Not implemented')
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
 *   - Access error (403/404): error message shown in the log panel
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
  // TODO: implement
  throw new Error('Not implemented')
}

export default LogStreamerPage
