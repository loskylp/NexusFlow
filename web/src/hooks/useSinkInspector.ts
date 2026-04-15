/**
 * useSinkInspector — hook that manages SSE subscription and snapshot state for
 * the Sink Inspector demo view.
 *
 * On task selection, subscribes to GET /events/sink/{taskId} via SSE and
 * populates Before/After snapshot state from incoming events:
 *   - sink:before-snapshot  → sets beforeSnapshot; clears afterSnapshot
 *   - sink:after-result     → sets afterSnapshot and rolledBack flag
 *
 * On task change, clears all snapshot state and re-subscribes to the new channel.
 *
 * Admin-only: the SSE endpoint enforces admin-only access server-side (403 for
 * non-admin callers). The hook surfaces this as an accessError in the return value.
 *
 * See: DEMO-003, ADR-007, TASK-032, TASK-033
 */

import type { SinkSnapshot } from '@/types/domain'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

/**
 * UseSinkInspectorOptions configures the useSinkInspector hook.
 */
export interface UseSinkInspectorOptions {
  /** The task ID to subscribe to. null means no active subscription. */
  taskId: string | null
}

/**
 * UseSinkInspectorReturn is the hook's public surface.
 */
export interface UseSinkInspectorReturn {
  /**
   * The Before snapshot received from the sink:before-snapshot event.
   * null until the Sink phase begins for the selected task.
   */
  beforeSnapshot: SinkSnapshot | null

  /**
   * The After snapshot received from the sink:after-result event.
   * null until the Sink phase completes or rolls back.
   */
  afterSnapshot: SinkSnapshot | null

  /**
   * True when the sink:after-result event indicated a rollback.
   * false until the after event is received.
   */
  rolledBack: boolean

  /**
   * True when waiting for the sink:before-snapshot event after task selection.
   * Drives the "Waiting for sink phase to begin..." spinner in the Before panel.
   */
  isWaitingForSinkPhase: boolean

  /**
   * The current SSE connection status for the selected task channel.
   * 'idle' when no task is selected.
   */
  sseStatus: 'idle' | 'connecting' | 'connected' | 'reconnecting' | 'error'

  /**
   * Non-null when the SSE endpoint returned a 403. Indicates the current user
   * is not an Admin and cannot access the Sink Inspector.
   */
  accessError: string | null

  /**
   * The write error message from the After event, if the Sink write failed.
   * Empty string or null when the write succeeded.
   */
  writeError: string | null
}

// ---------------------------------------------------------------------------
// Hook
// ---------------------------------------------------------------------------

/**
 * useSinkInspector subscribes to the SSE sink event channel for the given task
 * and returns Before/After snapshot state for the Sink Inspector view.
 *
 * @param options - { taskId } — the task to inspect. null disconnects SSE.
 * @returns Snapshot state, SSE status, and error signals.
 *
 * @throws Never — all errors are surfaced via sseStatus or accessError.
 *
 * Preconditions:
 *   - When taskId is non-null, it must be a valid UUID string.
 *
 * Postconditions:
 *   - When taskId changes, all snapshot state is reset before the new subscription.
 *   - When taskId is null, sseStatus is 'idle' and all snapshots are null.
 *   - After sink:before-snapshot: beforeSnapshot is non-null, afterSnapshot is null.
 *   - After sink:after-result: afterSnapshot is non-null, isWaitingForSinkPhase is false.
 */
// Stub — see TASK-032 (scaffold: process/scaffolder/cycle-4-scaffold.md)
export function useSinkInspector(): UseSinkInspectorReturn {
  // TODO: implement — wire options: UseSinkInspectorOptions when TASK-032 is implemented
  throw new Error('Not implemented')
}
