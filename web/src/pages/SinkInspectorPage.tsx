/**
 * SinkInspectorPage — Admin-only demo view showing Before/After sink destination
 * state comparison with atomicity verification.
 *
 * Layout:
 *   - Header: "Sink Inspector" title, "DEMO" badge, monitoring status
 *   - Task selector: dropdown of recent tasks
 *   - Split panel (50/50): Before Snapshot (left), After Result (right)
 *   - Atomicity verification section below panels
 *
 * SSE subscription: when a task is selected, subscribes to
 *   GET /events/sink/{taskId}
 * and populates panels from incoming sink:before-snapshot and sink:after-result events.
 *
 * Access: Admin only. The SSE endpoint returns 403 for non-admin callers; this is
 * surfaced as an access error message in the view.
 *
 * See: DEMO-003, UX Spec (Sink Inspector), TASK-032, TASK-033
 */

import React from 'react'

// Stub — see TASK-032 (scaffold: process/scaffolder/cycle-4-scaffold.md)
// Sub-components (SinkInspectorHeader, TaskSelector, SnapshotPanel, AtomicityVerification)
// and their props interfaces will be implemented in TASK-032.

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

/**
 * SinkInspectorPage is the root component for the Sink Inspector demo view.
 *
 * Orchestrates:
 *   - Task list fetch via useTasks (for the selector dropdown)
 *   - SSE snapshot subscription via useSinkInspector
 *   - Before/After panel rendering
 *   - Atomicity verification display
 *
 * Access guard: if the current user is not an Admin (surfaced via useSinkInspector
 * accessError), renders a 403 access denied message instead of the view content.
 *
 * Route: /demo/sink-inspector
 *
 * Postconditions:
 *   - Selecting a task immediately subscribes to the SSE channel.
 *   - Changing the selected task resets all snapshot state.
 *   - Non-admin users see an access denied message, not the data panels.
 */
function SinkInspectorPage(): React.ReactElement {
  // TODO: implement
  throw new Error('Not implemented')
}

export default SinkInspectorPage
