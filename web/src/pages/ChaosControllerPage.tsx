/**
 * ChaosControllerPage — Admin-only demo view for injecting disturbances into the
 * running system to demonstrate auto-recovery capabilities.
 *
 * Layout:
 *   - Header: "Chaos Controller" title with "DEMO" and "DESTRUCTIVE" badges,
 *             system status indicator
 *   - Three action cards stacked vertically:
 *     1. Kill Worker: worker selector, kill button, expected result, activity log
 *     2. Disconnect Database: duration selector, disconnect button, expected result,
 *        activity log
 *     3. Flood Queue: task count input, pipeline selector, submit burst button,
 *        expected result, activity log
 *
 * All destructive actions (Kill Worker, Disconnect DB) require a confirmation dialog.
 * Flood Queue does not require confirmation (non-destructive).
 *
 * Access: Admin only. Non-admin users are shown an access denied message.
 *
 * See: DEMO-004, UX Spec (Chaos Controller), TASK-034
 */

import React from 'react'
import type { Worker, Pipeline, ChaosActivityEntry, SystemHealthStatus } from '@/types/domain'

// ---------------------------------------------------------------------------
// Sub-component contracts
// ---------------------------------------------------------------------------

/**
 * ChaosHeaderProps feeds the page header.
 */
interface ChaosHeaderProps {
  /** The current system health status. */
  systemStatus: SystemHealthStatus
}

/**
 * ChaosHeader renders the page title with DEMO and DESTRUCTIVE badges and the
 * system status indicator.
 *
 * Status indicator colors:
 *   - nominal:  green badge "System Status: Nominal"
 *   - degraded: yellow badge "System Status: Degraded"
 *   - critical: red badge "System Status: Critical"
 *
 * Postconditions:
 *   - Both DEMO and DESTRUCTIVE badges are always visible.
 *   - Status badge transitions at 200ms ease (per UX spec).
 */
function ChaosHeader({ systemStatus }: ChaosHeaderProps): React.ReactElement {
  // TODO: implement
  throw new Error('Not implemented')
}

// ---------------------------------------------------------------------------

/**
 * KillWorkerCardProps feeds the Kill Worker action card.
 */
interface KillWorkerCardProps {
  /** Online workers available for selection. */
  workers: Worker[]
  /** Currently selected worker ID, or null if none is selected. */
  selectedWorkerId: string | null
  /** Called when the user changes the worker selection. */
  onSelectWorker: (workerId: string | null) => void
  /**
   * Called when the user confirms the kill action.
   * The card triggers a confirmation dialog before calling this.
   */
  onKill: () => void
  /** True while the kill API call is in progress. Disables the kill button. */
  isKilling: boolean
  /** Activity log entries for this card. */
  activityLog: ChaosActivityEntry[]
}

/**
 * KillWorkerCard renders the Kill Worker action card.
 *
 * Kill button:
 *   - Red background, labelled "Kill Worker".
 *   - Disabled when no worker is selected or isKilling is true.
 *   - Opens a confirmation dialog before triggering onKill.
 *
 * Confirmation dialog text:
 *   "Kill worker {workerId}? This will terminate the container immediately.
 *    Any in-flight tasks will be reclaimed by the Monitor."
 *
 * Activity log:
 *   - Monospace font, precise timestamps, newest entries at bottom.
 *   - Scrollable when content overflows.
 *
 * Postconditions:
 *   - onKill is never called without prior user confirmation.
 *   - Kill button shows inline spinner while isKilling is true.
 */
function KillWorkerCard({
  workers,
  selectedWorkerId,
  onSelectWorker,
  onKill,
  isKilling,
  activityLog,
}: KillWorkerCardProps): React.ReactElement {
  // TODO: implement
  throw new Error('Not implemented')
}

// ---------------------------------------------------------------------------

/**
 * DisconnectDatabaseCardProps feeds the Disconnect Database action card.
 */
interface DisconnectDatabaseCardProps {
  /** Selected disconnect duration in seconds. One of 15, 30, or 60. */
  durationSeconds: 15 | 30 | 60
  /** Called when the user changes the duration selector. */
  onSelectDuration: (seconds: 15 | 30 | 60) => void
  /**
   * Called when the user confirms the disconnect action.
   * The card triggers a confirmation dialog before calling this.
   */
  onDisconnect: () => void
  /** True while a disconnect is active. Shows countdown timer; disables button. */
  isDisconnecting: boolean
  /** Remaining seconds of the active disconnect, or null when inactive. */
  remainingSeconds: number | null
  /** True while the disconnect API call is in progress. */
  isSubmitting: boolean
  /** Activity log entries for this card. */
  activityLog: ChaosActivityEntry[]
}

/**
 * DisconnectDatabaseCard renders the Disconnect Database action card.
 *
 * Duration selector: radio buttons or select for 15s / 30s / 60s.
 *
 * Disconnect button:
 *   - Red outlined button, labelled "Disconnect DB".
 *   - Disabled when isDisconnecting or isSubmitting is true.
 *   - Opens a confirmation dialog before triggering onDisconnect.
 *
 * Active state (isDisconnecting is true):
 *   - Shows countdown timer: "Disconnected — reconnects in {remainingSeconds}s".
 *   - System status indicator should show yellow during this state.
 *
 * Postconditions:
 *   - onDisconnect is never called without prior user confirmation.
 *   - Cannot trigger a second disconnect while one is active.
 */
function DisconnectDatabaseCard({
  durationSeconds,
  onSelectDuration,
  onDisconnect,
  isDisconnecting,
  remainingSeconds,
  isSubmitting,
  activityLog,
}: DisconnectDatabaseCardProps): React.ReactElement {
  // TODO: implement
  throw new Error('Not implemented')
}

// ---------------------------------------------------------------------------

/**
 * FloodQueueCardProps feeds the Flood Queue action card.
 */
interface FloodQueueCardProps {
  /** Available pipelines for the flood burst. */
  pipelines: Pipeline[]
  /** Currently selected pipeline ID for the flood. */
  selectedPipelineId: string | null
  /** Called when the user changes the pipeline selection. */
  onSelectPipeline: (pipelineId: string | null) => void
  /** The number of tasks to submit in the burst. Controlled by parent. */
  taskCount: number
  /** Called when the task count input changes. */
  onChangeTaskCount: (count: number) => void
  /** Called when the user clicks "Submit Burst". No confirmation required. */
  onFlood: () => void
  /** True while the flood submission is in progress. */
  isFlooding: boolean
  /**
   * Submission progress (0–100). Non-null while isFlooding is true.
   * Drives the progress indicator.
   */
  progress: number | null
  /** Activity log entries for this card. */
  activityLog: ChaosActivityEntry[]
}

/**
 * FloodQueueCard renders the Flood Queue action card.
 *
 * Task count input: numeric input, minimum 1, maximum 1000.
 *
 * Submit Burst button:
 *   - Amber background, labelled "Submit Burst".
 *   - No confirmation dialog (non-destructive action).
 *   - Disabled when no pipeline is selected or isFlooding is true.
 *   - Shows progress bar while isFlooding is true.
 *
 * Activity log:
 *   - Shows distribution metrics after flood completes (tasks per worker/stream).
 *
 * Postconditions:
 *   - onFlood is called directly on button click, without a confirmation dialog.
 *   - Task count is clamped to [1, 1000] before calling onFlood.
 */
function FloodQueueCard({
  pipelines,
  selectedPipelineId,
  onSelectPipeline,
  taskCount,
  onChangeTaskCount,
  onFlood,
  isFlooding,
  progress,
  activityLog,
}: FloodQueueCardProps): React.ReactElement {
  // TODO: implement
  throw new Error('Not implemented')
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

/**
 * ChaosControllerPage is the root component for the Chaos Controller demo view.
 *
 * Orchestrates:
 *   - Worker list fetch (for Kill Worker selector)
 *   - Pipeline list fetch (for Flood Queue selector)
 *   - System health polling via GET /api/health (refreshed after chaos actions)
 *   - API calls for each chaos action:
 *     - POST /api/chaos/kill-worker     — Kill Worker action
 *     - POST /api/chaos/disconnect-db   — Disconnect Database action
 *     - POST /api/chaos/flood-queue     — Flood Queue action
 *   - Activity log state management per card
 *
 * System status:
 *   - Derives SystemHealthStatus from the health endpoint response.
 *   - Refreshes after each chaos action completes.
 *
 * Route: /demo/chaos
 *
 * Postconditions:
 *   - Each destructive action (Kill Worker, Disconnect DB) appends timeline entries
 *     to the relevant card's activity log.
 *   - All confirmation dialogs are handled by the respective card components.
 *   - Non-admin users see an access denied message (admin guard applies at route level).
 */
function ChaosControllerPage(): React.ReactElement {
  // TODO: implement
  throw new Error('Not implemented')
}

export default ChaosControllerPage
